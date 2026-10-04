package main

import "goxuetimer/internal/timer"

// TimerService 是暴露给前端的计时服务。
//
// 它只是 backend 的薄封装：状态的唯一权威是 internal/timer.Service，
// 这里负责把结果翻译成 DTO。之所以放在 main 包里，是因为 Wails 的绑定生成器
// 以 main 包的服务注册为入口，放在这里最稳。
type TimerService struct {
	b *backend
}

// NewTimerService 构造计时服务。
func NewTimerService(b *backend) *TimerService { return &TimerService{b: b} }

// GetState 返回当前状态。界面启动时先调它拉一次初始值。
func (s *TimerService) GetState() (TimerStateDTO, error) {
	s.b.noteFrontendConnected()
	return s.b.stateDTO()
}

// Start 开始一次新的学习会话。
//
// 归属的计划由后端解析，界面不传：这样托盘的「开始」也不需要先问界面
// 当前选的是哪个计划，只有一处真相。
func (s *TimerService) Start() (TimerStateDTO, error) {
	planID, err := s.b.activePlanID()
	if err != nil {
		return TimerStateDTO{}, err
	}
	if _, err := s.b.timer.Start(planID); err != nil {
		return TimerStateDTO{}, err
	}
	return s.b.stateDTO()
}

// Pause 暂停计时。
func (s *TimerService) Pause() (TimerStateDTO, error) {
	if _, err := s.b.timer.Pause(); err != nil {
		return TimerStateDTO{}, err
	}
	return s.b.stateDTO()
}

// Resume 从暂停恢复。
func (s *TimerService) Resume() (TimerStateDTO, error) {
	if _, err := s.b.timer.Resume(); err != nil {
		return TimerStateDTO{}, err
	}
	return s.b.stateDTO()
}

// Stop 结束会话并保存记录。时长不足 5 秒时会返回 Saved=false 而不是报错。
func (s *TimerService) Stop() (StopResultDTO, error) {
	out, err := s.b.timer.Stop()
	if err != nil {
		return StopResultDTO{}, err
	}
	if out.Saved {
		s.b.emitRecordsChanged("stopped")
	}
	return stopResultDTOFrom(out), nil
}

// Discard 放弃当前会话，不留下任何记录。
func (s *TimerService) Discard() (TimerStateDTO, error) {
	if _, err := s.b.timer.Discard(); err != nil {
		return TimerStateDTO{}, err
	}
	s.b.emitRecordsChanged("discarded")
	return s.b.stateDTO()
}

func stopResultDTOFrom(out timer.StopResult) StopResultDTO {
	dto := StopResultDTO{
		Saved:       out.Saved,
		Reason:      out.Reason,
		DurationSec: out.DurationSec,
	}
	if out.Session != nil {
		session := sessionDTOFrom(*out.Session)
		dto.Session = &session
	}
	return dto
}
