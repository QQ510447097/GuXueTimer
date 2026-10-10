package store

import (
	"database/sql"
	"errors"
	"fmt"
)

const sessionColumns = `id, started_at, ended_at, duration_sec, note, source, created_at, plan_id`

// planArg 把应用层的 0（未分类）转成 SQL 的 NULL。
func planArg(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

// planFilter 是"按计划筛选"的公共条件。
//
// 传 0 表示不筛选（含未分类的记录一起返回）；传具体 id 时 NULL 行自然被排除，
// 这正是想要的语义：筛某个计划时不该混进未分类的记录。
const planFilter = `(? = 0 OR plan_id = ?)`

// SessionsByDay 返回 startDate 当天、planID 对应的记录，按开始时间升序。
//
// startDate 形如 "2006-01-02"，语义是 started_at 的本地日期。
// 跨零点的会话整体归属开始日，不拆分：一次会话是一个语义单位。
// planID 传 0 表示不过滤。
func (d *DB) SessionsByDay(startDate string, planID int64) ([]SessionRow, error) {
	rows, err := d.sql.Query(
		`SELECT `+sessionColumns+`
		   FROM sessions
		  WHERE substr(started_at, 1, 10) = ?
		    AND `+planFilter+`
		  ORDER BY started_at ASC, id ASC`,
		startDate, planID, planID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询学习记录失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SessionRow, 0, 16)
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历学习记录失败: %w", err)
	}
	return out, nil
}

// rowScanner 让 scanSession 同时适配 *sql.Row 与 *sql.Rows。
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(src rowScanner) (SessionRow, error) {
	var s SessionRow
	var planID sql.NullInt64
	if err := src.Scan(&s.ID, &s.StartedAt, &s.EndedAt, &s.DurationSec,
		&s.Note, &s.Source, &s.CreatedAt, &planID); err != nil {
		return SessionRow{}, fmt.Errorf("读取学习记录失败: %w", err)
	}
	if planID.Valid {
		s.PlanID = planID.Int64
	}
	return s, nil
}

// SummaryByDay 汇总 startDate 当天、planID 对应的时长与条数。
func (d *DB) SummaryByDay(startDate string, planID int64) (DaySummary, error) {
	sum := DaySummary{Date: startDate}
	err := d.sql.QueryRow(
		`SELECT COALESCE(SUM(duration_sec), 0),
		        COUNT(*),
		        COALESCE(MIN(started_at), ''),
		        COALESCE(MAX(ended_at), '')
		   FROM sessions
		  WHERE substr(started_at, 1, 10) = ?
		    AND `+planFilter,
		startDate, planID, planID,
	).Scan(&sum.TotalSec, &sum.Count, &sum.FirstStart, &sum.LastEnd)
	if err != nil {
		return DaySummary{}, fmt.Errorf("统计当日学习时长失败: %w", err)
	}
	return sum, nil
}

// DailyTotals 返回 [from, to] 闭区间内每天的合计（只返回有记录的日子）。
//
// 柱状图与月历热力图共用它。缺的日期不在这里补零：调用方按自己的区间填，
// 比让 SQL 造一串日期干净得多。
func (d *DB) DailyTotals(from, to string, planID int64) ([]DailyTotal, error) {
	rows, err := d.sql.Query(
		`SELECT substr(started_at, 1, 10) AS day,
		        COALESCE(SUM(duration_sec), 0),
		        COUNT(*)
		   FROM sessions
		  WHERE substr(started_at, 1, 10) >= ?
		    AND substr(started_at, 1, 10) <= ?
		    AND `+planFilter+`
		  GROUP BY day
		  ORDER BY day ASC`,
		from, to, planID, planID,
	)
	if err != nil {
		return nil, fmt.Errorf("统计每日时长失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]DailyTotal, 0, 32)
	for rows.Next() {
		var t DailyTotal
		if err := rows.Scan(&t.Date, &t.TotalSec, &t.Count); err != nil {
			return nil, fmt.Errorf("读取每日时长失败: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历每日时长失败: %w", err)
	}
	return out, nil
}

// InsertSession 写入一条学习记录并返回自增 ID。
func (d *DB) InsertSession(s SessionRow) (int64, error) {
	res, err := d.sql.Exec(
		`INSERT INTO sessions(started_at, ended_at, duration_sec, note, source, created_at, plan_id)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		s.StartedAt, s.EndedAt, s.DurationSec, s.Note, s.Source, s.CreatedAt, planArg(s.PlanID),
	)
	if err != nil {
		return 0, fmt.Errorf("写入学习记录失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取学习记录 ID 失败: %w", err)
	}
	return id, nil
}

// DeleteSession 删除一条记录，不存在时返回 ErrNotFound。
func (d *DB) DeleteSession(id int64) error {
	res, err := d.sql.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除学习记录失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("确认删除结果失败: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SettleInput 是"把一次会话并进进度"的输入。
//
// 全部由 store 在**同一个事务里**读好传进来，所以规则那一层是纯函数：
// 它只需要这几个数字就能算出新进度，不必自己去查库（也就没法在事务中间读脏数据）。
type SettleInput struct {
	// Progress 是结算前的进度。
	Progress ProgressRow
	// Day 是这次会话归属的日期（YYYY-MM-DD），即 started_at 的前 10 个字符。
	Day string
	// DayQualified 表示这天是否已经记进达标账本。
	DayQualified bool
	// DayTotalSec 是这天（含刚写入的这条记录）的总时长。
	DayTotalSec int64
	// TotalSec 是全部历史（含刚写入的这条记录）的总时长，
	// 单次成就"累计 N 小时"的越线判定要用它。
	TotalSec int64
	// DurationSec 是刚写入这条记录的时长。
	DurationSec int64
	// QualifyingDays 是**这次之前**账本里的达标日总数。
	QualifyingDays int
	// States 是各成就的当前状态（快照，规则那一层只看不写）。
	States map[string]AchievementStateRow
	// Now 是当前时刻（RFC3339），写"达成时间"用。
	Now string
}

// SettleOutcome 是结算结果。调用方把新进度与需要落库的状态一并返回，store 只负责写。
type SettleOutcome struct {
	Progress ProgressRow
	// QualifyDay 非 nil 表示这天首次达标，要记进账本。
	QualifyDay *QualifyingDay
	// States 是需要写回的成就状态（达成次数 / 上次达成 / 单次达成时间）。
	States []AchievementStateRow
}

// SettleFunc 是规则那一层注入的结算函数。为 nil 时表示这次不结算（例如测试）。
type SettleFunc func(in SettleInput) (SettleOutcome, error)

// FinishSession 在一个事务里写入学习记录、复位计时状态，并把这次会话并进进度。
//
// 必须原子：拆开会产生「记录已落库但状态没复位」的中间态，进程此时崩溃的话，
// 下次启动的恢复逻辑会把同一段时长再结算一次。进度也不能拆出去——
// 拆开后崩溃会漏发经验，而进度是只增不减的账，漏了就补不回来。
func (d *DB) FinishSession(sess SessionRow, idle TimerStateRow, settle SettleFunc) (int64, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return 0, fmt.Errorf("开启结算事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(
		`INSERT INTO sessions(started_at, ended_at, duration_sec, note, source, created_at, plan_id)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		sess.StartedAt, sess.EndedAt, sess.DurationSec, sess.Note, sess.Source, sess.CreatedAt,
		planArg(sess.PlanID),
	)
	if err != nil {
		return 0, fmt.Errorf("写入学习记录失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取学习记录 ID 失败: %w", err)
	}

	if _, err := tx.Exec(
		`UPDATE timer_state
		    SET state = ?, started_at = ?, last_resumed_at = ?, accumulated_sec = ?, plan_id = ?
		  WHERE id = 1`,
		idle.State, idle.StartedAt, idle.LastResumedAt, idle.AccumulatedSec, idle.PlanID,
	); err != nil {
		return 0, fmt.Errorf("复位计时状态失败: %w", err)
	}

	if settle != nil {
		if err := settleInTx(tx, sess, settle); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交结算事务失败: %w", err)
	}
	return id, nil
}

// settleInTx 在事务里读齐结算需要的数字、调用规则函数、写回结果。
//
// 规则（倍率、门槛、周期）一概不在这层：这里只做"读—问—写"，
// 免得同一套规则散在 SQL 与 Go 两处。
func settleInTx(tx *sql.Tx, sess SessionRow, settle SettleFunc) error {
	var cur ProgressRow
	if err := tx.QueryRow(
		`SELECT xp_tenths, carry_sec, level, medals_week, medals_month, medals_quarter, medals_year
		   FROM progress WHERE id = 1`,
	).Scan(
		&cur.XpTenths, &cur.CarrySec, &cur.Level,
		&cur.MedalsWeek, &cur.MedalsMonth, &cur.MedalsQuarter, &cur.MedalsYear,
	); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("读取进度失败: %w", err)
	}
	if cur.Level == 0 {
		cur.Level = 1
	}

	day := ""
	if len(sess.StartedAt) >= 10 {
		day = sess.StartedAt[:10]
	}

	var dayTotal int64
	if day != "" {
		if err := tx.QueryRow(
			`SELECT COALESCE(SUM(duration_sec), 0) FROM sessions WHERE substr(started_at, 1, 10) = ?`,
			day,
		).Scan(&dayTotal); err != nil {
			return fmt.Errorf("统计当天时长失败: %w", err)
		}
	}

	var qualified int
	dayQualified := false
	if day != "" {
		err := tx.QueryRow(`SELECT 1 FROM qualifying_days WHERE date = ?`, day).Scan(&qualified)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("查询达标日失败: %w", err)
		}
		dayQualified = err == nil
	}

	var total int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(duration_sec), 0) FROM sessions`).Scan(&total); err != nil {
		return fmt.Errorf("统计累计时长失败: %w", err)
	}

	var qualifyingCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM qualifying_days`).Scan(&qualifyingCount); err != nil {
		return fmt.Errorf("统计达标日失败: %w", err)
	}

	states, err := achievementStatesInTx(tx)
	if err != nil {
		return err
	}

	out, err := settle(SettleInput{
		Progress:       cur,
		Day:            day,
		DayQualified:   dayQualified,
		DayTotalSec:    dayTotal,
		TotalSec:       total,
		DurationSec:    sess.DurationSec,
		QualifyingDays: qualifyingCount,
		States:         states,
		Now:            sess.CreatedAt,
	})
	if err != nil {
		return err
	}

	if _, err := tx.Exec(
		`UPDATE progress
		    SET xp_tenths = ?, carry_sec = ?, level = ?,
		        medals_week = ?, medals_month = ?, medals_quarter = ?, medals_year = ?
		  WHERE id = 1`,
		out.Progress.XpTenths, out.Progress.CarrySec, out.Progress.Level,
		out.Progress.MedalsWeek, out.Progress.MedalsMonth, out.Progress.MedalsQuarter, out.Progress.MedalsYear,
	); err != nil {
		return fmt.Errorf("写入进度失败: %w", err)
	}

	if out.QualifyDay != nil {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO qualifying_days(date, sec) VALUES(?, ?)`,
			out.QualifyDay.Date, out.QualifyDay.Sec,
		); err != nil {
			return fmt.Errorf("记录达标日失败: %w", err)
		}
	}

	for _, st := range out.States {
		if _, err := tx.Exec(
			`INSERT INTO achievement_state(id, count, last_at, achieved_at) VALUES(?, ?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET count = excluded.count,
			                               last_at = excluded.last_at,
			                               achieved_at = excluded.achieved_at`,
			st.ID, st.Count, st.LastAt, st.AchievedAt,
		); err != nil {
			return fmt.Errorf("写入成就状态失败: %w", err)
		}
	}
	return nil
}

// achievementStatesInTx 在事务里读出全部成就状态。
func achievementStatesInTx(tx *sql.Tx) (map[string]AchievementStateRow, error) {
	rows, err := tx.Query(`SELECT id, count, last_at, achieved_at FROM achievement_state`)
	if err != nil {
		return nil, fmt.Errorf("读取成就状态失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]AchievementStateRow{}
	for rows.Next() {
		var item AchievementStateRow
		if err := rows.Scan(&item.ID, &item.Count, &item.LastAt, &item.AchievedAt); err != nil {
			return nil, fmt.Errorf("读取成就状态失败: %w", err)
		}
		out[item.ID] = item
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历成就状态失败: %w", err)
	}
	return out, nil
}

// sessionExists 仅用于测试断言。
func (d *DB) sessionExists(id int64) (bool, error) {
	var one int
	err := d.sql.QueryRow(`SELECT 1 FROM sessions WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
