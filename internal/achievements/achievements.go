// Package achievements 定义成就与它们的解锁判据。
//
// 成就**不做持久化**：它完全由 sessions 表推导出来。这样做的好处是不必迁移、
// 也不会出现"数据库里的解锁状态与真实记录不一致"这种没法修的问题；
// 代价是没有"解锁日期"（想要那个就得再加一张表，见 README 的说明）。
//
// 因此有一条硬约束：**判据必须是单调不减的量**。用户一旦拿到徽章就不该再丢，
// 所以不能拿"当前连续天数"这类会归零的值当判据（要用"历史最长连续"）。
// 加新成就时请守住这一条。
package achievements

// Tier 是徽章的稀有度，前端据此换配色。
const (
	TierBronze = "bronze"
	TierSilver = "silver"
	TierGold   = "gold"
)

// Stats 是评价成就需要的全部统计量。
//
// 它是纯数据（不含时间字段），所以成就的判据可以在测试里用几个数字直接构造，
// 不必去造一堆 sessions。streak 之类的日期逻辑在 store 里算好再传进来。
type Stats struct {
	TotalSec      int64
	SessionCount  int
	LongestSec    int64
	ActiveDays    int
	CurrentStreak int
	LongestStreak int
	PlanCount     int
}

// Definition 是一个成就的定义。
type Definition struct {
	ID          string
	Name        string
	Description string
	Tier        string
	// Unit 是进度条旁边的单位（次 / 小时 / 天）。
	Unit string
	// Target 是解锁门槛，与 Value 的返回值同单位。
	Target int64
	// Value 从统计里取出这个成就关心的那个量。必须是单调不减的。
	Value func(Stats) int64
}

// All 是全部成就。加新成就就是往这里追加一条——
// 前端的徽章图按 ID 命名（frontend/public/badges/<id>.png），没有图也有兜底圆章。
var All = []Definition{
	{
		ID:          "first-step",
		Name:        "第一步",
		Description: "完成第一次计时。开始了，就已经比昨天近一步。",
		Tier:        TierBronze,
		Unit:        "次",
		Target:      1,
		Value:       func(s Stats) int64 { return int64(s.SessionCount) },
	},
	{
		ID:          "hours-10",
		Name:        "十小时",
		Description: "累计专注满 10 小时。这是把「想做」变成「在做」的第一道坎。",
		Tier:        TierSilver,
		Unit:        "小时",
		Target:      10,
		// 按整小时计，进度条才会一格一格走，而不是每秒钟都在动。
		Value: func(s Stats) int64 { return s.TotalSec / 3600 },
	},
	{
		ID:          "streak-3",
		Name:        "三日不辍",
		Description: "连续 3 天留下记录。看的是一天一天地来，不是一次坐了多久。",
		Tier:        TierGold,
		Unit:        "天",
		Target:      3,
		// 用**最长**连续天数而不是当前连续：后者会归零，会让已经拿到的徽章消失。
		Value: func(s Stats) int64 { return int64(s.LongestStreak) },
	},
}

// Result 是一个成就的评价结果。
type Result struct {
	Definition
	Unlocked bool
	// Progress 是已经达到的量，最多显示到 Target（进度条不必超过 100%）。
	Progress int64
}

// Evaluate 评价全部成就。
func Evaluate(stats Stats) []Result {
	out := make([]Result, 0, len(All))
	for _, def := range All {
		value := def.Value(stats)
		if value < 0 {
			value = 0
		}
		progress := value
		if progress > def.Target {
			progress = def.Target
		}
		out = append(out, Result{
			Definition: def,
			Unlocked:   value >= def.Target,
			Progress:   progress,
		})
	}
	return out
}
