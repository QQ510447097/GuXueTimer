package main

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"goxuetimer/internal/store"
)

// 计划相关的错误。文案直接给用户看，所以写成完整的中文句子。
var (
	// ErrNoPlan 在"还没有任何计划就想开始计时"时返回。
	ErrNoPlan = errors.New("请先创建一个计划")
	// ErrPlanNameEmpty 名称去空白后为空。
	ErrPlanNameEmpty = errors.New("计划名称不能为空")
	// ErrPlanNameTooLong 名称超过 MaxPlanNameLen 个字符。
	ErrPlanNameTooLong = fmt.Errorf("计划名称最多 %d 个字符", MaxPlanNameLen)
	// ErrPlanNameTaken 未归档计划里已经有同名。
	ErrPlanNameTaken = errors.New("已经有一个同名的计划了")
	// ErrPlanColorInvalid 颜色不在白名单里。
	ErrPlanColorInvalid = errors.New("这个颜色不在可选范围内")
	// ErrPlanRunning 计时进行中，不允许改动当前计划。
	ErrPlanRunning = errors.New("计时进行中，请先结束本次会话")
)

// MaxPlanNameLen 是计划名称的长度上限（按字符数，不是字节数）。
const MaxPlanNameLen = 24

// PlansService 管理学习计划。
type PlansService struct {
	b *backend
}

// NewPlansService 构造计划服务。
func NewPlansService(b *backend) *PlansService { return &PlansService{b: b} }

// Palette 返回可选配色的白名单。
//
// 由后端提供而不是前端硬编码：颜色是要被校验的，两处各写一份必然漂移。
func (s *PlansService) Palette() []string {
	out := make([]string, len(planPalette))
	copy(out, planPalette)
	return out
}

// List 返回计划列表（含统计）。includeArchived 为 false 时只返回在用的。
func (s *PlansService) List(includeArchived bool) ([]PlanDTO, error) {
	rows, err := s.b.db.ListPlans(s.b.clock().Format(layoutDateKey))
	if err != nil {
		return nil, err
	}
	out := make([]PlanDTO, 0, len(rows))
	for _, row := range rows {
		if row.Archived && !includeArchived {
			continue
		}
		out = append(out, planDTOFrom(row))
	}
	return out, nil
}

// Create 新建计划。第一个计划会自动成为当前计划。
func (s *PlansService) Create(name string, color string) (PlanDTO, error) {
	clean, err := normalizePlanName(name)
	if err != nil {
		return PlanDTO{}, err
	}
	if !isPlanColor(color) {
		return PlanDTO{}, ErrPlanColorInvalid
	}
	if id, taken, err := s.b.db.ActivePlanByName(clean); err != nil {
		return PlanDTO{}, err
	} else if taken {
		_ = id
		return PlanDTO{}, ErrPlanNameTaken
	}
	id, err := s.b.db.InsertPlan(clean, canonicalPlanColor(color), s.b.clock().Format(layoutRecord))
	if err != nil {
		return PlanDTO{}, err
	}

	// 第一个计划直接设为当前，省掉用户再点一次"设为当前"。
	if s.b.currentPlanID() <= 0 {
		if err := s.b.setCurrentPlan(id); err != nil {
			return PlanDTO{}, err
		}
	}
	if err := s.b.repairCurrentPlan(); err != nil {
		return PlanDTO{}, err
	}
	s.b.emitPlansChanged("created")
	s.b.emitState()
	return s.planByID(id)
}

// Update 改名字与颜色。
func (s *PlansService) Update(id int64, name string, color string) (PlanDTO, error) {
	clean, err := normalizePlanName(name)
	if err != nil {
		return PlanDTO{}, err
	}
	if !isPlanColor(color) {
		return PlanDTO{}, ErrPlanColorInvalid
	}
	// 重名检查要排除自己，否则"只改颜色不改名"会把自己判成重名。
	if other, taken, err := s.b.db.ActivePlanByName(clean); err != nil {
		return PlanDTO{}, err
	} else if taken && other != id {
		return PlanDTO{}, ErrPlanNameTaken
	}
	if err := s.b.db.UpdatePlan(id, clean, canonicalPlanColor(color)); err != nil {
		return PlanDTO{}, err
	}
	s.b.emitPlansChanged("updated")
	s.b.emitState()
	return s.planByID(id)
}

// SetArchived 归档 / 恢复。归档只是从选择器里藏起来，历史记录不受影响。
func (s *PlansService) SetArchived(id int64, archived bool) (PlanDTO, error) {
	plan, err := s.b.db.GetPlan(id)
	if err != nil {
		return PlanDTO{}, err
	}
	if archived && plan.ID == s.b.currentPlanID() && s.b.timerRunning() {
		return PlanDTO{}, ErrPlanRunning
	}
	if err := s.b.db.SetPlanArchived(id, archived); err != nil {
		return PlanDTO{}, err
	}
	if err := s.b.repairCurrentPlan(); err != nil {
		return PlanDTO{}, err
	}
	s.b.emitPlansChanged("archived")
	s.b.emitState()
	return s.planByID(id)
}

// Delete 物理删除。有记录的计划拒绝删除——请改成归档。
func (s *PlansService) Delete(id int64) error {
	if id == s.b.currentPlanID() && s.b.timerRunning() {
		return ErrPlanRunning
	}
	n, err := s.b.db.CountPlanSessions(id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("这个计划下有 %d 条记录，不能删除；请改成「归档」", n)
	}
	if err := s.b.db.DeletePlan(id); err != nil {
		return err
	}
	if err := s.b.repairCurrentPlan(); err != nil {
		return err
	}
	s.b.emitPlansChanged("deleted")
	s.b.emitState()
	return nil
}

// SetCurrent 切换当前计划。计时进行中不允许切换：否则"这段时长算谁的"会在
// 用户没察觉的时候改变。
func (s *PlansService) SetCurrent(id int64) (SettingsDTO, error) {
	plan, err := s.b.db.GetPlan(id)
	if err != nil {
		return SettingsDTO{}, err
	}
	if plan.Archived {
		return SettingsDTO{}, errors.New("这个计划已归档，先恢复再切换")
	}
	if s.b.timerRunning() && id != s.b.currentPlanID() {
		return SettingsDTO{}, ErrPlanRunning
	}
	if err := s.b.setCurrentPlan(id); err != nil {
		return SettingsDTO{}, err
	}
	s.b.emitPlansChanged("current")
	s.b.emitState()
	return settingsDTOFrom(s.b.Settings()), nil
}

// Reorder 按传入顺序重写排序。
func (s *PlansService) Reorder(ids []int64) error {
	if err := s.b.db.ReorderPlans(ids); err != nil {
		return err
	}
	s.b.emitPlansChanged("reordered")
	return nil
}

// planByID 重新读一条计划（带上统计），用于把删改结果回给界面。
func (s *PlansService) planByID(id int64) (PlanDTO, error) {
	rows, err := s.b.db.ListPlans(s.b.clock().Format(layoutDateKey))
	if err != nil {
		return PlanDTO{}, err
	}
	for _, row := range rows {
		if row.ID == id {
			return planDTOFrom(row), nil
		}
	}
	return PlanDTO{}, store.ErrNotFound
}

// normalizePlanName 去空白并校验长度与字符。
//
// 换行与控制字符直接拒掉：名字会出现在列表、日历图例和托盘提示里，
// 混进换行会把那些地方的排版弄乱。
func normalizePlanName(name string) (string, error) {
	clean := strings.TrimSpace(name)
	if clean == "" {
		return "", ErrPlanNameEmpty
	}
	if utf8.RuneCountInString(clean) > MaxPlanNameLen {
		return "", ErrPlanNameTooLong
	}
	for _, r := range clean {
		if unicode.IsControl(r) {
			return "", errors.New("计划名称不能包含换行或控制字符")
		}
	}
	return clean, nil
}
