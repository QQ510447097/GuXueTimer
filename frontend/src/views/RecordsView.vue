<script setup lang="ts">
import { computed, onMounted } from "vue";
import BarChart from "../components/BarChart.vue";
import GlassPanel from "../components/GlassPanel.vue";
import HeatmapCalendar from "../components/HeatmapCalendar.vue";
import PlanChip from "../components/PlanChip.vue";
import StatTile from "../components/StatTile.vue";
import { formatDurationCN, monthLabel, shortDayLabel, todayKey } from "../composables/format";
import { useAchievements } from "../composables/useAchievements";
import { usePlans } from "../composables/usePlans";
import { useStats, type StatsRange } from "../composables/useStats";

/**
 * 记录页。
 *
 * 三块：区间工具栏（本周/本月 + 计划筛选）、柱状图 + 月历、累计统计。
 *
 * "看多长的区间 / 筛哪个计划 / 月历翻到哪个月"全在 useStats 里。两处各存一份选择，
 * 必然出现「图是本周、日历却是别的月份」这种自相矛盾的画面。
 *
 * 累计统计（累计时长、场次、天数、单次最长、连续天数）来自**成就服务**：
 * 那几个数字本来就是它为了评价徽章而算的，同一份聚合没必要在记录服务里再写一遍。
 * 它只读，不参与成就判定——成就页现在只有徽章网格，统计归记录页。
 */
const { plans } = usePlans();
const {
	range,
	setRange,
	filterPlanId,
	setFilterPlanId,
	selectedDate,
	setSelectedDate,
	month,
	setMonth,
	chartPoints,
	chartStart,
	chartEnd,
	monthTotals,
	rangeTotalSec,
	rangeCount,
	error: statsError,
	refresh: refreshStats,
} = useStats();
// lifetime 就是"历史累计"：起这个名字是因为统计口径是全部记录，不是本周/本月。
const { summary: lifetime, error: lifetimeError, refresh: refreshLifetime } = useAchievements();

const ranges: { key: StatsRange; label: string }[] = [
	{ key: "week", label: "本周" },
	{ key: "month", label: "本月" },
];

// 图表区间的标题：本周写清这七天的起止，本月直接写月份（图表就是月历那个月）。
const periodLabel = computed(() => {
	if (range.value === "month") return monthLabel(month.value);
	if (!chartStart.value || !chartEnd.value) return "本周";
	return `本周（${shortDayLabel(chartStart.value)} – ${shortDayLabel(chartEnd.value)}）`;
});

// 渲染期取一次就够：一直开着跨过午夜属于边角情况，那时后端会用空串自行判定今天。
const today = todayKey();

onMounted(() => {
	void refreshStats();
	void refreshLifetime();
});
</script>

<template>
	<section class="records-view gx-col">
		<GlassPanel padding="12px 16px">
			<div class="head">
				<div class="ranges">
					<button
						v-for="item in ranges"
						:key="item.key"
						type="button"
						class="range gx-ghost gx-pill"
						:class="{ 'is-active': range === item.key }"
						:data-range="item.key"
						@click="setRange(item.key)"
					>
						{{ item.label }}
					</button>
				</div>
				<!-- 区间标题并进这一行，不再单独占一格：
				     :data-period 要把值绑上，写成无值的 data-period 时 dataset.period
				     是空串，tools/ui_probe.mjs 就断言不到区间切换了 -->
				<span class="period gx-faint" :data-period="periodLabel">{{ periodLabel }}</span>
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

		<!-- 顶部 20px 而不是 14px：最高的那条刻度线就在绘图区顶端，它的文字有一半在
		     绘图区之上，贴着面板边缘会显得"顶到头了" -->
		<GlassPanel padding="20px 15px 14px">
			<BarChart :points="chartPoints" />

			<div class="divider"></div>

			<HeatmapCalendar
				:month="month"
				:totals="monthTotals"
				:selected="selectedDate"
				:today="today"
				@update:month="setMonth"
				@update:selected="setSelectedDate"
			/>

			<p v-if="statsError" class="error">{{ statsError }}</p>
		</GlassPanel>

		<!-- 累计统计：历史全部记录的口径（不是本周/本月）。
		     不加"累计"小标题——下面那行的"累计专注"已经说清了，两个"累计"叠着看很啰嗦 -->
		<GlassPanel padding="14px 16px">
			<div class="totals">
				<div class="totals-text">
					<span class="totals-main gx-tabular">
						累计专注 {{ formatDurationCN(lifetime.totalSec) }}
					</span>
					<span class="gx-faint">
						{{ lifetime.sessions }} 次 · {{ lifetime.activeDays }} 天有记录 ·
						单次最长 {{ formatDurationCN(lifetime.longestSec) }}
					</span>
					<span v-if="lifetime.earlyMorningCount || lifetime.lateNightCount" class="gx-faint">
						<span v-if="lifetime.earlyMorningCount">
							早起开始过 {{ lifetime.earlyMorningCount }} 次
						</span>
						<span v-if="lifetime.earlyMorningCount && lifetime.lateNightCount"> · </span>
						<span v-if="lifetime.lateNightCount">
							深夜开始过 {{ lifetime.lateNightCount }} 次
						</span>
					</span>
				</div>
				<div class="tiles">
					<StatTile label="当前连续" :value="`${lifetime.currentStreak} 天`" accent />
					<StatTile label="最长连续" :value="`${lifetime.longestStreak} 天`" />
				</div>
			</div>

			<p v-if="lifetimeError" class="error">{{ lifetimeError }}</p>
		</GlassPanel>
	</section>
</template>

<style scoped>
.records-view {
	/* 12px 而不是全局的 14：整页高度是按"一屏放得下"量出来的，间距也算在内的 */
	gap: 12px;
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

/* 区间标题跟着胶囊走，不再单独占一行：省下的高度直接换成"整页不用滚" */
.period {
	font-size: 11.5px;
	white-space: nowrap;
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
	margin-top: 9px;
	padding-top: 9px;
	border-top: 1px solid var(--gx-line);
}

/* 计划名可能很长：色块宽度跟着内容走，给它一个上限，免得一个筛选芯片独占一整行 */
.filters :deep(.chip) {
	max-width: 132px;
}

.divider {
	height: 1px;
	margin: 10px 0 8px;
	background: var(--gx-line);
}

.totals {
	display: flex;
	align-items: center;
	gap: 14px;
	flex-wrap: wrap;
	margin-top: 10px;
}

.totals-text {
	display: flex;
	flex-direction: column;
	gap: 2px;
	flex: 1 1 auto;
	min-width: 0;
}

.totals-main {
	font-size: 16px;
	font-weight: 650;
	color: var(--gx-text);
}

.totals-text .gx-faint {
	font-size: 12px;
	line-height: 1.6;
}

.tiles {
	display: flex;
	gap: 10px;
	flex: 0 0 auto;
}

.error {
	margin-top: 10px;
	font-size: 12.5px;
	color: var(--gx-danger);
}
</style>
