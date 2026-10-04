<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useDialog, useMessage } from "naive-ui";
import * as PlansService from "../../bindings/goxuetimer/plansservice";
import * as RecordsService from "../../bindings/goxuetimer/recordsservice";
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
import { usePlans } from "../composables/usePlans";
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
const dialog = useDialog();
const message = useMessage();

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

async function onSelectPlan(id: number): Promise<void> {
	if (switching.value || !isIdle.value) return;
	switching.value = true;
	try {
		await PlansService.SetCurrent(id);
		await refreshPlans();
		// SetCurrent 会改后端设置里的当前计划，权威状态要重新拉一次。
		await pullState();
	} catch (err) {
		message.error(errText(err));
	} finally {
		switching.value = false;
	}
}
</script>

<template>
	<section class="timer-view gx-col">
		<GlassPanel padding="14px 16px 18px">
			<!-- 计划选择器并进主面板：它和时钟是同一件事的两半，单独包一层玻璃只会多一个盒子 -->
			<div class="plan-row">
				<span class="gx-section-title">当前计划</span>
				<span class="gx-spacer"></span>
				<PlanPicker
					:plans="activePlans"
					:current-id="currentPlanId"
					:disabled="!isIdle || switching"
					:hint="isIdle ? '正在切换…' : '计时中不能切换计划'"
					@select="onSelectPlan"
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

		<GlassPanel padding="12px 16px">
			<p v-if="reminderOff" class="reminder-off gx-muted">休息提醒已关闭</p>
			<div v-else class="reminder">
				<div class="reminder-head">
					<span>距下次休息提醒还有 {{ formatCountdownCN(breakRemainSec) }}</span>
					<span class="gx-faint gx-tabular">间隔 {{ breakIntervalMin }} 分钟</span>
				</div>
				<div class="track">
					<div class="fill" :style="{ width: `${breakPercent}%` }"></div>
				</div>
			</div>
		</GlassPanel>
	</section>
</template>

<style scoped>
.timer-view {
	gap: var(--gx-gap);
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

.reminder-off {
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
