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
	eventPlansChange   = "plans:changed"
	eventReminderBreak = "reminder:break"
	eventNotice        = "notice"
	// eventNavGoto 让后端请求界面切换到某一页。
	//
	// 需要它是因为"从外部打开某一页"这件事只有后端知道（托盘菜单的「设置」、
	// 以后可能的全局快捷键）。导航状态的归属在前端，所以这里只发意图，不直接改。
	eventNavGoto = "nav:goto"
)

// 时间戳统一用带本地时区偏移的 RFC3339 字符串。
//
// 刻意不用 time.Time：绑定生成器会把它变成 JS 的 Date，再叠一层浏览器的时区解释，
// 是这个项目里最容易悄悄算错的地方。字符串由前端 Date.parse 自行解释，语义明确。
const (
	layoutRecord  = time.RFC3339
	layoutDateKey = "2006-01-02"
)

// planPalette 是计划配色的白名单。
//
// 必须是白名单而不是"任意颜色字符串"：颜色会被前端拼进 CSS，
// 放开了等于留了一个样式注入的口子。这八个都是冷色，
// 任意几个同时出现在日历和列表里，整体仍然是"蓝白"的基调。
var planPalette = []string{
	"#3B82F6", // 蓝
	"#0EA5E9", // 天蓝
	"#22D3EE", // 青
	"#14B8A6", // 蓝绿
	"#2DD4BF", // 绿松
	"#6366F1", // 靛
	"#8B5CF6", // 紫
	"#A78BFA", // 浅紫
}

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

	// PlanID/PlanName/PlanColor 是当前会话归属的计划；未选或已不可查时为空串。
	PlanID    int64  `json:"planId"`
	PlanName  string `json:"planName"`
	PlanColor string `json:"planColor"`

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
	// PlanID 为 0 表示未分类；名字与颜色由前端用 PlansService.List 的结果本地解析，
	// 这样列表接口不必 JOIN，计划改了名也不会出现两处不一致的缓存。
	PlanID int64 `json:"planId"`
}

// PlanDTO 是一个学习计划（含统计）。
type PlanDTO struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder int    `json:"sortOrder"`
	Archived  bool   `json:"archived"`
	CreatedAt string `json:"createdAt"`
	TodaySec  int64  `json:"todaySec"`
	TotalSec  int64  `json:"totalSec"`
	Sessions  int64  `json:"sessions"`
}

// DaySummaryDTO 是某一天的汇总。
type DaySummaryDTO struct {
	Date       string `json:"date"`
	TotalSec   int64  `json:"totalSec"`
	Count      int64  `json:"count"`
	FirstStart string `json:"firstStart"`
	LastEnd    string `json:"lastEnd"`
}

// DailyTotalDTO 是某一天的总时长，柱状图与月历热力图共用。
type DailyTotalDTO struct {
	Date     string `json:"date"`
	TotalSec int64  `json:"totalSec"`
	Count    int64  `json:"count"`
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
	// CurrentPlanID 只读：由 PlansService.SetCurrent 掌管。
	// SettingsService.Save 会忽略界面回传的值，因为设置页里没有这个字段，
	// 拿零值覆盖会把"当前计划"清掉。
	CurrentPlanID int64 `json:"currentPlanId"`
	// ---- 计时页最下方那块区域 ----
	// CardMode 是 none（留空）或 image（背景图）。语录卡片已整条删除，
	// 老库里的 "quote" 会被 Normalize 收敛成 none。
	CardMode string `json:"cardMode"`
	// 注意：背景图文件名**不在**这个 DTO 里。它由 PersonalizationService 掌管
	// （选图要弹文件框、要复制文件、要通过资源中间件提供），
	// 前端拿到的是 PersonalizationDTO 里带版本号的可用地址。
	// 与 CurrentPlanID 同理：确实存在 settings 表里，但不经过这个 DTO 编辑。
}

// ChangedDTO 是"某类数据变了"的通用载荷。
//
// records:changed 与 plans:changed 共用它：前端只关心"要不要重新拉"，
// 多一种几乎相同的载荷类型没有收益。
type ChangedDTO struct {
	Reason string `json:"reason"`
}

// NavGotoDTO 请求界面切换到某一页。View 取计时/记录/计划/成就/个性化/设置 的页面键。
type NavGotoDTO struct {
	View string `json:"view"`
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

// timerStateDTOFrom 组装状态 DTO。
//
// planID 由调用方决定：空闲时是"当前计划"，进行中时是"这次会话归属的计划"。
func timerStateDTOFrom(snap timer.Snapshot, now time.Time, todayTotal, breakElapsed, breakInterval int64,
	planID int64, planName, planColor string) TimerStateDTO {
	dto := TimerStateDTO{
		State:            string(snap.State),
		AccumulatedSec:   snap.AccumulatedSec,
		CurrentSec:       snap.ElapsedSec(now),
		TodayTotalSec:    todayTotal,
		PlanID:           planID,
		PlanName:         planName,
		PlanColor:        planColor,
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
		PlanID:      row.PlanID,
	}
}

func planDTOFrom(row store.PlanRow) PlanDTO {
	return PlanDTO{
		ID:        row.ID,
		Name:      row.Name,
		Color:     row.Color,
		SortOrder: row.SortOrder,
		Archived:  row.Archived,
		CreatedAt: row.CreatedAt,
		TodaySec:  row.TodaySec,
		TotalSec:  row.TotalSec,
		Sessions:  row.Sessions,
	}
}

func dailyTotalDTOFrom(row store.DailyTotal) DailyTotalDTO {
	return DailyTotalDTO{Date: row.Date, TotalSec: row.TotalSec, Count: row.Count}
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
		CurrentPlanID:        s.CurrentPlanID,
		CardMode:             s.CardMode,
	}
}

func settingsFromDTO(d SettingsDTO) settings.Settings {
	return settings.Settings{
		BreakReminderEnabled: d.BreakReminderEnabled,
		BreakIntervalMin:     d.BreakIntervalMin,
		MinimizeToTray:       d.MinimizeToTray,
		Theme:                d.Theme,
		CurrentPlanID:        d.CurrentPlanID,
		CardMode:             d.CardMode,
	}
}

// isPlanColor 判断颜色是否在白名单里（比较时忽略大小写，存储统一用大写）。
func isPlanColor(color string) bool {
	for _, c := range planPalette {
		if equalFoldASCII(c, color) {
			return true
		}
	}
	return false
}

// canonicalPlanColor 返回白名单里的规范写法。
func canonicalPlanColor(color string) string {
	for _, c := range planPalette {
		if equalFoldASCII(c, color) {
			return c
		}
	}
	return planPalette[0]
}

// equalFoldASCII 只处理 ASCII 大小写；颜色值本来就是 ASCII。
func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
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
