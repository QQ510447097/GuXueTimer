<script setup lang="ts">
import { computed } from "vue";
import { NTooltip } from "naive-ui";
import { formatDurationCN, shortDayLabel } from "../composables/format";

/**
 * 逐日柱状图（纯 CSS，不用 SVG）。
 *
 * 固定 viewBox 的 SVG 会随面板宽度拉伸，柱子粗细与圆角都会变形；
 * 这里用 flex 柱子 + 百分比高度 + 绝对定位的网格线：窗口缩放时柱宽自适应、
 * 高度按比例缩放，圆角与渐变都是免费的。
 */
interface BarPoint {
	date: string;
	totalSec: number;
	count?: number;
}

const props = defineProps<{ points: BarPoint[] }>();

/** 刻度上限只取"人一眼看得懂"的量：15 分、30 分、1 时……而不是 4173 秒。 */
const niceSteps = [
	60, 120, 300, 600, 900, 1200, 1800, 2700, 3600, 5400, 7200, 10800, 14400, 21600, 28800, 43200,
	64800, 86400,
];

const maxSec = computed(() => {
	let max = 0;
	for (const p of props.points) {
		if (p.totalSec > max) max = p.totalSec;
	}
	return max;
});

const niceMax = computed(() => {
	const max = maxSec.value;
	for (const step of niceSteps) {
		if (step >= max) return step;
	}
	return Math.ceil(max / 3600) * 3600;
});

const ticks = computed(() =>
	[1, 2, 3].map((i) => ({
		pct: (i / 3) * 100,
		label: axisLabel((niceMax.value * i) / 3),
	})),
);

function axisLabel(sec: number): string {
	if (sec >= 3600) {
		const hours = sec / 3600;
		return `${Number.isInteger(hours) ? hours : hours.toFixed(1)} 时`;
	}
	return `${Math.round(sec / 60)} 分`;
}

function heightPct(sec: number): number {
	if (sec <= 0) return 0;
	return Math.min(100, (sec / niceMax.value) * 100);
}

/** 14 / 30 天时每根柱子只有十来像素，日期全画会糊成一片，只留首、中、尾。 */
function showLabel(index: number): boolean {
	const total = props.points.length;
	if (total <= 7) return true;
	return index === 0 || index === total - 1 || index === Math.floor((total - 1) / 2);
}

/** 首尾标签贴边对齐：居中的话会各自溢出容器半个标签宽度。 */
function labelClass(index: number): string {
	const total = props.points.length;
	if (total <= 7) return "is-mid";
	if (index === 0) return "is-first";
	if (index === total - 1) return "is-last";
	return "is-mid";
}
</script>

<template>
	<!-- data-points 给 tools/ui_probe.mjs 断言用：区间切换后柱子根数必须真的变，
	     否则"筛选生效了"只能靠肉眼判断。 -->
	<div class="chart" :data-points="points.length">
		<div class="plot">
			<div class="grid" aria-hidden="true">
				<div v-for="tick in ticks" :key="tick.pct" class="grid-line" :style="{ bottom: `${tick.pct}%` }">
					<span class="grid-label gx-tabular">{{ tick.label }}</span>
				</div>
			</div>

			<div class="bars">
				<NTooltip
					v-for="point in points"
					:key="point.date"
					trigger="hover"
					placement="top"
					:delay="60"
					:duration="0"
				>
					<template #trigger>
						<div class="bar-col">
							<div
								class="bar"
								:class="{ 'is-zero': point.totalSec <= 0 }"
								:style="{ height: `${heightPct(point.totalSec)}%` }"
							></div>
						</div>
					</template>
					<div class="tip">
						<div class="tip-title">{{ shortDayLabel(point.date) }}</div>
						<div class="tip-line">
							{{ formatDurationCN(point.totalSec) }} · {{ point.count ?? 0 }} 次
						</div>
					</div>
				</NTooltip>
			</div>
		</div>

		<div class="labels" aria-hidden="true">
			<span
				v-for="(point, index) in points"
				:key="point.date"
				class="label"
				:class="labelClass(index)"
			>{{ showLabel(index) ? shortDayLabel(point.date) : "" }}</span>
		</div>
	</div>
</template>

<style scoped>
/*
 * 这块图要能"长高"：外面那块面板吸收剩余高度，绘图区跟着长。
 * 柱子的高度是百分比，所以图一变高柱子自然变高，不用另算。
 */
.chart {
	display: flex;
	flex-direction: column;
	flex: 1 1 auto;
	min-height: 0;
	width: 100%;
	min-width: 0;
}

/*
 * 左侧 34px 是给刻度文字留的槽。绝对定位的网格线以 padding box 为包含块，
 * 所以 left: 0 仍然贴齐整块图的左边（含槽），文字就落在槽里不压柱子。
 *
 * 高度是弹性的：flex 撑满面板，104px 是下限（还能看清刻度与柱子的高度）。
 * 窗口矮到这个下限就让整页去滚动——记录页在最小窗口下本来就要滚。
 */
.plot {
	position: relative;
	flex: 1 1 auto;
	min-height: 104px;
	padding-left: 34px;
}

.grid {
	position: absolute;
	inset: 0;
	pointer-events: none;
}

.grid-line {
	position: absolute;
	left: 0;
	right: 0;
	height: 0;
	border-top: 1px solid var(--gx-line);
}

.grid-label {
	position: absolute;
	left: 0;
	bottom: 3px;
	font-size: 10px;
	color: var(--gx-text-3);
	white-space: nowrap;
}

/*
 * 柱子的容器必须**绝对定位**，不能靠 `height: 100%`。
 *
 * 绘图区的高度现在是弹性来的（flex），它的 height 属性是 auto——
 * 百分比高度在"高度不确定"的父元素里解析不了，会退化成 auto，
 * 于是柱子只剩 min-height 那 3px，整张图变成顶端几条细线（实测踩到过）。
 * 绝对定位把上下边钉在绘图区上，高度就是确定的，百分比才成立。
 *
 * left: 34px 与 .plot 的 padding-left 对齐：绝对定位的包含块是 padding box
 * （从边框内侧算），所以这个 34px 正好落在刻度槽的右边缘，和下面的日期标签对齐。
 */
.bars {
	position: absolute;
	top: 0;
	bottom: 0;
	left: 34px;
	right: 0;
	display: flex;
	gap: 2px;
	min-width: 0;
}

.bar-col {
	flex: 1 1 0;
	display: flex;
	flex-direction: column;
	justify-content: flex-end;
	min-width: 0;
	height: 100%;
}

.bar {
	width: 100%;
	/* 零值也要留一个底座：整列"存在但为空"比整列消失更好读。
	   3px 而不是 2px——2px 在 132px 高的图里实测几乎看不见，等于没画。 */
	min-height: 3px;
	border-radius: 6px 6px 2px 2px;
	background: var(--gx-accent-grad);
	opacity: 0.9;
	transition: opacity var(--gx-dur) var(--gx-ease);
}

.bar-col:hover .bar {
	opacity: 1;
}

.bar.is-zero {
	background: var(--gx-track);
	opacity: 1;
}

.labels {
	display: flex;
	gap: 2px;
	margin-top: 6px;
	padding-left: 34px;
}

.label {
	flex: 1 1 0;
	min-width: 0;
	font-size: 10px;
	color: var(--gx-text-3);
	white-space: nowrap;
}

.label.is-mid {
	text-align: center;
}

.label.is-first {
	text-align: left;
}

.label.is-last {
	text-align: right;
}

.tip {
	display: flex;
	flex-direction: column;
	gap: 1px;
}

.tip-title {
	font-size: 12px;
	font-weight: 600;
}

.tip-line {
	font-size: 12px;
	color: var(--gx-text-2);
}
</style>
