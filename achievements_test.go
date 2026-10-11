package main

import (
	"testing"
	"time"

	"goxuetimer/internal/achievements"
	"goxuetimer/internal/store"
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
这一组测成就页的**汇总与账**：

  - 汇总数字（累计时长、场次、天数、连击）由记录推导，口径错了徽章一定错；
  - 等级与经验是**存下来的账**，只在"结束并保存"时结算；
  - 两条硬约束在这里各有一条用例守着：账不等于回放（同一套规则下必须相等）、
    以及删记录不回收已发的进度。
*/

// 全新库里：等级从 Lv.1 / 0 经验开始，九个成就里只有"第一次创建计划"已达成
// （夹具建过默认计划），其余都还没解锁。
func TestAchievements_FreshStart(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.Total != 9 {
		t.Errorf("成就数 = %d, 期望 9", sum.Total)
	}
	if sum.Level.Level != 1 || sum.Level.XpTenths != 0 {
		t.Errorf("初始等级/经验 = %d / %d, 期望 1 / 0", sum.Level.Level, sum.Level.XpTenths)
	}
	if sum.Level.RateTenths != 10 {
		t.Errorf("初始每小时经验 = %d, 期望 10（即 1.0）", sum.Level.RateTenths)
	}
	if sum.Level.NeedTenths != 1000 {
		t.Errorf("1 级升 2 级需要 %d, 期望 1000（即 100 点）", sum.Level.NeedTenths)
	}
	if sum.Unlocked != 1 {
		t.Errorf("解锁数 = %d, 期望 1（只有 first-plan）", sum.Unlocked)
	}
	for _, item := range sum.Items {
		switch item.ID {
		case "first-plan":
			if !item.Unlocked || item.AchievedAt == "" {
				t.Errorf("first-plan 应当已达成并记下时间: %+v", item)
			}
		default:
			if item.Unlocked {
				t.Errorf("%s 在空库上就解锁了", item.ID)
			}
		}
	}
}

// 结束并保存时结算：满 1 小时的那次会话给"每小时经验 + 每日奖励"。
func TestAchievements_SettlesOnSave(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	// 两次半小时，同一天。
	record(t, h, 30*time.Minute)
	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.Level.XpTenths != 0 || sum.QualifyingDays != 0 {
		t.Errorf("只学半小时就有经验了: %+v", sum.Level)
	}

	record(t, h, 30*time.Minute)
	sum, err = svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	// 满 1 小时：成长 1.0 点（10）+ 每日 10 点（100）= 110 个十分之一
	if sum.Level.XpTenths != 110 {
		t.Errorf("经验 = %d, 期望 110", sum.Level.XpTenths)
	}
	if sum.QualifyingDays != 1 {
		t.Errorf("达标日 = %d, 期望 1", sum.QualifyingDays)
	}
	if sum.Level.HoursCounted != 1 {
		t.Errorf("已计入的整小时 = %d, 期望 1", sum.Level.HoursCounted)
	}
	for _, item := range sum.Items {
		if item.ID != "daily-hour" {
			continue
		}
		if !item.Unlocked || item.Count != 1 || item.LastAt == "" || !item.TodayDone {
			t.Errorf("每日成就状态 = %+v", item)
		}
	}
}

// 放弃的那段不给经验（只有落库的记录才算数）。
func TestAchievements_DiscardGivesNoXP(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(2 * time.Hour)
	if _, err := h.timer.Discard(); err != nil {
		t.Fatalf("Discard() 失败: %v", err)
	}

	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.Level.XpTenths != 0 || sum.QualifyingDays != 0 || sum.Sessions != 0 {
		t.Errorf("放弃之后不该有任何进度: %+v", sum.Level)
	}
}

// 删记录不回收已经发放的经验与奖章（进度只增不减）。
func TestAchievements_DeleteDoesNotTakeBackProgress(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	record(t, h, 2*time.Hour)
	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	before := sum.Level.XpTenths
	if before == 0 {
		t.Fatal("两小时应当已经结算出经验")
	}

	// 直接把记录删掉（界面里暂时没有入口，但接口在）。
	rows, err := h.b.db.SessionsByDay(h.clock.Now().Format(layoutDateKey), 0)
	if err != nil {
		t.Fatalf("SessionsByDay() 失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("记录条数 = %d, 期望 1", len(rows))
	}
	if err := h.b.db.DeleteSession(rows[0].ID); err != nil {
		t.Fatalf("DeleteSession() 失败: %v", err)
	}

	sum, err = svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.Level.XpTenths != before {
		t.Errorf("删记录之后经验从 %d 变成 %d，进度被回收了", before, sum.Level.XpTenths)
	}
	if sum.QualifyingDays != 1 {
		t.Errorf("达标日 = %d, 期望仍然是 1（账本只增不减）", sum.QualifyingDays)
	}
}

// 硬断言：同一套规则下，**存下来的账必须等于从记录回放出来的账**。
//
// 这条是"把账存下来"这套设计的保险：存的东西一旦与记录分叉，
// 说明某条写路径漏了结算或者算错了，测试会立刻发现。
func TestAchievements_StoredStateMatchesReplay(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)

	// 造几天数据：一天一小时、一天两小时、一天十分钟。
	at := func(day, hour int) {
		now := h.clock.Now()
		target := time.Date(2026, 5, day, hour, 0, 0, 0, now.Location())
		h.clock.Advance(target.Sub(now))
	}
	at(1, 9)
	record(t, h, time.Hour)
	at(2, 9)
	record(t, h, 2*time.Hour)
	at(3, 9)
	record(t, h, 10*time.Minute)

	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}

	stats, err := h.b.db.Stats(h.clock.Now().Format(layoutDateKey))
	if err != nil {
		t.Fatalf("Stats() 失败: %v", err)
	}
	days := make([]achievements.Day, 0, len(stats.Days))
	for _, d := range stats.Days {
		days = append(days, achievements.Day{Date: d.Date, Sec: d.TotalSec})
	}
	firstPlanAt, err := h.b.db.FirstPlanCreatedAt()
	if err != nil {
		t.Fatalf("FirstPlanCreatedAt() 失败: %v", err)
	}
	replayed, _, _ := achievements.Replay(days, firstPlanAt)

	if replayed.XpTenths != sum.Level.XpTenths {
		t.Errorf("账里的经验 = %d, 回放 = %d", sum.Level.XpTenths, replayed.XpTenths)
	}
	if replayed.CarrySec != sum.Level.CarrySec {
		t.Errorf("账里的零头 = %d, 回放 = %d", sum.Level.CarrySec, replayed.CarrySec)
	}
	if replayed.Medals.Week != sum.Level.Medals.Week ||
		replayed.Medals.Month != sum.Level.Medals.Month ||
		replayed.Medals.Quarter != sum.Level.Medals.Quarter ||
		replayed.Medals.Year != sum.Level.Medals.Year {
		t.Errorf("账里的奖章 = %+v, 回放 = %+v", sum.Level.Medals, replayed.Medals)
	}
}

// 老库升级：账是空的但有记录时，按记录回放补一次初值。
func TestAchievements_BackfillsFromRecords(t *testing.T) {
	h := newHarness(t)

	// 造一段记录，然后把账清空——模拟"刚从 v2 升到 v3"。
	record(t, h, 3*time.Hour)
	if err := h.b.db.ReplaceProgress(store.ProgressRow{Level: 1}, nil, nil, ""); err != nil {
		t.Fatalf("清空进度失败: %v", err)
	}

	// 重新构造 backend：这一步会触发回填。
	again, err := newBackend(h.b.db, h.clock.Now)
	if err != nil {
		t.Fatalf("newBackend() 失败: %v", err)
	}
	sum, err := NewAchievementsService(again).Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	// 3 小时：3 个整点（3.0 点）+ 每日 10 点 = 130 个十分之一
	if sum.Level.XpTenths != 130 {
		t.Errorf("回填后的经验 = %d, 期望 130", sum.Level.XpTenths)
	}
	if sum.QualifyingDays != 1 {
		t.Errorf("回填后的达标日 = %d, 期望 1", sum.QualifyingDays)
	}
	// 回填的达成时间要精确到分钟（用那天最后一条记录的结束时刻），不是只有日期。
	for _, item := range sum.Items {
		if item.ID != "daily-hour" {
			continue
		}
		if len(item.LastAt) <= len("2006-01-02") {
			t.Errorf("回填后的达成时间只有日期: %q", item.LastAt)
		}
	}
}

// 只到日期的旧达成时间会被补精确（只填空，不动日期/次数/经验）。
func TestAchievements_EnrichesDateOnlyStamps(t *testing.T) {
	h := newHarness(t)
	svc := NewAchievementsService(h.b)
	record(t, h, 2*time.Hour)

	after, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	var before int64
	for _, item := range after.Items {
		if item.ID == "daily-hour" {
			if item.Count != 1 {
				t.Fatalf("每日达成次数 = %d, 期望 1", item.Count)
			}
			// 人为退回"只有日期"的老形态。
			day := item.LastAt[:10]
			if err := h.b.db.SaveAchievementState(store.AchievementStateRow{
				ID: "daily-hour", Count: 1, LastAt: day,
			}); err != nil {
				t.Fatalf("退回旧形态失败: %v", err)
			}
		}
	}
	before = after.Level.XpTenths

	if err := h.b.enrichStampPrecision(); err != nil {
		t.Fatalf("enrichStampPrecision() 失败: %v", err)
	}

	sum, err := svc.Summary()
	if err != nil {
		t.Fatalf("Summary() 失败: %v", err)
	}
	if sum.Level.XpTenths != before {
		t.Errorf("补精度动了经验: %d -> %d", before, sum.Level.XpTenths)
	}
	for _, item := range sum.Items {
		if item.ID != "daily-hour" {
			continue
		}
		if item.Count != 1 {
			t.Errorf("补精度动了达成次数: %d", item.Count)
		}
		if len(item.LastAt) <= len("2006-01-02") {
			t.Errorf("补精度之后仍然只有日期: %q", item.LastAt)
		}
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
