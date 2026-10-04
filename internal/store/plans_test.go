package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

// v1 时期的一条旧记录用的时间戳。
const legacyStarted = "2026-05-01T09:00:00+08:00"

// buildV1DB 造一个**真正的** v1 库：直接跑 v1 的建表语句、把版本号定成 1、
// 再塞进旧数据。这样测的是真实的升级路径，而不是我"以为"的 v1 结构。
func buildV1DB(t *testing.T, path string) {
	t.Helper()

	raw, err := sql.Open(driverName, filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("打开裸连接失败: %v", err)
	}
	defer func() { _ = raw.Close() }()

	for i, stmt := range schemaV1 {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("建 v1 第 %d 条失败: %v", i+1, err)
		}
	}
	if _, err := raw.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatalf("设定 v1 版本号失败: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO sessions(started_at, ended_at, duration_sec, note, source, created_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		legacyStarted, "2026-05-01T09:30:00+08:00", 1800, "v1 的旧记录", "timer",
		"2026-05-01T09:30:00+08:00",
	); err != nil {
		t.Fatalf("写入旧记录失败: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO settings(key, value) VALUES('theme', 'dark')`); err != nil {
		t.Fatalf("写入旧设置失败: %v", err)
	}
}

// v1 -> v2 的迁移必须一条旧记录都不丢，并且旧记录显示为"未分类"。
//
// 这是整个数据层最要紧的一个用例：迁移写错了，用户的历史就没了。
func TestMigrate_V1ToV2PreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	buildV1DB(t, path)

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() 失败: %v", err)
	}
	defer func() { _ = db.Close() }()

	version, err := db.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion() 失败: %v", err)
	}
	if version != 2 {
		t.Errorf("user_version = %d, 期望 2", version)
	}

	// 旧记录还在，而且被归为未分类（plan_id 为 NULL -> 0）。
	rows, err := db.SessionsByDay("2026-05-01", 0)
	if err != nil {
		t.Fatalf("SessionsByDay() 失败: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("旧记录条数 = %d, 期望 1", len(rows))
	}
	if rows[0].Note != "v1 的旧记录" || rows[0].DurationSec != 1800 {
		t.Errorf("旧记录内容变了: %+v", rows[0])
	}
	if rows[0].PlanID != 0 {
		t.Errorf("旧记录的 PlanID = %d, 期望 0（未分类）", rows[0].PlanID)
	}

	// 旧设置也还在。
	if v, ok, err := db.GetSetting("theme"); err != nil || !ok || v != "dark" {
		t.Errorf("旧设置丢了: (%q, %v, %v)", v, ok, err)
	}

	// 新表建好了，且初始为空。
	plans, err := db.ListPlans("2026-05-01")
	if err != nil {
		t.Fatalf("ListPlans() 失败: %v", err)
	}
	if len(plans) != 0 {
		t.Errorf("新版库不该自带计划，实际 %d 条", len(plans))
	}

	// 迁移后按计划筛选：未分类的旧记录应当被排除在外。
	id, err := db.InsertPlan("数学", "#3B82F6", "2026-05-02T10:00:00+08:00")
	if err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}
	filtered, err := db.SessionsByDay("2026-05-01", id)
	if err != nil {
		t.Fatalf("SessionsByDay(筛选) 失败: %v", err)
	}
	if len(filtered) != 0 {
		t.Errorf("按计划筛选后仍有未分类记录: %+v", filtered)
	}

	// 幂等：再打开一次不会重复迁移、也不会报错。
	again, err := Open(path)
	if err != nil {
		t.Fatalf("二次 Open() 失败: %v", err)
	}
	defer func() { _ = again.Close() }()
	plain, err := again.SessionsByDay("2026-05-01", 0)
	if err != nil {
		t.Fatalf("二次读取失败: %v", err)
	}
	if len(plain) != 1 {
		t.Errorf("二次打开后记录数 = %d, 期望 1", len(plain))
	}
}

func TestPlans_CreateReadUpdate(t *testing.T) {
	db := newTestDB(t)

	id, err := db.InsertPlan("数学", "#3B82F6", "2026-05-01T08:00:00+08:00")
	if err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}
	if id <= 0 {
		t.Fatalf("InsertPlan() 返回 id = %d", id)
	}

	got, err := db.GetPlan(id)
	if err != nil {
		t.Fatalf("GetPlan() 失败: %v", err)
	}
	if got.Name != "数学" || got.Color != "#3B82F6" || got.Archived {
		t.Errorf("GetPlan() = %+v", got)
	}

	if err := db.UpdatePlan(id, "高等数学", "#22D3EE"); err != nil {
		t.Fatalf("UpdatePlan() 失败: %v", err)
	}
	got, err = db.GetPlan(id)
	if err != nil {
		t.Fatalf("GetPlan() 失败: %v", err)
	}
	if got.Name != "高等数学" || got.Color != "#22D3EE" {
		t.Errorf("更新后 = %+v", got)
	}

	// 写回完全相同的值：RowsAffected 为 0，但不应被误判成缺行。
	if err := db.UpdatePlan(id, "高等数学", "#22D3EE"); err != nil {
		t.Errorf("重复 UpdatePlan() 失败: %v", err)
	}

	if _, err := db.GetPlan(9999); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetPlan(不存在) error = %v, 期望 ErrNotFound", err)
	}
	if err := db.UpdatePlan(9999, "x", "#3B82F6"); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdatePlan(不存在) error = %v, 期望 ErrNotFound", err)
	}
}

// 新计划的排序号自动排在最后，界面上的顺序才稳定。
func TestPlans_SortOrderIncrements(t *testing.T) {
	db := newTestDB(t)

	for _, name := range []string{"一", "二", "三"} {
		if _, err := db.InsertPlan(name, "#3B82F6", "2026-05-01T08:00:00+08:00"); err != nil {
			t.Fatalf("InsertPlan(%s) 失败: %v", name, err)
		}
	}
	plans, err := db.ListPlans("2026-05-01")
	if err != nil {
		t.Fatalf("ListPlans() 失败: %v", err)
	}
	if len(plans) != 3 {
		t.Fatalf("计划数 = %d, 期望 3", len(plans))
	}
	for i, p := range plans {
		if p.SortOrder != i+1 {
			t.Errorf("第 %d 个计划 sort_order = %d, 期望 %d", i, p.SortOrder, i+1)
		}
	}

	// 倒过来排。
	if err := db.ReorderPlans([]int64{plans[2].ID, plans[1].ID, plans[0].ID}); err != nil {
		t.Fatalf("ReorderPlans() 失败: %v", err)
	}
	after, err := db.ListPlans("2026-05-01")
	if err != nil {
		t.Fatalf("ListPlans() 失败: %v", err)
	}
	if after[0].ID != plans[2].ID || after[2].ID != plans[0].ID {
		t.Errorf("重排后的顺序不对: %+v", after)
	}
}

func TestPlans_ArchiveAndReuseName(t *testing.T) {
	db := newTestDB(t)

	id, err := db.InsertPlan("英语", "#3B82F6", "2026-05-01T08:00:00+08:00")
	if err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}

	// 在用的时候能查到。
	if got, ok, err := db.ActivePlanByName("英语"); err != nil || !ok || got != id {
		t.Fatalf("ActivePlanByName(在用) = (%d, %v, %v)", got, ok, err)
	}

	if err := db.SetPlanArchived(id, true); err != nil {
		t.Fatalf("SetPlanArchived() 失败: %v", err)
	}
	// 归档之后名字可以被复用。
	if _, ok, err := db.ActivePlanByName("英语"); err != nil || ok {
		t.Errorf("归档后仍被视为在用: (%v, %v)", ok, err)
	}
	// 但列表里仍然能看到它（归档区要显示）。
	plans, err := db.ListPlans("2026-05-01")
	if err != nil {
		t.Fatalf("ListPlans() 失败: %v", err)
	}
	if len(plans) != 1 || !plans[0].Archived {
		t.Errorf("归档计划没出现在列表里: %+v", plans)
	}

	if err := db.SetPlanArchived(id, false); err != nil {
		t.Fatalf("恢复归档失败: %v", err)
	}
	if _, ok, err := db.ActivePlanByName("英语"); err != nil || !ok {
		t.Errorf("恢复后应当又在用: (%v, %v)", ok, err)
	}
}

func TestPlans_FirstActivePlan(t *testing.T) {
	db := newTestDB(t)

	if _, ok, err := db.FirstActivePlanID(); err != nil || ok {
		t.Fatalf("空库应当没有可用计划: (%v, %v)", ok, err)
	}

	if _, err := db.InsertPlan("甲", "#3B82F6", "2026-05-01T08:00:00+08:00"); err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}
	second, err := db.InsertPlan("乙", "#22D3EE", "2026-05-01T08:01:00+08:00")
	if err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}
	if id, ok, err := db.FirstActivePlanID(); err != nil || !ok || id == second {
		t.Errorf("FirstActivePlanID() = (%d, %v, %v)，期望排在前的那个", id, ok, err)
	}

	// 只剩乙没归档时，应当返回乙。
	plans, _ := db.ListPlans("2026-05-01")
	if err := db.SetPlanArchived(plans[0].ID, true); err != nil {
		t.Fatalf("归档失败: %v", err)
	}
	if id, ok, err := db.FirstActivePlanID(); err != nil || !ok || id != second {
		t.Errorf("FirstActivePlanID() = (%d, %v, %v)，期望 %d", id, ok, err, second)
	}
}

func TestPlans_ListStats(t *testing.T) {
	db := newTestDB(t)

	mathID, err := db.InsertPlan("数学", "#3B82F6", "2026-05-01T08:00:00+08:00")
	if err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}
	englishID, err := db.InsertPlan("英语", "#22D3EE", "2026-05-01T08:01:00+08:00")
	if err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}

	// 今天两条属于数学，一条属于英语；昨天一条属于数学；一条未分类。
	insert := func(started string, dur int64, planID int64) {
		t.Helper()
		if _, err := db.InsertSession(SessionRow{
			StartedAt: started, EndedAt: started, DurationSec: dur,
			Source: "timer", CreatedAt: started, PlanID: planID,
		}); err != nil {
			t.Fatalf("InsertSession() 失败: %v", err)
		}
	}
	insert("2026-05-02T09:00:00+08:00", 600, mathID)
	insert("2026-05-02T14:00:00+08:00", 900, mathID)
	insert("2026-05-02T20:00:00+08:00", 300, englishID)
	insert("2026-05-01T09:00:00+08:00", 1200, mathID)
	insert("2026-05-02T22:00:00+08:00", 60, 0)

	plans, err := db.ListPlans("2026-05-02")
	if err != nil {
		t.Fatalf("ListPlans() 失败: %v", err)
	}
	byID := map[int64]PlanRow{}
	for _, p := range plans {
		byID[p.ID] = p
	}

	if got := byID[mathID]; got.TodaySec != 1500 || got.TotalSec != 2700 || got.Sessions != 3 {
		t.Errorf("数学统计 = 今日 %d / 累计 %d / 场次 %d，期望 1500/2700/3",
			got.TodaySec, got.TotalSec, got.Sessions)
	}
	if got := byID[englishID]; got.TodaySec != 300 || got.TotalSec != 300 || got.Sessions != 1 {
		t.Errorf("英语统计 = 今日 %d / 累计 %d / 场次 %d，期望 300/300/1",
			got.TodaySec, got.TotalSec, got.Sessions)
	}
}

func TestSessionPlanFilter(t *testing.T) {
	db := newTestDB(t)

	mathID, err := db.InsertPlan("数学", "#3B82F6", "2026-05-01T08:00:00+08:00")
	if err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}
	englishID, err := db.InsertPlan("英语", "#22D3EE", "2026-05-01T08:01:00+08:00")
	if err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}

	insert := func(started string, dur int64, planID int64) {
		t.Helper()
		if _, err := db.InsertSession(SessionRow{
			StartedAt: started, EndedAt: started, DurationSec: dur,
			Source: "timer", CreatedAt: started, PlanID: planID,
		}); err != nil {
			t.Fatalf("InsertSession() 失败: %v", err)
		}
	}
	insert("2026-05-02T09:00:00+08:00", 600, mathID)
	insert("2026-05-02T10:00:00+08:00", 300, englishID)
	insert("2026-05-02T11:00:00+08:00", 120, 0) // 未分类

	// 不筛选：三条都在。
	all, err := db.SessionsByDay("2026-05-02", 0)
	if err != nil {
		t.Fatalf("SessionsByDay() 失败: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("全部记录 = %d, 期望 3", len(all))
	}

	// 筛数学：只有一条，未分类的被排除。
	math, err := db.SessionsByDay("2026-05-02", mathID)
	if err != nil {
		t.Fatalf("SessionsByDay(数学) 失败: %v", err)
	}
	if len(math) != 1 || math[0].PlanID != mathID || math[0].DurationSec != 600 {
		t.Errorf("数学筛选 = %+v", math)
	}

	sumAll, err := db.SummaryByDay("2026-05-02", 0)
	if err != nil {
		t.Fatalf("SummaryByDay() 失败: %v", err)
	}
	if sumAll.TotalSec != 1020 || sumAll.Count != 3 {
		t.Errorf("全部汇总 = %+v, 期望 total=1020 count=3", sumAll)
	}
	sumEnglish, err := db.SummaryByDay("2026-05-02", englishID)
	if err != nil {
		t.Fatalf("SummaryByDay(英语) 失败: %v", err)
	}
	if sumEnglish.TotalSec != 300 || sumEnglish.Count != 1 {
		t.Errorf("英语汇总 = %+v, 期望 total=300 count=1", sumEnglish)
	}
}

func TestDailyTotals(t *testing.T) {
	db := newTestDB(t)

	mathID, err := db.InsertPlan("数学", "#3B82F6", "2026-05-01T08:00:00+08:00")
	if err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}

	insert := func(started string, dur int64, planID int64) {
		t.Helper()
		if _, err := db.InsertSession(SessionRow{
			StartedAt: started, EndedAt: started, DurationSec: dur,
			Source: "timer", CreatedAt: started, PlanID: planID,
		}); err != nil {
			t.Fatalf("InsertSession() 失败: %v", err)
		}
	}
	insert("2026-05-01T09:00:00+08:00", 600, mathID)
	insert("2026-05-01T15:00:00+08:00", 300, 0)
	insert("2026-05-03T09:00:00+08:00", 1200, mathID)
	insert("2026-05-10T09:00:00+08:00", 60, mathID) // 区间外

	// 闭区间：5/1 到 5/3 应当拿到 5/1 与 5/3，缺的 5/2 不补。
	totals, err := db.DailyTotals("2026-05-01", "2026-05-03", 0)
	if err != nil {
		t.Fatalf("DailyTotals() 失败: %v", err)
	}
	if len(totals) != 2 {
		t.Fatalf("有记录的天数 = %d, 期望 2（缺的日期不该补）", len(totals))
	}
	if totals[0].Date != "2026-05-01" || totals[0].TotalSec != 900 || totals[0].Count != 2 {
		t.Errorf("5/1 = %+v, 期望 total=900 count=2", totals[0])
	}
	if totals[1].Date != "2026-05-03" || totals[1].TotalSec != 1200 {
		t.Errorf("5/3 = %+v, 期望 total=1200", totals[1])
	}

	// 按计划筛选：只剩数学，且区间外的不算。
	onlyMath, err := db.DailyTotals("2026-05-01", "2026-05-10", mathID)
	if err != nil {
		t.Fatalf("DailyTotals(筛选) 失败: %v", err)
	}
	if len(onlyMath) != 3 {
		t.Fatalf("数学的天数 = %d, 期望 3", len(onlyMath))
	}
	if onlyMath[0].TotalSec != 600 {
		t.Errorf("5/1 数学 = %d, 期望 600（未分类的 300 不该计入）", onlyMath[0].TotalSec)
	}
	if onlyMath[2].Date != "2026-05-10" || onlyMath[2].TotalSec != 60 {
		t.Errorf("区间上界的天数丢了: %+v", onlyMath[2])
	}

	// 空区间返回空切片而不是 nil。
	empty, err := db.DailyTotals("2026-01-01", "2026-01-31", 0)
	if err != nil {
		t.Fatalf("DailyTotals(空) 失败: %v", err)
	}
	if empty == nil {
		t.Error("返回了 nil 切片，会被序列化成 null")
	}
	if len(empty) != 0 {
		t.Errorf("空区间 = %d 条", len(empty))
	}
}

func TestCountPlanSessionsAndDelete(t *testing.T) {
	db := newTestDB(t)

	id, err := db.InsertPlan("数学", "#3B82F6", "2026-05-01T08:00:00+08:00")
	if err != nil {
		t.Fatalf("InsertPlan() 失败: %v", err)
	}
	if n, err := db.CountPlanSessions(id); err != nil || n != 0 {
		t.Fatalf("空计划引用数 = (%d, %v)", n, err)
	}

	if _, err := db.InsertSession(SessionRow{
		StartedAt: "2026-05-01T09:00:00+08:00", EndedAt: "2026-05-01T09:10:00+08:00",
		DurationSec: 600, Source: "timer", CreatedAt: "2026-05-01T09:10:00+08:00", PlanID: id,
	}); err != nil {
		t.Fatalf("InsertSession() 失败: %v", err)
	}

	if n, err := db.CountPlanSessions(id); err != nil || n != 1 {
		t.Fatalf("引用数 = (%d, %v), 期望 1", n, err)
	}

	// 库里没有外键约束，所以 DeletePlan 本身不会拦；拦截在服务层。
	// 这里验证"删掉之后记录还在，只是变成孤儿"这个前提是安全的：
	// 记录不该被级联删除。
	if err := db.DeletePlan(id); err != nil {
		t.Fatalf("DeletePlan() 失败: %v", err)
	}
	rows, err := db.SessionsByDay("2026-05-01", 0)
	if err != nil {
		t.Fatalf("SessionsByDay() 失败: %v", err)
	}
	if len(rows) != 1 || rows[0].DurationSec != 600 {
		t.Errorf("计划被删后记录也丢了: %+v", rows)
	}
	// 查不到计划时，PlanID 仍然保留原值，由界面降级显示成"未分类"。
	if rows[0].PlanID != id {
		t.Errorf("记录的 PlanID = %d, 期望保留 %d", rows[0].PlanID, id)
	}

	if err := db.DeletePlan(id); !errors.Is(err, ErrNotFound) {
		t.Errorf("重复删除 error = %v, 期望 ErrNotFound", err)
	}
}
