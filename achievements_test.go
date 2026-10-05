package main

import (
	"testing"
	"time"
)

// record 跑一次完整的会话：开始 -> 推进假时钟 -> 结束并保存。
func record(t *testing.T, h *harness, dur time.Duration) {
	t.Helper()
	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(dur)
	if _, err := h.timer.Stop(); err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}
}

func achievementByID(t *testing.T, sum AchievementsDTO, id string) AchievementDTO {
	t.Helper()
	for _, item := range sum.Items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("没有找到成就 %s", id)
	return AchievementDTO{}
}

// 空库也要给出一份完整的汇总：三个成就全锁着，数字全 0，不能报错。
func TestAchievements_Empty(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.Unlocked != 0 || sum.Total != 3 {
		t.Errorf("解锁进度 = %d/%d, 期望 0/3", sum.Unlocked, sum.Total)
	}
	if sum.TotalSec != 0 || sum.Sessions != 0 || sum.ActiveDays != 0 ||
		sum.CurrentStreak != 0 || sum.LongestStreak != 0 {
		t.Errorf("空库汇总 = %+v, 期望全 0", sum)
	}
	for _, item := range sum.Items {
		if item.Unlocked {
			t.Errorf("%s 在空库上就解锁了", item.ID)
		}
		if item.Progress != 0 {
			t.Errorf("%s 进度 = %d, 期望 0", item.ID, item.Progress)
		}
	}
}

// 第一次计时就解锁"第一步"，并让汇总数字跟着动。
func TestAchievements_FirstStep(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)
	record(t, h, 25*time.Minute)

	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.Sessions != 1 || sum.ActiveDays != 1 {
		t.Errorf("场次/天数 = %d/%d, 期望 1/1", sum.Sessions, sum.ActiveDays)
	}
	if sum.TotalSec != 25*60 {
		t.Errorf("累计时长 = %d, 期望 %d", sum.TotalSec, 25*60)
	}
	if sum.LongestSec != 25*60 {
		t.Errorf("单次最长 = %d, 期望 %d", sum.LongestSec, 25*60)
	}
	if sum.CurrentStreak != 1 || sum.LongestStreak != 1 {
		t.Errorf("连击 = (%d, %d), 期望 (1, 1)", sum.CurrentStreak, sum.LongestStreak)
	}
	if item := achievementByID(t, sum, "first-step"); !item.Unlocked {
		t.Errorf("first-step 应当解锁: %+v", item)
	}
	if sum.Unlocked != 1 {
		t.Errorf("解锁数 = %d, 期望 1", sum.Unlocked)
	}
}

// 连着三天留记录解锁"三日不辍"；断掉之后"最长连续"仍然保留（成就不会丢）。
func TestAchievements_StreakSurvivesBreak(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	for day := 0; day < 3; day++ {
		if day > 0 {
			h.clock.Advance(24 * time.Hour)
		}
		record(t, h, 10*time.Minute)
	}

	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.LongestStreak != 3 || sum.CurrentStreak != 3 {
		t.Errorf("连击 = (%d, %d), 期望 (3, 3)", sum.CurrentStreak, sum.LongestStreak)
	}
	if item := achievementByID(t, sum, "streak-3"); !item.Unlocked {
		t.Errorf("streak-3 应当解锁: %+v", item)
	}

	// 隔两天再来一次：当前连击归 1，但最长连击还是 3，成就不能因此消失。
	h.clock.Advance(72 * time.Hour)
	record(t, h, 10*time.Minute)

	sum, err = svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.CurrentStreak != 1 {
		t.Errorf("断掉之后当前连击 = %d, 期望 1", sum.CurrentStreak)
	}
	if sum.LongestStreak != 3 {
		t.Errorf("最长连击 = %d, 期望仍是 3", sum.LongestStreak)
	}
	if item := achievementByID(t, sum, "streak-3"); !item.Unlocked {
		t.Error("断掉之后 streak-3 不该重新锁上——判据是单调的")
	}
}

// 累计满十小时解锁"十小时"，进度按整小时走。
func TestAchievements_TenHours(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	// 先记 9 小时：还差一点。
	record(t, h, 9*time.Hour)
	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	item := achievementByID(t, sum, "hours-10")
	if item.Unlocked {
		t.Errorf("9 小时就解锁了十小时: %+v", item)
	}
	if item.Progress != 9 || item.Target != 10 || item.Unit != "小时" {
		t.Errorf("进度 = %d/%d %s, 期望 9/10 小时", item.Progress, item.Target, item.Unit)
	}

	// 再补 2 小时（累计 11 小时）。
	h.clock.Advance(24 * time.Hour)
	record(t, h, 2*time.Hour)

	sum, err = svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	item = achievementByID(t, sum, "hours-10")
	if !item.Unlocked {
		t.Errorf("11 小时应当解锁: %+v", item)
	}
	// 进度封顶在门槛上：进度条按百分比画，超过 100% 会溢出。
	if item.Progress != 10 {
		t.Errorf("进度 = %d, 期望封顶在 10", item.Progress)
	}
	if sum.Unlocked != 2 {
		t.Errorf("解锁数 = %d, 期望 2（第一步 + 十小时）", sum.Unlocked)
	}
}

// 深夜与早起的场次按**本地小时**统计（started_at 自带偏移）。
func TestAchievements_NightAndMorningCounts(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	// 假时钟从 2026-05-01 09:00 +08:00 起步（见 service_integration_test.go 的 t0）。
	// at 把时钟推到指定的 5 月某天某时刻。
	at := func(day, hour, minute int) {
		now := h.clock.Now()
		target := time.Date(2026, 5, day, hour, minute, 0, 0, now.Location())
		h.clock.Advance(target.Sub(now))
	}

	at(1, 23, 30) // 深夜
	record(t, h, 10*time.Minute)
	at(2, 6, 10) // 第二天清晨
	record(t, h, 10*time.Minute)

	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.LateNightCount != 1 {
		t.Errorf("深夜场次 = %d, 期望 1", sum.LateNightCount)
	}
	if sum.EarlyMorningCount != 1 {
		t.Errorf("早起场次 = %d, 期望 1", sum.EarlyMorningCount)
	}
	// 两天的记录顺带验证：这两天是连着的。
	if sum.ActiveDays != 2 || sum.LongestStreak != 2 {
		t.Errorf("天数/最长连击 = %d/%d, 期望 2/2", sum.ActiveDays, sum.LongestStreak)
	}
}
