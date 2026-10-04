package reminder

import (
	"testing"
	"time"
)

// armed 把计数器推到"已对齐基准"的状态，等价于心跳刚跑完第一拍。
func armed(r *Reminder, enabled bool, interval time.Duration) {
	r.Configure(enabled, interval)
	r.Tick(0)
}

func TestTick_DisabledNeverFires(t *testing.T) {
	r := New()
	armed(r, false, time.Minute)

	for i := int64(1); i <= 10; i++ {
		if r.Tick(i * 30) {
			t.Fatal("提醒已关闭，不应触发")
		}
	}
	if r.FiredCount() != 0 {
		t.Errorf("FiredCount = %d, 期望 0", r.FiredCount())
	}
	if _, interval := r.Snapshot(); interval != 0 {
		t.Errorf("关闭时 intervalSec = %d, 期望 0（界面据此隐藏进度条）", interval)
	}
}

func TestTick_ZeroIntervalNeverFires(t *testing.T) {
	r := New()
	armed(r, true, 0)

	for i := int64(1); i <= 5; i++ {
		if r.Tick(i * 60) {
			t.Fatal("间隔为 0，不应触发")
		}
	}
}

// 刚好等于间隔时触发（闭区间）。
func TestTick_ExactIntervalFires(t *testing.T) {
	r := New()
	armed(r, true, time.Minute)

	if !r.Tick(60) {
		t.Fatal("恰好到 60s 时应触发")
	}
	if r.FiredCount() != 1 {
		t.Errorf("FiredCount = %d, 期望 1", r.FiredCount())
	}
	if elapsed, _ := r.Snapshot(); elapsed != 0 {
		t.Errorf("触发后 elapsed = %d, 期望清零", elapsed)
	}
}

// 触发后重新计数，可以周期性重复提醒。
func TestTick_FiresRepeatedly(t *testing.T) {
	r := New()
	armed(r, true, time.Minute)

	fires := 0
	for i := int64(1); i <= 20; i++ {
		if r.Tick(i * 30) {
			fires++
		}
	}
	if fires != 10 {
		t.Errorf("触发次数 = %d, 期望 10", fires)
	}
}

// 暂停期间计时器的秒数不动，所以提醒进度也不该前进。
func TestTick_PauseDoesNotAccumulate(t *testing.T) {
	r := New()
	armed(r, true, 2*time.Minute)

	if r.Tick(30) {
		t.Fatal("30s 时不应触发")
	}
	// 暂停 10 分钟后恢复，秒数一直停在 30。
	for i := 0; i < 5; i++ {
		if r.Tick(30) {
			t.Fatal("暂停期间不应触发")
		}
	}
	if elapsed, _ := r.Snapshot(); elapsed != 30 {
		t.Fatalf("暂停后 elapsed = %d, 期望仍为 30", elapsed)
	}

	// 恢复后继续按真实学习秒数推进：再 90s 到点。
	if r.Tick(60) || r.Tick(90) {
		t.Fatal("90s 时不应触发")
	}
	if !r.Tick(120) {
		t.Fatal("累计到 120s 时应触发")
	}
	if elapsed, _ := r.Snapshot(); elapsed != 0 {
		t.Errorf("触发后 elapsed = %d, 期望清零", elapsed)
	}
}

// 新会话让计时器秒数归零：提醒必须跟着从零重算，而不是当成负增量。
func TestTick_NewSessionStartsOver(t *testing.T) {
	r := New()
	armed(r, true, time.Minute)

	if r.Tick(50) {
		t.Fatal("50s 时不应触发")
	}
	if elapsed, _ := r.Snapshot(); elapsed != 50 {
		t.Fatalf("elapsed = %d, 期望 50", elapsed)
	}

	// 上一个会话结束、新会话开始：秒数归零。
	if r.Tick(0) {
		t.Fatal("归零时不应触发")
	}
	if elapsed, _ := r.Snapshot(); elapsed != 0 {
		t.Errorf("新会话开始后 elapsed = %d, 期望 0", elapsed)
	}

	// 新会话的第一拍只对齐基准。
	if r.Tick(30) {
		t.Fatal("新会话 30s 时不应触发")
	}
	if !r.Tick(60) {
		t.Fatal("新会话累计到 60s 时应触发")
	}
}

// 缩短间隔后应立即触发，而不是再等一个完整周期。
func TestConfigure_ShrinkingIntervalFiresImmediately(t *testing.T) {
	r := New()
	armed(r, true, 45*time.Minute)

	// 已学 10 分钟。
	for i := int64(1); i <= 20; i++ {
		if r.Tick(i * 30) {
			t.Fatal("10 分钟内不应触发")
		}
	}

	r.Configure(true, 5*time.Minute)
	if !r.Tick(630) {
		t.Fatal("把间隔调成 5 分钟后应立刻触发")
	}
}

// 从关闭切回开启时，基准要重新对齐，不能把关闭期间跳变的秒数算进来。
func TestConfigure_ReenableRealignsBase(t *testing.T) {
	r := New()
	armed(r, false, time.Minute)

	if r.Tick(600) {
		t.Fatal("关闭时不应触发")
	}

	r.Configure(true, time.Minute)
	if r.Tick(6000) {
		t.Fatal("重新开启后的第一拍只应对齐基准")
	}
	if elapsed, _ := r.Snapshot(); elapsed != 0 {
		t.Errorf("elapsed = %d, 期望 0", elapsed)
	}
	if !r.Tick(6060) {
		t.Fatal("对齐基准后再累计 60s 应触发")
	}
}

func TestReset_ClearsProgressAndBase(t *testing.T) {
	r := New()
	armed(r, true, time.Minute)

	if r.Tick(30) {
		t.Fatal("30s 时不应触发")
	}
	r.Reset()
	if elapsed, _ := r.Snapshot(); elapsed != 0 {
		t.Errorf("Reset() 后 elapsed = %d, 期望 0", elapsed)
	}

	// 重置后第一拍只对齐基准。
	if r.Tick(600) {
		t.Fatal("重置后的第一拍不应触发")
	}
	if elapsed, _ := r.Snapshot(); elapsed != 0 {
		t.Errorf("elapsed = %d, 期望 0", elapsed)
	}
}

// 系统时间回拨导致秒数倒退时，不能算出负数，也不能累积负值。
func TestTick_BackwardsSecResetsNotNegative(t *testing.T) {
	r := New()
	armed(r, true, time.Minute)

	if r.Tick(45) {
		t.Fatal("45s 时不应触发")
	}
	if r.Tick(10) {
		t.Fatal("倒退时不应触发")
	}
	if elapsed, _ := r.Snapshot(); elapsed != 0 {
		t.Errorf("elapsed = %d, 期望 0（倒退视为新会话，而不是累加负数）", elapsed)
	}
	// 从 10 继续推进：需要再 60s。
	if r.Tick(50) {
		t.Fatal("50s 时不应触发")
	}
	if !r.Tick(70) {
		t.Fatal("累计到 60s 时应触发")
	}
}
