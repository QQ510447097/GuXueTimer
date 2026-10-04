package main

import (
	"fmt"
	"time"
)

// RecordsService 提供学习记录的只读查询与单条删除。
//
// 有意不做补录与编辑：v1 的范围只有"记录 + 纠错"。
type RecordsService struct {
	b *backend
}

// NewRecordsService 构造记录服务。
func NewRecordsService(b *backend) *RecordsService { return &RecordsService{b: b} }

// ListByDay 返回某天（YYYY-MM-DD）的记录，按开始时间升序。
// date 传空串表示今天；planID 传 0 表示不按计划筛选。
func (s *RecordsService) ListByDay(date string, planID int64) ([]SessionDTO, error) {
	key, err := s.resolveDate(date)
	if err != nil {
		return nil, err
	}
	rows, err := s.b.db.SessionsByDay(key, planID)
	if err != nil {
		return nil, err
	}
	// 显式构造非 nil 切片：nil 切片会被序列化成 null，前端的 .map 就会炸。
	out := make([]SessionDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionDTOFrom(row))
	}
	return out, nil
}

// DaySummary 返回某天的汇总。date 传空串表示今天，planID 传 0 表示全部计划。
//
// 取代了原来的 TodaySummary：计时页要同时显示"今日全部"和"本计划今日"，
// 同一个方法用不同 planID 调两次，比维护两个方法干净。
func (s *RecordsService) DaySummary(date string, planID int64) (DaySummaryDTO, error) {
	key, err := s.resolveDate(date)
	if err != nil {
		return DaySummaryDTO{}, err
	}
	sum, err := s.b.db.SummaryByDay(key, planID)
	if err != nil {
		return DaySummaryDTO{}, err
	}
	return daySummaryDTOFrom(sum), nil
}

// DailyTotals 返回 [from, to] 闭区间内每天的合计，柱状图与月历热力图共用。
//
// 只返回有记录的日子；补零由前端按自己的区间做，比让 SQL 造一串日期干净。
func (s *RecordsService) DailyTotals(from string, to string, planID int64) ([]DailyTotalDTO, error) {
	fromKey, err := s.resolveDate(from)
	if err != nil {
		return nil, err
	}
	toKey, err := s.resolveDate(to)
	if err != nil {
		return nil, err
	}
	if fromKey > toKey {
		return nil, fmt.Errorf("起始日期 %s 晚于结束日期 %s", fromKey, toKey)
	}
	rows, err := s.b.db.DailyTotals(fromKey, toKey, planID)
	if err != nil {
		return nil, err
	}
	out := make([]DailyTotalDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, dailyTotalDTOFrom(row))
	}
	return out, nil
}

// DeleteSession 删除一条记录。
func (s *RecordsService) DeleteSession(id int64) error {
	if id <= 0 {
		return fmt.Errorf("无效的记录 ID: %d", id)
	}
	if err := s.b.db.DeleteSession(id); err != nil {
		return err
	}
	s.b.emitRecordsChanged("deleted")
	return nil
}

// resolveDate 校验并归一化日期参数，空串视为今天。
//
// 用 2006-01-02 布局解析是严格的：不接受 "2026-5-1" 这种非零填充写法，
// 免得前端传错格式却静默返回空列表。
func (s *RecordsService) resolveDate(date string) (string, error) {
	if date == "" {
		return s.b.clock().Format(layoutDateKey), nil
	}
	parsed, err := time.ParseInLocation(layoutDateKey, date, time.Local)
	if err != nil {
		return "", fmt.Errorf("日期格式应为 YYYY-MM-DD，收到 %q", date)
	}
	return parsed.Format(layoutDateKey), nil
}
