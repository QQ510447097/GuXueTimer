// Package timer 实现学习计时的状态机与崩溃恢复。
//
// 这个包刻意不依赖 Wails：状态变化和提示通过回调抛给调用方，
// 因此可以用注入的假时钟在毫秒内跑完所有时间相关的用例。
package timer

import (
	"errors"
	"fmt"
	"time"
)

// State 是计时器的状态。
type State string

const (
	StateIdle    State = "idle"
	StateRunning State = "running"
	StatePaused  State = "paused"
)

const (
	// TickInterval 是后端结算心跳周期，也是休眠判定后的计入上限。
	TickInterval = 30 * time.Second

	// SuspiciousDelta：一次心跳里墙钟跳了这么多，判定为休眠/待机，
	// 本次只计入 TickInterval，避免合盖过夜把记录撑爆。
	SuspiciousDelta = 5 * time.Minute

	// ResumeGraceGap：重启后离线时长在此以内，认为计时是连续的，全额计入。
	ResumeGraceGap = 15 * time.Minute

	// ResumeAbandonGap：重启后离线时长超过此值，认为会话已在中断时结束，
	// 按 last_resumed_at 结算并落库。
	ResumeAbandonGap = 8 * time.Hour

	// MinSessionSec：短于此的会话视为误触，结束时不落库。
	MinSessionSec = 5
)

const (
	// timeLayout 用于内部计时状态：保留纳秒，重启后的 gap 才不受截断影响。
	timeLayout = time.RFC3339Nano
	// recordLayout 用于落库的学习记录：秒级足够，且更可读。
	recordLayout = time.RFC3339
)

var (
	// ErrAlreadyRunning 表示已有会话在进行中。
	ErrAlreadyRunning = errors.New("已有正在进行的计时")
	// ErrNotRunning 表示当前不在运行状态。
	ErrNotRunning = errors.New("当前没有正在进行的计时")
	// ErrNotPaused 表示当前不在暂停状态。
	ErrNotPaused = errors.New("当前不处于暂停状态")
	// ErrIdle 表示当前没有任何计时会话。
	ErrIdle = errors.New("当前没有计时会话")
)

// Snapshot 是计时器在某一时刻的完整状态，是前端与托盘渲染的唯一依据。
type Snapshot struct {
	State          State
	StartedAt      time.Time // 本次会话开始时刻；idle 时为零值
	AccumulatedSec int64     // 已确定的时长，不含正在走的这一段
	RunningSince   time.Time // 最近一次进入 running 的时刻；仅 running 时有意义
}

// ElapsedSec 返回 now 时刻的会话总时长（秒）。
func (s Snapshot) ElapsedSec(now time.Time) int64 {
	sec := s.AccumulatedSec
	if s.State == StateRunning && !s.RunningSince.IsZero() {
		if extra := now.Sub(s.RunningSince); extra > 0 {
			sec += int64(extra / time.Second)
		}
	}
	return sec
}

// Idle 返回一个干净的 idle 快照。
func Idle() Snapshot { return Snapshot{State: StateIdle} }

// NoticeLevel 与前端 NNotification 的语义对齐。
type NoticeLevel string

const (
	NoticeInfo    NoticeLevel = "info"
	NoticeWarning NoticeLevel = "warning"
)

// RecoveryAction 说明 Recover 做了什么，用于启动时给用户一句可读的交代。
type RecoveryAction string

const (
	RecoveryNone             RecoveryAction = "none"
	RecoveryContinued        RecoveryAction = "continued"
	RecoveryContinuedClamped RecoveryAction = "continued-clamped"
	RecoveryAutoStopped      RecoveryAction = "auto-stopped"
	RecoveryReset            RecoveryAction = "reset"
)

// RecoveryOutcome 是 Recover 的结果。
type RecoveryOutcome struct {
	Action      RecoveryAction
	Saved       bool  // AutoStopped 时是否真的落库（时长过短则为 false）
	SkippedSec  int64 // 因超出宽限而未计入的秒数
	DurationSec int64
	SessionID   int64
	Snapshot    Snapshot
}

// humanDuration 把时长渲染成「1 小时 5 分钟」这类可读文本，用于提示语。
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s == 0 {
			return fmt.Sprintf("%d 分钟", m)
		}
		return fmt.Sprintf("%d 分 %d 秒", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%d 小时", h)
	}
	return fmt.Sprintf("%d 小时 %d 分", h, m)
}
