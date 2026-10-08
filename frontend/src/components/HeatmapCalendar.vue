<script setup lang="ts">
import { computed } from "vue";
import IconBack from "./IconBack.vue";
import {
	daysInMonth,
	formatDurationCN,
	monthLabel,
	shiftMonth,
	shortDayLabel,
	weekdayFromMonday,
} from "../composables/format";
import type { DayTotal } from "../composables/useStats";

/**
 * 月历热力图（CSS grid，7 列）。
 *
 * 格子用定高（不是 aspect-ratio）+ minmax(0, 1fr) 自适应：最小窗口下内容区约 490px，
 * 7 列各 ~67px；写成固定像素宽立刻就会撑出横向滚动，而正方形格子会让
 * 一个月的日历高达 500px，把下面的当日明细整段挤出屏幕。
 */
interface DayCell {
	day: number;
	key: string;
}

const props = defineProps<{
	/** YYYY-MM */
	month: string;
	/** 日期键 -> {秒数, 次数}，只有有记录的日子在表里 */
	totals: Record<string, DayTotal>;
	/** 当前选中日 YYYY-MM-DD */
	selected: string;
	/** 今天 YYYY-MM-DD，用来画"今天"的圈 */
	today: string;
}>();

const emit = defineEmits<{
	(e: "update:month", value: string): void;
	(e: "update:selected", value: string): void;
}>();

const weekdays = ["一", "二", "三", "四", "五", "六", "日"];

const cells = computed<(DayCell | null)[]>(() => {
	const total = daysInMonth(props.month);
	const lead = weekdayFromMonday(`${props.month}-01`);
	const out: (DayCell | null)[] = [];
	for (let i = 0; i < lead; i += 1) out.push(null);
	for (let day = 1; day <= total; day += 1) {
		out.push({ day, key: `${props.month}-${String(day).padStart(2, "0")}` });
	}
	return out;
});

const maxTotalInMonth = computed(() => {
	let max = 0;
	for (const cell of cells.value) {
		if (!cell) continue;
		const sec = props.totals[cell.key]?.sec ?? 0;
		if (sec > max) max = sec;
	}
	return max;
});

/**
 * 强度参考值的下限是 1800 秒：不设下限的话，
 * 某天只学了 1 分钟也会被涂成最深的一档（它是当月最大值的 100%）。
 */
const maxRef = computed(() => Math.max(maxTotalInMonth.value, 1800));

function level(key: string): number {
	const sec = props.totals[key]?.sec ?? 0;
	if (sec <= 0) return 0;
	return Math.min(4, Math.max(1, Math.ceil((4 * sec) / maxRef.value)));
}

const currentMonth = computed(() => props.today.slice(0, 7));
/** 未来没有记录，不让往后翻；否则用户会翻进一片永远空白的月份。 */
const canGoNext = computed(() => props.month < currentMonth.value);

function isFuture(key: string): boolean {
	return key > props.today;
}

function cellTitle(cell: DayCell): string {
	const sec = props.totals[cell.key]?.sec ?? 0;
	return sec > 0
		? `${shortDayLabel(cell.key)} · ${formatDurationCN(sec)}`
		: `${shortDayLabel(cell.key)} · 无记录`;
}

function pick(key: string): void {
	emit("update:selected", key);
}
</script>

<template>
	<div class="heatmap">
		<div class="head">
			<span class="month gx-section-title">{{ monthLabel(month) }}</span>
			<span class="gx-spacer"></span>
			<button type="button" class="nav" title="上个月" @click="emit('update:month', shiftMonth(month, -1))">
				<IconBack :size="14" />
			</button>
			<button
				type="button"
				class="nav"
				:disabled="!canGoNext"
				title="下个月"
				@click="emit('update:month', shiftMonth(month, 1))"
			>
				<span class="flip"><IconBack :size="14" /></span>
			</button>
		</div>

		<div class="weekdays" aria-hidden="true">
			<span v-for="name in weekdays" :key="name">{{ name }}</span>
		</div>

		<div class="grid">
			<template v-for="(cell, index) in cells" :key="index">
				<div v-if="!cell" class="cell is-blank" aria-hidden="true"></div>
				<button
					v-else
					type="button"
					class="cell"
					:class="{
						'is-today': cell.key === today,
						'is-selected': cell.key === selected,
						'is-future': isFuture(cell.key),
						'is-strong': level(cell.key) >= 3,
					}"
					:style="{ background: `var(--gx-heat-${level(cell.key)})` }"
					:disabled="isFuture(cell.key)"
					:title="cellTitle(cell)"
					@click="pick(cell.key)"
				>
					<span class="day-num">{{ cell.day }}</span>
				</button>
			</template>
		</div>

		<div class="legend" aria-hidden="true">
			<span class="legend-text">少</span>
			<span v-for="n in 5" :key="n" class="legend-cell" :style="{ background: `var(--gx-heat-${n - 1})` }"></span>
			<span class="legend-text">多</span>
		</div>
	</div>
</template>

<style scoped>
.heatmap {
	display: flex;
	flex-direction: column;
	gap: 6px;
	width: 100%;
	min-width: 0;
}

.head {
	display: flex;
	align-items: center;
	gap: 6px;
}

.month {
	min-width: 0;
}

.nav {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	width: 24px;
	height: 24px;
	padding: 0;
	border: 1px solid var(--gx-glass-2-border);
	border-radius: 50%;
	background: var(--gx-glass-2-bg);
	color: var(--gx-text-2);
	cursor: pointer;
	transition: color var(--gx-dur) var(--gx-ease), border-color var(--gx-dur) var(--gx-ease);
}

.nav:hover:not(:disabled) {
	color: var(--gx-accent);
	border-color: var(--gx-accent-line);
}

.nav:disabled {
	opacity: 0.4;
	cursor: not-allowed;
}

.flip {
	display: inline-flex;
	transform: scaleX(-1);
}

.weekdays,
.grid {
	display: grid;
	grid-template-columns: repeat(7, minmax(0, 1fr));
	gap: 3px;
}

.weekdays {
	font-size: 10px;
	color: var(--gx-text-3);
	text-align: center;
}

/*
 * 格子刻意**不用** aspect-ratio: 1。
 *
 * 正方形格子会让一个月的日历高达 500px 以上，把下面的"当日明细"整段挤出屏幕。
 * 改成定高之后一个月只占约 190px，而且这个高度放得下日期数字——光靠颜色只能
 * 看出深浅，看不出是哪一天。
 *
 * 26px 是配合窗口尺寸压出来的（原来 30px）：记录页是窗口高度的约束来源，
 * 六行格子省下的 24px 直接换成"整页不用滚"。
 */
.cell {
	display: flex;
	align-items: center;
	justify-content: center;
	height: 26px;
	min-width: 0;
	padding: 0;
	border: none;
	border-radius: 7px;
	cursor: pointer;
	transition: transform var(--gx-dur) var(--gx-ease), box-shadow var(--gx-dur) var(--gx-ease);
}

.day-num {
	font-size: 10.5px;
	line-height: 1;
	color: var(--gx-text-3);
	font-variant-numeric: tabular-nums;
}

/* 深档底色上灰字看不清，换成近白色 */
.cell.is-strong .day-num {
	color: rgba(255, 255, 255, 0.94);
}

.cell:not(.is-blank):not(:disabled):hover {
	transform: translateY(-1px);
}

.cell.is-blank {
	cursor: default;
	background: transparent;
}

.cell.is-future {
	cursor: default;
	opacity: 0.55;
}

.cell.is-today {
	box-shadow: inset 0 0 0 1.5px var(--gx-accent-line);
}

/* 选中态写在今天之后：同一天既选中又是今天时，由这条更重的圈接管 */
.cell.is-selected {
	box-shadow: inset 0 0 0 2px var(--gx-accent), 0 0 0 1px var(--gx-accent-line);
}

.legend {
	display: flex;
	align-items: center;
	justify-content: flex-end;
	gap: 3px;
	margin-top: 1px;
}

.legend-text {
	font-size: 10px;
	color: var(--gx-text-3);
	margin: 0 2px;
}

.legend-cell {
	width: 10px;
	height: 10px;
	border-radius: 3px;
}
</style>
