package settings

import (
	"path/filepath"
	"testing"
	"time"

	"goxuetimer/internal/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open() 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestLoad_EmptyDBReturnsDefaults(t *testing.T) {
	got, err := Load(newTestDB(t))
	if err != nil {
		t.Fatalf("Load() 失败: %v", err)
	}
	if got != Defaults() {
		t.Errorf("Load(空库) = %+v, 期望 %+v", got, Defaults())
	}
	if got.Interval() != time.Duration(DefaultBreakIntervalMin)*time.Minute {
		t.Errorf("Interval() = %v", got.Interval())
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	db := newTestDB(t)

	want := Settings{
		BreakReminderEnabled: false,
		BreakIntervalMin:     25,
		MinimizeToTray:       false,
		Theme:                ThemeDark,
	}
	if err := want.Save(db); err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}

	got, err := Load(db)
	if err != nil {
		t.Fatalf("Load() 失败: %v", err)
	}
	if got != want {
		t.Errorf("Load() = %+v, 期望 %+v", got, want)
	}
}

// 越界或无法解析的值必须被收敛回默认值，而不是让功能整个失效。
func TestLoad_CorruptValuesFallBackToDefaults(t *testing.T) {
	db := newTestDB(t)

	if err := db.SaveSettings(map[string]string{
		KeyBreakReminderEnabled: "不是布尔值",
		KeyBreakIntervalMin:     "0",
		KeyMinimizeToTray:       "yes-please",
		KeyTheme:                "赛博朋克",
	}); err != nil {
		t.Fatalf("SaveSettings() 失败: %v", err)
	}

	got, err := Load(db)
	if err != nil {
		t.Fatalf("Load() 失败: %v", err)
	}
	if got != Defaults() {
		t.Errorf("Load(脏数据) = %+v, 期望回落成 %+v", got, Defaults())
	}
}

func TestLoad_IntervalOutOfRangeClamped(t *testing.T) {
	db := newTestDB(t)

	if err := db.SaveSettings(map[string]string{KeyBreakIntervalMin: "9999"}); err != nil {
		t.Fatalf("SaveSettings() 失败: %v", err)
	}
	got, err := Load(db)
	if err != nil {
		t.Fatalf("Load() 失败: %v", err)
	}
	if got.BreakIntervalMin != DefaultBreakIntervalMin {
		t.Errorf("BreakIntervalMin = %d, 期望回落为 %d", got.BreakIntervalMin, DefaultBreakIntervalMin)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		in      Settings
		wantErr bool
	}{
		{"默认值合法", Defaults(), false},
		{"下界", Settings{BreakIntervalMin: MinBreakIntervalMin, Theme: ThemeSystem}, false},
		{"上界", Settings{BreakIntervalMin: MaxBreakIntervalMin, Theme: ThemeLight}, false},
		{"低于下界", Settings{BreakIntervalMin: MinBreakIntervalMin - 1, Theme: ThemeSystem}, true},
		{"高于上界", Settings{BreakIntervalMin: MaxBreakIntervalMin + 1, Theme: ThemeSystem}, true},
		{"未知主题", Settings{BreakIntervalMin: 25, Theme: "rainbow"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.in.Validate()
			if c.wantErr && err == nil {
				t.Error("期望报错，实际 nil")
			}
			if !c.wantErr && err != nil {
				t.Errorf("期望无错，实际 %v", err)
			}
		})
	}
}

// Save 应当先收敛再落库：写进库里的值本身就得是合法的。
func TestSave_NormalizesBeforePersisting(t *testing.T) {
	db := newTestDB(t)

	if err := (Settings{BreakIntervalMin: -5, Theme: "rainbow"}).Save(db); err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}
	raw, err := db.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() 失败: %v", err)
	}
	if raw[KeyBreakIntervalMin] != "45" {
		t.Errorf("落库间隔 = %q, 期望 \"45\"", raw[KeyBreakIntervalMin])
	}
	if raw[KeyTheme] != ThemeSystem {
		t.Errorf("落库主题 = %q, 期望 %q", raw[KeyTheme], ThemeSystem)
	}
}
