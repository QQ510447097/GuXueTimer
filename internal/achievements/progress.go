package achievements

import "sort"

/*
# 为什么经验要存下来，而不是每次从记录重算

同一小时在"拿到奖章之前"和"之后"值多少经验是不同的，而规则本身还会被调整。
如果每次都按当前规则把整段历史重放一遍，那么：

  - 把每级经验从 100 调到 120，用户就会**掉级**；
  - 把周度门槛从 7 天调到 10 天，已经拿到的奖章会**被收回**。

这两件事都不该发生。所以经验、等级、奖章数是**存下来的账**（见 store 的 progress 表），
只在"结束并保存"那一刻增量结算；成就本身仍由记录推导（它问的是"总量够没够"，
重算永远得到同一个答案，不会漂）。

本文件里的 Replay 只用于两件事：老库升级时算一次初值，以及核对"账"与"记录"是否一致。
平时不要拿它覆盖账。
*/

// MedalCounts 是四类奖章的累计数量，也是"每小时经验"的来源。
type MedalCounts struct {
	Week    int
	Month   int
	Quarter int
	Year    int
}

// State 是"账"：累计经验、没凑满一小时的零头、等级高水位、四类奖章数。
//
// 所有字段都只增不减。
type State struct {
	// XpTenths 是累计经验，单位是十分之一经验（见 achievements.go 的说明）。
	XpTenths int64
	// CarrySec 是没凑满一小时的零头，跨会话累计。
	CarrySec int64
	// Level 是等级高水位。
	Level  int
	Medals MedalCounts
}

// AchievementState 是一个成就的持久化状态。
type AchievementState struct {
	Count      int
	LastAt     string
	AchievedAt string
}

// RateTenths 返回当前"每小时学习"能拿到的经验（十分之一经验）。
//
// 公式：1 + 0.1W + 0.2M + 0.3Q + 1.2Y；乘 10 化成整数就是
// 10 + W + 2M + 3Q + 12Y，没有任何浮点误差。
func RateTenths(m MedalCounts) int64 {
	return 10 + int64(m.Week) + 2*int64(m.Month) + 3*int64(m.Quarter) + 12*int64(m.Year)
}

// LevelCost 返回第 level 级升到 level+1 级需要的经验（单位：点，不是十分之一）。
//
// 1~10 级各 100、11~20 级各 110、21~30 级各 120……即每十级加 10。
func LevelCost(level int) int64 {
	if level < 1 {
		level = 1
	}
	return 100 + 10*int64((level-1)/10)
}

// LevelAt 从累计经验算出等级，以及当前等级内已攒 / 升级所需的经验（十分之一）。
func LevelAt(xpTenths int64) (level int, intoTenths, needTenths int64) {
	level = 1
	rest := xpTenths
	if rest < 0 {
		rest = 0
	}
	for {
		need := LevelCost(level) * 10
		if rest < need {
			return level, rest, need
		}
		rest -= need
		level++
	}
}

// LevelFloorTenths 返回"刚好达到第 level 级"所需的累计经验（十分之一）。
//
// 等级高水位比曲线算出来的高时（曲线被改陡过），进度条要按高水位那一级算，
// 靠它把起点换算出来。
func LevelFloorTenths(level int) int64 {
	if level < 2 {
		return 0
	}
	var sum int64
	for l := 1; l < level; l++ {
		sum += LevelCost(l) * 10
	}
	return sum
}

// SettleInput 是"把一次会话并进账"的输入，全部由 store 在事务里读好。
type SettleInput struct {
	State State
	// States 是各成就的当前状态（可能缺条目，视为零值）。
	States map[string]AchievementState
	// Day 是这次会话归属的日期（YYYY-MM-DD）。
	Day string
	// DayQualified 表示这天是否已经记进达标账本。
	DayQualified bool
	// DayTotalSec 是这天（含刚写入的这条记录）的总时长。
	DayTotalSec int64
	// TotalSec 是全部历史（含刚写入的这条记录）的总时长。
	TotalSec int64
	// DurationSec 是刚写入这条记录的时长。
	DurationSec int64
	// QualifyingDays 是**这次之前**账本里的达标日总数。
	QualifyingDays int
	// Now 是当前时刻，写"达成时间"用。
	Now string
}

// SettleOutput 是结算结果：新的账 + 需要落库的状态 + 是否要记一个新的达标日。
type SettleOutput struct {
	State State
	// States 只包含**发生变化**的成就状态。
	States map[string]AchievementState
	// QualifyDay 为 true 表示这天首次达标，要记进账本。
	QualifyDay bool
}

// Settle 把一次会话并进账里。
//
// 顺序是刻意的：**先判当天的达标与奖章，再用（可能刚被抬高的）倍率结算当天的小时**。
// 这样"今天拿到的奖章今天就生效"，符合"经验是即时发放"的预期。
func Settle(in SettleInput) SettleOutput {
	out := SettleOutput{State: in.State, States: map[string]AchievementState{}}
	if out.State.Level < 1 {
		out.State.Level = 1
	}
	if out.State.CarrySec < 0 {
		out.State.CarrySec = 0
	}

	// 状态读取器：优先取本次已改过的，其次取调用方给的。
	get := func(id string) AchievementState {
		if st, ok := out.States[id]; ok {
			return st
		}
		return in.States[id]
	}
	put := func(id string, st AchievementState) { out.States[id] = st }

	// ① 当天首次达标：记一个达标日、发每日奖励、推进四个循环
	qualifying := in.QualifyingDays
	if in.DayTotalSec >= ThresholdSec && !in.DayQualified {
		out.QualifyDay = true
		qualifying++
	}

	for _, def := range All {
		switch def.Kind {
		case KindDaily:
			if !out.QualifyDay {
				continue
			}
			st := get(def.ID)
			st.Count++
			st.LastAt = in.Day
			put(def.ID, st)
			out.State.XpTenths += def.RewardTenths

		case KindCycle:
			if !out.QualifyDay || def.PeriodDays <= 0 {
				continue
			}
			st := get(def.ID)
			// 应得的枚数由"达标日总数 / 周期"决定；取 max 是因为门槛可能被调大，
			// 而已经发出去的奖章绝不收回（进度只增不减）。
			want := qualifying / def.PeriodDays
			if want <= st.Count {
				continue
			}
			gained := want - st.Count
			st.Count = want
			st.LastAt = in.Day
			put(def.ID, st)
			out.State.XpTenths += int64(gained) * def.RewardTenths
			switch def.Medal {
			case MedalBronze:
				out.State.Medals.Week += gained
			case MedalSilver:
				out.State.Medals.Month += gained
			case MedalGold:
				out.State.Medals.Quarter += gained
			case MedalRuby:
				out.State.Medals.Year += gained
			}
		}
	}

	// ② 用此刻的倍率结算这次会话带来的整点小时
	carry := out.State.CarrySec + in.DurationSec
	if carry < 0 {
		carry = 0
	}
	hours := carry / HourSec
	out.State.CarrySec = carry % HourSec
	if out.State.CarrySec > CarryCapSec {
		out.State.CarrySec = CarryCapSec
	}
	if hours > 0 {
		out.State.XpTenths += hours * RateTenths(out.State.Medals)
	}

	// ③ 单次成就：够线就记达成时间（已经记过的保持原样，达成时间不会漂）
	for _, def := range All {
		if def.Kind != KindOnce || def.OnSettle == nil {
			continue
		}
		st := get(def.ID)
		if st.AchievedAt != "" || !def.OnSettle(in) {
			continue
		}
		st.AchievedAt = in.Now
		put(def.ID, st)
	}

	// ④ 等级高水位
	if level, _, _ := LevelAt(out.State.XpTenths); level > out.State.Level {
		out.State.Level = level
	}
	return out
}

// Day 是回放用的一天。
type Day struct {
	Date string
	Sec  int64
}

// Replay 按天回放历史，算出一份完整的账。
//
// 只用于两件事：老库升级时算初值、以及核对"账"与"记录"是否一致。
// 它按**当前规则**回放，所以不能拿它去覆盖已经结算过的账（那会把规则改动
// 追溯地套到历史上，正是我们把账存下来的原因）。
func Replay(days []Day, firstPlanAt string) (State, []Day, map[string]AchievementState) {
	sorted := make([]Day, len(days))
	copy(sorted, days)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })

	st := State{Level: 1}
	states := map[string]AchievementState{}
	var qualifying []Day
	qualifyingCount := 0
	var totalSec int64

	for _, day := range sorted {
		totalSec += day.Sec
		qualified := day.Sec >= ThresholdSec
		if qualified {
			qualifying = append(qualifying, day)
			qualifyingCount++
		}

		// 与 Settle 保持同一套顺序：先达标/奖章，再结算当天小时。
		for _, def := range All {
			switch def.Kind {
			case KindDaily:
				if !qualified {
					continue
				}
				cur := states[def.ID]
				cur.Count++
				cur.LastAt = day.Date
				states[def.ID] = cur
				st.XpTenths += def.RewardTenths
			case KindCycle:
				if !qualified || def.PeriodDays <= 0 {
					continue
				}
				cur := states[def.ID]
				want := qualifyingCount / def.PeriodDays
				if want <= cur.Count {
					continue
				}
				gained := want - cur.Count
				cur.Count = want
				cur.LastAt = day.Date
				states[def.ID] = cur
				st.XpTenths += int64(gained) * def.RewardTenths
				switch def.Medal {
				case MedalBronze:
					st.Medals.Week += gained
				case MedalSilver:
					st.Medals.Month += gained
				case MedalGold:
					st.Medals.Quarter += gained
				case MedalRuby:
					st.Medals.Year += gained
				}
			}
		}

		carry := st.CarrySec + day.Sec
		hours := carry / HourSec
		st.CarrySec = carry % HourSec
		if hours > 0 {
			st.XpTenths += hours * RateTenths(st.Medals)
		}
		if level, _, _ := LevelAt(st.XpTenths); level > st.Level {
			st.Level = level
		}

		// 单次成就：达成时间就是"越线那天"。
		for _, def := range All {
			if def.Kind != KindOnce || def.OnSettle == nil {
				continue
			}
			cur := states[def.ID]
			if cur.AchievedAt != "" {
				continue
			}
			if def.OnSettle(SettleInput{TotalSec: totalSec}) {
				cur.AchievedAt = day.Date
				states[def.ID] = cur
			}
		}
	}

	// "第一次创建学习计划"不在回放里推进（它由建计划那条路径负责），
	// 但老库回填时要把它补上：有计划的创建时间就算达成过。
	if firstPlanAt != "" {
		cur := states["first-plan"]
		if cur.AchievedAt == "" {
			cur.AchievedAt = firstPlanAt
		}
		states["first-plan"] = cur
	}

	return st, qualifying, states
}
