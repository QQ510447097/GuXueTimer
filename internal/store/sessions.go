package store

import (
	"database/sql"
	"errors"
	"fmt"
)

const sessionColumns = `id, started_at, ended_at, duration_sec, note, source, created_at`

// InsertSession 写入一条学习记录并返回自增 ID。
func (d *DB) InsertSession(s SessionRow) (int64, error) {
	res, err := d.sql.Exec(
		`INSERT INTO sessions(started_at, ended_at, duration_sec, note, source, created_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		s.StartedAt, s.EndedAt, s.DurationSec, s.Note, s.Source, s.CreatedAt,
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

// SessionsByDay 返回 startDate 当天的全部记录，按开始时间升序。
//
// startDate 形如 "2006-01-02"，语义是 started_at 的本地日期。
// 跨零点的会话整体归属开始日，不拆分：一次会话是一个语义单位。
func (d *DB) SessionsByDay(startDate string) ([]SessionRow, error) {
	rows, err := d.sql.Query(
		`SELECT `+sessionColumns+`
		   FROM sessions
		  WHERE substr(started_at, 1, 10) = ?
		  ORDER BY started_at ASC, id ASC`,
		startDate,
	)
	if err != nil {
		return nil, fmt.Errorf("查询学习记录失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]SessionRow, 0, 16)
	for rows.Next() {
		var s SessionRow
		if err := rows.Scan(&s.ID, &s.StartedAt, &s.EndedAt, &s.DurationSec,
			&s.Note, &s.Source, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("读取学习记录失败: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历学习记录失败: %w", err)
	}
	return out, nil
}

// SummaryByDay 汇总 startDate 当天的时长与条数。
func (d *DB) SummaryByDay(startDate string) (DaySummary, error) {
	sum := DaySummary{Date: startDate}
	err := d.sql.QueryRow(
		`SELECT COALESCE(SUM(duration_sec), 0),
		        COUNT(*),
		        COALESCE(MIN(started_at), ''),
		        COALESCE(MAX(ended_at), '')
		   FROM sessions
		  WHERE substr(started_at, 1, 10) = ?`,
		startDate,
	).Scan(&sum.TotalSec, &sum.Count, &sum.FirstStart, &sum.LastEnd)
	if err != nil {
		return DaySummary{}, fmt.Errorf("统计当日学习时长失败: %w", err)
	}
	return sum, nil
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

// FinishSession 在一个事务里写入学习记录并同时把计时状态复位。
//
// 必须原子：拆成 INSERT + UPDATE 两条语句会产生「记录已落库但状态没复位」的
// 中间态，进程此时崩溃的话，下次启动的恢复逻辑会把同一段时长再结算一次。
func (d *DB) FinishSession(sess SessionRow, idle TimerStateRow) (int64, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return 0, fmt.Errorf("开启结算事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(
		`INSERT INTO sessions(started_at, ended_at, duration_sec, note, source, created_at)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		sess.StartedAt, sess.EndedAt, sess.DurationSec, sess.Note, sess.Source, sess.CreatedAt,
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
		    SET state = ?, started_at = ?, last_resumed_at = ?, accumulated_sec = ?
		  WHERE id = 1`,
		idle.State, idle.StartedAt, idle.LastResumedAt, idle.AccumulatedSec,
	); err != nil {
		return 0, fmt.Errorf("复位计时状态失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("提交结算事务失败: %w", err)
	}
	return id, nil
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
