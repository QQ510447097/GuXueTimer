// Package reminder 统计"连续学习时长"，到点提醒用户起来休息。
//
// 它刻意不自己算时间：直接镜像计时器的权威秒数（会话总时长）。
// 计时器那边已经处理了暂停排除、休眠钳制和写库，这里再实现一遍墙钟差
// 只会得到两套互相不一致的判定。
package reminder

import (
	"sync"
	"time"
)

// Reminder 是休息提醒的计数器。
type Reminder struct {
	mu sync.Mutex

	enabled     bool
	intervalSec int64

	elapsedSec int64 // 本轮已累计的学习秒数
	baseSec    int64 // 上一次观测到的计时器秒数
	tracking   bool  // 是否已经对齐过基准
	firedCount int64
}

// New 返回一个未配置的计数器（默认关闭）。
func New() *Reminder { return &Reminder{} }

// Configure 更新开关与间隔。
//
// 关闭时把 intervalSec 直接归零，这样 Snapshot 的返回值就是自解释的：
// intervalSec == 0 等价于"提醒已关闭"，界面据此隐藏进度条。
//
// 缩短间隔时**不**清零已累计的时长：用户把 45 分钟改成 5 分钟，
// 本来就该立刻收到提醒，而不是再等 5 分钟。
func (r *Reminder) Configure(enabled bool, interval time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
	if !enabled || interval <= 0 {
		r.intervalSec = 0
	} else {
		r.intervalSec = int64(interval / time.Second)
	}
}

// Tick 由统一心跳驱动，currentSec 是计时器此刻的会话总秒数。
// 返回 true 表示本轮到达阈值、应发一次提醒。
//
// 暂停期间计时器的秒数不变，所以这里的增量自然是 0，不需要额外的状态判断。
func (r *Reminder) Tick(currentSec int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.enabled || r.intervalSec <= 0 {
		r.tracking = false
		return false
	}

	if !r.tracking {
		// 第一次观测，或刚从关闭/重置状态恢复：只对齐基准，不累加。
		r.baseSec = currentSec
		r.tracking = true
		return false
	}

	delta := currentSec - r.baseSec
	r.baseSec = currentSec
	switch {
	case delta > 0:
		// 正常推进。
	case delta < 0:
		// 计数器倒退只可能是新会话开始（计时器秒数归零），
		// 或者是系统时间回拨。两种情况都该从零重新算连续学习。
		r.elapsedSec = 0
		return false
	default:
		return false
	}

	r.elapsedSec += delta
	if r.elapsedSec >= r.intervalSec {
		r.elapsedSec = 0
		r.firedCount++
		return true
	}
	return false
}

// Reset 清零计数并丢掉基准。会话结束（Stop/Discard）时调用。
func (r *Reminder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.elapsedSec = 0
	r.baseSec = 0
	r.tracking = false
}

// Snapshot 一次性取回进度，避免调用方为两个字段各加一次锁。
// 返回的 intervalSec 为 0 表示提醒已关闭。
func (r *Reminder) Snapshot() (elapsedSec, intervalSec int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.elapsedSec, r.intervalSec
}

// FiredCount 返回累计触发次数，供测试断言。
func (r *Reminder) FiredCount() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.firedCount
}
