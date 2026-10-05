package store

import (
	"path/filepath"
	"testing"
)

// insertSession 造一条记录：起止时间都用同一个 started，时长单独给。
func insertSession(t *testing.T, db *DB, started string, sec int64, planID int64) {
	t.Helper()
	if _, err := db.InsertSession(SessionRow{
		StartedAt: started, EndedAt: started, DurationSec: sec,
		Source: "timer", CreatedAt: started, PlanID: planID,
	}); err != nil {
		t.Fatalf("InsertSession() 失败: %v", err)
	}
}

// daysOf 把日期串列表转成 DayTotal（时长对连击没有影响）。
func daysOf(dates ...string) []DayTotal {
	out := make([]DayTotal, 0, len(dates))
	for _, d := range dates {
		out = append(out, DayTotal{Date: d, TotalSec: 60})
	}
	return out
}

func TestStreaks(t *testing.T) {
	cases := []struct {
		name        string
		days        []string
		today       string
		wantCurrent int
		wantLongest int
	}{
		{"没有记录", nil, "2026-05-01", 0, 0},
		{"只有今天", []string{"2026-05-01"}, "2026-05-01", 1, 1},
		{"今天与昨天连着", []string{"2026-04-30", "2026-05-01"}, "2026-05-01", 2, 2},
		{
			// 今天还没开始学不算断：最后一天是昨天，连击仍然算"活着"。
			name: "最后一天是昨天仍算连着", days: []string{"2026-04-29", "2026-04-30"},
			today: "2026-05-01", wantCurrent: 2, wantLongest: 2,
		},
		{
			name: "前天结束就算断了", days: []string{"2026-04-28", "2026-04-29"},
			today: "2026-05-01", wantCurrent: 0, wantLongest: 2,
		},
		{
			// 当前连击是"最后一段"，最长连击是"最长的一段"——两者可以不同。
			name:  "历史最长大于当前",
			days:  []string{"2026-04-01", "2026-04-02", "2026-04-03", "2026-04-20", "2026-05-01"},
			today: "2026-05-01", wantCurrent: 1, wantLongest: 3,
		},
		{
			name:  "跨月连续",
			days:  []string{"2026-04-29", "2026-04-30", "2026-05-01"},
			today: "2026-05-01", wantCurrent: 3, wantLongest: 3,
		},
		{
			// 2026 不是闰年，2 月只有 28 天；数字符串的写法会在这里断掉。
			name:  "跨 2 月底连续",
			days:  []string{"2026-02-27", "2026-02-28", "2026-03-01"},
			today: "2026-03-01", wantCurrent: 3, wantLongest: 3,
		},
		{
			name:  "跨年连续",
			days:  []string{"2025-12-31", "2026-01-01"},
			today: "2026-01-01", wantCurrent: 2, wantLongest: 2,
		},
		{
			name: "断成好几段",
			days: []string{
				"2026-04-01", "2026-04-02",
				"2026-04-10",
				"2026-04-20", "2026-04-21", "2026-04-22", "2026-04-23",
			},
			today: "2026-04-25", wantCurrent: 0, wantLongest: 4,
		},
		{
			// 库里的日期坏了不能把连击算错：坏的那天跳过，前后两天不算相邻。
			name:  "跳过无法解析的日期",
			days:  []string{"2026-04-01", "坏日期", "2026-04-03"},
			today: "2026-04-03", wantCurrent: 1, wantLongest: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			current, longest := streaks(daysOf(c.days...), c.today)
			if current != c.wantCurrent || longest != c.wantLongest {
				t.Errorf("streaks() = (%d, %d), 期望 (%d, %d)",
					current, longest, c.wantCurrent, c.wantLongest)
			}
		})
	}
}

// 空库也要能统计：COALESCE 让所有聚合量都是 0 而不是 NULL。
func TestStats_EmptyDB(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stats, err := db.Stats("2026-05-01")
	if err != nil {
		t.Fatalf("Stats() 失败: %v", err)
	}
	if stats.TotalSec != 0 || stats.SessionCount != 0 || stats.LongestSec != 0 {
		t.Errorf("空库统计 = %+v, 期望全 0", stats)
	}
	if len(stats.Days) != 0 {
		t.Errorf("空库不该有天数据, 实际 %d 天", len(stats.Days))
	}
	if stats.CurrentStreak != 0 || stats.LongestStreak != 0 {
		t.Errorf("空库连击 = (%d, %d), 期望 (0, 0)", stats.CurrentStreak, stats.LongestStreak)
	}
}

// 统计要按**本地**日期与小时分组，而 started_at 自带偏移。
func TestStats_GroupsByLocalDateAndHour(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// 同一时刻的 UTC 表示会落到前一天，所以这里刻意都用 +08:00 的本地时间。
	// planID 只用两个值，好验证 COUNT(DISTINCT plan_id) 真的在去重（4 条记录 2 个计划）。
	rows := []struct {
		started string
		sec     int64
		planID  int64
	}{
		{"2026-05-01T23:30:00+08:00", 600, 1},  // 深夜
		{"2026-05-01T09:00:00+08:00", 1200, 1}, // 白天
		{"2026-05-02T06:10:00+08:00", 300, 2},  // 早起
		{"2026-05-02T02:00:00+08:00", 900, 2},  // 深夜（凌晨）
	}
	for _, r := range rows {
		insertSession(t, db, r.started, r.sec, r.planID)
	}

	stats, err := db.Stats("2026-05-02")
	if err != nil {
		t.Fatalf("Stats() 失败: %v", err)
	}
	if stats.TotalSec != 3000 {
		t.Errorf("总时长 = %d, 期望 3000", stats.TotalSec)
	}
	if stats.SessionCount != 4 {
		t.Errorf("场次 = %d, 期望 4", stats.SessionCount)
	}
	if stats.LongestSec != 1200 {
		t.Errorf("单次最长 = %d, 期望 1200", stats.LongestSec)
	}
	if stats.FirstDay != "2026-05-01" || stats.LastDay != "2026-05-02" {
		t.Errorf("首末日 = (%q, %q)", stats.FirstDay, stats.LastDay)
	}
	if stats.LateNightCount != 2 {
		t.Errorf("深夜场次 = %d, 期望 2", stats.LateNightCount)
	}
	if stats.EarlyMorningCount != 1 {
		t.Errorf("早起场次 = %d, 期望 1", stats.EarlyMorningCount)
	}
	if stats.PlanCount != 2 {
		t.Errorf("有记录的计划数 = %d, 期望 2（4 条记录只落在 2 个计划上）", stats.PlanCount)
	}
	if len(stats.Days) != 2 || stats.Days[0].TotalSec != 1800 || stats.Days[1].TotalSec != 1200 {
		t.Errorf("按天汇总 = %+v, 期望 05-01=1800 / 05-02=1200", stats.Days)
	}
	if stats.CurrentStreak != 2 || stats.LongestStreak != 2 {
		t.Errorf("连击 = (%d, %d), 期望 (2, 2)", stats.CurrentStreak, stats.LongestStreak)
	}
}

// 未分类的记录（plan_id 为 NULL）不该被算成一个"计划"。
func TestStats_UnclassifiedIsNotAPlan(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.InsertSession(SessionRow{
		StartedAt: "2026-05-01T09:00:00+08:00", EndedAt: "2026-05-01T09:10:00+08:00",
		DurationSec: 600, Source: "timer", CreatedAt: "2026-05-01T09:10:00+08:00",
		PlanID: 0, // 未分类
	}); err != nil {
		t.Fatalf("造记录失败: %v", err)
	}
	stats, err := db.Stats("2026-05-01")
	if err != nil {
		t.Fatalf("Stats() 失败: %v", err)
	}
	if stats.PlanCount != 0 {
		t.Errorf("未分类记录被算成了计划, PlanCount = %d", stats.PlanCount)
	}
}
