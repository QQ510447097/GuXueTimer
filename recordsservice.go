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

// ListByDay 返回某天（YYYY-MM-DD）的全部记录，按开始时间升序。
// date 传空串表示今天。
func (s *RecordsService) ListByDay(date string) ([]SessionDTO, error) {
	key, err := s.resolveDate(date)
	if err != nil {
		return nil, err
	}
	rows, err := s.b.db.SessionsByDay(key)
	if err != nil {
		return nil, err
	}
	// 显式构造非 nil 切片：nil 切片会被序列化成 null，
	// 前端的 .map 就会炸。
	out := make([]SessionDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionDTOFrom(row))
	}
	return out, nil
}

// TodaySummary 返回今天的汇总。
func (s *RecordsService) TodaySummary() (DaySummaryDTO, error) {
	key := s.b.clock().Format(layoutDateKey)
	sum, err := s.b.db.SummaryByDay(key)
	if err != nil {
		return DaySummaryDTO{}, err
	}
	return daySummaryDTOFrom(sum), nil
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
