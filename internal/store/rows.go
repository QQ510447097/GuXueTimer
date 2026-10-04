package store

// 时间戳一律存成带本地时区偏移的 RFC3339 字符串，例如：
//
//	2026-05-01T14:03:05+08:00
//
// 好处：可读、可直接字典序比较（同一偏移下）、跨 SQLite 版本稳定，
// 并且前 10 个字符恰好就是本地日期，按日分组不需要任何时区换算。

// SessionRow 是一条已保存的学习记录。
type SessionRow struct {
	ID          int64
	StartedAt   string // RFC3339
	EndedAt     string // RFC3339
	DurationSec int64  // 纯学习秒数，已排除暂停区间
	Note        string
	Source      string // timer | recovered（预留给未来的补录）
	CreatedAt   string // RFC3339
}

// TimerStateRow 是 timer_state 单行表的快照。
type TimerStateRow struct {
	State          string // idle | running | paused
	StartedAt      string // 本次会话开始时间；idle 时为空串
	LastResumedAt  string // 最近一次进入 running 的时刻；仅 running 时有意义
	AccumulatedSec int64  // 已确定的时长，不含当前正在走的这一段
}

// DaySummary 是某一天的汇总。
type DaySummary struct {
	Date       string // 2006-01-02
	TotalSec   int64
	Count      int64
	FirstStart string // 当天最早的开始时间；无记录时为空串
	LastEnd    string // 当天最晚的结束时间；无记录时为空串
}
