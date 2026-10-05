// Package settings 管理用户设置的默认值、合法性收敛与持久化。
package settings

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"goxuetimer/internal/store"
)

// 存储键。已发布后不要改名，否则旧设置会读不出来。
//
// 曾经有过一个 windowTranslucent（窗口毛玻璃开关），现在它恒为开、界面里也没有开关，
// 所以键与字段都删掉了。旧库里残留的那个键不会有人再读，留着无害。
const (
	KeyBreakReminderEnabled = "breakReminderEnabled"
	KeyBreakIntervalMin     = "breakIntervalMin"
	KeyMinimizeToTray       = "minimizeToTray"
	KeyTheme                = "theme"
	KeyCurrentPlanID        = "currentPlanId"
	KeyCardMode             = "cardMode"
	KeyCustomQuote          = "customQuote"
	KeyQuoteIndex           = "quoteIndex"
	KeyBackgroundImage      = "backgroundImage"
)

// 主题取值。
const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"
	// DefaultTheme 同时用于"首次启动的默认值"和"读到无法识别的值时的回退值"。
	// 两处必须是同一个：否则用户把设置文件改坏之后，界面会变成另一种外观，
	// 让人以为主题被重置成了别的东西。软件的主色调是蓝白，两者都是浅色。
	DefaultTheme = ThemeLight
)

// 休息提醒间隔的合法区间（分钟）。
const (
	MinBreakIntervalMin     = 1
	MaxBreakIntervalMin     = 480
	DefaultBreakIntervalMin = 45
)

// 计时页装饰卡片的模式。
const (
	CardModeQuote = "quote"
	CardModeImage = "image"
	// DefaultCardMode 是首次启动的模式。
	DefaultCardMode = CardModeQuote
	// QuoteIndexAuto 表示"每日一句"：按日期自动挑一条，每天换。
	// 用户点过"换一句"之后才会固定成某个序号。
	//
	// 用 0 而不是 -1 当哨兵，是为了让**零值即默认**：一个只有部分字段的
	// Settings{} 经 Normalize 之后应当落到"每日一句"。若用 -1，
	// 零值 0 会被解释成"第 0 条语录"，同一份结构就有了两种含义。
	// 这与 CurrentPlanID = 0 表示"未设置"是同一套惯例。
	QuoteIndexAuto = 0
	// MaxCustomQuoteRunes 是自定义语录的长度上限（按字符数，不是字节数——
	// 中文一个字三字节，按字节限会把上限变成 40 个汉字）。
	MaxCustomQuoteRunes = 120
)

// Settings 是全部用户设置。
type Settings struct {
	BreakReminderEnabled bool
	BreakIntervalMin     int
	MinimizeToTray       bool
	Theme                string
	// CurrentPlanID 是当前选中的学习计划；0 表示还没有（首次启动）。
	//
	// 它是"设置"而不是"计划表的一列"：这是用户的当下选择，
	// 与计划本身的属性无关；计划被归档/删除时由 backend 负责纠正。
	CurrentPlanID int64

	// ---- 计时页的装饰卡片 ----
	//
	// 计时页下方本来就有一块空白（时钟只占屏幕上半部分），放一张可定制的卡片
	// 比留白好看，也给了这个工具一点"人味"。
	// CardMode 是 quote 或 image；CustomQuote 为空时用内置的每日一句；
	// QuoteIndex 为 0 表示每日一句（自动），大于 0 表示固定第 QuoteIndex 条（1 起算）。
	CardMode    string
	CustomQuote string
	QuoteIndex  int
	// BackgroundImage 是用户选的背景图文件名（只存文件名，不存路径）。
	//
	// 存文件名而不是完整路径有个安全上的原因：这个值会被拼进 URL 交给
	// 资源中间件去读文件，存路径等于让数据库里的一个字符串决定读哪个文件。
	// 只允许"数据目录下的一个文件名"，路径拼接就没有发挥空间。
	BackgroundImage string
}

// Defaults 返回首次启动使用的默认设置。
func Defaults() Settings {
	return Settings{
		BreakReminderEnabled: true,
		BreakIntervalMin:     DefaultBreakIntervalMin,
		MinimizeToTray:       true,
		Theme:                DefaultTheme,
		CardMode:             DefaultCardMode,
		QuoteIndex:           QuoteIndexAuto,
	}
}

// Intervals 是休息提醒的间隔时长。
func (s Settings) Interval() time.Duration {
	return time.Duration(s.BreakIntervalMin) * time.Minute
}

// Normalize 把越界或无法识别的值收敛回合法范围。
//
// 设置可能来自手改的数据库或旧版本，所以这里不报错、只纠正——
// 一个坏掉的间隔值不应该让提醒功能整个失效。
func (s Settings) Normalize() Settings {
	if s.BreakIntervalMin < MinBreakIntervalMin || s.BreakIntervalMin > MaxBreakIntervalMin {
		s.BreakIntervalMin = DefaultBreakIntervalMin
	}
	if s.CurrentPlanID < 0 {
		s.CurrentPlanID = 0
	}
	switch s.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		s.Theme = DefaultTheme
	}
	switch s.CardMode {
	case CardModeQuote, CardModeImage:
	default:
		s.CardMode = DefaultCardMode
	}
	// 负数没有意义：收敛到"每日一句"。
	if s.QuoteIndex < QuoteIndexAuto {
		s.QuoteIndex = QuoteIndexAuto
	}
	s.CustomQuote = truncateRunes(strings.TrimSpace(s.CustomQuote), MaxCustomQuoteRunes)
	// 背景图只留文件名：任何路径成分都丢掉（见 BackgroundImage 的注释）。
	s.BackgroundImage = filepath.Base(filepath.Clean(s.BackgroundImage))
	if s.BackgroundImage == "." || s.BackgroundImage == string(filepath.Separator) {
		s.BackgroundImage = ""
	}
	return s
}

// truncateRunes 按**字符**截断，不是按字节。中文一个字三字节，
// 按字节截会把最后一个字劈成半个、存进去就是乱码。
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// Load 从数据库读取设置。缺失或损坏的键回落到默认值。
func Load(st *store.DB) (Settings, error) {
	out := Defaults()
	raw, err := st.LoadSettings()
	if err != nil {
		return out, err
	}

	if v, ok := raw[KeyBreakReminderEnabled]; ok {
		if b, err := strconv.ParseBool(v); err == nil {
			out.BreakReminderEnabled = b
		}
	}
	if v, ok := raw[KeyBreakIntervalMin]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			out.BreakIntervalMin = n
		}
	}
	if v, ok := raw[KeyMinimizeToTray]; ok {
		if b, err := strconv.ParseBool(v); err == nil {
			out.MinimizeToTray = b
		}
	}
	if v, ok := raw[KeyTheme]; ok && v != "" {
		out.Theme = v
	}
	if v, ok := raw[KeyCurrentPlanID]; ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			out.CurrentPlanID = n
		}
	}
	if v, ok := raw[KeyCardMode]; ok && v != "" {
		out.CardMode = v
	}
	if v, ok := raw[KeyCustomQuote]; ok {
		out.CustomQuote = v
	}
	if v, ok := raw[KeyQuoteIndex]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			out.QuoteIndex = n
		}
	}
	if v, ok := raw[KeyBackgroundImage]; ok {
		out.BackgroundImage = v
	}

	return out.Normalize(), nil
}

// Save 持久化设置（写之前先收敛）。
func (s Settings) Save(st *store.DB) error {
	n := s.Normalize()
	return st.SaveSettings(map[string]string{
		KeyBreakReminderEnabled: strconv.FormatBool(n.BreakReminderEnabled),
		KeyBreakIntervalMin:     strconv.Itoa(n.BreakIntervalMin),
		KeyMinimizeToTray:       strconv.FormatBool(n.MinimizeToTray),
		KeyTheme:                n.Theme,
		KeyCurrentPlanID:        strconv.FormatInt(n.CurrentPlanID, 10),
		KeyCardMode:             n.CardMode,
		KeyCustomQuote:          n.CustomQuote,
		KeyQuoteIndex:           strconv.Itoa(n.QuoteIndex),
		KeyBackgroundImage:      n.BackgroundImage,
	})
}

// Validate 用于校验来自界面的输入，返回面向用户的中文错误。
//
// 枚举字段的**空串表示"没指定"**，不报错——Normalize 会把它填成默认值。
// 这一点是有意的：界面可能只带了一部分字段（老版本前端、或只改某一项的调用），
// 若把空串当成非法值，用户改个提醒间隔就会收到"未知的卡片模式 """，
// 一个不相关的字段把整次保存搞失败，是比静默回落更糟的失败方式。
// 真正需要拦住的是"有值但不认识"（比如手改库把 theme 写成 rainbow）。
func (s Settings) Validate() error {
	if s.BreakIntervalMin < MinBreakIntervalMin || s.BreakIntervalMin > MaxBreakIntervalMin {
		return fmt.Errorf("休息提醒间隔必须在 %d~%d 分钟之间",
			MinBreakIntervalMin, MaxBreakIntervalMin)
	}
	switch s.Theme {
	case "", ThemeSystem, ThemeLight, ThemeDark:
	default:
		return fmt.Errorf("未知的主题取值 %q", s.Theme)
	}
	switch s.CardMode {
	case "", CardModeQuote, CardModeImage:
	default:
		return fmt.Errorf("未知的卡片模式 %q", s.CardMode)
	}
	if utf8.RuneCountInString(s.CustomQuote) > MaxCustomQuoteRunes {
		return fmt.Errorf("自定义语录最多 %d 个字", MaxCustomQuoteRunes)
	}
	return nil
}
