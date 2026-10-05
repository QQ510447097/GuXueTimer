package settings

import (
	"path/filepath"
	"strings"
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
		// 卡片字段不能留零值：Save 会先 Normalize，CardMode 的空串会变成默认的
		// quote，所以读回来的值里它一定不是空的。想让这条断言成立，
		// 期望值就必须和"存进去之后真正会变成什么"一致。
		CardMode: DefaultCardMode,
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
		// 空串 = "没指定"，要放行：界面只带部分字段是正常情况，
		// 让一个不相关的字段把整次保存搞失败是更糟的失败方式。
		{"只带部分字段", Settings{BreakIntervalMin: 25}, false},
		{"未知卡片模式", Settings{BreakIntervalMin: 25, CardMode: "video"}, true},
		{"卡片模式为空算未指定", Settings{BreakIntervalMin: 25, CardMode: ""}, false},
		{"自定义语录超长", Settings{
			BreakIntervalMin: 25,
			CustomQuote:      strings.Repeat("雪", MaxCustomQuoteRunes+1),
		}, true},
		{"自定义语录到上限", Settings{
			BreakIntervalMin: 25,
			CustomQuote:      strings.Repeat("雪", MaxCustomQuoteRunes),
		}, false},
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
	if raw[KeyTheme] != DefaultTheme {
		t.Errorf("落库主题 = %q, 期望 %q", raw[KeyTheme], DefaultTheme)
	}
}

// 新字段的收敛与持久化：卡片模式回落默认、语录按字符截断、背景图只留文件名。
func TestNormalize_CardFields(t *testing.T) {
	cases := []struct {
		name       string
		in         Settings
		wantMode   string
		wantQuote  string
		wantBg     string
		wantIndex  int
		quoteRunes int
	}{
		{
			name:      "未知模式回落默认",
			in:        Settings{CardMode: "video", QuoteIndex: 3},
			wantMode:  DefaultCardMode,
			wantIndex: 3,
		},
		{
			name:      "负索引收敛为每日一句",
			in:        Settings{CardMode: CardModeImage, QuoteIndex: -5},
			wantMode:  CardModeImage,
			wantIndex: QuoteIndexAuto,
		},
		{
			name:       "超长语录按字符截断（不是字节）",
			in:         Settings{CardMode: CardModeQuote, CustomQuote: strings.Repeat("雪", 200)},
			wantMode:   CardModeQuote,
			wantIndex:  QuoteIndexAuto,
			quoteRunes: MaxCustomQuoteRunes,
		},
		{
			name:      "背景图只留文件名",
			in:        Settings{CardMode: CardModeImage, BackgroundImage: `..\..\Windows\System32\config\SAM`},
			wantMode:  CardModeImage,
			wantIndex: QuoteIndexAuto,
			wantBg:    "SAM",
		},
		{
			// 路径末尾是分隔符时 filepath.Base 会把最后一段目录名当文件名，
			// 这里断言的是"剩下的仍然只是一个名字、不含任何路径成分"。
			name:      "背景图带路径时只留最后一段",
			in:        Settings{CardMode: CardModeImage, BackgroundImage: `C:\Users\`},
			wantMode:  CardModeImage,
			wantIndex: QuoteIndexAuto,
			wantBg:    "Users",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.in.Normalize()
			if got.CardMode != c.wantMode {
				t.Errorf("CardMode = %q, 期望 %q", got.CardMode, c.wantMode)
			}
			if got.QuoteIndex != c.wantIndex {
				t.Errorf("QuoteIndex = %d, 期望 %d", got.QuoteIndex, c.wantIndex)
			}
			if c.wantQuote != "" && got.CustomQuote != c.wantQuote {
				t.Errorf("CustomQuote = %q, 期望 %q", got.CustomQuote, c.wantQuote)
			}
			if c.quoteRunes > 0 && len([]rune(got.CustomQuote)) != c.quoteRunes {
				t.Errorf("CustomQuote 字符数 = %d, 期望 %d", len([]rune(got.CustomQuote)), c.quoteRunes)
			}
			if c.wantBg != "" && got.BackgroundImage != c.wantBg {
				t.Errorf("BackgroundImage = %q, 期望 %q", got.BackgroundImage, c.wantBg)
			}
		})
	}
}

// 卡片相关字段要能落库并原样读回。
func TestCardFields_RoundTrip(t *testing.T) {
	db := newTestDB(t)

	in := Settings{
		BreakIntervalMin: DefaultBreakIntervalMin,
		Theme:            ThemeDark,
		CardMode:         CardModeImage,
		CustomQuote:      "把今天过完就行",
		QuoteIndex:       7,
		BackgroundImage:  "background.png",
	}
	if err := in.Save(db); err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}
	got, err := Load(db)
	if err != nil {
		t.Fatalf("Load() 失败: %v", err)
	}
	if got.CardMode != CardModeImage || got.CustomQuote != "把今天过完就行" ||
		got.QuoteIndex != 7 || got.BackgroundImage != "background.png" {
		t.Errorf("读回不一致: %+v", got)
	}
}

// 旧库没有卡片相关的键时，必须回落到默认（每日一句），而不是变成空模式。
func TestLoad_LegacyDBWithoutCardKeys(t *testing.T) {
	db := newTestDB(t)

	if err := db.SaveSettings(map[string]string{
		KeyBreakIntervalMin: "30",
		KeyTheme:            ThemeDark,
	}); err != nil {
		t.Fatalf("SaveSettings() 失败: %v", err)
	}

	got, err := Load(db)
	if err != nil {
		t.Fatalf("Load() 失败: %v", err)
	}
	if got.CardMode != DefaultCardMode {
		t.Errorf("CardMode = %q, 期望回落为 %q", got.CardMode, DefaultCardMode)
	}
	if got.QuoteIndex != QuoteIndexAuto {
		t.Errorf("QuoteIndex = %d, 期望 %d", got.QuoteIndex, QuoteIndexAuto)
	}
}
