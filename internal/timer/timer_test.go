package timer

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"goxuetimer/internal/store"
)

// t0 是所有用例的时间原点。用固定时区偏移而不是 time.Local，
// 好让断言里的 RFC3339 字符串在 CI 换时区时依然稳定。
var t0 = time.Date(2026, 5, 1, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

// recorder 收集回调，用于断言事件与提示。
type recorder struct {
	mu      sync.Mutex
	states  []Snapshot
	notices []string
}

func (r *recorder) attach(s *Service) {
	s.OnStateChange(func(snap Snapshot) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.states = append(r.states, snap)
	})
	s.OnNotice(func(level NoticeLevel, msg string) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.notices = append(r.notices, string(level)+": "+msg)
	})
}

func (r *recorder) stateCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.states)
}

func (r *recorder) joinedNotices() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.notices, " | ")
}

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open() 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newService(t *testing.T, db *store.DB, clock *fakeClock) *Service {
	t.Helper()
	s, err := New(db, clock.Now)
	if err != nil {
		t.Fatalf("timer.New() 失败: %v", err)
	}
	return s
}

func mustStart(t *testing.T, s *Service) Snapshot {
	t.Helper()
	snap, err := s.Start()
	if err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	return snap
}

func dateOf(tm time.Time) string { return tm.Format("2006-01-02") }

// ---------------------------------------------------------------- 状态机错误

func TestStart_WhenRunning_Errors(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)
	mustStart(t, s)

	if _, err := s.Start(); err != ErrAlreadyRunning {
		t.Errorf("第二次 Start() error = %v, 期望 ErrAlreadyRunning", err)
	}

	clock.Advance(10 * time.Second)
	if _, err := s.Pause(); err != nil {
		t.Fatalf("Pause() 失败: %v", err)
	}
	if _, err := s.Start(); err != ErrAlreadyRunning {
		t.Errorf("暂停态 Start() error = %v, 期望 ErrAlreadyRunning", err)
	}
}

func TestPause_WhenIdle_Errors(t *testing.T) {
	db := newTestDB(t)
	s := newService(t, db, newFakeClock(t0))

	if _, err := s.Pause(); err != ErrNotRunning {
		t.Errorf("idle Pause() error = %v, 期望 ErrNotRunning", err)
	}

	mustStart(t, s)
	if _, err := s.Pause(); err != nil {
		t.Fatalf("运行态 Pause() 失败: %v", err)
	}
	if _, err := s.Pause(); err != ErrNotRunning {
		t.Errorf("重复 Pause() error = %v, 期望 ErrNotRunning", err)
	}
}

func TestResume_WhenRunning_Errors(t *testing.T) {
	db := newTestDB(t)
	s := newService(t, db, newFakeClock(t0))

	if _, err := s.Resume(); err != ErrNotPaused {
		t.Errorf("idle Resume() error = %v, 期望 ErrNotPaused", err)
	}

	mustStart(t, s)
	if _, err := s.Resume(); err != ErrNotPaused {
		t.Errorf("运行态 Resume() error = %v, 期望 ErrNotPaused", err)
	}
}

func TestStop_WhenIdle_Errors(t *testing.T) {
	db := newTestDB(t)
	s := newService(t, db, newFakeClock(t0))

	if _, err := s.Stop(); err != ErrIdle {
		t.Errorf("idle Stop() error = %v, 期望 ErrIdle", err)
	}
	if _, err := s.Discard(); err != ErrIdle {
		t.Errorf("idle Discard() error = %v, 期望 ErrIdle", err)
	}
}

// ---------------------------------------------------------------- 核心时长

// 学 10s、停 60s、再学 10s，总时长必须是 20s。
func TestPauseResume_AccumulatesExcludingPaused(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)

	mustStart(t, s)
	clock.Advance(10 * time.Second)
	if _, err := s.Pause(); err != nil {
		t.Fatalf("Pause() 失败: %v", err)
	}

	clock.Advance(60 * time.Second)
	if got := s.ElapsedSec(); got != 10 {
		t.Fatalf("暂停 60s 后 ElapsedSec() = %d, 期望停在 10", got)
	}

	if _, err := s.Resume(); err != nil {
		t.Fatalf("Resume() 失败: %v", err)
	}
	clock.Advance(10 * time.Second)

	out, err := s.Stop()
	if err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}
	if !out.Saved {
		t.Fatalf("Stop() 未保存: %+v", out)
	}
	if out.DurationSec != 20 {
		t.Errorf("DurationSec = %d, 期望 20（暂停区间必须排除）", out.DurationSec)
	}
	if out.Session == nil || out.Session.DurationSec != 20 {
		t.Errorf("落库记录 = %+v", out.Session)
	}
	if out.Snapshot.State != StateIdle {
		t.Errorf("结束后的状态 = %s, 期望 idle", out.Snapshot.State)
	}
}

func TestStop_PersistsExactlyOneRow(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)

	mustStart(t, s)
	clock.Advance(90 * time.Second)

	out, err := s.Stop()
	if err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}
	if out.Session == nil {
		t.Fatal("Stop() 没有返回记录")
	}

	sum, err := db.SummaryByDay(dateOf(t0))
	if err != nil {
		t.Fatalf("SummaryByDay() 失败: %v", err)
	}
	if sum.Count != 1 {
		t.Errorf("记录条数 = %d, 期望 1", sum.Count)
	}
	if sum.TotalSec != 90 {
		t.Errorf("当日总时长 = %d, 期望 90", sum.TotalSec)
	}

	// 会话必须已复位，且落库的时间戳是带本地偏移的 RFC3339。
	if st, err := db.LoadTimerState(); err != nil {
		t.Fatalf("LoadTimerState() 失败: %v", err)
	} else if st.State != string(StateIdle) || st.AccumulatedSec != 0 {
		t.Errorf("落库计时状态 = %+v, 期望 idle/0", st)
	}
	rows, err := db.SessionsByDay(dateOf(t0))
	if err != nil {
		t.Fatalf("SessionsByDay() 失败: %v", err)
	}
	if rows[0].StartedAt != "2026-05-01T09:00:00+08:00" {
		t.Errorf("StartedAt = %q, 期望 2026-05-01T09:00:00+08:00", rows[0].StartedAt)
	}
	if rows[0].EndedAt != "2026-05-01T09:01:30+08:00" {
		t.Errorf("EndedAt = %q, 期望 2026-05-01T09:01:30+08:00", rows[0].EndedAt)
	}
	if rows[0].Source != "timer" {
		t.Errorf("Source = %q, 期望 timer", rows[0].Source)
	}
}

func TestStop_BelowMinDuration_Discards(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)

	mustStart(t, s)
	clock.Advance(3 * time.Second) // < MinSessionSec

	out, err := s.Stop()
	if err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}
	if out.Saved {
		t.Error("3s 的会话被保存了，期望丢弃")
	}
	if out.Reason != "too-short" {
		t.Errorf("Reason = %q, 期望 \"too-short\"", out.Reason)
	}
	if out.Session != nil {
		t.Errorf("丢弃时不应返回记录: %+v", out.Session)
	}
	if out.Snapshot.State != StateIdle {
		t.Errorf("丢弃后状态 = %s, 期望 idle", out.Snapshot.State)
	}

	sum, err := db.SummaryByDay(dateOf(t0))
	if err != nil {
		t.Fatalf("SummaryByDay() 失败: %v", err)
	}
	if sum.Count != 0 {
		t.Errorf("记录条数 = %d, 期望 0", sum.Count)
	}
}

// 刚好等于阈值时应当保存（边界是闭区间）。
func TestStop_ExactlyMinDuration_Saved(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)

	mustStart(t, s)
	clock.Advance(time.Duration(MinSessionSec) * time.Second)

	out, err := s.Stop()
	if err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}
	if !out.Saved || out.DurationSec != MinSessionSec {
		t.Errorf("Stop() = %+v, 期望保存 %ds", out, MinSessionSec)
	}
}

func TestDiscard_WritesNothing(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)

	mustStart(t, s)
	clock.Advance(600 * time.Second)
	if _, err := s.Discard(); err != nil {
		t.Fatalf("Discard() 失败: %v", err)
	}

	sum, err := db.SummaryByDay(dateOf(t0))
	if err != nil {
		t.Fatalf("SummaryByDay() 失败: %v", err)
	}
	if sum.Count != 0 {
		t.Errorf("Discard 后记录条数 = %d, 期望 0", sum.Count)
	}
	st, err := db.LoadTimerState()
	if err != nil {
		t.Fatalf("LoadTimerState() 失败: %v", err)
	}
	if st.State != string(StateIdle) {
		t.Errorf("Discard 后落库状态 = %q, 期望 idle", st.State)
	}
	// 放弃之后应该能直接开始新会话。
	mustStart(t, s)
}

// ---------------------------------------------------------------- 心跳与休眠

// 正常心跳：多次结算不因逐次取整而漂移。
func TestTick_AccumulatesWithoutDrift(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)

	mustStart(t, s)
	const ticks = 10
	const step = 30400 * time.Millisecond // 304s 总计，故意不是整秒的倍数组合

	for i := 0; i < ticks; i++ {
		clock.Advance(step)
		if _, err := s.Tick(); err != nil {
			t.Fatalf("第 %d 次 Tick() 失败: %v", i+1, err)
		}
	}

	want := int64((ticks * step) / time.Second)
	if got := s.Snapshot().AccumulatedSec; got != want {
		t.Errorf("AccumulatedSec = %d, 期望 %d（不应有取整漂移）", got, want)
	}
}

// 一次心跳里墙钟跳了 9 小时：判定为休眠，只计入一个 TickInterval。
func TestTick_SuspiciousDelta_ClampsToTickInterval(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)
	rec := &recorder{}
	rec.attach(s)

	mustStart(t, s)
	clock.Advance(9 * time.Hour)

	if _, err := s.Tick(); err != nil {
		t.Fatalf("Tick() 失败: %v", err)
	}

	snap := s.Snapshot()
	if snap.AccumulatedSec != int64(TickInterval/time.Second) {
		t.Errorf("AccumulatedSec = %d, 期望 %d", snap.AccumulatedSec, int64(TickInterval/time.Second))
	}
	if got := snap.ElapsedSec(clock.Now()); got != int64(TickInterval/time.Second) {
		t.Errorf("ElapsedSec = %d, 期望 %d", got, int64(TickInterval/time.Second))
	}
	if !strings.Contains(rec.joinedNotices(), "休眠") {
		t.Errorf("提示 = %q, 期望包含「休眠」", rec.joinedNotices())
	}

	// 钳制后必须落库，否则重启会把 9 小时又补进来。
	st, err := db.LoadTimerState()
	if err != nil {
		t.Fatalf("LoadTimerState() 失败: %v", err)
	}
	if st.AccumulatedSec != int64(TickInterval/time.Second) {
		t.Errorf("落库 AccumulatedSec = %d, 期望 %d", st.AccumulatedSec, int64(TickInterval/time.Second))
	}
}

func TestTick_WhilePaused_IsNoop(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)

	mustStart(t, s)
	clock.Advance(10 * time.Second)
	if _, err := s.Pause(); err != nil {
		t.Fatalf("Pause() 失败: %v", err)
	}

	clock.Advance(3 * time.Hour)
	if _, err := s.Tick(); err != nil {
		t.Fatalf("暂停态 Tick() 失败: %v", err)
	}

	if got := s.Snapshot().AccumulatedSec; got != 10 {
		t.Errorf("暂停态 Tick 后 AccumulatedSec = %d, 期望仍为 10", got)
	}
}

// ---------------------------------------------------------------- 重启恢复

// gap 在宽限内：整段计入并继续计时。
func TestRecover_GapWithinGrace(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	first := newService(t, db, clock)
	mustStart(t, first)

	// 模拟"刚开始就崩了"：没有心跳，直接重启。
	clock.Advance(5 * time.Minute)

	second := newService(t, db, clock)
	rec := &recorder{}
	rec.attach(second)

	out, err := second.Recover()
	if err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}
	if out.Action != RecoveryContinued {
		t.Errorf("Action = %s, 期望 %s", out.Action, RecoveryContinued)
	}
	if out.SkippedSec != 0 {
		t.Errorf("SkippedSec = %d, 期望 0", out.SkippedSec)
	}
	if out.Snapshot.State != StateRunning {
		t.Errorf("恢复后状态 = %s, 期望 running", out.Snapshot.State)
	}
	if got := out.Snapshot.AccumulatedSec; got != 300 {
		t.Errorf("AccumulatedSec = %d, 期望 300", got)
	}
	// 锚点必须推进到现在，否则每次 Tick 都会把同一段 gap 再计一次。
	if got := out.Snapshot.ElapsedSec(clock.Now()); got != 300 {
		t.Errorf("ElapsedSec = %d, 期望 300", got)
	}
}

// gap 恰好等于宽限上限：闭区间，仍然全额计入。
func TestRecover_GapAtGraceBoundary(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	mustStart(t, newService(t, db, clock))

	clock.Advance(ResumeGraceGap)

	out, err := newService(t, db, clock).Recover()
	if err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}
	if out.Action != RecoveryContinued || out.SkippedSec != 0 {
		t.Errorf("边界处 = %+v, 期望全额计入", out)
	}
	if out.Snapshot.AccumulatedSec != int64(ResumeGraceGap/time.Second) {
		t.Errorf("AccumulatedSec = %d, 期望 %d",
			out.Snapshot.AccumulatedSec, int64(ResumeGraceGap/time.Second))
	}
}

// gap 超过宽限但在放弃阈值内：只补入宽限时长，其余不计入。
func TestRecover_GapBeyondGrace_ClampsToGrace(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	mustStart(t, newService(t, db, clock))

	gap := 2 * time.Hour
	clock.Advance(gap)

	second := newService(t, db, clock)
	rec := &recorder{}
	rec.attach(second)

	out, err := second.Recover()
	if err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}
	if out.Action != RecoveryContinuedClamped {
		t.Errorf("Action = %s, 期望 %s", out.Action, RecoveryContinuedClamped)
	}
	if out.Snapshot.State != StateRunning {
		t.Errorf("恢复后状态 = %s, 期望 running", out.Snapshot.State)
	}
	wantAcc := int64(ResumeGraceGap / time.Second)
	if out.Snapshot.AccumulatedSec != wantAcc {
		t.Errorf("AccumulatedSec = %d, 期望 %d", out.Snapshot.AccumulatedSec, wantAcc)
	}
	wantSkipped := int64((gap - ResumeGraceGap) / time.Second)
	if out.SkippedSec != wantSkipped {
		t.Errorf("SkippedSec = %d, 期望 %d", out.SkippedSec, wantSkipped)
	}
	if !strings.Contains(rec.joinedNotices(), "未计入") {
		t.Errorf("提示 = %q, 期望包含「未计入」", rec.joinedNotices())
	}
}

// gap 超过放弃阈值：按最后一次心跳结算并落库。
func TestRecover_GapBeyondAbandon_AutoStops(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	first := newService(t, db, clock)
	mustStart(t, first)

	// 先正常跑一次心跳，让 accumulated 落库。
	clock.Advance(30 * time.Second)
	if _, err := first.Tick(); err != nil {
		t.Fatalf("Tick() 失败: %v", err)
	}

	// 合盖过夜：从最后一次心跳算起 10 小时。
	clock.Advance(10 * time.Hour)

	second := newService(t, db, clock)
	rec := &recorder{}
	rec.attach(second)

	out, err := second.Recover()
	if err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}
	if out.Action != RecoveryAutoStopped {
		t.Errorf("Action = %s, 期望 %s", out.Action, RecoveryAutoStopped)
	}
	if !out.Saved {
		t.Fatalf("期望落库，实际 = %+v", out)
	}
	if out.DurationSec != 30 {
		t.Errorf("DurationSec = %d, 期望 30（只结算到最后一次心跳）", out.DurationSec)
	}
	if out.Snapshot.State != StateIdle {
		t.Errorf("恢复后状态 = %s, 期望 idle", out.Snapshot.State)
	}

	sum, err := db.SummaryByDay(dateOf(t0))
	if err != nil {
		t.Fatalf("SummaryByDay() 失败: %v", err)
	}
	if sum.Count != 1 || sum.TotalSec != 30 {
		t.Errorf("落库汇总 = %+v, 期望 1 条 30s", sum)
	}
	rows, err := db.SessionsByDay(dateOf(t0))
	if err != nil {
		t.Fatalf("SessionsByDay() 失败: %v", err)
	}
	if rows[0].Source != "recovered" {
		t.Errorf("Source = %q, 期望 recovered", rows[0].Source)
	}
	// 结算时刻必须是最后一次心跳，而不是"现在"。
	if rows[0].EndedAt != "2026-05-01T09:00:30+08:00" {
		t.Errorf("EndedAt = %q, 期望 2026-05-01T09:00:30+08:00", rows[0].EndedAt)
	}
	if st, err := db.LoadTimerState(); err != nil {
		t.Fatalf("LoadTimerState() 失败: %v", err)
	} else if st.State != string(StateIdle) {
		t.Errorf("落库状态 = %q, 期望 idle", st.State)
	}
}

// 中断过久且时长过短：既不落库也不留下运行态。
func TestRecover_GapBeyondAbandon_TooShortNotSaved(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	mustStart(t, newService(t, db, clock))

	clock.Advance(10 * time.Hour)

	out, err := newService(t, db, clock).Recover()
	if err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}
	if out.Action != RecoveryAutoStopped || out.Saved {
		t.Errorf("Recover() = %+v, 期望 auto-stopped 且未保存", out)
	}
	sum, err := db.SummaryByDay(dateOf(t0))
	if err != nil {
		t.Fatalf("SummaryByDay() 失败: %v", err)
	}
	if sum.Count != 0 {
		t.Errorf("记录条数 = %d, 期望 0", sum.Count)
	}
	if out.Snapshot.State != StateIdle {
		t.Errorf("恢复后状态 = %s, 期望 idle", out.Snapshot.State)
	}
}

// 暂停态恢复：原样保留，不涉及离线时长。
func TestRecover_PausedStateRestored(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	first := newService(t, db, clock)
	mustStart(t, first)
	clock.Advance(10 * time.Second)
	if _, err := first.Pause(); err != nil {
		t.Fatalf("Pause() 失败: %v", err)
	}

	clock.Advance(3 * time.Hour)

	out, err := newService(t, db, clock).Recover()
	if err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}
	if out.Action != RecoveryNone {
		t.Errorf("Action = %s, 期望 %s", out.Action, RecoveryNone)
	}
	if out.Snapshot.State != StatePaused {
		t.Errorf("恢复后状态 = %s, 期望 paused", out.Snapshot.State)
	}
	if out.Snapshot.AccumulatedSec != 10 {
		t.Errorf("AccumulatedSec = %d, 期望 10", out.Snapshot.AccumulatedSec)
	}
	if !out.Snapshot.StartedAt.Equal(t0) {
		t.Errorf("StartedAt = %v, 期望 %v", out.Snapshot.StartedAt, t0)
	}
}

// 系统时间被回拨：不猜，锚点拉回现在，且不能算出负数时长。
func TestRecover_TimeWentBackwards(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	mustStart(t, newService(t, db, clock))

	clock.Set(t0.Add(-1 * time.Hour))

	second := newService(t, db, clock)
	rec := &recorder{}
	rec.attach(second)

	out, err := second.Recover()
	if err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}
	if out.Snapshot.State != StateRunning {
		t.Errorf("恢复后状态 = %s, 期望 running", out.Snapshot.State)
	}
	if out.Snapshot.AccumulatedSec != 0 {
		t.Errorf("AccumulatedSec = %d, 期望 0", out.Snapshot.AccumulatedSec)
	}
	if got := out.Snapshot.ElapsedSec(clock.Now()); got != 0 {
		t.Errorf("ElapsedSec = %d, 期望 0", got)
	}
	if !strings.Contains(rec.joinedNotices(), "回拨") {
		t.Errorf("提示 = %q, 期望包含「回拨」", rec.joinedNotices())
	}
}

// 持久化状态损坏：降级为 idle 并提示，而不是让应用起不来。
func TestRecover_CorruptRow_Resets(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)

	if err := db.SaveTimerState(store.TimerStateRow{
		State:          "running",
		StartedAt:      "不是时间",
		LastResumedAt:  "也不是时间",
		AccumulatedSec: 999,
	}); err != nil {
		t.Fatalf("SaveTimerState() 失败: %v", err)
	}

	s := newService(t, db, clock)
	rec := &recorder{}
	rec.attach(s)

	out, err := s.Recover()
	if err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}
	if out.Action != RecoveryReset {
		t.Errorf("Action = %s, 期望 %s", out.Action, RecoveryReset)
	}
	if out.Snapshot.State != StateIdle {
		t.Errorf("恢复后状态 = %s, 期望 idle", out.Snapshot.State)
	}
	if out.Snapshot.AccumulatedSec != 0 {
		t.Errorf("AccumulatedSec = %d, 期望 0", out.Snapshot.AccumulatedSec)
	}
	if !strings.Contains(rec.joinedNotices(), "损坏") {
		t.Errorf("提示 = %q, 期望包含「损坏」", rec.joinedNotices())
	}
	if st, err := db.LoadTimerState(); err != nil {
		t.Fatalf("LoadTimerState() 失败: %v", err)
	} else if st.State != string(StateIdle) {
		t.Errorf("落库状态 = %q, 期望 idle", st.State)
	}

	// 清理之后应当能正常开始新会话。
	mustStart(t, s)
}

// ---------------------------------------------------------------- 写库失败

// 写库失败时内存状态必须原样回滚，否则界面会显示一个不存在的状态。
func TestDBWriteFailure_LeavesMemoryUnchanged(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)

	mustStart(t, s)
	clock.Advance(75 * time.Second)

	before := s.Snapshot()
	if err := db.Close(); err != nil {
		t.Fatalf("Close() 失败: %v", err)
	}

	if _, err := s.Pause(); err == nil {
		t.Fatal("Pause() 在库关闭后仍返回 nil，期望报错")
	}
	after := s.Snapshot()
	if after.State != before.State {
		t.Errorf("状态 = %s, 期望回滚为 %s", after.State, before.State)
	}
	if after.AccumulatedSec != before.AccumulatedSec {
		t.Errorf("AccumulatedSec = %d, 期望回滚为 %d", after.AccumulatedSec, before.AccumulatedSec)
	}
	if !after.RunningSince.Equal(before.RunningSince) {
		t.Errorf("RunningSince = %v, 期望回滚为 %v", after.RunningSince, before.RunningSince)
	}

	// Stop 也必须失败并且不改变状态。
	if _, err := s.Stop(); err == nil {
		t.Fatal("Stop() 在库关闭后仍返回 nil，期望报错")
	}
	if got := s.Snapshot(); got.State != StateRunning || got.AccumulatedSec != before.AccumulatedSec {
		t.Errorf("Stop 失败后状态 = %+v, 期望保持 %+v", got, before)
	}
}

// 回调必须在锁外触发，否则回调里再访问 Service 会死锁。
func TestCallbacks_DoNotDeadlock(t *testing.T) {
	db := newTestDB(t)
	clock := newFakeClock(t0)
	s := newService(t, db, clock)

	called := make(chan struct{}, 1)
	s.OnStateChange(func(Snapshot) {
		// 回调内回读状态：若回调在持锁时被调用，这里会卡死。
		_ = s.Snapshot()
		called <- struct{}{}
	})

	errc := make(chan error, 1)
	go func() {
		clock.Advance(time.Second)
		_, err := s.Start()
		errc <- err
	}()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("Start() 失败: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() 疑似死锁")
	}

	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("状态回调没有被触发")
	}
}
