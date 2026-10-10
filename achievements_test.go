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

/*
这一组测的是**汇总数字**，不是具体的徽章。

原因：成就表（internal/achievements.All）现在是有意留空的——旧的三条是占位，
已经清掉。所以这里不再断言"某某成就解锁了"，而是断言那些数字本身
（累计时长、场次、天数、连击、深夜/早起），它们才是将来评价成就的原料；
汇总口径错了，徽章一定错，徽章对了也掩盖不了汇总错。
*/

// 成就表为空时，成就页应当是一面空墙，但汇总数字照常给全。
func TestAchievements_NoDefinitions(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if len(sum.Items) != 0 {
		t.Errorf("成就表为空时 items = %d 条, 期望 0 条", len(sum.Items))
	}
	if sum.Total != 0 || sum.Unlocked != 0 {
		t.Errorf("解锁进度 = %d/%d, 期望 0/0", sum.Unlocked, sum.Total)
	}
	if sum.TotalSec != 0 || sum.Sessions != 0 || sum.ActiveDays != 0 ||
		sum.CurrentStreak != 0 || sum.LongestStreak != 0 {
		t.Errorf("空库汇总 = %+v, 期望全 0", sum)
	}
}

// 第一次计时就要把汇总数字算对。
func TestAchievements_FirstRecordUpdatesSummary(t *testing.T) {
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
}

// 连着三天留记录：当前与最长连击都是 3；断掉之后最长连击仍然保留。
//
// 这条留着的意义是守"最长连续"这个量——将来它大概率会被某条成就当判据，
// 而它一旦会往回走，用户就会"徽章突然消失"。
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

	// 隔两天再来一次：当前连击归 1，但最长连击还是 3。
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
		t.Errorf("最长连击 = %d, 期望仍是 3（它会当成就判据，不能往回走）", sum.LongestStreak)
	}
}

// 累计时长按秒累加，不受会话次数影响。
func TestAchievements_TotalAccumulates(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	record(t, h, 9*time.Hour)
	h.clock.Advance(24 * time.Hour)
	record(t, h, 2*time.Hour)

	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.TotalSec != 11*3600 {
		t.Errorf("累计时长 = %d, 期望 %d", sum.TotalSec, 11*3600)
	}
	if sum.Sessions != 2 || sum.ActiveDays != 2 {
		t.Errorf("场次/天数 = %d/%d, 期望 2/2", sum.Sessions, sum.ActiveDays)
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
