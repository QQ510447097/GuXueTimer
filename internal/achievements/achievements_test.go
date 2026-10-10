package achievements

import "testing"

/*
 * All 现在是有意留空的（旧的三条是占位，已清掉），所以这一组测试**自己造成就**
 * 来验框架本身。这样做的另一个好处是：往 All 里加正式成就时，这些用例不受影响。
 *
 * 三条合成成就刚好代表三种形态：布尔（做过没有）、累计（攒够多少）、连续（连着几天）。
 */
func synthetic() []Definition {
	return []Definition{
		{
			ID: "did-it", Name: "做过", Description: "至少一次", Tier: TierBronze,
			Unit: "次", Target: 1,
			Value: func(s Stats) int64 { return int64(s.SessionCount) },
		},
		{
			ID: "ten", Name: "十单位", Description: "攒够十个", Tier: TierSilver,
			Unit: "个", Target: 10,
			Value: func(s Stats) int64 { return s.TotalSec / 3600 },
		},
		{
			ID: "streak", Name: "三天", Description: "连着三天", Tier: TierGold,
			Unit: "天", Target: 3,
			Value: func(s Stats) int64 { return int64(s.LongestStreak) },
		},
	}
}

func TestEvaluate_Thresholds(t *testing.T) {
	cases := []struct {
		name      string
		stats     Stats
		wantID    string
		wantOpen  bool
		wantProg  int64
		wantTarge int64
	}{
		{"一次都没有", Stats{}, "did-it", false, 0, 1},
		{"记过一次就解锁", Stats{SessionCount: 1}, "did-it", true, 1, 1},
		{"差一点", Stats{TotalSec: 10*3600 - 1}, "ten", false, 9, 10},
		{"刚好够", Stats{TotalSec: 10 * 3600}, "ten", true, 10, 10},
		{"超量后进度封顶", Stats{TotalSec: 40 * 3600}, "ten", true, 10, 10},
		{"连续两天还不够", Stats{LongestStreak: 2}, "streak", false, 2, 3},
		{"连续三天解锁", Stats{LongestStreak: 3}, "streak", true, 3, 3},
	}

	defs := synthetic()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var found bool
			for _, r := range EvaluateWith(defs, c.stats) {
				if r.ID != c.wantID {
					continue
				}
				found = true
				if r.Unlocked != c.wantOpen {
					t.Errorf("%s 解锁 = %v, 期望 %v", c.wantID, r.Unlocked, c.wantOpen)
				}
				if r.Progress != c.wantProg {
					t.Errorf("%s 进度 = %d, 期望 %d", c.wantID, r.Progress, c.wantProg)
				}
				if r.Target != c.wantTarge {
					t.Errorf("%s 门槛 = %d, 期望 %d", c.wantID, r.Target, c.wantTarge)
				}
			}
			if !found {
				t.Fatalf("没有找到成就 %s", c.wantID)
			}
		})
	}
}

// 进度不能超过门槛：进度条按百分比画，超过 100% 会溢出。
func TestEvaluate_ProgressNeverExceedsTarget(t *testing.T) {
	stats := Stats{TotalSec: 9999 * 3600, SessionCount: 5000, LongestStreak: 900}
	for _, r := range EvaluateWith(synthetic(), stats) {
		if r.Progress > r.Target {
			t.Errorf("%s 进度 %d 超过了门槛 %d", r.ID, r.Progress, r.Target)
		}
		if !r.Unlocked {
			t.Errorf("%s 在超量统计下仍未解锁", r.ID)
		}
	}
}

// 负数（理论上不该出现，但库里的值可能坏掉）按 0 处理，不能画出负进度。
func TestEvaluate_NegativeStatsClampToZero(t *testing.T) {
	negative := Stats{TotalSec: -100, SessionCount: -5, LongestStreak: -3}
	for _, r := range EvaluateWith(synthetic(), negative) {
		if r.Progress < 0 {
			t.Errorf("%s 进度 = %d, 不能是负数", r.ID, r.Progress)
		}
		if r.Unlocked {
			t.Errorf("%s 在负统计下解锁了", r.ID)
		}
	}
}

/*
守一条硬约束：判据必须单调不减。

成就没有持久化（完全由 sessions 推导），所以一旦判据会往回走，用户就会
"徽章突然消失"。这里逐个检查：把所有统计量调大之后，已解锁的成就不能重新锁上。
*/
func TestEvaluate_Monotonic(t *testing.T) {
	defs := synthetic()
	small := Stats{TotalSec: 5 * 3600, SessionCount: 3, LongestStreak: 2}
	big := Stats{TotalSec: 50 * 3600, SessionCount: 30, LongestStreak: 20}

	lockedBefore := map[string]bool{}
	for _, r := range EvaluateWith(defs, small) {
		lockedBefore[r.ID] = r.Unlocked
	}
	for _, r := range EvaluateWith(defs, big) {
		if lockedBefore[r.ID] && !r.Unlocked {
			t.Errorf("%s 在统计量变大之后反而锁上了——判据不是单调的", r.ID)
		}
	}
}

/*
成就表本身的规范检查。

All 现在是空的，所以这条断言眼下是"空转"的——它的用处是**等正式成就加进来时**
立刻把 ID 重复、缺名字、门槛非正、Tier 写错这类问题挡下来。已经在 All 里的
每条都会逐字段检查，请别把它删掉。
*/
func TestDefinitions_AreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, def := range All {
		if def.ID == "" {
			t.Error("有成就没写 ID")
		}
		if seen[def.ID] {
			t.Errorf("成就 ID 重复: %s", def.ID)
		}
		seen[def.ID] = true
		if def.Name == "" || def.Description == "" {
			t.Errorf("%s 缺少名称或说明", def.ID)
		}
		if def.Target <= 0 {
			t.Errorf("%s 的门槛应当为正数, 实际 %d", def.ID, def.Target)
		}
		if def.Value == nil {
			t.Errorf("%s 没有取值函数", def.ID)
		}
		switch def.Tier {
		case TierBronze, TierSilver, TierGold:
		default:
			t.Errorf("%s 的稀有度取值不认识: %q", def.ID, def.Tier)
		}
	}

	// 同一份规范检查也用在合成成就上，免得它本身写错了却测不出来。
	if len(synthetic()) == 0 {
		t.Error("合成成就为空，上面那些用例其实什么都没验")
	}
}
