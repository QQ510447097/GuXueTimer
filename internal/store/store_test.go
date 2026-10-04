package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpen_CreatesSchemaAndEnablesWAL(t *testing.T) {
	db := newTestDB(t)

	version, err := db.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion() 失败: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("user_version = %d, 期望 %d", version, schemaVersion)
	}

	mode, err := db.JournalMode()
	if err != nil {
		t.Fatalf("JournalMode() 失败: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, 期望 \"wal\"", mode)
	}

	// 迁移已种下 idle 行。
	st, err := db.LoadTimerState()
	if err != nil {
		t.Fatalf("LoadTimerState() 失败: %v", err)
	}
	if st.State != "idle" || st.AccumulatedSec != 0 {
		t.Errorf("初始计时状态 = %+v, 期望 idle/0", st)
	}
}

// 迁移重复执行必须幂等：同一连接重跑，以及重新打开文件重跑。
func TestMigrate_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("首次 Open() 失败: %v", err)
	}
	if err := db.migrate(); err != nil {
		t.Fatalf("重复 migrate() 失败: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() 失败: %v", err)
	}

	again, err := Open(path)
	if err != nil {
		t.Fatalf("重新 Open() 失败: %v", err)
	}
	defer func() { _ = again.Close() }()

	version, err := again.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion() 失败: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("重开后 user_version = %d, 期望 %d", version, schemaVersion)
	}

	// 单行表不能因为重复迁移长出第二行。
	rows, err := again.sql.Query(`SELECT COUNT(*) FROM timer_state`)
	if err != nil {
		t.Fatalf("统计 timer_state 失败: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var n int
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("扫描 timer_state 行数失败: %v", err)
		}
	}
	if n != 1 {
		t.Errorf("timer_state 行数 = %d, 期望 1", n)
	}
}

func TestInsertSession_AndSessionsByDay(t *testing.T) {
	db := newTestDB(t)

	id, err := db.InsertSession(SessionRow{
		StartedAt:   "2026-05-01T09:00:00+08:00",
		EndedAt:     "2026-05-01T09:20:00+08:00",
		DurationSec: 1200,
		Note:        "线性代数",
		Source:      "timer",
		CreatedAt:   "2026-05-01T09:20:00+08:00",
	})
	if err != nil {
		t.Fatalf("InsertSession() 失败: %v", err)
	}
	if id <= 0 {
		t.Fatalf("InsertSession() 返回 id = %d, 期望正数", id)
	}

	got, err := db.SessionsByDay("2026-05-01")
	if err != nil {
		t.Fatalf("SessionsByDay() 失败: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("记录数 = %d, 期望 1", len(got))
	}
	if got[0].ID != id || got[0].DurationSec != 1200 || got[0].Note != "线性代数" {
		t.Errorf("记录 = %+v, 与写入不一致", got[0])
	}

	empty, err := db.SessionsByDay("2026-05-02")
	if err != nil {
		t.Fatalf("SessionsByDay(空日) 失败: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("空日记录数 = %d, 期望 0", len(empty))
	}
}

// 跨零点的会话整体归属开始日；刚好落在 00:00:00 的会话归属新的一天。
func TestSessionsByDay_MidnightBoundary(t *testing.T) {
	db := newTestDB(t)

	mustInsert := func(started, ended string, dur int64) {
		t.Helper()
		if _, err := db.InsertSession(SessionRow{
			StartedAt:   started,
			EndedAt:     ended,
			DurationSec: dur,
			Source:      "timer",
			CreatedAt:   ended,
		}); err != nil {
			t.Fatalf("InsertSession(%s) 失败: %v", started, err)
		}
	}

	mustInsert("2026-05-01T23:40:00+08:00", "2026-05-02T00:20:00+08:00", 2400)
	mustInsert("2026-05-02T00:00:00+08:00", "2026-05-02T00:30:00+08:00", 1800)
	mustInsert("2026-05-02T08:00:00+08:00", "2026-05-02T08:10:00+08:00", 600)

	day1, err := db.SessionsByDay("2026-05-01")
	if err != nil {
		t.Fatalf("SessionsByDay(05-01) 失败: %v", err)
	}
	if len(day1) != 1 || day1[0].DurationSec != 2400 {
		t.Errorf("05-01 记录 = %+v, 期望只有跨零点的那 1 条(2400s)", day1)
	}

	day2, err := db.SessionsByDay("2026-05-02")
	if err != nil {
		t.Fatalf("SessionsByDay(05-02) 失败: %v", err)
	}
	if len(day2) != 2 {
		t.Fatalf("05-02 记录数 = %d, 期望 2", len(day2))
	}
	// 升序：00:00 那条在前。
	if day2[0].StartedAt != "2026-05-02T00:00:00+08:00" {
		t.Errorf("05-02 首条 = %s, 期望 00:00:00 那条", day2[0].StartedAt)
	}

	sum1, err := db.SummaryByDay("2026-05-01")
	if err != nil {
		t.Fatalf("SummaryByDay(05-01) 失败: %v", err)
	}
	if sum1.TotalSec != 2400 || sum1.Count != 1 {
		t.Errorf("05-01 汇总 = %+v, 期望 total=2400 count=1", sum1)
	}

	sum2, err := db.SummaryByDay("2026-05-02")
	if err != nil {
		t.Fatalf("SummaryByDay(05-02) 失败: %v", err)
	}
	if sum2.TotalSec != 2400 || sum2.Count != 2 {
		t.Errorf("05-02 汇总 = %+v, 期望 total=2400 count=2", sum2)
	}
	if sum2.FirstStart != "2026-05-02T00:00:00+08:00" {
		t.Errorf("05-02 FirstStart = %s", sum2.FirstStart)
	}
	if sum2.LastEnd != "2026-05-02T08:10:00+08:00" {
		t.Errorf("05-02 LastEnd = %s", sum2.LastEnd)
	}
}

func TestSummaryByDay_EmptyDayIsZeroNotError(t *testing.T) {
	db := newTestDB(t)

	sum, err := db.SummaryByDay("2026-01-01")
	if err != nil {
		t.Fatalf("SummaryByDay(空日) 失败: %v", err)
	}
	if sum.TotalSec != 0 || sum.Count != 0 || sum.FirstStart != "" || sum.LastEnd != "" {
		t.Errorf("空日汇总 = %+v, 期望全零", sum)
	}
}

func TestDeleteSession(t *testing.T) {
	db := newTestDB(t)

	id, err := db.InsertSession(SessionRow{
		StartedAt: "2026-05-01T09:00:00+08:00",
		EndedAt:   "2026-05-01T09:20:00+08:00",
		CreatedAt: "2026-05-01T09:20:00+08:00",
	})
	if err != nil {
		t.Fatalf("InsertSession() 失败: %v", err)
	}

	if err := db.DeleteSession(id); err != nil {
		t.Fatalf("DeleteSession() 失败: %v", err)
	}
	exists, err := db.sessionExists(id)
	if err != nil {
		t.Fatalf("sessionExists() 失败: %v", err)
	}
	if exists {
		t.Error("删除后记录仍然存在")
	}

	if err := db.DeleteSession(id); !errors.Is(err, ErrNotFound) {
		t.Errorf("重复删除 error = %v, 期望 ErrNotFound", err)
	}
}

func TestSettings_RoundTrip(t *testing.T) {
	db := newTestDB(t)

	// 空库：键不存在，而不是报错。
	if v, ok, err := db.GetSetting("theme"); err != nil || ok {
		t.Fatalf("GetSetting(缺失) = (%q, %v, %v), 期望 (\"\", false, nil)", v, ok, err)
	}

	if err := db.SaveSettings(map[string]string{
		"theme":             "dark",
		"breakIntervalMin":  "45",
		"minimizeToTray":    "true",
		"breakReminderOn":   "true",
		"themeUnusedLegacy": "x",
	}); err != nil {
		t.Fatalf("SaveSettings() 失败: %v", err)
	}

	// 再次写入同一键走 upsert 分支。
	if err := db.SaveSettings(map[string]string{"breakIntervalMin": "50"}); err != nil {
		t.Fatalf("SaveSettings(覆盖) 失败: %v", err)
	}

	all, err := db.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() 失败: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("设置条数 = %d, 期望 5 (%v)", len(all), all)
	}
	if all["breakIntervalMin"] != "50" {
		t.Errorf("breakIntervalMin = %q, 期望 \"50\"", all["breakIntervalMin"])
	}
	if all["theme"] != "dark" {
		t.Errorf("theme = %q, 期望 \"dark\"", all["theme"])
	}

	if v, ok, err := db.GetSetting("theme"); err != nil || !ok || v != "dark" {
		t.Errorf("GetSetting(theme) = (%q, %v, %v)", v, ok, err)
	}

	// 空 map 不应报错，也不应改动任何东西。
	if err := db.SaveSettings(nil); err != nil {
		t.Errorf("SaveSettings(nil) 失败: %v", err)
	}
}

func TestTimerState_RoundTrip(t *testing.T) {
	db := newTestDB(t)

	want := TimerStateRow{
		State:          "running",
		StartedAt:      "2026-05-01T09:00:00+08:00",
		LastResumedAt:  "2026-05-01T09:05:00+08:00",
		AccumulatedSec: 300,
	}
	if err := db.SaveTimerState(want); err != nil {
		t.Fatalf("SaveTimerState() 失败: %v", err)
	}

	got, err := db.LoadTimerState()
	if err != nil {
		t.Fatalf("LoadTimerState() 失败: %v", err)
	}
	if got != want {
		t.Errorf("LoadTimerState() = %+v, 期望 %+v", got, want)
	}

	// 写回相同值：RowsAffected 为 0，但不应被误判为缺行。
	if err := db.SaveTimerState(want); err != nil {
		t.Errorf("重复 SaveTimerState() 失败: %v", err)
	}

	if err := db.TouchTimerState("2026-05-01T09:06:00+08:00"); err != nil {
		t.Errorf("TouchTimerState() 失败: %v", err)
	}
}
