package main

import (
	"goxuetimer/internal/achievements"
)

// AchievementsService 汇总历史统计并评价成就。
type AchievementsService struct {
	b *backend
}

// NewAchievementsService 构造成就服务。
func NewAchievementsService(b *backend) *AchievementsService {
	return &AchievementsService{b: b}
}

// AchievementDTO 是一个成就的状态。
type AchievementDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Tier        string `json:"tier"` // bronze | silver | gold
	Unit        string `json:"unit"`
	Unlocked    bool   `json:"unlocked"`
	Progress    int64  `json:"progress"`
	Target      int64  `json:"target"`
}

// AchievementsDTO 是成就页需要的全部数据。
//
// 顶部那几个汇总数字与成就本身来自同一次统计，所以不可能出现
// "累计 9.5 小时但十小时成就已解锁"这种自相矛盾的画面。
type AchievementsDTO struct {
	Unlocked int `json:"unlocked"`
	Total    int `json:"total"`
	// TotalSec 是累计时长（秒）。前端自己决定显示成"12 小时 34 分"还是别的。
	TotalSec int64 `json:"totalSec"`
	// LongestSec 是单次最长时长。
	LongestSec int64 `json:"longestSec"`
	// Sessions / ActiveDays 分别是总场次与有记录的天数。
	Sessions   int `json:"sessions"`
	ActiveDays int `json:"activeDays"`
	// CurrentStreak 是当前连续天数（最后一天是今天或昨天才算连着）。
	CurrentStreak int `json:"currentStreak"`
	// LongestStreak 是历史最长连续天数，也是"三日不辍"的判据。
	LongestStreak int `json:"longestStreak"`
	// LateNightCount / EarlyMorningCount 是深夜与早起开始过的场次。
	LateNightCount    int `json:"lateNightCount"`
	EarlyMorningCount int `json:"earlyMorningCount"`
	// PlanCount 是有记录的计划数。
	PlanCount int              `json:"planCount"`
	Items     []AchievementDTO `json:"items"`
}

// Summary 返回成就页的全部数据。
func (s *AchievementsService) Summary() (AchievementsDTO, error) {
	stats, err := s.b.db.Stats(s.b.clock().Format(layoutDateKey))
	if err != nil {
		return AchievementsDTO{}, err
	}

	plain := achievements.Stats{
		TotalSec:      stats.TotalSec,
		SessionCount:  stats.SessionCount,
		LongestSec:    stats.LongestSec,
		ActiveDays:    len(stats.Days),
		CurrentStreak: stats.CurrentStreak,
		LongestStreak: stats.LongestStreak,
		PlanCount:     stats.PlanCount,
	}

	results := achievements.Evaluate(plain)
	items := make([]AchievementDTO, 0, len(results))
	unlocked := 0
	for _, r := range results {
		if r.Unlocked {
			unlocked++
		}
		items = append(items, AchievementDTO{
			ID:          r.ID,
			Name:        r.Name,
			Description: r.Description,
			Tier:        r.Tier,
			Unit:        r.Unit,
			Unlocked:    r.Unlocked,
			Progress:    r.Progress,
			Target:      r.Target,
		})
	}

	return AchievementsDTO{
		Unlocked:          unlocked,
		Total:             len(items),
		TotalSec:          stats.TotalSec,
		LongestSec:        stats.LongestSec,
		Sessions:          stats.SessionCount,
		ActiveDays:        len(stats.Days),
		CurrentStreak:     stats.CurrentStreak,
		LongestStreak:     stats.LongestStreak,
		LateNightCount:    stats.LateNightCount,
		EarlyMorningCount: stats.EarlyMorningCount,
		PlanCount:         stats.PlanCount,
		Items:             items,
	}, nil
}
