<script setup lang="ts">
import { computed, onMounted, watch } from "vue";
import { useMessage } from "naive-ui";
import BarChart from "../components/BarChart.vue";
import GlassPanel from "../components/GlassPanel.vue";
import HeatmapCalendar from "../components/HeatmapCalendar.vue";
import PlanChip from "../components/PlanChip.vue";
import SessionRow from "../components/SessionRow.vue";
import { dayLabel, formatDurationCN, todayKey } from "../composables/format";
import { usePlans } from "../composables/usePlans";
import { refreshRecords, useRecords } from "../composables/useRecords";
import { useStats, type RangeDays } from "../composables/useStats";

/**
 * 记录页。
 *
 * "选中哪天 / 筛哪个计划 / 看多长的区间"全在 useStats 里；这里只负责
 * 按那个选择取当日明细并把结果画出来。两处各存一份选择，
 * 必然出现「图是近七天、列表却是别的日子」这种自相矛盾的画面。
 */
const { plans, nameOf, colorOf } = usePlans();
const {
	rangeDays,
	setRangeDays,
	filterPlanId,
	setFilterPlanId,
	selectedDate,
	setSelectedDate,
	month,
	setMonth,
	dailyTotals,
	monthTotals,
	rangeTotalSec,
	rangeCount,
	error: statsError,
	refresh: refreshStats,
} = useStats();
const { sessions, summary, loading: dayLoading, error: dayError, stale, remove } = useRecords();
const message = useMessage();

const ranges: { days: RangeDays; label: string }[] = [
	{ days: 7, label: "近 7 天" },
	{ days: 14, label: "近 14 天" },
	{ days: 30, label: "近 30 天" },
];

// 渲染期取一次就够：一直开着跨过午夜属于边角情况，那时后端会用空串自行判定今天。
const today = todayKey();

// 换"选中日"只影响当日明细；区间与月份由 useStats 自己的 watcher 负责重取。
watch([selectedDate, filterPlanId], () => {
	void refreshRecords();
});

onMounted(() => {
	void refreshRecords();
	void refreshStats();
});

const dayTotalSec = computed(() => summary.value?.totalSec ?? 0);
const dayCount = computed(() => summary.value?.count ?? 0);

/** 当日按计划拆分，按时长降序：这一栏看的是"时间花在哪了"。 */
const breakdown = computed(() => {
	const byPlan = new Map<number, number>();
	for (const session of sessions.value) {
		byPlan.set(session.planId, (byPlan.get(session.planId) ?? 0) + session.durationSec);
	}
	let total = 0;
	for (const sec of byPlan.values()) total += sec;
	const rows = Array.from(byPlan.entries()).map(([planId, sec]) => ({
		planId,
		sec,
		name: nameOf(planId),
		color: colorOf(planId),
	}));
	rows.sort((a, b) => b.sec - a.sec);
	return { rows, total };
});

const isEmpty = computed(() => !stale.value && !dayLoading.value && sessions.value.length === 0);

async function onRemove(id: number): Promise<void> {
	const ok = await remove(id);
	if (ok) message.success("已删除这条记录");
	else message.error(dayError.value || "删除失败");
}
</script>

<template>
	<section class="records-view gx-col">
		<GlassPanel padding="12px 16px">
			<div class="head">
				<div class="ranges">
					<button
						v-for="range in ranges"
						:key="range.days"
						type="button"
						class="range gx-ghost gx-pill"
						:class="{ 'is-active': rangeDays === range.days }"
						@click="setRangeDays(range.days)"
					>
						{{ range.label }}
					</button>
				</div>
				<div class="total">
					<span class="total-value gx-tabular">{{ formatDurationCN(rangeTotalSec) }}</span>
					<span class="total-hint gx-faint">{{ rangeCount }} 次</span>
				</div>
			</div>

			<div class="filters">
				<PlanChip
					label="全部"
					:selected="filterPlanId === 0"
					color="var(--gx-accent)"
					@click="setFilterPlanId(0)"
				/>
				<PlanChip
					v-for="plan in plans"
					:key="plan.id"
					:label="plan.name"
					:color="plan.color"
					:selected="filterPlanId === plan.id"
					@click="setFilterPlanId(plan.id)"
				/>
			</div>
		</GlassPanel>

		<GlassPanel padding="14px 16px">
			<div class="section-head">
				<span class="gx-section-title">每日时长</span>
				<span class="gx-faint">近 {{ rangeDays }} 天</span>
			</div>
			<BarChart :points="dailyTotals" />

			<div class="divider"></div>

			<HeatmapCalendar
				:month="month"
				:totals="monthTotals"
				:selected="selectedDate"
				:today="today"
				@update:month="setMonth"
				@update:selected="setSelectedDate"
			/>
		</GlassPanel>

		<GlassPanel padding="14px 16px">
			<div class="day-head">
				<div class="day-titles">
					<span class="day-title">{{ dayLabel(selectedDate) }}</span>
					<span class="gx-faint">
						共 {{ formatDurationCN(dayTotalSec) }} · {{ dayCount }} 次
					</span>
				</div>
				<span class="gx-spacer"></span>
				<span v-if="selectedDate === today" class="today-tag">今天</span>
			</div>

			<div v-if="breakdown.rows.length" class="stack" aria-hidden="true">
				<span
					v-for="row in breakdown.rows"
					:key="row.planId"
					class="seg"
					:style="{ flexGrow: row.sec, background: row.color }"
				></span>
			</div>

			<div v-if="breakdown.rows.length" class="legend">
				<div v-for="row in breakdown.rows" :key="row.planId" class="legend-row">
					<span class="gx-dot" :style="{ background: row.color, color: row.color }"></span>
					<span class="legend-name gx-truncate">{{ row.name }}</span>
					<span class="gx-spacer"></span>
					<span class="legend-sec gx-mono gx-tabular">{{ formatDurationCN(row.sec) }}</span>
				</div>
			</div>

			<div class="list">
				<SessionRow
					v-for="session in sessions"
					:key="session.id"
					:session="session"
					@remove="onRemove"
				/>
			</div>

			<div v-if="isEmpty" class="empty">
				<p class="empty-title">这一天还没有记录</p>
				<p class="gx-faint">回到「计时」开始一次学习</p>
			</div>

			<p v-if="dayError" class="error">{{ dayError }}</p>
			<p v-if="statsError" class="error">{{ statsError }}</p>
		</GlassPanel>
	</section>
</template>

<style scoped>
.records-view {
	gap: var(--gx-gap);
}

.head {
	display: flex;
	align-items: center;
	gap: 10px;
	min-width: 0;
	/* 窄窗口 + 超大累计时长时宁可让总量换行，也不要横向溢出 */
	flex-wrap: wrap;
}

.ranges {
	display: flex;
	flex-wrap: wrap;
	gap: 6px;
	min-width: 0;
}

.range {
	padding: 5px 11px;
	font-size: 12.5px;
	font-family: inherit;
	cursor: pointer;
}

.total {
	display: flex;
	flex-direction: column;
	align-items: flex-end;
	margin-left: auto;
	white-space: nowrap;
}

.total-value {
	font-size: 16px;
	font-weight: 650;
	color: var(--gx-text);
}

.total-hint {
	font-size: 11px;
}

.filters {
	display: flex;
	flex-wrap: wrap;
	gap: 7px;
	margin-top: 11px;
	padding-top: 11px;
	border-top: 1px solid var(--gx-line);
}

/* 计划名可能很长：色块宽度跟着内容走，给它一个上限，免得一个筛选芯片独占一整行 */
.filters :deep(.chip) {
	max-width: 132px;
}

.section-head {
	display: flex;
	align-items: baseline;
	justify-content: space-between;
	gap: 10px;
	margin-bottom: 10px;
	font-size: 11.5px;
}

.divider {
	height: 1px;
	margin: 16px 0 14px;
	background: var(--gx-line);
}

.day-head {
	display: flex;
	align-items: center;
	gap: 10px;
	margin-bottom: 12px;
}

.day-titles {
	display: flex;
	flex-direction: column;
	gap: 1px;
	min-width: 0;
}

.day-title {
	font-size: 14.5px;
	font-weight: 650;
}

.today-tag {
	flex: 0 0 auto;
	padding: 1px 9px;
	border-radius: var(--gx-r-pill);
	background: var(--gx-accent-soft);
	color: var(--gx-accent);
	font-size: 11px;
}

.stack {
	display: flex;
	height: 8px;
	border-radius: var(--gx-r-pill);
	overflow: hidden;
	background: var(--gx-track);
}

.seg {
	flex: 1 1 0;
	min-width: 3px;
}

.legend {
	display: flex;
	flex-direction: column;
	gap: 5px;
	margin-top: 10px;
}

.legend-row {
	display: flex;
	align-items: center;
	gap: 8px;
	min-width: 0;
	font-size: 12.5px;
}

.legend-name {
	min-width: 0;
}

.legend-sec {
	color: var(--gx-text-2);
	white-space: nowrap;
}

.list {
	display: flex;
	flex-direction: column;
	gap: 7px;
	margin-top: 12px;
}

.empty {
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: 2px;
	padding: 26px 10px 18px;
	text-align: center;
}

.empty-title {
	font-size: 13.5px;
	color: var(--gx-text-2);
}

.error {
	margin-top: 10px;
	font-size: 12.5px;
	color: var(--gx-danger);
}
</style>
