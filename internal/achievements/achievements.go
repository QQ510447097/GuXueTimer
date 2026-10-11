// Package achievements 定义成就、等级与经验规则。
//
// 这个包是**纯函数**：不认识数据库，也不认识时间——所有输入都由调用方给，
// 所有输出都由调用方落库。好处是规则可以在测试里用几个数字直接验，
// 不必造一堆会话。
//
// # 三种成就
//
//   - once（单次）：判据是一个单调不减的量够了没有（累计 10 小时、建过计划）。
//     不给经验，只记达成时间。
//   - cycle（循环）：**累计**若干个"达标日"达成一次，可以反复达成。
//     每次达成给一份经验，并发一枚对应等级的奖章。
//   - daily（每日）：同样是达标日，只是周期为 1 天。
//
// # 两条硬约束
//
//  1. **判据必须单调不减**：经验与等级是存下来的账（见 progress.go 的说明），
//     而成就本身虽由记录推导，也绝不能让用户"已经拿到的徽章消失"。
//  2. **累计不是连续**：达标日之间断开不影响推进——周一、周三、下周二……
//     凑满 7 天照样算一次周度达成。不达标的天只是不推进，不清零。
package achievements

// Kind 是成就的类别。
const (
	KindOnce  = "once"
	KindCycle = "cycle"
	KindDaily = "daily"
)

// 奖章等级。循环成就每达成一次发一枚，并永久抬高"每小时经验"。
const (
	MedalBronze = "bronze" // 周度
	MedalSilver = "silver" // 月度
	MedalGold   = "gold"   // 季度
	MedalRuby   = "ruby"   // 年度
)

// 规则常量。
const (
	// ThresholdSec 是"达标日"的门槛：当天累计学习满 1 小时。
	ThresholdSec = 3600
	// HourSec 是成长经验的计费单位：每满 1 小时结算一次。
	HourSec = 3600
	// DailyTenths 是每日成就的经验（10 点）。
	DailyTenths = 100
	// CarryCapSec 是零头的上限，防止坏数据把 carry_sec 撑成天文数字。
	CarryCapSec = HourSec * 24
)

// Stats 是评价单次成就需要的统计量。
//
// 刻意保持纯数据：streak、深夜/早起这类日期逻辑在 store 里算好再传进来。
type Stats struct {
	TotalSec     int64
	SessionCount int
	LongestSec   int64
	ActiveDays   int
	PlanCount    int
	// FirstPlanAt 是**最早创建的那个计划**的时间；一个计划都没有时为空串。
	//
	// 用它而不是 PlanCount："第一次创建学习计划"问的是"有没有建过"，
	// 而 PlanCount 统计的是"有记录的计划数"，两者不是一回事。
	FirstPlanAt string
}

// Definition 是一个成就的定义。
type Definition struct {
	ID          string
	Name        string
	Description string
	Kind        string

	// ---- once ----
	Unit   string
	Target int64
	// Value 取出这个成就关心的量。必须是单调不减的。
	Value func(Stats) int64
	// OnSettle 在"结束并保存"时判断这条是否达成（nil 表示不在结算时判断，
	// 例如"第一次创建计划"由建计划那条路径负责）。
	OnSettle func(in SettleInput) bool

	// ---- cycle / daily ----
	// PeriodDays 是达成一次需要的达标日数量（日 = 1、周 = 7、月 = 30、季 = 90、年 = 365）。
	PeriodDays int
	// RewardTenths 是每次达成的经验奖励（十分之一经验）。
	RewardTenths int64
	// Medal 是达成一次发的奖章；每日成就没有奖章。
	Medal string
}

// Progress 是"已凑够多少个达标日"的进度，供进度条用。
func (d Definition) Remainder(qualifyingDays int, granted int) int {
	if d.PeriodDays <= 0 {
		return 0
	}
	rest := qualifyingDays - granted*d.PeriodDays
	if rest < 0 {
		// 门槛被调大之后会出现负数：夹到 0，进度条不显示倒退。
		return 0
	}
	if rest >= d.PeriodDays {
		// 门槛被调小之后又可能超出一个周期：也只显示到满。
		return d.PeriodDays
	}
	return rest
}

// All 是全部成就。
//
// 加新成就就是往这里追加一条：前端的徽章图按 ID 命名
// （frontend/public/badges/<id>.png），没有图会回退到应用图标。
// 单次成就的判据必须是单调不减的量（不能用"当前连续天数"这种会归零的值）。
var All = []Definition{
	{
		ID:          "first-plan",
		Name:        "千里之行，始于足下",
		Description: "第一次创建学习计划",
		Kind:        KindOnce,
		Unit:        "次",
		Target:      1,
		Value: func(s Stats) int64 {
			if s.FirstPlanAt == "" {
				return 0
			}
			return 1
		},
		// 由建计划那条路径写达成时间，不在保存会话时判断。
	},
	{
		ID:          "hours-10",
		Name:        "不积跬步，无以至千里",
		Description: "累计学习满 10 小时",
		Kind:        KindOnce,
		Unit:        "小时",
		Target:      10,
		Value:       func(s Stats) int64 { return s.TotalSec / 3600 },
		OnSettle:    func(in SettleInput) bool { return in.TotalSec >= 10*3600 },
	},
	{
		ID:          "hours-100",
		Name:        "锲而不舍，金石可镂",
		Description: "累计学习满 100 小时",
		Kind:        KindOnce,
		Unit:        "小时",
		Target:      100,
		Value:       func(s Stats) int64 { return s.TotalSec / 3600 },
		OnSettle:    func(in SettleInput) bool { return in.TotalSec >= 100*3600 },
	},
	{
		ID:          "hours-1000",
		Name:        "水滴石穿，非一日之功",
		Description: "累计学习满 1000 小时",
		Kind:        KindOnce,
		Unit:        "小时",
		Target:      1000,
		Value:       func(s Stats) int64 { return s.TotalSec / 3600 },
		OnSettle:    func(in SettleInput) bool { return in.TotalSec >= 1000*3600 },
	},
	{
		ID:           "week-7",
		Name:         "七日之约",
		Description:  "累计 7 天学习满 1 小时",
		Kind:         KindCycle,
		PeriodDays:   7,
		RewardTenths: 1000,
		Medal:        MedalBronze,
	},
	{
		ID:           "month-30",
		Name:         "积月成岁",
		Description:  "累计 30 天学习满 1 小时",
		Kind:         KindCycle,
		PeriodDays:   30,
		RewardTenths: 5000,
		Medal:        MedalSilver,
	},
	{
		ID:           "quarter-90",
		Name:         "一季之恒",
		Description:  "累计 90 天学习满 1 小时",
		Kind:         KindCycle,
		PeriodDays:   90,
		RewardTenths: 20000,
		Medal:        MedalGold,
	},
	{
		ID:           "year-365",
		Name:         "岁岁不辍",
		Description:  "累计 365 天学习满 1 小时",
		Kind:         KindCycle,
		PeriodDays:   365,
		RewardTenths: 100000,
		Medal:        MedalRuby,
	},
	{
		ID:           "daily-hour",
		Name:         "今日之课",
		Description:  "当天累计学习满 1 小时",
		Kind:         KindDaily,
		PeriodDays:   1,
		RewardTenths: DailyTenths,
	},
}

// ByID 按 id 取定义。
func ByID(id string) (Definition, bool) {
	for _, def := range All {
		if def.ID == id {
			return def, true
		}
	}
	return Definition{}, false
}
