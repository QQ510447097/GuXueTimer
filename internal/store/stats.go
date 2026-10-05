package store

import (
	"fmt"
	"time"
)

// dateLayout 是日期键的格式，与 sessions.started_at 的前 10 个字符一致。
//
// started_at 存的是带本地偏移的 RFC3339，所以 substr(...,1,10) 拿到的就是**本地**日期；
// 同理 substr(...,12,2) 是本地小时。这与应用层看到的日期一致，不需要再做时区换算。
const dateLayout = "2006-01-02"

// DayTotal 是某一天的总时长。
type DayTotal struct {
	Date     string
	TotalSec int64
}

// SessionStats 是全部历史记录的聚合结果，供成就页使用。
//
// 一次查完所有需要的量，而不是让调用方按成就逐个查：现在只有三个成就，
// 但成就这种东西一定会越加越多，每加一个就多一次全表扫描不划算。
type SessionStats struct {
	TotalSec     int64
	SessionCount int
	// LongestSec 是单次最长时长；一次都没记过时为 0。
	LongestSec int64
	// FirstDay / LastDay 是首末记录日期（无记录时为空串）。
	FirstDay string
	LastDay  string
	// PlanCount 是有记录的计划数（未分类的记录不计入）。
	PlanCount int
	// LateNightCount 是"深夜开始"的会话数：23 点之后或凌晨 5 点之前。
	LateNightCount int
	// EarlyMorningCount 是"早起开始"的会话数：5 点到 7 点之间。
	//
	// 两者刻意不重叠：凌晨 2 点算深夜而不是早起——那是熬夜的延续，
	// 归到"早起"里会把一个熬夜的人夸成早起的人。
	EarlyMorningCount int
	// Days 是有记录的每一天，按日期升序、不重复。
	Days []DayTotal
	// CurrentStreak 是"到今天为止还连着"的天数。
	//
	// 最后一天是今天或昨天都算连着：今天还没开始学不该把连击清零。
	CurrentStreak int
	// LongestStreak 是历史最长连续天数。
	LongestStreak int
}

// Stats 汇总全部历史记录。today 形如 "2006-01-02"，用来判断"今天"。
func (d *DB) Stats(today string) (SessionStats, error) {
	var out SessionStats

	// COALESCE 让空库也返回一行，调用方不必区分 nil 与 0。
	// COUNT(DISTINCT plan_id) 会忽略 NULL，所以未分类的记录不会算成一个"计划"。
	err := d.sql.QueryRow(
		`SELECT COALESCE(SUM(duration_sec), 0),
		        COUNT(*),
		        COALESCE(MAX(duration_sec), 0),
		        COALESCE(MIN(substr(started_at, 1, 10)), ''),
		        COALESCE(MAX(substr(started_at, 1, 10)), ''),
		        COUNT(DISTINCT plan_id),
		        COALESCE(SUM(CASE WHEN CAST(substr(started_at, 12, 2) AS INTEGER) >= 23
		                           OR CAST(substr(started_at, 12, 2) AS INTEGER) < 5
		                          THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN CAST(substr(started_at, 12, 2) AS INTEGER) >= 5
		                           AND CAST(substr(started_at, 12, 2) AS INTEGER) < 7
		                          THEN 1 ELSE 0 END), 0)
		   FROM sessions`,
	).Scan(
		&out.TotalSec,
		&out.SessionCount,
		&out.LongestSec,
		&out.FirstDay,
		&out.LastDay,
		&out.PlanCount,
		&out.LateNightCount,
		&out.EarlyMorningCount,
	)
	if err != nil {
		return SessionStats{}, fmt.Errorf("统计历史记录失败: %w", err)
	}

	days, err := d.statsDays()
	if err != nil {
		return SessionStats{}, err
	}
	out.Days = days
	out.CurrentStreak, out.LongestStreak = streaks(days, today)
	return out, nil
}

// statsDays 返回有记录的每一天及其总时长，升序。
func (d *DB) statsDays() ([]DayTotal, error) {
	rows, err := d.sql.Query(
		`SELECT substr(started_at, 1, 10) AS day, SUM(duration_sec)
		   FROM sessions
		  GROUP BY day
		  ORDER BY day ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("按天汇总历史记录失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []DayTotal
	for rows.Next() {
		var item DayTotal
		if err := rows.Scan(&item.Date, &item.TotalSec); err != nil {
			return nil, fmt.Errorf("读取按天汇总结果失败: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历按天汇总结果失败: %w", err)
	}
	return out, nil
}

// streaks 从升序的日期列表里算出"当前连击"与"历史最长连击"。
//
// 用日期串解析成 time.Time 再比较：跨月、跨年、闰年都由标准库处理，
// 自己数字符串会在 2 月与年底出错。解析用 UTC（dateLayout 没有时区），
// 于是"相邻两天"恰好是 24 小时，不受夏令时影响。
//
// 两个量要分别跟踪：循环结束时 run 描述的是**最后一段**连续（当前连击要用它），
// 而 longest 是**最长的一段**。用一个变量同时记两者会拿"最长那段的结尾"去判断
// "今天还连着没有"，那是错的。
func streaks(days []DayTotal, today string) (current, longest int) {
	if len(days) == 0 {
		return 0, 0
	}

	run := 0
	lastEnd := ""
	var prev time.Time

	for _, day := range days {
		at, err := time.Parse(dateLayout, day.Date)
		if err != nil {
			// 库里的日期坏了：跳过它，但不让它把连击算错。
			continue
		}
		if !prev.IsZero() && at.Sub(prev) == 24*time.Hour {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
		lastEnd = day.Date
		prev = at
	}

	// 连击要"还活着"才算当前：最后一段连续要结束在今天或昨天。
	// 昨天也算，是因为今天还没开始学不该把连击清零。
	yesterday := ""
	if t, err := time.Parse(dateLayout, today); err == nil {
		yesterday = t.AddDate(0, 0, -1).Format(dateLayout)
	}
	if lastEnd == today || (yesterday != "" && lastEnd == yesterday) {
		current = run
	}
	// 断了的话当前连击是 0，最长连击照旧保留——成就是据最长连击解锁的，不会因为断掉而丢。
	return current, longest
}
