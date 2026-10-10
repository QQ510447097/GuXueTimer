package main

import (
	"log/slog"

	"goxuetimer/internal/achievements"
	"goxuetimer/internal/store"
)

// AchievementsService 汇总统计、等级与成就状态。
type AchievementsService struct {
	b *backend
}

// NewAchievementsService 构造成就服务。
func NewAchievementsService(b *backend) *AchievementsService {
	return &AchievementsService{b: b}
}

// AchievementDTO 是一个成就的状态。
//
// 三种类别（kind）用同一份结构，靠 kind 决定前端怎么画：
//   - once：unlocked / progress / target（progress 按 definition 的 Unit）
//   - cycle：count（已达成次数）、lastAt（上次达成）、progress/target = 本轮达标天数 / 周期
//   - daily：count（达标天数）、lastAt、progress/target = 今天已学秒数 / 3600
type AchievementDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Kind        string `json:"kind"` // once | cycle | daily
	Unit        string `json:"unit"`
	Unlocked    bool   `json:"unlocked"`
	Progress    int64  `json:"progress"`
	Target      int64  `json:"target"`
	// Count 是达成次数（循环=轮数、每日=达标天数）。
	Count int `json:"count"`
	// LastAt 是上次达成日期；从未达成为空串。
	LastAt string `json:"lastAt"`
	// AchievedAt 是单次成就的达成时间；未达成为空串。
	AchievedAt string `json:"achievedAt"`
	// TodayDone 只对每日成就意义：今天是否已经达标。
	TodayDone bool `json:"todayDone"`
	// Medal 是循环成就达成一次发的奖章（bronze|silver|gold|ruby）；其它类别为空。
	Medal string `json:"medal"`
	// RewardTenths 是达成一次的经验奖励（十分之一经验）。
	RewardTenths int64 `json:"rewardTenths"`
}

// MedalCountsDTO 是四类奖章的累计数量。
type MedalCountsDTO struct {
	Week    int `json:"week"`
	Month   int `json:"month"`
	Quarter int `json:"quarter"`
	Year    int `json:"year"`
}

// LevelDTO 是等级条需要的一切。
//
// 经验一律用**十分之一经验**的整数传输（见 internal/achievements 的说明），
// 前端只负责显示成一位小数。
type LevelDTO struct {
	Level int `json:"level"`
	// XpTenths 是累计经验；IntoTenths/NeedTenths 是当前等级内的进度与升级所需。
	XpTenths   int64 `json:"xpTenths"`
	IntoTenths int64 `json:"intoTenths"`
	NeedTenths int64 `json:"needTenths"`
	// RateTenths 是当前"每小时学习"能拿到的经验（含奖章加成）。
	RateTenths int64 `json:"rateTenths"`
	// HoursCounted 是已经折算过经验的整小时数（成长经验的来源）。
	HoursCounted int64 `json:"hoursCounted"`
	// CarrySec 是没凑满一小时的零头，成长卡片的进度条用它。
	CarrySec int64          `json:"carrySec"`
	Medals   MedalCountsDTO `json:"medals"`
}

// AchievementsDTO 是成就页需要的全部数据。
//
// 顶部那几个汇总数字与下面各条来自同一次读取，所以不可能出现
// "累计 9.5 小时但十小时成就已解锁"这种自相矛盾的画面。
type AchievementsDTO struct {
	Unlocked int `json:"unlocked"`
	Total    int `json:"total"`
	// TotalSec 是累计时长（秒）。前端自己决定显示成"12 小时 34 分"还是别的。
	TotalSec int64 `json:"totalSec"`
	// TodaySec 是今天已落库的时长（秒）；每日成就的进度条用它。
	TodaySec int64 `json:"todaySec"`
	// LongestSec 是单次最长时长。
	LongestSec int64 `json:"longestSec"`
	// Sessions / ActiveDays 分别是总场次与有记录的天数。
	Sessions   int `json:"sessions"`
	ActiveDays int `json:"activeDays"`
	// CurrentStreak 是当前连续天数（最后一天是今天或昨天才算连着）。
	CurrentStreak int `json:"currentStreak"`
	// LongestStreak 是历史最长连续天数。
	LongestStreak int `json:"longestStreak"`
	// LateNightCount / EarlyMorningCount 是深夜与早起开始过的场次。
	LateNightCount    int `json:"lateNightCount"`
	EarlyMorningCount int `json:"earlyMorningCount"`
	// PlanCount 是有记录的计划数。
	PlanCount int `json:"planCount"`
	// QualifyingDays 是"达标日"总数（当天满 1 小时），循环成就的轮数由它推出来。
	QualifyingDays int `json:"qualifyingDays"`
	// Level 是等级与经验。
	Level LevelDTO `json:"level"`
	// Items 是全部成就，顺序与 internal/achievements.All 一致。
	Items []AchievementDTO `json:"items"`
}

// Summary 返回成就页的全部数据。
//
// 统计数字来自记录（可重算），等级与经验来自**存下来的账**（不可重算，见
// internal/achievements/progress.go 的说明），两者在这里汇合。
func (s *AchievementsService) Summary() (AchievementsDTO, error) {
	today := s.b.clock().Format(layoutDateKey)
	stats, err := s.b.db.Stats(today)
	if err != nil {
		return AchievementsDTO{}, err
	}
	progress, err := s.b.db.LoadProgress()
	if err != nil {
		return AchievementsDTO{}, err
	}
	qualifying, err := s.b.db.QualifyingDays()
	if err != nil {
		return AchievementsDTO{}, err
	}
	states, err := s.b.db.LoadAchievementStates()
	if err != nil {
		return AchievementsDTO{}, err
	}

	var todaySec int64
	if n := len(stats.Days); n > 0 && stats.Days[n-1].Date == today {
		todaySec = stats.Days[n-1].TotalSec
	}

	plain := achievements.Stats{
		TotalSec:     stats.TotalSec,
		SessionCount: stats.SessionCount,
		LongestSec:   stats.LongestSec,
		ActiveDays:   len(stats.Days),
		PlanCount:    stats.PlanCount,
	}
	firstPlanAt, err := s.b.db.FirstPlanCreatedAt()
	if err != nil {
		return AchievementsDTO{}, err
	}
	plain.FirstPlanAt = firstPlanAt

	// 等级：按曲线算出来的与存下来的高水位取较大者——曲线被改陡时不允许掉级。
	derived, into, need := achievements.LevelAt(progress.XpTenths)
	level := derived
	if progress.Level > level {
		level = progress.Level
		floor := achievements.LevelFloorTenths(level)
		into = progress.XpTenths - floor
		if into < 0 {
			into = 0
		}
		need = achievements.LevelCost(level) * 10
	}

	medals := achievements.MedalCounts{
		Week:    progress.MedalsWeek,
		Month:   progress.MedalsMonth,
		Quarter: progress.MedalsQuarter,
		Year:    progress.MedalsYear,
	}

	items := make([]AchievementDTO, 0, len(achievements.All))
	unlocked := 0
	for _, def := range achievements.All {
		st := states[def.ID]
		item := AchievementDTO{
			ID:           def.ID,
			Name:         def.Name,
			Description:  def.Description,
			Kind:         def.Kind,
			Unit:         def.Unit,
			Count:        st.Count,
			LastAt:       st.LastAt,
			AchievedAt:   st.AchievedAt,
			Medal:        def.Medal,
			RewardTenths: def.RewardTenths,
			Target:       def.Target,
		}

		switch def.Kind {
		case achievements.KindOnce:
			value := def.Value(plain)
			if value < 0 {
				value = 0
			}
			item.Unlocked = value >= def.Target
			item.Progress = value
			if item.Progress > def.Target {
				item.Progress = def.Target
			}

		case achievements.KindCycle:
			item.Unit = "天"
			item.Target = int64(def.PeriodDays)
			item.Progress = int64(def.Remainder(len(qualifying), st.Count))
			// "达成过至少一次"就算解锁：奖章不会因为门槛调整而消失。
			item.Unlocked = st.Count > 0

		case achievements.KindDaily:
			item.Unit = "秒"
			item.Target = achievements.ThresholdSec
			item.Progress = todaySec
			if item.Progress > item.Target {
				item.Progress = item.Target
			}
			item.TodayDone = todaySec >= achievements.ThresholdSec
			item.Unlocked = st.Count > 0
		}

		if item.Unlocked {
			unlocked++
		}
		items = append(items, item)
	}

	return AchievementsDTO{
		Unlocked:          unlocked,
		Total:             len(items),
		TotalSec:          stats.TotalSec,
		TodaySec:          todaySec,
		LongestSec:        stats.LongestSec,
		Sessions:          stats.SessionCount,
		ActiveDays:        len(stats.Days),
		CurrentStreak:     stats.CurrentStreak,
		LongestStreak:     stats.LongestStreak,
		LateNightCount:    stats.LateNightCount,
		EarlyMorningCount: stats.EarlyMorningCount,
		PlanCount:         stats.PlanCount,
		QualifyingDays:    len(qualifying),
		Level: LevelDTO{
			Level:        level,
			XpTenths:     progress.XpTenths,
			IntoTenths:   into,
			NeedTenths:   need,
			RateTenths:   achievements.RateTenths(medals),
			HoursCounted: hoursCounted(stats.TotalSec, progress.CarrySec),
			CarrySec:     progress.CarrySec,
			Medals: MedalCountsDTO{
				Week:    medals.Week,
				Month:   medals.Month,
				Quarter: medals.Quarter,
				Year:    medals.Year,
			},
		},
		Items: items,
	}, nil
}

// settleProgress 是注入给计时器的结算函数：把"结束并保存"的这一段并进经验账。
//
// store 已经在事务里读好了进度、当天总时长、累计总时长与各成就状态，
// 这里只做类型转换与规则调用，不碰数据库——所以它天然不会读到事务外的脏数据。
func (b *backend) settleProgress(in store.SettleInput) (store.SettleOutcome, error) {
	states := make(map[string]achievements.AchievementState, len(in.States))
	for id, row := range in.States {
		states[id] = achievements.AchievementState{
			Count:      row.Count,
			LastAt:     row.LastAt,
			AchievedAt: row.AchievedAt,
		}
	}

	out := achievements.Settle(achievements.SettleInput{
		State: achievements.State{
			XpTenths: in.Progress.XpTenths,
			CarrySec: in.Progress.CarrySec,
			Level:    in.Progress.Level,
			Medals: achievements.MedalCounts{
				Week:    in.Progress.MedalsWeek,
				Month:   in.Progress.MedalsMonth,
				Quarter: in.Progress.MedalsQuarter,
				Year:    in.Progress.MedalsYear,
			},
		},
		States:         states,
		Day:            in.Day,
		DayQualified:   in.DayQualified,
		DayTotalSec:    in.DayTotalSec,
		TotalSec:       in.TotalSec,
		DurationSec:    in.DurationSec,
		QualifyingDays: in.QualifyingDays,
		Now:            in.Now,
	})

	result := store.SettleOutcome{
		Progress: store.ProgressRow{
			XpTenths:      out.State.XpTenths,
			CarrySec:      out.State.CarrySec,
			Level:         out.State.Level,
			MedalsWeek:    out.State.Medals.Week,
			MedalsMonth:   out.State.Medals.Month,
			MedalsQuarter: out.State.Medals.Quarter,
			MedalsYear:    out.State.Medals.Year,
		},
	}
	if out.QualifyDay {
		result.QualifyDay = &store.QualifyingDay{Date: in.Day, Sec: in.DayTotalSec}
	}
	for id, st := range out.States {
		result.States = append(result.States, store.AchievementStateRow{
			ID:         id,
			Count:      st.Count,
			LastAt:     st.LastAt,
			AchievedAt: st.AchievedAt,
		})
	}
	return result, nil
}

// hoursCounted 由"累计时长里已经折算过经验的那部分"推出来，仅供展示。
//
// 账里只存了零头，所以已结算的整小时数 = (总时长 - 零头) / 3600。
func hoursCounted(totalSec, carrySec int64) int64 {
	if totalSec <= carrySec {
		return 0
	}
	return (totalSec - carrySec) / achievements.HourSec
}

// backfillProgressIfNeeded 在"账是空的、但已经有学习记录"时按记录回放一次补上初值。
//
// 触发条件只有两种：老库刚升级到 v3（表刚建出来还是空的），或者进度表被人清过。
// 两种情况都该以记录为准重建，所以用回放；平时的增量路径绝不会走到这里。
func (b *backend) backfillProgressIfNeeded() error {
	count, err := b.db.CountSessions()
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}

	progress, err := b.db.LoadProgress()
	if err != nil {
		return err
	}
	days, err := b.db.QualifyingDays()
	if err != nil {
		return err
	}
	states, err := b.db.LoadAchievementStates()
	if err != nil {
		return err
	}
	if progress.XpTenths != 0 || progress.CarrySec != 0 || len(days) != 0 || len(states) != 0 {
		return nil
	}

	today := b.clock().Format(layoutDateKey)
	stats, err := b.db.Stats(today)
	if err != nil {
		return err
	}
	firstPlanAt, err := b.db.FirstPlanCreatedAt()
	if err != nil {
		return err
	}

	dayList := make([]achievements.Day, 0, len(stats.Days))
	for _, d := range stats.Days {
		dayList = append(dayList, achievements.Day{Date: d.Date, Sec: d.TotalSec})
	}
	state, qualifying, st := achievements.Replay(dayList, firstPlanAt)

	rows := make([]store.QualifyingDay, 0, len(qualifying))
	for _, d := range qualifying {
		rows = append(rows, store.QualifyingDay{Date: d.Date, Sec: d.Sec})
	}
	stateRows := make([]store.AchievementStateRow, 0, len(st))
	for id, item := range st {
		stateRows = append(stateRows, store.AchievementStateRow{
			ID: id, Count: item.Count, LastAt: item.LastAt, AchievedAt: item.AchievedAt,
		})
	}

	if err := b.db.ReplaceProgress(store.ProgressRow{
		XpTenths:      state.XpTenths,
		CarrySec:      state.CarrySec,
		Level:         state.Level,
		MedalsWeek:    state.Medals.Week,
		MedalsMonth:   state.Medals.Month,
		MedalsQuarter: state.Medals.Quarter,
		MedalsYear:    state.Medals.Year,
	}, rows, stateRows, b.clock().Format(layoutRecord)); err != nil {
		return err
	}

	slog.Info("已按历史记录回填经验与等级",
		"xpTenths", state.XpTenths, "level", state.Level,
		"qualifyingDays", len(rows), "sessions", count)
	return nil
}

// markFirstPlan 在第一次创建计划时记下达成时间（只写一次）。
func (b *backend) markFirstPlan() error {
	states, err := b.db.LoadAchievementStates()
	if err != nil {
		return err
	}
	if cur := states["first-plan"]; cur.AchievedAt != "" {
		return nil
	}
	return b.db.SaveAchievementState(store.AchievementStateRow{
		ID:         "first-plan",
		AchievedAt: b.clock().Format(layoutRecord),
	})
}
