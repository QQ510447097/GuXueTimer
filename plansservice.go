package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
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
	// ErrPlanDeleteRunning 在"删除正在计时的当前计划"时返回。
	//
	// 这一条和别的"顺手结算"不一样，是**故意拒绝**的：删除的前提是"这个计划没有任何
	// 记录"，而结算进行中的会话恰好会创建一条记录——先结算再删，必然删不掉，
	// 而且用户点击删除的本意也不是"结束这次计时"。它会平白停掉计时器、再报一句
	// "这个计划下有 1 条记录，不能删除"，两件事都出乎意料。
	//
	// 所以这里提前拦下，并且**不做任何状态改变**：计时继续跑，用户自己决定是先结束
	// 还是先切到别的计划。
	ErrPlanDeleteRunning = errors.New("正在这个计划上计时，请先结束本次会话或切换到别的计划")
)

// PlanSwitchDTO 是"切换当前计划"的结果。
//
// 它比单纯的设置多带一份"刚才顺手做了什么"：切换会自动把正在跑的会话结算掉，
// 用户需要知道那段时间记到哪去了。没有这份信息，界面只能干巴巴地说一句"已切换"，
// 而用户眼看着计时器停了、却不知道那 25 分钟去哪了。
type PlanSwitchDTO struct {
	Plan     PlanDTO     `json:"plan"`
	Settings SettingsDTO `json:"settings"`
	// Settled 表示这次切换顺手结算了一次进行中的会话。
	Settled bool `json:"settled"`
	// Saved 表示那次结算确实写进了记录（时长过短时不会写）。
	Saved bool `json:"saved"`
	// SettledSec 是结算下来的秒数。
	SettledSec int64 `json:"settledSec"`
	// SettledPlanName 是那段时间原来挂在哪个计划上。
	SettledPlanName string `json:"settledPlanName"`
}

// MaxPlanNameLen 是计划名称的长度上限（按字符数，不是字节数）。
const MaxPlanNameLen = 24

// PlansService 管理学习计划。
//
// switchMu 让"结算 + 切换"成为一个不可分割的动作。
//
// 不加锁也能跑（用户没法同时点两个计划，界面自己也有防重入），但"把一段时间记到
// 哪个计划上"这件事值得多一层保证：交错执行时可能出现 A 结算完、B 也结算（空转）、
// 两个切换按反序落地，最后当前计划与用户点的不一致。
type PlansService struct {
	b        *backend
	switchMu sync.Mutex
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
//
// 归档"当前计划"时先把进行中的会话结算掉：否则那段时长会挂在一个即将被藏起来的
// 计划上，用户再想找到它就得先恢复归档。
func (s *PlansService) SetArchived(id int64, archived bool) (PlanDTO, error) {
	s.switchMu.Lock()
	defer s.switchMu.Unlock()

	plan, err := s.b.db.GetPlan(id)
	if err != nil {
		return PlanDTO{}, err
	}
	if archived && plan.ID == s.b.currentPlanID() {
		if _, err := s.b.settleIfRunning(); err != nil {
			return PlanDTO{}, err
		}
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
//
// 正在这个计划上计时的情况提前拒绝，理由见 ErrPlanDeleteRunning。
func (s *PlansService) Delete(id int64) error {
	s.switchMu.Lock()
	defer s.switchMu.Unlock()

	if id == s.b.currentPlanID() && s.b.timerRunning() {
		return ErrPlanDeleteRunning
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

// SetCurrent 切换当前计划。
//
// 计时进行中也允许切换：先把这段会话结算掉（时长算给它原来所属的计划），再换过去。
//
// 早先的版本是"计时中拒绝切换"（ErrPlanRunning）。那个做法把"归属清晰"这件事
// 交给了用户的记性——他得先想起来手动结束一次、再切、再开始，多两步，还容易忘。
// 而现在用户的心智很直接：我想换到另一科，刚才那段时间当然还算在原来那科。
// 顺手结算既保住了归属，也少了两步操作。
//
// 结算失败时**不切换**：宁可这次切换没发生，也不能把用户的一段时间弄丢。
func (s *PlansService) SetCurrent(id int64) (PlanSwitchDTO, error) {
	s.switchMu.Lock()
	defer s.switchMu.Unlock()

	plan, err := s.b.db.GetPlan(id)
	if err != nil {
		return PlanSwitchDTO{}, err
	}
	if plan.Archived {
		return PlanSwitchDTO{}, errors.New("这个计划已归档，先恢复再切换")
	}

	out := PlanSwitchDTO{}

	// 点的是当前计划：什么都不做。这里必须早退——否则"点一下当前计划"
	// 会顺手把正在跑的会话结算掉，那是个不该发生的副作用。
	if id != s.b.currentPlanID() {
		settled, err := s.b.settleIfRunning()
		if err != nil {
			return PlanSwitchDTO{}, err
		}
		out.Settled = settled.Settled
		out.Saved = settled.Saved
		out.SettledSec = settled.Sec
		out.SettledPlanName = settled.PlanName

		if err := s.b.setCurrentPlan(id); err != nil {
			return PlanSwitchDTO{}, err
		}
	}

	s.b.emitPlansChanged("current")
	s.b.emitState()

	planDTO, err := s.planByID(id)
	if err != nil {
		return PlanSwitchDTO{}, err
	}
	out.Plan = planDTO
	out.Settings = settingsDTOFrom(s.b.Settings())
	return out, nil
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
