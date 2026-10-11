package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// ProgressRow 是"等级与经验"的持久化状态（progress 单行表）。
//
// 这一行是**账**，不是推导结果：同一小时在拿到奖章前后值多少经验不同，
// 而规则以后还会调，所以已发放的值必须留下来，规则变化只影响之后。
// 所有字段都只增不减（Level 是高水位，奖章数同理）。
type ProgressRow struct {
	// XpTenths 是累计经验，单位是**十分之一经验**。
	//
	// 用整数而不是浮点：成长倍率是 1 + 0.1W + 0.2M + 0.3Q + 1.2Y，
	// 0.1 在二进制浮点里存不准，拿它去比等级阈值（99.9999 还是 100）迟早出错。
	// 换算成十分之一就是 10 + W + 2M + 3Q + 12Y，全是整数。
	XpTenths int64
	// CarrySec 是没凑满一小时的零头，跨会话累计。
	CarrySec int64
	// Level 是等级高水位：等级本身由 XpTenths 按曲线算出，但存下来的这个只增不减，
	// 免得以后把曲线改陡时老用户掉级。
	Level int
	// 四类奖章数（累计发放量）。改门槛不会让已经拿到的奖章消失。
	MedalsWeek    int
	MedalsMonth   int
	MedalsQuarter int
	MedalsYear    int
}

// AchievementStateRow 是一个成就的持久化状态。
type AchievementStateRow struct {
	ID string
	// Count 是达成次数（循环=发过几轮；每日=达标过几天）。
	Count int
	// LastAt 是上次达成日期（YYYY-MM-DD）；从未达成为空串。
	LastAt string
	// AchievedAt 是单次成就的达成时刻（RFC3339 或日期）；未达成为空串。
	AchievedAt string
}

// QualifyingDay 是一个"达标日"（当天累计学习满 1 小时）。
type QualifyingDay struct {
	Date string
	Sec  int64
}

// LoadProgress 读取进度单行。行不存在时返回零值（迁移会种下它，所以正常不会缺）。
func (d *DB) LoadProgress() (ProgressRow, error) {
	var out ProgressRow
	err := d.sql.QueryRow(
		`SELECT xp_tenths, carry_sec, level, medals_week, medals_month, medals_quarter, medals_year
		   FROM progress WHERE id = 1`,
	).Scan(
		&out.XpTenths, &out.CarrySec, &out.Level,
		&out.MedalsWeek, &out.MedalsMonth, &out.MedalsQuarter, &out.MedalsYear,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ProgressRow{Level: 1}, nil
	}
	if err != nil {
		return ProgressRow{}, fmt.Errorf("读取进度失败: %w", err)
	}
	return out, nil
}

// SaveProgress 覆盖写进度单行（写之前调用方已经把新值算好）。
func (d *DB) SaveProgress(p ProgressRow, updatedAt string) error {
	_, err := d.sql.Exec(
		`UPDATE progress
		    SET xp_tenths = ?, carry_sec = ?, level = ?,
		        medals_week = ?, medals_month = ?, medals_quarter = ?, medals_year = ?,
		        updated_at = ?
		  WHERE id = 1`,
		p.XpTenths, p.CarrySec, p.Level,
		p.MedalsWeek, p.MedalsMonth, p.MedalsQuarter, p.MedalsYear,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("写入进度失败: %w", err)
	}
	return nil
}

// QualifyingDayExists 判断某天是否已经记进达标账本。
func (d *DB) QualifyingDayExists(date string) (bool, error) {
	var one int
	err := d.sql.QueryRow(`SELECT 1 FROM qualifying_days WHERE date = ?`, date).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("查询达标日失败: %w", err)
	}
	return true, nil
}

// AddQualifyingDay 记下一个达标日。重复记同一天是幂等的（IGNORE）。
func (d *DB) AddQualifyingDay(date string, sec int64) error {
	if _, err := d.sql.Exec(
		`INSERT OR IGNORE INTO qualifying_days(date, sec) VALUES(?, ?)`, date, sec,
	); err != nil {
		return fmt.Errorf("记录达标日失败: %w", err)
	}
	return nil
}

// CountQualifyingDays 返回达标日总数：循环成就的"已达成次数"与余数都由它算。
func (d *DB) CountQualifyingDays() (int, error) {
	var n int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM qualifying_days`).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计达标日失败: %w", err)
	}
	return n, nil
}

// QualifyingDays 返回全部达标日，按日期升序。
func (d *DB) QualifyingDays() ([]QualifyingDay, error) {
	rows, err := d.sql.Query(`SELECT date, sec FROM qualifying_days ORDER BY date ASC`)
	if err != nil {
		return nil, fmt.Errorf("读取达标日失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []QualifyingDay
	for rows.Next() {
		var item QualifyingDay
		if err := rows.Scan(&item.Date, &item.Sec); err != nil {
			return nil, fmt.Errorf("读取达标日失败: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历达标日失败: %w", err)
	}
	return out, nil
}

// LoadAchievementStates 读出全部成就状态，按 id 索引。
func (d *DB) LoadAchievementStates() (map[string]AchievementStateRow, error) {
	rows, err := d.sql.Query(`SELECT id, count, last_at, achieved_at FROM achievement_state`)
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

// SaveAchievementState 覆盖写一个成就的状态。
func (d *DB) SaveAchievementState(s AchievementStateRow) error {
	if _, err := d.sql.Exec(
		`INSERT INTO achievement_state(id, count, last_at, achieved_at) VALUES(?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET count = excluded.count,
		                               last_at = excluded.last_at,
		                               achieved_at = excluded.achieved_at`,
		s.ID, s.Count, s.LastAt, s.AchievedAt,
	); err != nil {
		return fmt.Errorf("写入成就状态失败: %w", err)
	}
	return nil
}

// ReplaceProgress 用一整份算好的进度覆盖三张表，在**一个事务**里完成。
//
// 只有一个正当用途：迁移时按历史记录回放一次算出初值（以及测试里构造现场）。
// 平时的进度只走 FinishSession 的增量路径，不要用它去"重算"——
// 重算会把规则改动套到整段历史上，而那正是我们把进度存下来的原因。
func (d *DB) ReplaceProgress(p ProgressRow, days []QualifyingDay, states []AchievementStateRow, updatedAt string) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("开启进度覆盖事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`UPDATE progress
		    SET xp_tenths = ?, carry_sec = ?, level = ?,
		        medals_week = ?, medals_month = ?, medals_quarter = ?, medals_year = ?,
		        updated_at = ?
		  WHERE id = 1`,
		p.XpTenths, p.CarrySec, p.Level,
		p.MedalsWeek, p.MedalsMonth, p.MedalsQuarter, p.MedalsYear,
		updatedAt,
	); err != nil {
		return fmt.Errorf("覆盖进度失败: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM qualifying_days`); err != nil {
		return fmt.Errorf("清空达标日失败: %w", err)
	}
	for _, day := range days {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO qualifying_days(date, sec) VALUES(?, ?)`, day.Date, day.Sec,
		); err != nil {
			return fmt.Errorf("回填达标日失败: %w", err)
		}
	}

	if _, err := tx.Exec(`DELETE FROM achievement_state`); err != nil {
		return fmt.Errorf("清空成就状态失败: %w", err)
	}
	for _, st := range states {
		if _, err := tx.Exec(
			`INSERT INTO achievement_state(id, count, last_at, achieved_at) VALUES(?, ?, ?, ?)`,
			st.ID, st.Count, st.LastAt, st.AchievedAt,
		); err != nil {
			return fmt.Errorf("回填成就状态失败: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交进度覆盖事务失败: %w", err)
	}
	return nil
}

// CountSessions 返回记录总数（迁移回填时判空用）。
func (d *DB) CountSessions() (int, error) {
	var n int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计记录数失败: %w", err)
	}
	return n, nil
}

// FirstPlanCreatedAt 返回最早那个计划的创建时间；一个计划都没有时返回空串。
//
// 用途只有一个：「千里之行」这条单次成就的达成时间。老库里没有
// achievement_state 记录时用它回退，不用迁移。
func (d *DB) FirstPlanCreatedAt() (string, error) {
	var at string
	err := d.sql.QueryRow(`SELECT COALESCE(MIN(created_at), '') FROM plans`).Scan(&at)
	if err != nil {
		return "", fmt.Errorf("查询最早的创建时间失败: %w", err)
	}
	return at, nil
}

// LastEndedAtByDay 返回每天最后一条记录的结束时刻（RFC3339），按日期索引。
//
// 只给"回填达成时刻"用：老库升级时回放看到的是按天汇总的时长，本身没有时刻，
// 于是用这天最后一条记录的结束时间近似"当天满 1 小时的那一刻"。
// 增量路径（结束并保存时结算）不用它——那里有精确的保存时刻。
func (d *DB) LastEndedAtByDay() (map[string]string, error) {
	rows, err := d.sql.Query(
		`SELECT substr(started_at, 1, 10) AS day, MAX(ended_at)
		   FROM sessions
		  GROUP BY day`,
	)
	if err != nil {
		return nil, fmt.Errorf("统计每天的结束时刻失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var day, ended string
		if err := rows.Scan(&day, &ended); err != nil {
			return nil, fmt.Errorf("读取每天的结束时刻失败: %w", err)
		}
		out[day] = ended
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历每天的结束时刻失败: %w", err)
	}
	return out, nil
}
