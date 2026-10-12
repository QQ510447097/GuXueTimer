// Package settings 管理用户设置的默认值、合法性收敛与持久化。
package settings

import (
	"fmt"
	"path/filepath"
	"strconv"

	"goxuetimer/internal/store"
)

// 存储键。已发布后不要改名，否则旧设置会读不出来。
//
// 三个已经死掉、但键仍留在库里的历史项（不会有人再读，留着无害）：
//   - windowTranslucent：窗口毛玻璃曾经是个开关，现在恒为开、界面里没有开关。
//   - customQuote / quoteIndex：语录功能已整条删除（见 CardModeNone 的注释）。
//   - breakReminderEnabled / breakIntervalMin：休息提醒已整条删除
//     （它会打断心流，见 docs/DEVELOPMENT.md 里「休息提醒已经整条删除」那段）。
const (
	KeyMinimizeToTray = "minimizeToTray"
	KeyTheme          = "theme"
	KeyCurrentPlanID  = "currentPlanId"
	// KeyCardMode 的名字里是"card"（卡片），值是 none | image。
	// 历史上还有过 quote（语录卡片），已废弃。键名保持不变。
	KeyCardMode        = "cardMode"
	KeyBackgroundImage = "backgroundImage"
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

// 计时页最下方那块区域显示什么（"卡片模式"）。
//
// 历史上还有过一个 CardModeQuote（语录卡片），已经删掉了：语录、自定义文字、
// "换一句"整套功能一并移除。老库里的 "quote" 会在 Normalize 里回落到 none。
//
// 存储键仍然是 cardMode，不改名——已发布过的键改名等于把老数据丢掉，
// 而且这样老库里选过 image 的人不会丢选择（见 KeyCardMode 的注释）。
const (
	// CardModeNone 留空：那块区域什么都不放。
	CardModeNone = "none"
	// CardModeImage 用户自己选的一张背景图。
	CardModeImage = "image"
	// DefaultCardMode 是首次启动的模式：留空。没有语录可显示，
	// 也没有图，默认摆一块空面板只会让人以为是加载失败。
	DefaultCardMode = CardModeNone
)

// Settings 是全部用户设置。
type Settings struct {
	MinimizeToTray bool
	Theme          string
	// CurrentPlanID 是当前选中的学习计划；0 表示还没有（首次启动）。
	//
	// 它是"设置"而不是"计划表的一列"：这是用户的当下选择，
	// 与计划本身的属性无关；计划被归档/删除时由 backend 负责纠正。
	CurrentPlanID int64

	// CardMode 决定计时页最下方那块区域显示什么：none（留空）或 image（背景图）。
	//
	// 名字里的"卡片"是历史叫法（原来那里是一张语录卡片）。它仍然是"卡片"的语义，
	// 所以字段名与存储键都没有跟着值的调整而改名。
	CardMode string
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
		MinimizeToTray: true,
		Theme:          DefaultTheme,
		CardMode:       DefaultCardMode,
	}
}

// Normalize 把越界或无法识别的值收敛回合法范围。
//
// 设置可能来自手改的数据库或旧版本，所以这里不报错、只纠正——
// 一个坏掉的取值不应该让整个设置加载失败。
func (s Settings) Normalize() Settings {
	if s.CurrentPlanID < 0 {
		s.CurrentPlanID = 0
	}
	switch s.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		s.Theme = DefaultTheme
	}
	// 空串（没指定）与不认识的值都回落到留空；
	// 老库里的 "quote" 也走这一支——那是被删掉的语录卡片。
	switch s.CardMode {
	case CardModeNone, CardModeImage:
	default:
		s.CardMode = DefaultCardMode
	}
	// 背景图只留文件名：任何路径成分都丢掉（见 BackgroundImage 的注释）。
	s.BackgroundImage = filepath.Base(filepath.Clean(s.BackgroundImage))
	if s.BackgroundImage == "." || s.BackgroundImage == string(filepath.Separator) {
		s.BackgroundImage = ""
	}
	return s
}

// Load 从数据库读取设置。缺失或损坏的键回落到默认值。
func Load(st *store.DB) (Settings, error) {
	out := Defaults()
	raw, err := st.LoadSettings()
	if err != nil {
		return out, err
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
	if v, ok := raw[KeyBackgroundImage]; ok {
		out.BackgroundImage = v
	}

	return out.Normalize(), nil
}

// Save 持久化设置（写之前先收敛）。
func (s Settings) Save(st *store.DB) error {
	n := s.Normalize()
	return st.SaveSettings(map[string]string{
		KeyMinimizeToTray:  strconv.FormatBool(n.MinimizeToTray),
		KeyTheme:           n.Theme,
		KeyCurrentPlanID:   strconv.FormatInt(n.CurrentPlanID, 10),
		KeyCardMode:        n.CardMode,
		KeyBackgroundImage: n.BackgroundImage,
	})
}

// Validate 用于校验来自界面的输入，返回面向用户的中文错误。
//
// 枚举字段的**空串表示"没指定"**，不报错——Normalize 会把它填成默认值。
// 这一点是有意的：界面可能只带了一部分字段（老版本前端、或只改某一项的调用），
// 若把空串当成非法值，用户改个主题就会收到"未知的卡片模式 """，
// 一个不相关的字段把整次保存搞失败，是比静默回落更糟的失败方式。
// 真正需要拦住的是"有值但不认识"（比如手改库把 theme 写成 rainbow）。
func (s Settings) Validate() error {
	switch s.Theme {
	case "", ThemeSystem, ThemeLight, ThemeDark:
	default:
		return fmt.Errorf("未知的主题取值 %q", s.Theme)
	}
	switch s.CardMode {
	case "", CardModeNone, CardModeImage:
	default:
		return fmt.Errorf("未知的卡片模式 %q", s.CardMode)
	}
	return nil
}
