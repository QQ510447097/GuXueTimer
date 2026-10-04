package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"goxuetimer/internal/apppath"
)

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

// DataDir 返回数据目录的绝对路径，供设置页展示。
//
// 由后端给而不是让界面拼 %LOCALAPPDATA%：目录在解析时是逐级降级的
// （Wails 的 PathDataHome → %LOCALAPPDATA% → XDG → UserConfigDir），
// 界面自己拼出来的那个很可能不是真正在用的。
func (s *SettingsService) DataDir() (string, error) {
	dir, err := apppath.EnsureDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(dir)
}

// OpenDataDir 在资源管理器里打开数据目录。
//
// 用 explorer.exe 而不是 Wails 的 API：v3 beta.27 没有导出打开文件管理器的
// 方法，而这正是 Windows 上最少依赖的做法。explorer 即使目录不存在也会返回 0，
// 所以这里先自己确认一遍目录存在，好给用户一个明确的错误。
func (s *SettingsService) OpenDataDir() error {
	dir, err := s.DataDir()
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	return exec.Command("explorer.exe", dir).Start()
}

// Save 校验并保存设置，返回归一化之后的值。
//
// 返回值不是"回显"而是"结论"：如果界面传了越界值，这里返回被纠正后的结果，
// 对方据此刷新输入框，用户能立刻看到实际生效的值。
func (s *SettingsService) Save(in SettingsDTO) (SettingsDTO, error) {
	next := settingsFromDTO(in)

	// CurrentPlanID 由 PlansService.SetCurrent 掌管，不接受界面回传：
	// 设置页里根本没有这个字段，界面回传的必然是零值，直接采信会把
	// "当前计划"清掉——表现为用户一保存设置，计时页的计划就没了。
	next.CurrentPlanID = s.b.currentPlanID()

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
