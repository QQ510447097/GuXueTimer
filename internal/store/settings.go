package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// LoadSettings 读取全部键值设置。
func (d *DB) LoadSettings() (map[string]string, error) {
	rows, err := d.sql.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("查询设置失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("读取设置失败: %w", err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历设置失败: %w", err)
	}
	return out, nil
}

// SaveSettings 在一个事务里 upsert 一批设置。
func (d *DB) SaveSettings(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("开启设置事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if err != nil {
		return fmt.Errorf("准备设置写入失败: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for k, v := range values {
		if _, err := stmt.Exec(k, v); err != nil {
			return fmt.Errorf("写入设置 %q 失败: %w", k, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交设置事务失败: %w", err)
	}
	return nil
}

// GetSetting 读取单个设置，不存在时返回 ("", false, nil)。
func (d *DB) GetSetting(key string) (string, bool, error) {
	var v string
	err := d.sql.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("读取设置 %q 失败: %w", key, err)
	}
	return v, true, nil
}
