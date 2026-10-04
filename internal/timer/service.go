package timer

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"goxuetimer/internal/store"
)

// Service 是计时状态的唯一权威。
//
// 托盘菜单、前端绑定、后台心跳全部走同一个实例——否则会出现"托盘显示运行中、
// 界面显示已结束"这类两套状态的经典问题。
//
// 所有变更遵循同一条铁律：先写库、后改内存；写库失败就把内存回滚到变更前。
type Service struct {
	st    *store.DB
	clock func() time.Time

	mu        sync.Mutex
	snap      Snapshot
	carryNano int64 // 不足 1 秒的余量，避免每次结算向下取整累积漂移
	sanitized bool  // 载入时发现持久化状态损坏，需在 Recover 里清理

	onStateChange func(Snapshot)
	onNotice      func(NoticeLevel, string)
}

// New 构造计时服务并载入持久化状态。
//
// 这里只负责"读"，不做任何恢复判定——恢复要提示用户，得由 Recover 来做，
// 而且必须等提示回调挂好之后再调用。
func New(st *store.DB, clock func() time.Time) (*Service, error) {
	if st == nil {
		return nil, errors.New("timer: store 不能为空")
	}
	if clock == nil {
		clock = time.Now
	}
	row, err := st.LoadTimerState()
	if err != nil {
		return nil, err
	}
	s := &Service{st: st, clock: clock}
	s.snap, s.sanitized = snapshotFromRow(row)
	return s, nil
}

// OnStateChange 注册状态变化回调（含每秒级心跳里的时长变化）。
// 应在应用启动前设置一次；回调在锁外调用，允许内部再访问 Service。
func (s *Service) OnStateChange(fn func(Snapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStateChange = fn
}

// OnNotice 注册面向用户的提示回调。
func (s *Service) OnNotice(fn func(NoticeLevel, string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onNotice = fn
}

// Snapshot 返回当前状态。
func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

// ElapsedSec 返回当前会话到此刻为止的总秒数。
func (s *Service) ElapsedSec() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap.ElapsedSec(s.clock())
}

// StopResult 是 Stop 的结果。
type StopResult struct {
	Saved       bool
	Reason      string // 空串表示已保存；"too-short" 表示时长过短被丢弃
	DurationSec int64
	Session     *store.SessionRow
	Snapshot    Snapshot
}

// Start 开始一次新的学习会话。
func (s *Service) Start() (Snapshot, error) {
	s.mu.Lock()
	if s.snap.State != StateIdle {
		snap := s.snap
		s.mu.Unlock()
		return snap, ErrAlreadyRunning
	}
	now := s.clock()
	prev, prevCarry := s.snap, s.carryNano
	s.snap = Snapshot{State: StateRunning, StartedAt: now, RunningSince: now}
	s.carryNano = 0

	if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
		s.snap, s.carryNano = prev, prevCarry
		snap := s.snap
		s.mu.Unlock()
		return snap, err
	}
	snap, cb := s.snap, s.onStateChange
	s.mu.Unlock()

	if cb != nil {
		cb(snap)
	}
	return snap, nil
}

// Pause 暂停计时，暂停区间不计入时长。
func (s *Service) Pause() (Snapshot, error) {
	s.mu.Lock()
	if s.snap.State != StateRunning {
		snap := s.snap
		s.mu.Unlock()
		return snap, ErrNotRunning
	}
	now := s.clock()
	prev, prevCarry := s.snap, s.carryNano
	notice := s.creditSinceLocked(now)
	s.snap.State = StatePaused
	s.snap.RunningSince = time.Time{}

	if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
		s.snap, s.carryNano = prev, prevCarry
		snap := s.snap
		s.mu.Unlock()
		return snap, err
	}
	snap, stateCB, noticeCB := s.snap, s.onStateChange, s.onNotice
	s.mu.Unlock()

	if notice != "" && noticeCB != nil {
		noticeCB(NoticeWarning, notice)
	}
	if stateCB != nil {
		stateCB(snap)
	}
	return snap, nil
}

// Resume 从暂停恢复计时。
func (s *Service) Resume() (Snapshot, error) {
	s.mu.Lock()
	if s.snap.State != StatePaused {
		snap := s.snap
		s.mu.Unlock()
		return snap, ErrNotPaused
	}
	prev := s.snap
	s.snap.State = StateRunning
	s.snap.RunningSince = s.clock()

	if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
		s.snap = prev
		snap := s.snap
		s.mu.Unlock()
		return snap, err
	}
	snap, cb := s.snap, s.onStateChange
	s.mu.Unlock()

	if cb != nil {
		cb(snap)
	}
	return snap, nil
}

// Stop 结束当前会话。时长不足 MinSessionSec 时丢弃、不落库。
func (s *Service) Stop() (StopResult, error) {
	s.mu.Lock()
	if s.snap.State == StateIdle {
		out := StopResult{Snapshot: s.snap}
		s.mu.Unlock()
		return out, ErrIdle
	}
	now := s.clock()
	prev, prevCarry := s.snap, s.carryNano
	notice := s.creditSinceLocked(now)

	dur := s.snap.AccumulatedSec
	startedAt := s.snap.StartedAt

	if dur < MinSessionSec {
		// 误触：只把状态复位，不污染记录。
		s.snap = Idle()
		s.carryNano = 0
		if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
			s.snap, s.carryNano = prev, prevCarry
			out := StopResult{Snapshot: s.snap}
			s.mu.Unlock()
			return out, err
		}
		out := StopResult{Reason: "too-short", DurationSec: dur, Snapshot: s.snap}
		snap, stateCB, noticeCB := s.snap, s.onStateChange, s.onNotice
		s.mu.Unlock()
		if notice != "" && noticeCB != nil {
			noticeCB(NoticeWarning, notice)
		}
		if stateCB != nil {
			stateCB(snap)
		}
		return out, nil
	}

	endedAt := now
	if endedAt.Before(startedAt) {
		// 时间回拨的极端情况下，别写出结束早于开始的记录。
		endedAt = startedAt.Add(time.Duration(dur) * time.Second)
	}
	rec := store.SessionRow{
		StartedAt:   startedAt.Format(recordLayout),
		EndedAt:     endedAt.Format(recordLayout),
		DurationSec: dur,
		Source:      "timer",
		CreatedAt:   now.Format(recordLayout),
	}

	id, err := s.st.FinishSession(rec, store.TimerStateRow{State: string(StateIdle)})
	if err != nil {
		s.snap, s.carryNano = prev, prevCarry
		out := StopResult{Snapshot: s.snap}
		s.mu.Unlock()
		return out, err
	}
	rec.ID = id
	s.snap = Idle()
	s.carryNano = 0

	out := StopResult{Saved: true, DurationSec: dur, Session: &rec, Snapshot: s.snap}
	snap, stateCB, noticeCB := s.snap, s.onStateChange, s.onNotice
	s.mu.Unlock()

	if notice != "" && noticeCB != nil {
		noticeCB(NoticeWarning, notice)
	}
	if stateCB != nil {
		stateCB(snap)
	}
	return out, nil
}

// Discard 放弃当前会话，不写入任何记录。
func (s *Service) Discard() (Snapshot, error) {
	s.mu.Lock()
	if s.snap.State == StateIdle {
		snap := s.snap
		s.mu.Unlock()
		return snap, ErrIdle
	}
	prev, prevCarry := s.snap, s.carryNano
	s.snap = Idle()
	s.carryNano = 0

	if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
		s.snap, s.carryNano = prev, prevCarry
		snap := s.snap
		s.mu.Unlock()
		return snap, err
	}
	snap, cb := s.snap, s.onStateChange
	s.mu.Unlock()

	if cb != nil {
		cb(snap)
	}
	return snap, nil
}

// Tick 由后台心跳调用，把已流逝的时间结算进 accumulated。
//
// 它同时是休眠防线：一次心跳里墙钟跳得离谱时只计入一个 TickInterval，
// 否则合盖过夜再打开会得到一个 8 小时的"学习记录"。
func (s *Service) Tick() (Snapshot, error) {
	s.mu.Lock()
	if s.snap.State != StateRunning {
		snap := s.snap
		s.mu.Unlock()
		return snap, nil
	}

	now := s.clock()
	delta := now.Sub(s.snap.RunningSince)

	credited := delta
	notice := ""
	switch {
	case delta < 0:
		credited = 0
		notice = "检测到系统时间回拨，本次时长未计入"
	case delta > SuspiciousDelta:
		credited = TickInterval
		notice = fmt.Sprintf("检测到系统休眠或挂起，本次只计入 %s", humanDuration(TickInterval))
	}

	prev, prevCarry := s.snap, s.carryNano
	s.snap.RunningSince = now
	s.creditNanos(credited)

	if err := s.st.SaveTimerState(rowFromSnapshot(s.snap)); err != nil {
		s.snap, s.carryNano = prev, prevCarry
		snap := s.snap
		s.mu.Unlock()
		return snap, err
	}
	snap, stateCB, noticeCB := s.snap, s.onStateChange, s.onNotice
	s.mu.Unlock()

	if notice != "" && noticeCB != nil {
		noticeCB(NoticeWarning, notice)
	}
	// 状态没变但时长变了，托盘与界面都要刷新。
	if stateCB != nil {
		stateCB(snap)
	}
	return snap, nil
}

// creditSinceLocked 把 RunningSince -> now 这一整段计入 accumulated，
// 并把锚点推进到 now。调用方必须持有锁。返回需要提示用户的文案。
func (s *Service) creditSinceLocked(now time.Time) string {
	if s.snap.State != StateRunning {
		return ""
	}
	delta := now.Sub(s.snap.RunningSince)
	s.snap.RunningSince = now
	if delta < 0 {
		return "检测到系统时间回拨，本次时长未计入"
	}
	s.creditNanos(delta)
	return ""
}

// creditNanos 以纳秒精度累加时长，把整秒并入 AccumulatedSec，
// 余量留在 carryNano 里。这样高频结算不会因为逐次取整而漂移。
// 调用方必须持有锁。
func (s *Service) creditNanos(d time.Duration) {
	if d <= 0 {
		return
	}
	s.carryNano += int64(d)
	if s.carryNano >= int64(time.Second) {
		s.snap.AccumulatedSec += s.carryNano / int64(time.Second)
		s.carryNano %= int64(time.Second)
	}
}

// snapshotFromRow 把持久化行还原成快照。
//
// 第二个返回值表示这行数据是否损坏。损坏时降级为 idle 而不是报错——
// 一条脏数据不该让应用起不来，但必须让用户知道被重置了。
func snapshotFromRow(row store.TimerStateRow) (Snapshot, bool) {
	switch State(row.State) {
	case StateIdle:
		return Idle(), false

	case StatePaused:
		started, err := time.Parse(timeLayout, row.StartedAt)
		if err != nil {
			return Idle(), true
		}
		return Snapshot{
			State:          StatePaused,
			StartedAt:      started,
			AccumulatedSec: row.AccumulatedSec,
		}, false

	case StateRunning:
		started, errStarted := time.Parse(timeLayout, row.StartedAt)
		resumed, errResumed := time.Parse(timeLayout, row.LastResumedAt)
		if errStarted != nil || errResumed != nil {
			return Idle(), true
		}
		return Snapshot{
			State:          StateRunning,
			StartedAt:      started,
			RunningSince:   resumed,
			AccumulatedSec: row.AccumulatedSec,
		}, false

	default:
		return Idle(), true
	}
}

// rowFromSnapshot 把快照写回持久化行。
func rowFromSnapshot(s Snapshot) store.TimerStateRow {
	row := store.TimerStateRow{
		State:          string(s.State),
		AccumulatedSec: s.AccumulatedSec,
	}
	if !s.StartedAt.IsZero() {
		row.StartedAt = s.StartedAt.Format(timeLayout)
	}
	if !s.RunningSince.IsZero() {
		row.LastResumedAt = s.RunningSince.Format(timeLayout)
	}
	return row
}
