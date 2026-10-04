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
)

// 主题取值。
const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"
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
}

// Defaults 返回首次启动使用的默认设置。
func Defaults() Settings {
	return Settings{
		BreakReminderEnabled: true,
		BreakIntervalMin:     DefaultBreakIntervalMin,
		MinimizeToTray:       true,
		Theme:                ThemeSystem,
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
	switch s.Theme {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		s.Theme = ThemeSystem
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
