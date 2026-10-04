package main

// SettingsService 读写用户设置。
type SettingsService struct {
	b *backend
}

// NewSettingsService 构造设置服务。
func NewSettingsService(b *backend) *SettingsService { return &SettingsService{b: b} }

// Get 返回当前设置。
func (s *SettingsService) Get() (SettingsDTO, error) {
	return settingsDTOFrom(s.b.Settings()), nil
}

// Save 校验并保存设置，返回归一化之后的值。
//
// 返回值不是"回显"而是"结论"：如果界面传了越界值，这里返回被纠正后的结果，
// 对方据此刷新输入框，用户能立刻看到实际生效的值。
func (s *SettingsService) Save(in SettingsDTO) (SettingsDTO, error) {
	next := settingsFromDTO(in)
	if err := next.Validate(); err != nil {
		return SettingsDTO{}, err
	}
	if err := s.b.updateSettings(next); err != nil {
		return SettingsDTO{}, err
	}
	// 提醒的开关/间隔变了，进度要同步给界面。
	s.b.emitState()
	return settingsDTOFrom(s.b.Settings()), nil
}
