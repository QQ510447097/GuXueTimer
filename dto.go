package main

import (
	"fmt"
	"time"

	"goxuetimer/internal/settings"
	"goxuetimer/internal/store"
	"goxuetimer/internal/timer"
)

// 事件名。前端用 Events.On 订阅这些名字。
const (
	eventTimerState    = "timer:state"
	eventRecordsChange = "records:changed"
	eventReminderBreak = "reminder:break"
	eventNotice        = "notice"
)

// 时间戳统一用带本地时区偏移的 RFC3339 字符串。
//
// 刻意不用 time.Time：绑定生成器会把它变成 JS 的 Date，再叠一层浏览器的时区解释，
// 是这个项目里最容易悄悄算错的地方。字符串由前端 Date.parse 自行解释，语义明确。
const (
	layoutRecord  = time.RFC3339
	layoutDateKey = "2006-01-02"
)

// TimerStateDTO 是前端渲染计时器所需的全部状态。
type TimerStateDTO struct {
	State string `json:"state"` // idle | running | paused

	// StartedAt 是本次会话开始时刻；idle 时为空串。
	StartedAt string `json:"startedAt"`
	// RunningSince 是最近一次结算的时刻：前端用它把时钟平滑地往前走，
	// 后端只在乎它作为"时长锚点"的语义。
	RunningSince string `json:"runningSince"`

	// AccumulatedSec 是已确定的时长（不含正在走的这一段）。
	AccumulatedSec int64 `json:"accumulatedSec"`
	// CurrentSec 是后端此刻认定的会话总时长，等于 accumulated + 当前这段。
	CurrentSec int64 `json:"currentSec"`
	// TodayTotalSec 是今天**已落库**的时长，不含当前会话。
	TodayTotalSec int64 `json:"todayTotalSec"`

	// BreakElapsedSec / BreakIntervalSec 是休息提醒的进度；
	// BreakIntervalSec 为 0 表示提醒已关闭。
	BreakElapsedSec  int64 `json:"breakElapsedSec"`
	BreakIntervalSec int64 `json:"breakIntervalSec"`
}

// SessionDTO 是一条学习记录。
type SessionDTO struct {
	ID          int64  `json:"id"`
	StartedAt   string `json:"startedAt"`
	EndedAt     string `json:"endedAt"`
	DurationSec int64  `json:"durationSec"`
	Note        string `json:"note"`
	Source      string `json:"source"` // timer | recovered
}

// DaySummaryDTO 是某一天的汇总。
type DaySummaryDTO struct {
	Date       string `json:"date"`
	TotalSec   int64  `json:"totalSec"`
	Count      int64  `json:"count"`
	FirstStart string `json:"firstStart"`
	LastEnd    string `json:"lastEnd"`
}

// StopResultDTO 是结束会话的结果。时长过短时 Saved 为 false、Reason 为 "too-short"。
type StopResultDTO struct {
	Saved       bool        `json:"saved"`
	Reason      string      `json:"reason"`
	DurationSec int64       `json:"durationSec"`
	Session     *SessionDTO `json:"session"`
}

// SettingsDTO 是全部用户设置。
type SettingsDTO struct {
	BreakReminderEnabled bool   `json:"breakReminderEnabled"`
	BreakIntervalMin     int    `json:"breakIntervalMin"`
	MinimizeToTray       bool   `json:"minimizeToTray"`
	Theme                string `json:"theme"` // system | light | dark
}

// RecordChangeDTO 说明记录为什么变了。
type RecordChangeDTO struct {
	Reason string `json:"reason"` // stopped | discarded | deleted | recovered
}

// ReminderEventDTO 是休息提醒事件的载荷。
type ReminderEventDTO struct {
	IntervalMin int `json:"intervalMin"`
}

// NoticeDTO 是后端抛给界面的一句话提示。
type NoticeDTO struct {
	Level   string `json:"level"` // info | warning
	Message string `json:"message"`
}

func timerStateDTOFrom(snap timer.Snapshot, now time.Time, todayTotal, breakElapsed, breakInterval int64) TimerStateDTO {
	dto := TimerStateDTO{
		State:            string(snap.State),
		AccumulatedSec:   snap.AccumulatedSec,
		CurrentSec:       snap.ElapsedSec(now),
		TodayTotalSec:    todayTotal,
		BreakElapsedSec:  breakElapsed,
		BreakIntervalSec: breakInterval,
	}
	if !snap.StartedAt.IsZero() {
		dto.StartedAt = snap.StartedAt.Format(layoutRecord)
	}
	if !snap.RunningSince.IsZero() {
		dto.RunningSince = snap.RunningSince.Format(layoutRecord)
	}
	return dto
}

func sessionDTOFrom(row store.SessionRow) SessionDTO {
	return SessionDTO{
		ID:          row.ID,
		StartedAt:   row.StartedAt,
		EndedAt:     row.EndedAt,
		DurationSec: row.DurationSec,
		Note:        row.Note,
		Source:      row.Source,
	}
}

func daySummaryDTOFrom(sum store.DaySummary) DaySummaryDTO {
	return DaySummaryDTO{
		Date:       sum.Date,
		TotalSec:   sum.TotalSec,
		Count:      sum.Count,
		FirstStart: sum.FirstStart,
		LastEnd:    sum.LastEnd,
	}
}

func settingsDTOFrom(s settings.Settings) SettingsDTO {
	return SettingsDTO{
		BreakReminderEnabled: s.BreakReminderEnabled,
		BreakIntervalMin:     s.BreakIntervalMin,
		MinimizeToTray:       s.MinimizeToTray,
		Theme:                s.Theme,
	}
}

func settingsFromDTO(d SettingsDTO) settings.Settings {
	return settings.Settings{
		BreakReminderEnabled: d.BreakReminderEnabled,
		BreakIntervalMin:     d.BreakIntervalMin,
		MinimizeToTray:       d.MinimizeToTray,
		Theme:                d.Theme,
	}
}

// formatDurationCN 把秒数渲染成「1 小时 23 分」这类托盘可读的文本。
func formatDurationCN(sec int64) string {
	if sec <= 0 {
		return "0 分钟"
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d 小时 %d 分", h, m)
	case h > 0:
		return fmt.Sprintf("%d 小时", h)
	case m > 0:
		return fmt.Sprintf("%d 分钟", m)
	default:
		return fmt.Sprintf("%d 秒", sec)
	}
}
