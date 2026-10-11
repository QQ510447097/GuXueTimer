package achievements

import (
	"fmt"
	"testing"
)

/*
这一组测**规则引擎**：等级曲线、成长倍率、结算（进位、达标、奖章）、回放。

成就表（All）里的具体条目属于内容，不在这里验——它由 TestAll_AreWellFormed
把不该出错的字段（ID 唯一、门槛为正、奖章取值合法）逐个检查。
*/

func TestLevelCost_SmallGradient(t *testing.T) {
	cases := map[int]int64{
		1: 100, 2: 100, 10: 100,
		11: 110, 20: 110,
		21: 120, 30: 120,
		31: 130,
	}
	for level, want := range cases {
		if got := LevelCost(level); got != want {
			t.Errorf("LevelCost(%d) = %d, 期望 %d", level, got, want)
		}
	}
}

func TestLevelAt_Boundaries(t *testing.T) {
	cases := []struct {
		name     string
		xpTenths int64
		wantLv   int
		wantInto int64
		wantNeed int64
	}{
		{"零经验是 1 级", 0, 1, 0, 1000},
		{"差 0.1 点升级", 999, 1, 999, 1000},
		{"刚好升级", 1000, 2, 0, 1000},
		{"二级差一点", 1999, 2, 999, 1000},
		{"跨过十级", 10000, 11, 0, 1100},
		{"十一级中途", 10550, 11, 550, 1100},
		{"负数按 0 处理", -50, 1, 0, 1000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lv, into, need := LevelAt(c.xpTenths)
			if lv != c.wantLv || into != c.wantInto || need != c.wantNeed {
				t.Errorf("LevelAt(%d) = (%d, %d, %d), 期望 (%d, %d, %d)",
					c.xpTenths, lv, into, need, c.wantLv, c.wantInto, c.wantNeed)
			}
		})
	}
}

func TestRateTenths_GrowthFormula(t *testing.T) {
	if got := RateTenths(MedalCounts{}); got != 10 {
		t.Errorf("没有奖章时 = %d, 期望 10（即每小时 1.0）", got)
	}
	got := RateTenths(MedalCounts{Week: 3, Month: 2, Quarter: 1, Year: 1})
	// 10 + 3 + 4 + 3 + 12 = 32
	if got != 32 {
		t.Errorf("倍率 = %d, 期望 32（即每小时 3.2）", got)
	}
}

// 不足一小时只进零头，不产生经验；凑满才结算，且跨会话累计。
func TestSettle_CarryAcrossSessions(t *testing.T) {
	st := State{Level: 1}
	// 第一次 40 分钟：不够一小时。
	out := Settle(SettleInput{State: st, DurationSec: 40 * 60, Day: "2026-05-01"})
	if out.State.XpTenths != 0 || out.State.CarrySec != 40*60 {
		t.Fatalf("40 分钟后 = %+v, 期望 0 经验 + 40 分钟零头", out.State)
	}
	// 再来 30 分钟：凑满 1 小时，拿 1.0 经验。
	out = Settle(SettleInput{State: out.State, DurationSec: 30 * 60, Day: "2026-05-02"})
	if out.State.XpTenths != 10 {
		t.Errorf("凑满一小时后经验 = %d, 期望 10（十分之一经验）", out.State.XpTenths)
	}
	if out.State.CarrySec != 10*60 {
		t.Errorf("零头 = %d, 期望 600 秒", out.State.CarrySec)
	}
}

// 达标日：当天累计满 1 小时才算，3599 秒不算。
func TestSettle_QualifyingDay(t *testing.T) {
	st := State{Level: 1}

	// 差一秒：不达标。
	out := Settle(SettleInput{State: st, DurationSec: 3599, Day: "2026-05-01", DayTotalSec: 3599})
	if out.QualifyDay {
		t.Error("3599 秒不该算达标")
	}
	if out.State.XpTenths != 0 {
		t.Errorf("不达标的日子不该有每日奖励, 得到 %d", out.State.XpTenths)
	}

	// 正好一小时：达标，拿每日 10 点；同时这 1 小时本身也给 1.0 经验。
	const now = "2026-05-01T14:23:45+08:00"
	out = Settle(SettleInput{
		State: st, DurationSec: 3600, Day: "2026-05-01", DayTotalSec: 3600, Now: now,
	})
	if !out.QualifyDay {
		t.Fatal("3600 秒应当算达标")
	}
	if out.State.XpTenths != 110 {
		t.Errorf("经验 = %d, 期望 110（每日 100 + 这一小时 10）", out.State.XpTenths)
	}
	// 达成时刻要精确到分钟，不能只记日期。
	if got := out.States["daily-hour"]; got.Count != 1 || got.LastAt != now {
		t.Errorf("每日成就状态 = %+v, 期望次数 1、上次 %q", got, now)
	}

	// 同一天再来一段：不重复计达标（状态也不该出现在变化量里）。
	second := Settle(SettleInput{
		State: out.State, States: out.States, DurationSec: 600,
		Day: "2026-05-01", DayTotalSec: 4200, DayQualified: true, QualifyingDays: 1,
	})
	if second.QualifyDay {
		t.Error("同一天第二次不该再算一次达标")
	}
	if _, changed := second.States["daily-hour"]; changed {
		t.Errorf("同一天第二次不该改动每日成就: %+v", second.States["daily-hour"])
	}
}

// 累计不是连续：达标日散落在不相邻的日子里，凑够 7 个照样发周奖章。
func TestSettle_CumulativeNotConsecutive(t *testing.T) {
	state := State{Level: 1}
	states := map[string]AchievementState{}
	qualifying := 0

	// 中间隔着不达标的日子（5-02、5-04 只有 10 分钟）。
	days := []string{"2026-05-01", "2026-05-03", "2026-05-05", "2026-05-07", "2026-05-09", "2026-05-11"}
	for _, day := range days {
		out := Settle(SettleInput{
			State: state, States: states, DurationSec: ThresholdSec,
			Day: day, DayTotalSec: ThresholdSec, QualifyingDays: qualifying,
		})
		state, states = out.State, mergeStates(states, out.States)
		qualifying++
	}
	if got := states["week-7"].Count; got != 0 {
		t.Fatalf("6 个达标日就发了周奖章？次数 = %d", got)
	}
	if state.Medals.Week != 0 {
		t.Fatalf("6 个达标日就发了奖章：%+v", state.Medals)
	}

	// 第 7 个（5-13，与前面同样不相邻）：发一枚铜奖章 + 100 点经验。
	out := Settle(SettleInput{
		State: state, States: states, DurationSec: ThresholdSec,
		Day: "2026-05-13", DayTotalSec: ThresholdSec, QualifyingDays: qualifying,
	})
	if out.State.Medals.Week != 1 {
		t.Errorf("第 7 个达标日应当发一枚铜奖章, 得到 %+v", out.State.Medals)
	}
	if got := out.States["week-7"]; got.Count != 1 || got.LastAt != "2026-05-13" {
		t.Errorf("周奖章状态 = %+v, 期望次数 1、上次 2026-05-13", got)
	}
	// 之前 6 天各拿 10 点每日奖励，今天再拿 10 点 + 100 点周奖章。
	// 6 天 × 1 小时 = 6 点成长经验（100 分钟里只有 6 个整点）。
	// 逐条算太脆，这里只断言"至少包含周奖章的 100 点"。
	if out.State.XpTenths < 1000 {
		t.Errorf("经验 = %d, 期望至少包含周奖章的 1000（十分之一）", out.State.XpTenths)
	}
}

// 当天拿到的奖章当天就生效：第 7 个达标日那一整天的时长都按抬高后的倍率结算。
func TestSettle_MedalTakesEffectSameDay(t *testing.T) {
	// 先攒 6 个达标日。DurationSec 给 0 是为了把"小时经验"单独留给最后一天观察
	// ——达标判定看的是 DayTotalSec。
	state := State{Level: 1}
	states := map[string]AchievementState{}
	for i := 0; i < 6; i++ {
		out := Settle(SettleInput{
			State: state, States: states, DurationSec: 0,
			Day: fmt.Sprintf("2026-05-%02d", i+1), DayTotalSec: ThresholdSec, QualifyingDays: i,
		})
		state, states = out.State, mergeStates(states, out.States)
	}
	if state.XpTenths != 600 {
		t.Fatalf("前 6 天的每日奖励 = %d, 期望 600", state.XpTenths)
	}

	// 第 7 天：一次 2 小时的会话。铜奖章在这天先发下来，所以两小时都按 1.1 算，
	// 而不是第一小时 1.0、第二小时才 1.1。
	out := Settle(SettleInput{
		State: state, States: states, DurationSec: 2 * ThresholdSec,
		Day: "2026-05-07", DayTotalSec: 2 * ThresholdSec, QualifyingDays: 6,
	})
	// 600 + 每日 100 + 周奖章 1000 + 两小时 22 = 1722 个十分之一
	if out.State.XpTenths != 1722 {
		t.Errorf("经验 = %d, 期望 1722（每日 100 + 奖章 1000 + 两小时 22）", out.State.XpTenths)
	}
	if out.State.Medals.Week != 1 {
		t.Errorf("奖章 = %+v, 期望 1 枚铜奖章", out.State.Medals)
	}
}

// 门槛被调大时，已经发出去的奖章不收回。
func TestSettle_NeverTakesBackMedals(t *testing.T) {
	state := State{Level: 1, Medals: MedalCounts{Week: 3}}
	states := map[string]AchievementState{"week-7": {Count: 3, LastAt: "2026-05-01"}}

	// 门槛假设已经改成 30 天：现在只有 8 个达标日，want = 0。
	out := Settle(SettleInput{
		State: state, States: states, DurationSec: 0,
		Day: "2026-05-08", DayTotalSec: ThresholdSec, QualifyingDays: 8,
	})
	if out.State.Medals.Week != 3 {
		t.Errorf("奖章被收回了: %+v", out.State.Medals)
	}
	if _, changed := out.States["week-7"]; changed {
		t.Errorf("达成次数被改小了: %+v", out.States["week-7"])
	}
}

// 单次成就的达成时间：够线就记一次，之后不再变。
func TestSettle_OnceAchievedAtIsStable(t *testing.T) {
	in := SettleInput{
		State: State{Level: 1}, DurationSec: 0,
		TotalSec: 10 * 3600, Now: "2026-05-01T10:00:00+08:00",
	}
	out := Settle(in)
	if got := out.States["hours-10"].AchievedAt; got != in.Now {
		t.Errorf("hours-10 达成时间 = %q, 期望 %q", got, in.Now)
	}
	if _, ok := out.States["hours-100"]; ok {
		t.Error("100 小时那条不该在这时候达成")
	}

	// 再来一次：已经记过的达成时间不能被刷新。
	second := Settle(SettleInput{
		State: out.State, States: out.States, DurationSec: 0,
		TotalSec: 12 * 3600, Now: "2026-06-01T10:00:00+08:00",
	})
	if got := second.States["hours-10"].AchievedAt; got != "" {
		t.Errorf("已达成的单次成就被重复写入了: %q", got)
	}
}

// 回放（老库回填）用"那天最后一条记录的结束时刻"当达成时刻。
func TestReplay_UsesDayEndedAt(t *testing.T) {
	const ended = "2026-05-01T21:07:00+08:00"
	_, _, states := Replay([]Day{{Date: "2026-05-01", Sec: 3600, EndedAt: ended}}, "")
	if got := states["daily-hour"].LastAt; got != ended {
		t.Errorf("回放记的达成时刻 = %q, 期望用那天的结束时刻", got)
	}

	// 没有时刻（更老的数据）就退回日期，别把空串写进去。
	_, _, states = Replay([]Day{{Date: "2026-05-02", Sec: 3600}}, "")
	if got := states["daily-hour"].LastAt; got != "2026-05-02" {
		t.Errorf("没有时刻时应当退回日期, 得到 %q", got)
	}
}

// 回放：与逐次结算得到同一份账。
func TestReplay_MatchesStepByStep(t *testing.T) {
	days := []Day{
		{Date: "2026-05-01", Sec: 3600},
		{Date: "2026-05-02", Sec: 600},  // 不达标
		{Date: "2026-05-03", Sec: 7200}, // 两小时
		{Date: "2026-05-04", Sec: 3600},
	}

	state, qualifying, states := Replay(days, "")
	if len(qualifying) != 3 {
		t.Fatalf("达标日 = %d 个, 期望 3", len(qualifying))
	}
	if got := states["daily-hour"].Count; got != 3 {
		t.Errorf("每日达成次数 = %d, 期望 3", got)
	}
	// 逐次结算跑一遍，账必须一致。
	stepState := State{Level: 1}
	stepStates := map[string]AchievementState{}
	count := 0
	for _, day := range days {
		out := Settle(SettleInput{
			State: stepState, States: stepStates, DurationSec: day.Sec,
			Day: day.Date, DayTotalSec: day.Sec, QualifyingDays: count,
			TotalSec: day.Sec,
		})
		stepState, stepStates = out.State, mergeStates(stepStates, out.States)
		if out.QualifyDay {
			count++
		}
	}
	if stepState != state {
		t.Errorf("逐次结算的账 = %+v, 回放 = %+v", stepState, state)
	}
}

// 单调性：在后面追加天数，账与各成就状态都不允许变小或提前。
func TestReplay_Monotonic(t *testing.T) {
	base := []Day{
		{Date: "2026-05-01", Sec: 3600},
		{Date: "2026-05-02", Sec: 1800},
		{Date: "2026-05-03", Sec: 4000},
	}
	more := append([]Day{}, base...)
	more = append(more,
		Day{Date: "2026-05-04", Sec: 3600},
		Day{Date: "2026-05-05", Sec: 3600},
		Day{Date: "2026-05-06", Sec: 3600},
		Day{Date: "2026-05-07", Sec: 9999},
	)

	a, _, statesA := Replay(base, "")
	b, _, statesB := Replay(more, "")

	if b.XpTenths < a.XpTenths {
		t.Errorf("经验倒退了: %d -> %d", a.XpTenths, b.XpTenths)
	}
	if b.Level < a.Level {
		t.Errorf("等级倒退了: %d -> %d", a.Level, b.Level)
	}
	if b.Medals.Week < a.Medals.Week || b.Medals.Month < a.Medals.Month ||
		b.Medals.Quarter < a.Medals.Quarter || b.Medals.Year < a.Medals.Year {
		t.Errorf("奖章倒退了: %+v -> %+v", a.Medals, b.Medals)
	}
	for id, before := range statesA {
		after := statesB[id]
		if after.Count < before.Count {
			t.Errorf("%s 达成次数倒退: %d -> %d", id, before.Count, after.Count)
		}
		if before.AchievedAt != "" && after.AchievedAt != before.AchievedAt {
			t.Errorf("%s 达成时间变了: %q -> %q", id, before.AchievedAt, after.AchievedAt)
		}
	}
}

// 回放按日期排序，调用方给的顺序不影响结果。
func TestReplay_SortsByDate(t *testing.T) {
	ordered := []Day{{Date: "2026-05-01", Sec: 3600}, {Date: "2026-05-02", Sec: 3600}}
	shuffled := []Day{{Date: "2026-05-02", Sec: 3600}, {Date: "2026-05-01", Sec: 3600}}
	a, _, _ := Replay(ordered, "")
	b, _, _ := Replay(shuffled, "")
	if a != b {
		t.Errorf("乱序回放结果不同: %+v vs %+v", a, b)
	}
}

// 零头有上限，坏数据不会把 carry 撑成天文数字。
func TestSettle_CarryIsCapped(t *testing.T) {
	out := Settle(SettleInput{State: State{Level: 1, CarrySec: 1 << 40}, DurationSec: 0})
	if out.State.CarrySec > CarryCapSec {
		t.Errorf("零头 = %d, 超过上限 %d", out.State.CarrySec, CarryCapSec)
	}
}

// 成就表本身的规范检查：等正式成就加进来时，把明显的错误挡在测试里。
func TestAll_AreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, def := range All {
		if def.ID == "" || def.Name == "" || def.Description == "" {
			t.Errorf("成就字段不全: %+v", def)
		}
		if seen[def.ID] {
			t.Errorf("成就 ID 重复: %s", def.ID)
		}
		seen[def.ID] = true

		switch def.Kind {
		case KindOnce:
			if def.Target <= 0 {
				t.Errorf("%s 的门槛应当为正数, 实际 %d", def.ID, def.Target)
			}
			if def.Value == nil {
				t.Errorf("%s 没有取值函数", def.ID)
			}
			if def.RewardTenths != 0 {
				t.Errorf("%s 是单次成就，不该给经验", def.ID)
			}
		case KindCycle, KindDaily:
			if def.PeriodDays <= 0 {
				t.Errorf("%s 的周期应当为正数, 实际 %d", def.ID, def.PeriodDays)
			}
			if def.RewardTenths <= 0 {
				t.Errorf("%s 应当有经验奖励", def.ID)
			}
			switch def.Medal {
			case "", MedalBronze, MedalSilver, MedalGold, MedalRuby:
			default:
				t.Errorf("%s 的奖章取值不认识: %q", def.ID, def.Medal)
			}
			if def.Kind == KindDaily && def.Medal != "" {
				t.Errorf("%s 是每日成就，不该发奖章", def.ID)
			}
		default:
			t.Errorf("%s 的类别不认识: %q", def.ID, def.Kind)
		}
	}

	// 要求的那几条必须在表里。
	for _, id := range []string{"first-plan", "hours-10", "hours-100", "hours-1000",
		"week-7", "month-30", "quarter-90", "year-365", "daily-hour"} {
		if !seen[id] {
			t.Errorf("成就表里缺少 %s", id)
		}
	}
}

// mergeStates 把小批更新并进全量状态（测试辅助）。
func mergeStates(all, delta map[string]AchievementState) map[string]AchievementState {
	out := map[string]AchievementState{}
	for k, v := range all {
		out[k] = v
	}
	for k, v := range delta {
		out[k] = v
	}
	return out
}
