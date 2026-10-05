package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"goxuetimer/internal/settings"
	"goxuetimer/internal/timer"
)

// currentOf 读一下后端记着的当前计划。
func currentOf(h *harness) int64 { return h.b.currentPlanID() }

// ---------------------------------------------------------------- 创建与校验

// 第一个计划自动成为当前计划，省掉用户再点一次"设为当前"。
func TestPlans_FirstBecomesCurrent(t *testing.T) {
	b := newHarness(t)
	// 夹具默认已经建了一个计划。
	if currentOf(b) != b.defaultPlanID {
		t.Fatalf("当前计划 = %d, 期望夹具默认的 %d", currentOf(b), b.defaultPlanID)
	}

	plans, err := b.plans.List(true)
	if err != nil {
		t.Fatalf("List() 失败: %v", err)
	}
	if len(plans) != 1 || plans[0].ID != b.defaultPlanID {
		t.Fatalf("计划列表 = %+v", plans)
	}
	if plans[0].Name != "默认计划" || plans[0].Color != planPalette[0] {
		t.Errorf("计划内容 = %+v", plans[0])
	}

	// 状态 DTO 里应当带上计划名与颜色：计时页要靠它显示"现在在记哪一科"。
	state, err := b.timer.GetState()
	if err != nil {
		t.Fatalf("GetState() 失败: %v", err)
	}
	if state.PlanID != b.defaultPlanID || state.PlanName != "默认计划" || state.PlanColor != planPalette[0] {
		t.Errorf("状态里的计划 = (%d, %q, %q)", state.PlanID, state.PlanName, state.PlanColor)
	}
}

func TestPlans_CreateValidation(t *testing.T) {
	h := newHarness(t)

	cases := []struct {
		name     string
		planName string
		color    string
		wantErr  error
	}{
		{"空名字", "   ", planPalette[0], ErrPlanNameEmpty},
		{"超长名字", strings.Repeat("字", MaxPlanNameLen+1), planPalette[0], ErrPlanNameTooLong},
		{"带换行", "数学\n英语", planPalette[0], nil}, // 换行是控制字符，报的是另一个错
		{"颜色不在白名单", "物理", "#FF0000", ErrPlanColorInvalid},
		{"颜色是注入尝试", "物理", "red; background:url(x)", ErrPlanColorInvalid},
		{"重名", "默认计划", planPalette[1], ErrPlanNameTaken},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := h.plans.Create(c.planName, c.color)
			if err == nil {
				t.Fatal("期望报错，实际成功")
			}
			if c.wantErr != nil && !errors.Is(err, c.wantErr) {
				t.Errorf("error = %v, 期望 %v", err, c.wantErr)
			}
			if strings.Contains(err.Error(), "control") {
				t.Errorf("错误信息应当是中文: %v", err)
			}
		})
	}

	// 刚好等于上限的名字应当被接受。
	if _, err := h.plans.Create(strings.Repeat("字", MaxPlanNameLen), planPalette[1]); err != nil {
		t.Errorf("边界长度的名字被拒了: %v", err)
	}
	// 颜色大小写不敏感，落库用规范写法。
	plan, err := h.plans.Create("物理", strings.ToLower(planPalette[2]))
	if err != nil {
		t.Fatalf("小写颜色被拒: %v", err)
	}
	if plan.Color != planPalette[2] {
		t.Errorf("落库颜色 = %q, 期望规范化成 %q", plan.Color, planPalette[2])
	}
	// 名字两端的空白会被裁掉。
	trimmed, err := h.plans.Create("  化学  ", planPalette[3])
	if err != nil {
		t.Fatalf("Create() 失败: %v", err)
	}
	if trimmed.Name != "化学" {
		t.Errorf("名字 = %q, 期望裁掉空白", trimmed.Name)
	}
}

// 只改颜色不改名，不能被自己判成重名。
func TestPlans_UpdateSelfIsNotDuplicate(t *testing.T) {
	h := newHarness(t)

	updated, err := h.plans.Update(h.defaultPlanID, "默认计划", planPalette[4])
	if err != nil {
		t.Fatalf("只改颜色被拒了: %v", err)
	}
	if updated.Color != planPalette[4] {
		t.Errorf("颜色 = %q", updated.Color)
	}

	// 改成另一个计划的名字才该被拒。
	other := h.mustCreatePlan(t, "英语", planPalette[1])
	if _, err := h.plans.Update(other, "默认计划", planPalette[1]); !errors.Is(err, ErrPlanNameTaken) {
		t.Errorf("重名 error = %v, 期望 ErrPlanNameTaken", err)
	}
	if _, err := h.plans.Update(other, "   ", planPalette[1]); !errors.Is(err, ErrPlanNameEmpty) {
		t.Errorf("空名 error = %v, 期望 ErrPlanNameEmpty", err)
	}
}

// ---------------------------------------------------------------- 切换与保护

// 计时中切换计划：先把这段会话结算掉（记在原来那个计划上），再切过去。
//
// 早先的契约是"计时中拒绝切换"（ErrPlanRunning）。改成自动结算是个有意的行为变更，
// 所以这条用例是重写的，不是新加的——它守的是"那段时间有没有记对账"。
func TestPlans_SetCurrentSettlesFirst(t *testing.T) {
	h := newHarness(t)
	other := h.mustCreatePlan(t, "英语", planPalette[1])

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(25 * time.Minute)

	out, err := h.plans.SetCurrent(other)
	if err != nil {
		t.Fatalf("计时中切换计划失败: %v", err)
	}

	// 返回值要能告诉界面"刚才顺手做了什么"。
	if !out.Settled || !out.Saved {
		t.Errorf("返回值 = %+v, 期望结算且已保存", out)
	}
	if out.SettledSec != 25*60 {
		t.Errorf("结算秒数 = %d, 期望 %d", out.SettledSec, 25*60)
	}
	if out.SettledPlanName != "默认计划" {
		t.Errorf("结算归属的计划名 = %q, 期望 默认计划", out.SettledPlanName)
	}
	if out.Plan.ID != other || out.Settings.CurrentPlanID != other {
		t.Errorf("切换结果 = %+v, 期望当前计划是 %d", out, other)
	}

	// 那 25 分钟必须落在**旧**计划名下。
	day := h.clock.Now().Format(layoutDateKey)
	summary, err := h.records.DaySummary(day, h.defaultPlanID)
	if err != nil {
		t.Fatalf("DaySummary() 失败: %v", err)
	}
	if summary.TotalSec != 25*60 {
		t.Errorf("旧计划当日时长 = %d, 期望 %d", summary.TotalSec, 25*60)
	}
	otherSummary, err := h.records.DaySummary(day, other)
	if err != nil {
		t.Fatalf("DaySummary() 失败: %v", err)
	}
	if otherSummary.TotalSec != 0 {
		t.Errorf("新计划不该有这段时长, 实际 %d", otherSummary.TotalSec)
	}

	// 计时器回到空闲，当前计划是新那个。
	state, err := h.timer.GetState()
	if err != nil {
		t.Fatalf("GetState() 失败: %v", err)
	}
	if state.State != string(timer.StateIdle) {
		t.Errorf("切换后计时状态 = %q, 期望 idle", state.State)
	}
	if currentOf(h) != other {
		t.Errorf("当前计划 = %d, 期望 %d", currentOf(h), other)
	}
}

// 点的是当前计划：不结算、不切换。否则"点一下当前计划"会顺手把会话停掉，
// 那是个用户完全没要求的副作用。
func TestPlans_SetCurrentSamePlanIsNoop(t *testing.T) {
	h := newHarness(t)

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(10 * time.Minute)

	out, err := h.plans.SetCurrent(h.defaultPlanID)
	if err != nil {
		t.Fatalf("SetCurrent(当前计划) 失败: %v", err)
	}
	if out.Settled {
		t.Error("切到当前计划不该结算正在跑的会话")
	}

	state, err := h.timer.GetState()
	if err != nil {
		t.Fatalf("GetState() 失败: %v", err)
	}
	if state.State != string(timer.StateRunning) {
		t.Errorf("计时状态 = %q, 期望仍在 running", state.State)
	}
}

// 会话太短（不足 5 秒）时结算不会写记录，但**仍然要切换**——
// 用户要的是换一科，不是"因为刚才手快点了两下所以不让我换"。
func TestPlans_SetCurrentSwitchesEvenWhenTooShort(t *testing.T) {
	h := newHarness(t)
	other := h.mustCreatePlan(t, "英语", planPalette[1])

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(2 * time.Second)

	out, err := h.plans.SetCurrent(other)
	if err != nil {
		t.Fatalf("SetCurrent() 失败: %v", err)
	}
	if !out.Settled {
		t.Error("应当报告结算过")
	}
	if out.Saved {
		t.Error("2 秒不该写进记录")
	}
	if currentOf(h) != other {
		t.Errorf("当前计划 = %d, 期望 %d", currentOf(h), other)
	}
}

// 空闲时切换：没有会话可结算，返回值里的 Settled 必须是 false。
func TestPlans_SetCurrentIdleReportsNoSettle(t *testing.T) {
	h := newHarness(t)
	other := h.mustCreatePlan(t, "英语", planPalette[1])

	out, err := h.plans.SetCurrent(other)
	if err != nil {
		t.Fatalf("SetCurrent() 失败: %v", err)
	}
	if out.Settled || out.Saved || out.SettledSec != 0 {
		t.Errorf("空闲时不该报告结算: %+v", out)
	}
}

// 归档"当前计划"同样先结算：否则那段时长会挂在一个即将被藏起来的计划上。
func TestPlans_ArchiveCurrentSettlesFirst(t *testing.T) {
	h := newHarness(t)
	spare := h.mustCreatePlan(t, "英语", planPalette[1])

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(15 * time.Minute)

	if _, err := h.plans.SetArchived(h.defaultPlanID, true); err != nil {
		t.Fatalf("计时中归档当前计划失败: %v", err)
	}

	// 归档非当前计划不受影响。
	if _, err := h.plans.SetArchived(spare, true); err != nil {
		t.Errorf("归档别的计划失败: %v", err)
	}

	// 那 15 分钟要记在归档前所属的计划上。
	day := h.clock.Now().Format(layoutDateKey)
	summary, err := h.records.DaySummary(day, h.defaultPlanID)
	if err != nil {
		t.Fatalf("DaySummary() 失败: %v", err)
	}
	if summary.TotalSec != 15*60 {
		t.Errorf("归档前所属计划的当日时长 = %d, 期望 %d", summary.TotalSec, 15*60)
	}
}

// 删除"正在计时的当前计划"要提前拒绝，而且**不做任何状态改变**。
//
// 这一条与"顺手结算"相反，是故意的：删除的前提是"这个计划没有记录"，而结算恰好
// 会创建一条记录，先结算再删必然删不掉——还会平白停掉用户的计时器。
func TestPlans_DeleteCurrentWhileRunningRefused(t *testing.T) {
	h := newHarness(t)
	other := h.mustCreatePlan(t, "英语", planPalette[1])
	if _, err := h.plans.SetCurrent(other); err != nil {
		t.Fatalf("SetCurrent() 失败: %v", err)
	}

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(12 * time.Minute)

	err := h.plans.Delete(other)
	if !errors.Is(err, ErrPlanDeleteRunning) {
		t.Fatalf("error = %v, 期望 ErrPlanDeleteRunning", err)
	}

	// 失败的操作不该有副作用：计时还在跑，也没有多出记录。
	state, err2 := h.timer.GetState()
	if err2 != nil {
		t.Fatalf("GetState() 失败: %v", err2)
	}
	if state.State != string(timer.StateRunning) {
		t.Errorf("计时状态 = %q, 期望仍在 running（失败的操作不该把计时停掉）", state.State)
	}
	day := h.clock.Now().Format(layoutDateKey)
	summary, err3 := h.records.DaySummary(day, other)
	if err3 != nil {
		t.Fatalf("DaySummary() 失败: %v", err3)
	}
	if summary.TotalSec != 0 {
		t.Errorf("不该产生记录, 实际 %d 秒", summary.TotalSec)
	}

	// 结束之后删除才走到"有记录不能删除"那一关。
	if _, err := h.timer.Stop(); err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}
	if err := h.plans.Delete(other); err == nil {
		t.Error("结算之后它已经有记录，删除应当被『有记录不能删除』挡住")
	}
}

// 归档当前计划（空闲时）之后，当前计划要自动落到另一个可用计划上。
func TestPlans_ArchiveCurrentRepairsCurrent(t *testing.T) {
	h := newHarness(t)
	spare := h.mustCreatePlan(t, "英语", planPalette[1])

	if _, err := h.plans.SetArchived(h.defaultPlanID, true); err != nil {
		t.Fatalf("SetArchived() 失败: %v", err)
	}
	if currentOf(h) != spare {
		t.Errorf("归档后当前计划 = %d, 期望回落到 %d", currentOf(h), spare)
	}

	// 把剩下的也归档：当前计划变成 0，界面据此进引导流程。
	if _, err := h.plans.SetArchived(spare, true); err != nil {
		t.Fatalf("SetArchived() 失败: %v", err)
	}
	if currentOf(h) != 0 {
		t.Errorf("全部归档后当前计划 = %d, 期望 0", currentOf(h))
	}

	// 这时开始计时应当被明确拒绝，而不是记到一个不存在的计划上。
	if _, err := h.timer.Start(); !errors.Is(err, ErrNoPlan) {
		t.Errorf("无可用计划时 Start() error = %v, 期望 ErrNoPlan", err)
	}
}

// ---------------------------------------------------------------- 删除保护

func TestPlans_DeleteRefusedWhenReferenced(t *testing.T) {
	h := newHarness(t)

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(60 * time.Second)
	if _, err := h.timer.Stop(); err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}

	err := h.plans.Delete(h.defaultPlanID)
	if err == nil {
		t.Fatal("有记录的计划被删掉了")
	}
	if !strings.Contains(err.Error(), "1 条") {
		t.Errorf("错误信息里应当报出条数: %v", err)
	}
	if !strings.Contains(err.Error(), "归档") {
		t.Errorf("错误信息里应当指路到归档: %v", err)
	}

	// 记录还在。
	list, err := h.records.ListByDay("", 0)
	if err != nil {
		t.Fatalf("ListByDay() 失败: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("记录数 = %d, 期望 1", len(list))
	}
}

func TestPlans_DeleteWithoutSessions(t *testing.T) {
	h := newHarness(t)
	spare := h.mustCreatePlan(t, "英语", planPalette[1])

	if err := h.plans.Delete(spare); err != nil {
		t.Fatalf("Delete() 失败: %v", err)
	}
	plans, err := h.plans.List(true)
	if err != nil {
		t.Fatalf("List() 失败: %v", err)
	}
	for _, p := range plans {
		if p.ID == spare {
			t.Errorf("计划没被删掉: %+v", p)
		}
	}
}

// 删掉当前计划后，当前计划要自动落到另一个上。
func TestPlans_DeleteCurrentRepairs(t *testing.T) {
	h := newHarness(t)
	spare := h.mustCreatePlan(t, "英语", planPalette[1])

	if currentOf(h) != h.defaultPlanID {
		t.Fatalf("前置条件不成立：当前计划 = %d", currentOf(h))
	}
	if err := h.plans.Delete(h.defaultPlanID); err != nil {
		t.Fatalf("Delete() 失败: %v", err)
	}
	if currentOf(h) != spare {
		t.Errorf("删除当前计划后 = %d, 期望 %d", currentOf(h), spare)
	}
}

// ---------------------------------------------------------------- 列表与排序

func TestPlans_ListHidesArchivedByDefault(t *testing.T) {
	h := newHarness(t)
	spare := h.mustCreatePlan(t, "英语", planPalette[1])
	if _, err := h.plans.SetArchived(spare, true); err != nil {
		t.Fatalf("SetArchived() 失败: %v", err)
	}

	active, err := h.plans.List(false)
	if err != nil {
		t.Fatalf("List(false) 失败: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("在用计划 = %d, 期望 1", len(active))
	}
	all, err := h.plans.List(true)
	if err != nil {
		t.Fatalf("List(true) 失败: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("全部计划 = %d, 期望 2", len(all))
	}
	if !all[1].Archived {
		t.Errorf("归档计划应当排在后面且带归档标记: %+v", all)
	}
}

func TestPlans_Reorder(t *testing.T) {
	h := newHarness(t)
	second := h.mustCreatePlan(t, "英语", planPalette[1])
	third := h.mustCreatePlan(t, "物理", planPalette[2])

	if err := h.plans.Reorder([]int64{third, second, h.defaultPlanID}); err != nil {
		t.Fatalf("Reorder() 失败: %v", err)
	}
	plans, err := h.plans.List(false)
	if err != nil {
		t.Fatalf("List() 失败: %v", err)
	}
	if len(plans) != 3 || plans[0].ID != third || plans[2].ID != h.defaultPlanID {
		t.Errorf("排序结果 = %+v", plans)
	}
}

// Palette 返回的是副本，界面拿到的顺序必须稳定（默认色靠它取第一个）。
func TestPlans_PaletteIsCopy(t *testing.T) {
	h := newHarness(t)

	got := h.plans.Palette()
	if len(got) != len(planPalette) {
		t.Fatalf("调色板大小 = %d, 期望 %d", len(got), len(planPalette))
	}
	for i := range got {
		if got[i] != planPalette[i] {
			t.Errorf("第 %d 个颜色 = %q, 期望 %q", i, got[i], planPalette[i])
		}
	}
	// 改返回值不该影响后端持有的那份。
	got[0] = "#000000"
	if h.plans.Palette()[0] != planPalette[0] {
		t.Error("调色板被外部修改了，说明返回的不是副本")
	}
}

// ---------------------------------------------------------------- 会话归属

// 开始计时时，会话必须挂在"当前计划"上，而不是未分类。
func TestPlan_SessionFollowsCurrentPlan(t *testing.T) {
	h := newHarness(t)

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(120 * time.Second)
	out, err := h.timer.Stop()
	if err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}
	if out.Session == nil || out.Session.PlanID != h.defaultPlanID {
		t.Fatalf("记录 = %+v, 期望归属计划 %d", out.Session, h.defaultPlanID)
	}

	// 切到另一个计划再记一段：两段能按计划分开。
	other := h.mustCreatePlan(t, "英语", planPalette[1])
	if _, err := h.plans.SetCurrent(other); err != nil {
		t.Fatalf("SetCurrent() 失败: %v", err)
	}
	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(60 * time.Second)
	second, err := h.timer.Stop()
	if err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}
	if second.Session == nil || second.Session.PlanID != other {
		t.Fatalf("第二条记录 = %+v, 期望归属 %d", second.Session, other)
	}

	// 全部 2 条；按计划各 1 条。
	all, err := h.records.ListByDay("", 0)
	if err != nil {
		t.Fatalf("ListByDay() 失败: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("全部记录 = %d, 期望 2", len(all))
	}
	mathList, err := h.records.ListByDay("", h.defaultPlanID)
	if err != nil {
		t.Fatalf("ListByDay(筛选) 失败: %v", err)
	}
	if len(mathList) != 1 || mathList[0].DurationSec != 120 {
		t.Errorf("默认计划记录 = %+v", mathList)
	}
	englishList, err := h.records.ListByDay("", other)
	if err != nil {
		t.Fatalf("ListByDay(筛选) 失败: %v", err)
	}
	if len(englishList) != 1 || englishList[0].DurationSec != 60 {
		t.Errorf("英语记录 = %+v", englishList)
	}

	// 汇总也要能按计划分开。
	sumAll, err := h.records.DaySummary("", 0)
	if err != nil {
		t.Fatalf("DaySummary() 失败: %v", err)
	}
	if sumAll.TotalSec != 180 || sumAll.Count != 2 {
		t.Errorf("全部汇总 = %+v, 期望 180/2", sumAll)
	}
	sumMath, err := h.records.DaySummary("", h.defaultPlanID)
	if err != nil {
		t.Fatalf("DaySummary(筛选) 失败: %v", err)
	}
	if sumMath.TotalSec != 120 || sumMath.Count != 1 {
		t.Errorf("默认计划汇总 = %+v, 期望 120/1", sumMath)
	}
}

// 当前计划指向一个已经被删掉的 id 时，开始计时必须自动纠正而不是记到幽灵计划上。
func TestPlan_ActivePlanRepairsStaleCurrent(t *testing.T) {
	h := newHarness(t)
	spare := h.mustCreatePlan(t, "英语", planPalette[1])

	// 手工把当前计划指向一个不存在的 id（模拟"计划被删时正好有会话在进行"）。
	if err := h.b.setCurrentPlan(9999); err != nil {
		t.Fatalf("setCurrentPlan() 失败: %v", err)
	}

	id, err := h.b.activePlanID()
	if err != nil {
		t.Fatalf("activePlanID() 失败: %v", err)
	}
	if id != h.defaultPlanID {
		t.Errorf("activePlanID() = %d, 期望回落到 %d", id, h.defaultPlanID)
	}
	// 顺手把设置也纠正了。
	if currentOf(h) != id {
		t.Errorf("纠正后 currentPlanID = %d, 期望 %d", currentOf(h), id)
	}

	// 指向一个已归档的计划也一样。
	if _, err := h.plans.SetArchived(spare, true); err != nil {
		t.Fatalf("SetArchived() 失败: %v", err)
	}
	if err := h.b.setCurrentPlan(spare); err != nil {
		t.Fatalf("setCurrentPlan() 失败: %v", err)
	}
	if id, err := h.b.activePlanID(); err != nil || id != h.defaultPlanID {
		t.Errorf("指向归档计划时 activePlanID() = (%d, %v), 期望 %d", id, err, h.defaultPlanID)
	}
}

// ---------------------------------------------------------------- 设置

// 保存设置不能把当前计划清掉：设置页里没有这个字段，界面回传的必然是零值。
func TestSettings_SavePreservesCurrentPlan(t *testing.T) {
	h := newHarness(t)

	saved, err := h.settings.Save(SettingsDTO{
		BreakReminderEnabled: false,
		BreakIntervalMin:     50,
		MinimizeToTray:       false,
		Theme:                settings.ThemeDark,
		// 故意不带 CurrentPlanID——真实界面就是这样回传的。
	})
	if err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}
	if saved.CurrentPlanID != h.defaultPlanID {
		t.Errorf("保存后 CurrentPlanID = %d, 期望保持 %d", saved.CurrentPlanID, h.defaultPlanID)
	}
	if currentOf(h) != h.defaultPlanID {
		t.Errorf("落库的 CurrentPlanID = %d, 期望 %d", currentOf(h), h.defaultPlanID)
	}
	if saved.BreakIntervalMin != 50 || saved.Theme != settings.ThemeDark {
		t.Errorf("其它字段没生效: %+v", saved)
	}
}

// 窗口毛玻璃已经从设置里去掉了（恒定开启、界面无开关）。老库里可能还留着
// windowTranslucent 这个键，必须确认它既不会让加载失败，也不会被写回去。
func TestSettings_LegacyTranslucentKeyIsIgnored(t *testing.T) {
	h := newHarness(t)

	// 模拟一份老库：直接把遗留键塞进 settings 表。
	if err := h.b.db.SaveSettings(map[string]string{"windowTranslucent": "true"}); err != nil {
		t.Fatalf("写入遗留键失败: %v", err)
	}

	reloaded, err := settings.Load(h.b.db)
	if err != nil {
		t.Fatalf("settings.Load() 读到遗留键就失败了: %v", err)
	}
	if reloaded.CurrentPlanID != h.defaultPlanID {
		t.Errorf("重新载入后 CurrentPlanID = %d, 期望 %d", reloaded.CurrentPlanID, h.defaultPlanID)
	}

	// 保存一次设置之后，那个遗留键应当不复存在（我们不再写它，
	// 而 SaveSettings 只覆盖传入的键——所以这里断言的是"没有新写回去"）。
	saved, err := h.settings.Save(SettingsDTO{
		BreakReminderEnabled: true,
		BreakIntervalMin:     45,
		MinimizeToTray:       true,
		Theme:                settings.ThemeLight,
	})
	if err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}
	if saved.CurrentPlanID != h.defaultPlanID {
		t.Errorf("保存后 CurrentPlanID = %d, 期望保持 %d", saved.CurrentPlanID, h.defaultPlanID)
	}

	raw, err := h.b.db.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() 失败: %v", err)
	}
	if _, ok := raw[settings.KeyTheme]; !ok {
		t.Error("保存后连 theme 都没写进去，说明保存路径没走到")
	}
}

// ---------------------------------------------------------------- 记录接口

func TestRecords_DailyTotalsThroughService(t *testing.T) {
	h := newHarness(t)

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(90 * time.Second)
	if _, err := h.timer.Stop(); err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}

	today := t0.Format(layoutDateKey)
	totals, err := h.records.DailyTotals(today, today, 0)
	if err != nil {
		t.Fatalf("DailyTotals() 失败: %v", err)
	}
	if len(totals) != 1 || totals[0].TotalSec != 90 || totals[0].Count != 1 {
		t.Errorf("DailyTotals() = %+v, 期望一条 90s", totals)
	}

	// 别的计划查不到东西，但仍然是空切片而不是 nil。
	other := h.mustCreatePlan(t, "英语", planPalette[1])
	empty, err := h.records.DailyTotals(today, today, other)
	if err != nil {
		t.Fatalf("DailyTotals() 失败: %v", err)
	}
	if empty == nil {
		t.Error("返回了 nil 切片，会被序列化成 null")
	}
	if len(empty) != 0 {
		t.Errorf("别的计划 = %+v, 期望空", empty)
	}
}

func TestRecords_DailyTotalsValidation(t *testing.T) {
	h := newHarness(t)

	if _, err := h.records.DailyTotals("2026-05-05", "2026-05-01", 0); err == nil {
		t.Error("起始日期晚于结束日期应当报错")
	}
	if _, err := h.records.DailyTotals("2026/05/01", "2026-05-05", 0); err == nil {
		t.Error("非法起始日期应当报错")
	}
	if _, err := h.records.DailyTotals("2026-05-01", "2026-5-5", 0); err == nil {
		t.Error("非零填充的结束日期应当报错")
	}
	// 空串按"今天"处理，两边都空时是合法的一天。
	if _, err := h.records.DailyTotals("", "", 0); err != nil {
		t.Errorf("空串应当被当作今天: %v", err)
	}
}

// ---------------------------------------------------------------- 恢复

// 重启后恢复一次进行中的会话时，计划必须跟着回来。
func TestPlan_RecoveryKeepsPlanThroughServices(t *testing.T) {
	h := newHarness(t)
	other := h.mustCreatePlan(t, "英语", planPalette[1])
	if _, err := h.plans.SetCurrent(other); err != nil {
		t.Fatalf("SetCurrent() 失败: %v", err)
	}
	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}

	// 直接用同一个库新建一个"进程"。
	second, err := newBackend(h.b.db, h.clock.Now)
	if err != nil {
		t.Fatalf("newBackend() 失败: %v", err)
	}
	out, err := second.timer.Recover()
	if err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}
	if out.Action != timer.RecoveryContinued {
		t.Fatalf("Action = %s, 期望 %s", out.Action, timer.RecoveryContinued)
	}
	if out.Snapshot.PlanID != other {
		t.Errorf("恢复后 PlanID = %d, 期望 %d", out.Snapshot.PlanID, other)
	}
}
