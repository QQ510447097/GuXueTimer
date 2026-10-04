package store

import (
	"database/sql"
	"errors"
	"fmt"
)

const planColumns = `id, name, color, sort_order, archived, created_at`

// ListPlans 返回全部计划（**含已归档**）并附上今日 / 累计 / 场次统计。
//
// 一次 JOIN 拿全：计划管理页要显示每个计划的时长，按月历/柱状图又要按计划筛选，
// 分成"列表"和"统计"两次查询只会让调用方多写一遍关联逻辑。
//
// today 形如 "2006-01-02"，用来算"今日"那一列。
func (d *DB) ListPlans(today string) ([]PlanRow, error) {
	rows, err := d.sql.Query(
		`SELECT p.id, p.name, p.color, p.sort_order, p.archived, p.created_at,
		        COALESCE(SUM(CASE WHEN substr(s.started_at, 1, 10) = ?
		                          THEN s.duration_sec ELSE 0 END), 0) AS today_sec,
		        COALESCE(SUM(s.duration_sec), 0) AS total_sec,
		        COUNT(s.id) AS session_count
		   FROM plans p
		   LEFT JOIN sessions s ON s.plan_id = p.id
		  GROUP BY p.id
		  ORDER BY p.archived ASC, p.sort_order ASC, p.id ASC`,
		today,
	)
	if err != nil {
		return nil, fmt.Errorf("查询学习计划失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]PlanRow, 0, 8)
	for rows.Next() {
		var p PlanRow
		if err := rows.Scan(&p.ID, &p.Name, &p.Color, &p.SortOrder, &p.Archived,
			&p.CreatedAt, &p.TodaySec, &p.TotalSec, &p.Sessions); err != nil {
			return nil, fmt.Errorf("读取学习计划失败: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历学习计划失败: %w", err)
	}
	return out, nil
}

// GetPlan 读取单个计划（不含统计）。
func (d *DB) GetPlan(id int64) (PlanRow, error) {
	var p PlanRow
	err := d.sql.QueryRow(
		`SELECT `+planColumns+` FROM plans WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.Color, &p.SortOrder, &p.Archived, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PlanRow{}, ErrNotFound
	}
	if err != nil {
		return PlanRow{}, fmt.Errorf("读取学习计划失败: %w", err)
	}
	return p, nil
}

// InsertPlan 新建计划，排序号自动排在最后。
//
// 排序号在 INSERT 里用子查询算，避免"先查 max 再插入"之间的竞态
// ——虽然桌面端单进程，但少一次往返也少一处出错的地方。
func (d *DB) InsertPlan(name, color, createdAt string) (int64, error) {
	res, err := d.sql.Exec(
		`INSERT INTO plans(name, color, sort_order, archived, created_at)
		 VALUES(?, ?, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM plans), 0, ?)`,
		name, color, createdAt,
	)
	if err != nil {
		return 0, fmt.Errorf("新建学习计划失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取学习计划 ID 失败: %w", err)
	}
	return id, nil
}

// UpdatePlan 改名字与颜色。排序交给 ReorderPlans。
func (d *DB) UpdatePlan(id int64, name, color string) error {
	res, err := d.sql.Exec(
		`UPDATE plans SET name = ?, color = ? WHERE id = ?`, name, color, id,
	)
	if err != nil {
		return fmt.Errorf("更新学习计划失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("确认更新结果失败: %w", err)
	}
	if n == 0 {
		// 值完全没变时 RowsAffected 也可能是 0，所以要再确认行是否存在。
		if _, err := d.GetPlan(id); err != nil {
			return err
		}
	}
	return nil
}

// SetPlanArchived 归档 / 恢复一个计划。归档是"从选择器里藏起来"，历史记录不受影响。
func (d *DB) SetPlanArchived(id int64, archived bool) error {
	res, err := d.sql.Exec(
		`UPDATE plans SET archived = ? WHERE id = ?`, boolToInt(archived), id,
	)
	if err != nil {
		return fmt.Errorf("归档学习计划失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("确认归档结果失败: %w", err)
	}
	if n == 0 {
		if _, err := d.GetPlan(id); err != nil {
			return err
		}
	}
	return nil
}

// DeletePlan 物理删除。调用方必须先用 CountPlanSessions 确认没有引用。
func (d *DB) DeletePlan(id int64) error {
	res, err := d.sql.Exec(`DELETE FROM plans WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除学习计划失败: %w", err)
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

// CountPlanSessions 返回挂在该计划下的记录数。
func (d *DB) CountPlanSessions(id int64) (int64, error) {
	var n int64
	if err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE plan_id = ?`, id,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("统计计划下的记录数失败: %w", err)
	}
	return n, nil
}

// ActivePlanByName 按名字查未归档的计划，用于重名校验与"归档后可复用同名"。
func (d *DB) ActivePlanByName(name string) (int64, bool, error) {
	var id int64
	err := d.sql.QueryRow(
		`SELECT id FROM plans WHERE archived = 0 AND name = ? ORDER BY id LIMIT 1`, name,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("按名字查询学习计划失败: %w", err)
	}
	return id, true, nil
}

// FirstActivePlanID 返回排序最靠前的未归档计划。
//
// 第二个返回值为 false 表示一个都没有——这时应用应该要求用户先建计划。
func (d *DB) FirstActivePlanID() (int64, bool, error) {
	var id int64
	err := d.sql.QueryRow(
		`SELECT id FROM plans WHERE archived = 0 ORDER BY sort_order ASC, id ASC LIMIT 1`,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("查询默认学习计划失败: %w", err)
	}
	return id, true, nil
}

// ReorderPlans 按下标顺序重写排序号（一次事务）。
//
// ids 里没提到的计划保持原样：调用方只传它看得见的那部分也没关系。
func (d *DB) ReorderPlans(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return fmt.Errorf("开启排序事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`UPDATE plans SET sort_order = ? WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("准备排序语句失败: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i, id := range ids {
		if _, err := stmt.Exec(i, id); err != nil {
			return fmt.Errorf("更新计划 %d 的排序失败: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交排序事务失败: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
