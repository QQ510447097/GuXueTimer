import { computed, readonly, ref, watch, type ComputedRef, type Ref } from "vue";
import * as RecordsService from "../../bindings/goxuetimer/recordsservice";
import type { DailyTotalDTO } from "../../bindings/goxuetimer/models";
import {
	addDays,
	dayRange,
	daysInMonth,
	errText,
	monthOf,
	shiftMonth,
	todayKey,
	weekStart,
} from "./format";

/**
 * 记录页的统计选择项（模块级单例）。
 *
 * 区间、计划筛选、选中日期、月历月份都放在这里，而不是各自的组件里：
 * 图、月历、区间合计数都来自同一份选择，分散存放必然出现
 * "图是本周、总量却是本月"这种自相矛盾的画面。
 *
 * 唯一补零的地方也在这里：后端刻意只返回有记录的日子，
 * 如果让柱状图和月历各自补一次，两边的空白天数迟早对不上。
 */

/**
 * 柱状图的区间。
 *
 * 只有"本周 / 本月"两种，不是"近 7 / 14 / 30 天"：
 * 后者与下方月历是两个互不相干的坐标系（"近 14 天"横跨两个月，而月历只画一个月），
 * 用户看不出两者怎么对应。改成本周/本月之后，**本月直接就是月历当前显示的那个月**，
 * 图与日历永远讲同一件事，还能复用同一份查询。
 */
export type StatsRange = "week" | "month";

/** 柱状图的一天：date 连续、无空洞（缺的日子补零）。 */
export interface DayPoint {
	date: string;
	totalSec: number;
	count: number;
}

/** 月历一天的数据：热力图要秒数，柱状图 tooltip 要次数。 */
export interface DayTotal {
	sec: number;
	count: number;
}

const range = ref<StatsRange>("week");
const filterPlanId = ref(0);
const selectedDate = ref(todayKey());
const month = ref(monthOf(todayKey()));
const chartPoints = ref<DayPoint[]>([]);
const monthTotals = ref<Record<string, DayTotal>>({});
const loading = ref(false);
const error = ref("");

/** 本次图表区间的起止（已夹到今天）。标题要用它，所以暴露出去。 */
const chartStart = ref("");
const chartEnd = ref("");

let seq = 0;

async function fetchAll(): Promise<void> {
	const mine = ++seq;
	loading.value = true;
	error.value = "";

	const today = todayKey();
	const planID = filterPlanId.value;
	const m = month.value;

	// 月历是整月，可能比图表区间更宽（用户会翻月）；未来没有记录，夹到今天。
	const monthStart = `${m}-01`;
	const monthEndRaw = `${m}-${String(daysInMonth(m)).padStart(2, "0")}`;
	const monthEnd = monthEndRaw > today ? today : monthEndRaw;

	// 本月模式下图表与月历是同一段区间，只需要查一次。
	const isMonth = range.value === "month";
	// 本周固定画周一到周日七格（而不是"周一到今天"）：周一打开时只到今天会只剩
	// 一根柱子，柱数每天变来变去也不好比较。未来的日子自然没有记录，是 0。
	const start = isMonth ? monthStart : weekStart(today);
	const end = isMonth ? monthEnd : addDays(weekStart(today), 6);
	// 用户把月历翻到未来月份时 start > end，DailyTotals 对这种入参会直接报错，所以先挡掉。
	const valid = start <= end;

	try {
		const [chartRows, monthRows] = await Promise.all([
			valid
				? RecordsService.DailyTotals(start, end, planID)
				: Promise.resolve<DailyTotalDTO[] | null>([]),
			// 本月模式下月历复用图表那份数据，不再重复查。
			isMonth || !valid
				? Promise.resolve<DailyTotalDTO[] | null>(null)
				: RecordsService.DailyTotals(monthStart, monthEnd, planID),
		]);
		if (mine !== seq) return;

		const chartByDate: Record<string, DailyTotalDTO> = {};
		for (const row of chartRows ?? []) chartByDate[row.date] = row;
		chartPoints.value = valid
			? dayRange(start, end).map((date) => ({
					date,
					totalSec: chartByDate[date]?.totalSec ?? 0,
					count: chartByDate[date]?.count ?? 0,
				}))
			: [];
		chartStart.value = valid ? start : "";
		chartEnd.value = valid ? end : "";

		const totals: Record<string, DayTotal> = {};
		for (const row of (isMonth ? chartRows : monthRows) ?? []) {
			totals[row.date] = { sec: row.totalSec, count: row.count };
		}
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
	watch([range, filterPlanId, month], () => {
		void fetchAll();
	});
}

/*
 * 在**模块作用域**里绑定，而不是在 useStats() 里。
 *
 * 这不是风格问题，是一个已经发生过的 bug：useStats() 是在组件的 setup 里调用的，
 * 在那里创建的 watch 属于那个组件实例，Vue 会在组件卸载时自动把它停掉；
 * 而 bound 标志是模块级的，它已经是 true，于是再也不会重新注册。
 * 后果是"离开记录页再回来"之后，改区间 / 改计划筛选 / 月历翻月都只改了按钮高亮，
 * 查询一次都不会再发——图和列表停在旧数据上。
 *
 * 模块作用域没有当前组件实例，watch 不会被自动回收，生命周期与应用一致。
 */
ensureBound();

const rangeTotalSec = computed(() => chartPoints.value.reduce((sum, p) => sum + p.totalSec, 0));
const rangeCount = computed(() => chartPoints.value.reduce((sum, p) => sum + p.count, 0));

export function useStats() {
	return {
		range: readonly(range) as Readonly<Ref<StatsRange>>,
		setRange(next: StatsRange): void {
			range.value = next;
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
		/** 图表区间内逐日数据，已补零。 */
		chartPoints: readonly(chartPoints) as Readonly<Ref<DayPoint[]>>,
		/** 图表区间的起止日期键（本周是周一到周日）。 */
		chartStart: readonly(chartStart) as Readonly<Ref<string>>,
		chartEnd: readonly(chartEnd) as Readonly<Ref<string>>,
		/** 当前显示月份里有记录的日子：日期键 -> {秒数, 次数}。 */
		monthTotals: readonly(monthTotals) as Readonly<Ref<Record<string, DayTotal>>>,
		rangeTotalSec: rangeTotalSec as ComputedRef<number>,
		rangeCount: rangeCount as ComputedRef<number>,
		loading: readonly(loading) as Readonly<Ref<boolean>>,
		error: readonly(error) as Readonly<Ref<string>>,
		refresh: refreshStats,
	};
}

export type UseStatsReturn = ReturnType<typeof useStats>;
