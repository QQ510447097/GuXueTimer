<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useDialog, useMessage } from "naive-ui";
import * as PlansService from "../../bindings/goxuetimer/plansservice";
import * as RecordsService from "../../bindings/goxuetimer/recordsservice";
import type { PlanDTO } from "../../bindings/goxuetimer/models";
import ClockDisplay from "../components/ClockDisplay.vue";
import GlassPanel from "../components/GlassPanel.vue";
import IconDiscard from "../components/IconDiscard.vue";
import IconPause from "../components/IconPause.vue";
import IconPlay from "../components/IconPlay.vue";
import IconStop from "../components/IconStop.vue";
import PlanPicker from "../components/PlanPicker.vue";
import StatTile from "../components/StatTile.vue";
import {
	errText,
	formatClock,
	formatCountdownCN,
	formatDurationCN,
	formatTodayLabel,
} from "../composables/format";
import { useNav } from "../composables/useNav";
import { usePersonalization } from "../composables/usePersonalization";
import { usePlans } from "../composables/usePlans";
import { useSettings } from "../composables/useSettings";
import {
	discardTimer,
	pauseTimer,
	pullState,
	resumeTimer,
	startTimer,
	stopTimer,
	useTimer,
	type TimerState,
} from "../composables/useTimer";

const { state, now, busy, lastError, elapsedSec, todayTotalSec, breakSec } = useTimer();
const { activePlans, nameOf, refresh: refreshPlans } = usePlans();
const { settings } = useSettings();
const { imageUrl, load: loadBackground } = usePersonalization();
const { selectView } = useNav();
const dialog = useDialog();
const message = useMessage();

/*
 * 最下方那块区域。
 *
 * 显示什么完全由个性化页决定：留空（默认）就什么都不放，图片就铺一张封面。
 * 这段原来是一个独立组件（里面分语录与图片两支），语录删掉之后只剩一个使用方，
 * 就并回这里了——将来做「其他组件」时再拆出真正的组件，那时它才有多个实现可切。
 */
const cardMode = computed(() => (settings.value.cardMode === "image" ? "image" : "none"));
const showImage = computed(() => cardMode.value === "image" && imageUrl.value !== "");

onMounted(() => {
	void loadBackground();
});

/** 图片模式下还没选图时的入口：切到个性化页去挑一张。 */
function goPersonalize(): void {
	selectView("personalize");
}

const switching = ref(false);
const planTodaySec = ref(0);
const todayCount = ref(0);

const timerState = computed<TimerState>(() => {
	const raw = state.value?.state;
	return raw === "running" || raw === "paused" ? raw : "idle";
});
const isIdle = computed(() => timerState.value === "idle");
const isRunning = computed(() => timerState.value === "running");
const currentPlanId = computed(() => state.value?.planId ?? 0);
const planHint = computed(() => (currentPlanId.value ? nameOf(currentPlanId.value) : "未选择计划"));

const clockText = computed(() => formatClock(elapsedSec.value));
const todayCaption = computed(() => `今日 ${formatTodayLabel(now.value)}`);

const primaryLabel = computed(() => {
	if (isRunning.value) return "暂停";
	if (timerState.value === "paused") return "继续";
	return "开始学习";
});

const reminderOff = computed(() => breakSec.value.interval <= 0);
const breakPercent = computed(() => {
	const { elapsed, interval } = breakSec.value;
	if (interval <= 0) return 0;
	return Math.min(100, Math.max(0, (elapsed / interval) * 100));
});
const breakRemainSec = computed(() =>
	Math.max(0, breakSec.value.interval - breakSec.value.elapsed),
);
const breakIntervalMin = computed(() => Math.round(breakSec.value.interval / 60));

/**
 * 「本计划今日」与「今日场次」来自记录接口，不在计时状态里。
 *
 * 只在后端状态真的变了（换计划 / 落库时长变化）时才重查：
 * todayTotalSec 每 250ms 都会因本地时钟推进而变，盯着它会让请求满天飞。
 * 统计查失败也刻意不弹提示：它是装饰性的，不该打断正在进行的计时。
 */
async function loadCounters(): Promise<void> {
	const planID = currentPlanId.value;
	try {
		const [all, own] = await Promise.all([
			RecordsService.DaySummary("", 0),
			RecordsService.DaySummary("", planID),
		]);
		todayCount.value = all.count;
		planTodaySec.value = own.totalSec;
	} catch {
		/* 忽略：计时页的主指标是时钟，统计失败不该报警 */
	}
}

watch(
	[() => state.value?.planId, () => state.value?.todayTotalSec],
	() => {
		void loadCounters();
	},
	{ immediate: true },
);

async function onPrimary(): Promise<void> {
	if (busy.value) return;
	if (isRunning.value) {
		await pauseTimer();
		return;
	}
	if (timerState.value === "paused") {
		await resumeTimer();
		return;
	}
	await startTimer();
}

async function onStop(): Promise<void> {
	if (busy.value || isIdle.value) return;
	const result = await stopTimer();
	if (!result) return;
	if (result.saved) {
		message.success(`已保存本次学习 ${formatDurationCN(result.durationSec)}`);
	} else if (result.reason === "too-short") {
		message.warning("时长不足 5 秒，本次未保存");
	} else {
		message.warning("本次会话已结束，但没有写入记录");
	}
	void loadCounters();
}

function onDiscard(): void {
	if (busy.value || isIdle.value) return;
	dialog.warning({
		title: "确认放弃",
		content: `当前已学习 ${formatDurationCN(elapsedSec.value)}，放弃后不会留下任何记录。`,
		positiveText: "放弃",
		negativeText: "继续学习",
		onPositiveClick: () => {
			void discardTimer();
		},
	});
}

/*
 * 切换计划。
 *
 * 计时中也能切：后端会把这段会话结算掉再切（见 PlansService.SetCurrent）。
 * 返回值告诉我们"刚才顺手记了多少、记到哪去了"——必须说出来，
 * 否则用户眼看着计时器停了，却不知道那 25 分钟去哪了。
 *
 * 原来的版本在计时中直接 return（因为后端会拒绝）。现在反过来：
 * 那不是非法操作，而是一个会自动收尾的正常操作。
 */
async function onSelectPlan(id: number): Promise<void> {
	if (switching.value) return;
	switching.value = true;
	try {
		const out = await PlansService.SetCurrent(id);
		await refreshPlans();
		// SetCurrent 会改后端设置里的当前计划，权威状态要重新拉一次。
		await pullState();

		const to = `已切到「${out.plan?.name ?? "新计划"}」`;
		if (!out.settled) {
			message.success(to);
		} else if (out.saved) {
			message.success(
				`已把 ${formatDurationCN(out.settledSec)} 记到「${out.settledPlanName}」，${to}`,
			);
		} else {
			// 时长不足 5 秒：没写记录，但切换照常完成了——这两件事要分开说清楚。
			message.warning(`时长不足 5 秒，未记录；${to}`);
		}
	} catch (err) {
		message.error(errText(err));
	} finally {
		switching.value = false;
	}
}

/** 在选择器里新建计划：直接设为当前，省掉用户再点一次。 */
async function onPlanCreated(plan: PlanDTO): Promise<void> {
	await refreshPlans();
	await onSelectPlan(plan.id);
}
</script>

<template>
	<!-- data-bottom 给 tools/ui_probe.mjs 断言"底部区域显示什么"用 -->
	<section class="timer-view gx-col" :data-bottom="cardMode">
		<GlassPanel padding="14px 16px 18px">
			<!-- 计划选择器并进主面板：它和时钟是同一件事的两半，单独包一层玻璃只会多一个盒子 -->
			<div class="plan-row">
				<span class="gx-section-title">当前计划</span>
				<span class="gx-spacer"></span>
				<PlanPicker
					:plans="activePlans"
					:current-id="currentPlanId"
					@select="onSelectPlan"
					@created="onPlanCreated"
				/>
			</div>

			<ClockDisplay :time="clockText" :state="timerState" :caption="todayCaption" />

			<div class="actions">
				<button
					type="button"
					class="primary gx-accent-btn gx-pill"
					:disabled="busy"
					@click="onPrimary"
				>
					<IconPause v-if="isRunning" :size="17" />
					<IconPlay v-else :size="17" />
					<span>{{ primaryLabel }}</span>
				</button>

				<div class="actions-row">
					<button
						type="button"
						class="secondary gx-ghost gx-pill"
						:disabled="busy || isIdle"
						@click="onStop"
					>
						<IconStop :size="15" />
						<span>结束并保存</span>
					</button>
					<button
						type="button"
						class="secondary gx-ghost gx-pill is-danger"
						:disabled="busy || isIdle"
						@click="onDiscard"
					>
						<IconDiscard :size="15" />
						<span>放弃</span>
					</button>
				</div>
			</div>

			<p v-if="lastError" class="error">{{ lastError }}</p>
		</GlassPanel>

		<GlassPanel padding="14px 16px">
			<div class="tiles">
				<div class="tile-cell">
					<StatTile
						label="今日累计"
						:value="formatDurationCN(todayTotalSec)"
						hint="全部计划"
						accent
					/>
				</div>
				<div class="tile-cell">
					<StatTile
						label="本计划今日"
						:value="formatDurationCN(planTodaySec)"
						:hint="planHint"
					/>
				</div>
				<div class="tile-cell">
					<StatTile label="今日场次" :value="`${todayCount} 次`" hint="全部计划" />
				</div>
			</div>
		</GlassPanel>

		<!-- 休息提醒：关掉之后这一块整个不出现（在个性化页里开关）。
		     判据用后端状态（关闭提醒时 breakIntervalSec 为 0）而不是本地设置，
		     这样首帧不会闪一下。 -->
		<GlassPanel v-if="!reminderOff" padding="12px 16px">
			<div class="reminder">
				<div class="reminder-head">
					<span>距下次休息提醒还有 {{ formatCountdownCN(breakRemainSec) }}</span>
					<span class="gx-faint gx-tabular">间隔 {{ breakIntervalMin }} 分钟</span>
				</div>
				<div class="track">
					<div class="fill" :style="{ width: `${breakPercent}%` }"></div>
				</div>
			</div>
		</GlassPanel>

		<!-- 最下方那块区域：显示什么由个性化页决定。
		     「留空」时这里连面板都没有，`min-height: 100%` 让下面自然留白。
		     选「图片」时面板吸收剩余高度（见 .card-panel），图铺满。 -->
		<GlassPanel v-if="cardMode === 'image'" class="card-panel" padding="14px 16px">
			<img v-if="showImage" class="photo" :src="imageUrl" alt="背景图" />
			<div v-else class="card-empty">
				<span class="hint gx-muted">还没有选背景图，去「个性化」挑一张</span>
				<button type="button" class="pick gx-ghost gx-pill gx-lift" @click="goPersonalize">
					去挑一张
				</button>
			</div>
		</GlassPanel>
	</section>
</template>

<style scoped>
.timer-view {
	gap: 12px;
	/*
	 * 撑满内容区的高度，好让最下面那块（选「图片」时）吸收剩余空间。
	 *
	 * 这里**不再是**"填掉下方空白"的意思：个性化里选「留空」时，下面就是故意留白的，
	 * 没有面板需要长高。留着 min-height 只是为了让图片模式能铺满，且在窗口变高时
	 * 不会把别的面板挤出去。
	 *
	 * 顺带说明：探针的"一屏放得下"检查**测不出**这种空白——scrollHeight 永远不会
	 * 小于 clientHeight，内容比容器矮时两者相等。所以留白是否合适得靠眼睛看截图。
	 */
	min-height: 100%;
}

/*
 * 图片面板吸收剩余高度；窗口矮时缩回内容高度（min-height），不会把别的面板挤出去。
 *
 * 这里不用"给图片写 height: 100%"：那要靠百分比高度逐级解析，中间任何一环
 * 高度是 auto 就会失效（实测就是这么失效的——面板长高了，里面的图没跟着长）。
 * 让面板本身变成一个 flex 列、图片 flex-grow，高度关系由 flex 布局直接算出来。
 */
.card-panel {
	display: flex;
	flex-direction: column;
	flex: 1 1 auto;
	min-height: 168px;
	padding: 0;
	overflow: hidden;
}

/*
 * 图片**不能**参与"这一段该多高"的计算。
 *
 * `<img>` 是替换元素，`min-height` 默认是 auto（= 它自己的固有高度）。一张 4:3 的横图
 * 在 510px 宽的容器里固有高度约 380px，于是整条 flex 链都以为"这一段至少要 380"，
 * 窗口 800 高时整页就被撑出十来个像素、多出一条滚动条——而它看起来只是"图铺满了"，
 * 很容易被当成没问题（实测：默认竖图/占位框时都不会暴露，换一张横图才会）。
 *
 * 所以给它 `flex-basis: 0` + `min-height: 0`：高度只由面板给多少决定（面板自己有
 * 168px 的下限），图片只管按 cover 填满那块地方。
 */
.photo {
	display: block;
	width: 100%;
	flex: 1 1 0;
	min-height: 0;
	object-fit: cover;
}

/* 还没选图时的入口：虚线框表达"这里还能放点东西"，实线会读成一个已经画好的面板 */
.card-empty {
	display: flex;
	flex-direction: column;
	align-items: center;
	justify-content: center;
	gap: 10px;
	flex: 1 1 auto;
	min-height: 168px;
	margin: 14px;
	border-radius: var(--gx-r-md);
	border: 1px dashed var(--gx-line);
}

.card-empty .hint {
	font-size: 12.5px;
}

.card-empty .pick {
	padding: 5px 12px;
	font-size: 12.5px;
	font-family: inherit;
	cursor: pointer;
}

.plan-row {
	display: flex;
	align-items: center;
	gap: 10px;
	min-width: 0;
	padding-bottom: 12px;
	margin-bottom: 6px;
	border-bottom: 1px solid var(--gx-line);
}

.actions {
	display: flex;
	flex-direction: column;
	gap: 9px;
	margin-top: 16px;
}

.primary,
.secondary {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	gap: 7px;
	font-family: inherit;
	cursor: pointer;
}

.primary {
	width: 100%;
	padding: 12px 18px;
	font-size: 15.5px;
	font-weight: 600;
	letter-spacing: 0.02em;
}

.primary:disabled {
	cursor: not-allowed;
}

.actions-row {
	display: flex;
	gap: 9px;
}

.secondary {
	flex: 1 1 0;
	min-width: 0;
	padding: 9px 10px;
	font-size: 13px;
}

.secondary:disabled {
	opacity: 0.45;
	cursor: not-allowed;
}

.secondary.is-danger:hover:not(:disabled) {
	color: var(--gx-danger);
	border-color: var(--gx-danger);
}

.error {
	margin-top: 10px;
	font-size: 12.5px;
	color: var(--gx-danger);
}

.tiles {
	display: flex;
	gap: 12px;
}

.tile-cell {
	flex: 1 1 0;
	min-width: 0;
}

.reminder {
	display: flex;
	flex-direction: column;
	gap: 8px;
}

.reminder-head {
	display: flex;
	align-items: baseline;
	justify-content: space-between;
	gap: 10px;
	font-size: 13px;
}

.track {
	height: 6px;
	border-radius: var(--gx-r-pill);
	background: var(--gx-track);
	overflow: hidden;
}

.fill {
	height: 100%;
	border-radius: var(--gx-r-pill);
	background: var(--gx-accent-grad);
	transition: width var(--gx-dur) linear;
}
</style>
