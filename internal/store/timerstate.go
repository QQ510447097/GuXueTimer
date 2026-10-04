package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// LoadTimerState 读取 timer_state 单行表。
func (d *DB) LoadTimerState() (TimerStateRow, error) {
	var s TimerStateRow
	err := d.sql.QueryRow(
		`SELECT state, started_at, last_resumed_at, accumulated_sec
		   FROM timer_state WHERE id = 1`,
	).Scan(&s.State, &s.StartedAt, &s.LastResumedAt, &s.AccumulatedSec)
	if errors.Is(err, sql.ErrNoRows) {
		// 迁移里已经种过这一行，走到这里说明库被外部改坏了。
		return TimerStateRow{}, fmt.Errorf("计时状态缺失: %w", ErrNotFound)
	}
	if err != nil {
		return TimerStateRow{}, fmt.Errorf("读取计时状态失败: %w", err)
	}
	return s, nil
}

// SaveTimerState 覆盖 timer_state 单行表。
func (d *DB) SaveTimerState(s TimerStateRow) error {
	res, err := d.sql.Exec(
		`UPDATE timer_state
		    SET state = ?, started_at = ?, last_resumed_at = ?, accumulated_sec = ?
		  WHERE id = 1`,
		s.State, s.StartedAt, s.LastResumedAt, s.AccumulatedSec,
	)
	if err != nil {
		return fmt.Errorf("保存计时状态失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("确认计时状态保存结果失败: %w", err)
	}
	// 值完全没变时 RowsAffected 也可能是 0，所以这里只在确实缺行时报错。
	if n == 0 {
		var exists int
		checkErr := d.sql.QueryRow(`SELECT 1 FROM timer_state WHERE id = 1`).Scan(&exists)
		if errors.Is(checkErr, sql.ErrNoRows) {
			return fmt.Errorf("计时状态行缺失: %w", ErrNotFound)
		}
		if checkErr != nil {
			return fmt.Errorf("校验计时状态行失败: %w", checkErr)
		}
	}
	return nil
}

// TimestampOf 是 timer_state 没有 updated_at 时的补充写入口，
// 单独暴露一个方法以免 SaveTimerState 的签名被诊断字段污染。
func (d *DB) TouchTimerState(updatedAt string) error {
	_, err := d.sql.Exec(`UPDATE timer_state SET updated_at = ? WHERE id = 1`, updatedAt)
	if err != nil {
		return fmt.Errorf("更新时间戳失败: %w", err)
	}
	return nil
}
