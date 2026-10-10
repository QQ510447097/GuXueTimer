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

// All 是全部成就。
//
// **现在是空的**：最早那三条（第一步 / 十小时 / 三日不辍）是占位，用来验证
// "徽章完全由记录推导"这条链路能不能跑通，已经按设计清掉了，等着换成正式那套。
// 因此成就页现在是一面空墙——这是有意的状态，不是坏了。
//
// 加新成就就是往这里追加一条 Definition：前端的徽章图按 ID 命名
// （frontend/public/badges/<id>.png），没有图会回退到内置的矢量奖章。
// 判据必须单调不减（理由见文件头）。
var All = []Definition{}

// Result 是一个成就的评价结果。
type Result struct {
	Definition
	Unlocked bool
	// Progress 是已经达到的量，最多显示到 Target（进度条不必超过 100%）。
	Progress int64
}

// Evaluate 用当前的成就表评价（服务层走这条）。
func Evaluate(stats Stats) []Result {
	return EvaluateWith(All, stats)
}

// EvaluateWith 用指定的成就表评价。
//
// 与 Evaluate 分开是为了能测：All 清空之后，测试不该依赖于"里面恰好有哪几条"，
// 而应该自己造几条来验框架本身（门槛、封顶、单调性）。
func EvaluateWith(defs []Definition, stats Stats) []Result {
	out := make([]Result, 0, len(defs))
	for _, def := range defs {
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
