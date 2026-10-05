import { computed, readonly, ref, watch, type ComputedRef, type Ref } from "vue";
import * as RecordsService from "../../bindings/goxuetimer/recordsservice";
import type { DailyTotalDTO } from "../../bindings/goxuetimer/models";
import { addDays, dayRange, daysInMonth, errText, monthOf, shiftMonth, todayKey } from "./format";

/**
 * 记录页的统计选择项（模块级单例）。
 *
 * 区间、计划筛选、选中日期、月历月份都放在这里，而不是各自的组件里：
 *   - 记录页要把它们喂给同一个 DailyTotals / ListByDay，分散存放必然出现
 *     "图是七天、总量是三十天"这种自相矛盾的画面
 *   - useRecords 也要读"选中日 + 筛选计划"来刷新记录列表，只有一份才不会错位
 *
 * 唯一补零的地方也在这里：后端刻意只返回有记录的日子，
 * 如果让柱状图和月历各自补一次，两边的空白天数迟早对不上。
 */

export type RangeDays = 7 | 14 | 30;

/** 柱状图的一天：date 连续、无空洞（缺的日子补零）。 */
export interface DayPoint {
	date: string;
	totalSec: number;
	count: number;
}

const rangeDays = ref<RangeDays>(7);
const filterPlanId = ref(0);
const selectedDate = ref(todayKey());
const month = ref(monthOf(todayKey()));
const dailyTotals = ref<DayPoint[]>([]);
const monthTotals = ref<Record<string, number>>({});
const loading = ref(false);
const error = ref("");

let bound = false;
/**
 * 请求序号。
 *
 * 区间按钮可以连点，两个 DailyTotals 会重叠；没有这个序号时，
 * 先发后到的旧结果会把新结果盖掉（画面回到上一个区间）。
 */
let seq = 0;

async function fetchAll(): Promise<void> {
	const mine = ++seq;
	loading.value = true;
	error.value = "";

	const end = todayKey();
	const start = addDays(end, -(rangeDays.value - 1));
	const planID = filterPlanId.value;

	// 月历要整月数据，可能比区间更宽（用户会翻月）。
	// 未来没有记录，而且 DailyTotals 对 from > to 直接报错，所以要夹到今天为止。
	const m = month.value;
	const monthStart = `${m}-01`;
	const monthEndRaw = `${m}-${String(daysInMonth(m)).padStart(2, "0")}`;
	const monthEnd = monthEndRaw > end ? end : monthEndRaw;

	try {
		const [rangeRows, monthRows] = await Promise.all([
			RecordsService.DailyTotals(start, end, planID),
			monthStart > monthEnd
				? Promise.resolve<DailyTotalDTO[] | null>([])
				: RecordsService.DailyTotals(monthStart, monthEnd, planID),
		]);
		if (mine !== seq) return;

		const byDate: Record<string, DailyTotalDTO> = {};
		for (const row of rangeRows ?? []) byDate[row.date] = row;
		dailyTotals.value = dayRange(start, end).map((date) => ({
			date,
			totalSec: byDate[date]?.totalSec ?? 0,
			count: byDate[date]?.count ?? 0,
		}));

		const totals: Record<string, number> = {};
		for (const row of monthRows ?? []) totals[row.date] = row.totalSec;
		monthTotals.value = totals;
	} catch (err) {
		if (mine === seq) error.value = errText(err);
	} finally {
		if (mine === seq) loading.value = false;
	}
}

export function refreshStats(): Promise<void> {
	return fetchAll();
}

/** 幂等绑定：多次调用 useStats 不会注册多组 watcher。 */
function ensureBound(): void {
	if (bound) return;
	bound = true;
	// 不 immediate：初次加载由记录页自己触发，避免在应用启动时就打多余的一次查询。
	watch([rangeDays, filterPlanId, month], () => {
		void fetchAll();
	});
}

/*
 * 在**模块作用域**里绑定，而不是在 useStats() 里。
 *
 * 这不是风格问题，是一个已经发生过的 bug：useStats() 是在组件的 setup 里调用的，
 * 在那里创建的 watch 属于那个组件实例，Vue 会在组件卸载时自动把它停掉；
 * 而上面的 bound 标志是模块级的，它已经是 true，于是再也不会重新注册。
 * 后果是"离开记录页再回来"之后，改区间 / 改计划筛选 / 月历翻月都只改了按钮高亮，
 * 查询一次都不会再发——图和列表停在旧数据上。
 *
 * 模块作用域没有当前组件实例，watch 不会被自动回收，生命周期与应用一致。
 */
ensureBound();

const rangeTotalSec = computed(() => dailyTotals.value.reduce((sum, p) => sum + p.totalSec, 0));
const rangeCount = computed(() => dailyTotals.value.reduce((sum, p) => sum + p.count, 0));

export function useStats() {
	ensureBound();
	return {
		rangeDays: readonly(rangeDays) as Readonly<Ref<RangeDays>>,
		setRangeDays(days: RangeDays): void {
			rangeDays.value = days;
		},
		filterPlanId: readonly(filterPlanId) as Readonly<Ref<number>>,
		setFilterPlanId(id: number): void {
			filterPlanId.value = id;
		},
		selectedDate: readonly(selectedDate) as Readonly<Ref<string>>,
		setSelectedDate(key: string): void {
			selectedDate.value = key;
		},
		month: readonly(month) as Readonly<Ref<string>>,
		setMonth(value: string): void {
			month.value = value;
		},
		shiftMonth(delta: number): void {
			month.value = shiftMonth(month.value, delta);
		},
		/** 区间内逐日数据，已补零。 */
		dailyTotals: readonly(dailyTotals) as Readonly<Ref<DayPoint[]>>,
		/** 当前显示月份里有记录的日子：日期键 -> 秒数。 */
		monthTotals: readonly(monthTotals) as Readonly<Ref<Record<string, number>>>,
		rangeTotalSec: rangeTotalSec as ComputedRef<number>,
		rangeCount: rangeCount as ComputedRef<number>,
		loading: readonly(loading) as Readonly<Ref<boolean>>,
		error: readonly(error) as Readonly<Ref<string>>,
		refresh: refreshStats,
	};
}

export type UseStatsReturn = ReturnType<typeof useStats>;
