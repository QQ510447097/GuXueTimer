// Package settings 管理用户设置的默认值、合法性收敛与持久化。
package settings

import (
	"fmt"
	"strconv"
	"time"

	"goxuetimer/internal/store"
)

// 存储键。已发布后不要改名，否则旧设置会读不出来。
const (
	KeyBreakReminderEnabled = "breakReminderEnabled"
	KeyBreakIntervalMin     = "breakIntervalMin"
	KeyMinimizeToTray       = "minimizeToTray"
	KeyTheme                = "theme"
	KeyCurrentPlanID        = "currentPlanId"
	KeyWindowTranslucent    = "windowTranslucent"
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
	// WindowTranslucent 让整个窗口半透明（桌面透出来并模糊）。
	// 需要重建窗口才能生效，所以改完要重启。
	WindowTranslucent bool
}

// Defaults 返回首次启动使用的默认设置。
func Defaults() Settings {
	return Settings{
		BreakReminderEnabled: true,
		BreakIntervalMin:     DefaultBreakIntervalMin,
		MinimizeToTray:       true,
		Theme:                DefaultTheme,
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
	return s
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
	if v, ok := raw[KeyWindowTranslucent]; ok {
		if b, err := strconv.ParseBool(v); err == nil {
			out.WindowTranslucent = b
		}
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
		KeyWindowTranslucent:    strconv.FormatBool(n.WindowTranslucent),
	})
}

// Validate 用于校验来自界面的输入，返回面向用户的中文错误。
func (s Settings) Validate() error {
	if s.BreakIntervalMin < MinBreakIntervalMin || s.BreakIntervalMin > MaxBreakIntervalMin {
		return fmt.Errorf("休息提醒间隔必须在 %d~%d 分钟之间",
			MinBreakIntervalMin, MaxBreakIntervalMin)
	}
	switch s.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
		return nil
	default:
		return fmt.Errorf("未知的主题取值 %q", s.Theme)
	}
}
