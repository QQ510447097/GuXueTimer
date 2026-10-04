<script setup lang="ts">
import { computed } from "vue";
import { NButton, NIcon, NProgress, NTag, useDialog, useMessage } from "naive-ui";
import ClockDisplay from "../components/ClockDisplay.vue";
import IconDiscard from "../components/IconDiscard.vue";
import IconPause from "../components/IconPause.vue";
import IconPlay from "../components/IconPlay.vue";
import IconStop from "../components/IconStop.vue";
import { useRecords } from "../composables/useRecords";
import {
	formatClock,
	formatCountdownCN,
	formatDurationCN,
	formatTodayLabel,
} from "../composables/format";
import {
	discardTimer,
	pauseTimer,
	resumeTimer,
	startTimer,
	stopTimer,
	useTimer,
	type TimerState,
} from "../composables/useTimer";

const { state, now, busy, lastError, elapsedSec, todayTotalSec, breakSec, stateText } = useTimer();
const { summary, refresh: refreshRecords } = useRecords();
const dialog = useDialog();
const message = useMessage();

const clockText = computed(() => formatClock(elapsedSec.value));
const todayCaption = computed(() => `今日 ${formatTodayLabel(now.value)}`);
const isIdle = computed(() => (state.value?.state ?? "idle") === "idle");
const isRunning = computed(() => state.value?.state === "running");
const isPaused = computed(() => state.value?.state === "paused");

const tagType = computed<"success" | "warning" | "default">(() => {
	if (isRunning.value) return "success";
	if (isPaused.value) return "warning";
	return "default";
});

const sessionCount = computed(() => summary.value?.count ?? 0);

const primaryLabel = computed(() => {
	if (isRunning.value) return "暂停";
	if (isPaused.value) return "继续";
	return "开始学习";
});

const reminderOff = computed(() => breakSec.value.interval <= 0);
const breakPercent = computed(() => {
	const { elapsed, interval } = breakSec.value;
	if (interval <= 0) return 0;
	return Math.min(100, Math.max(0, (elapsed / interval) * 100));
});
const breakRemainSec = computed(() => Math.max(0, breakSec.value.interval - breakSec.value.elapsed));
const breakRemainText = computed(() => `距下次休息提醒还有 ${formatCountdownCN(breakRemainSec.value)}`);

async function onPrimary(): Promise<void> {
	if (busy.value) return;
	if (isRunning.value) {
		await pauseTimer();
		return;
	}
	if (isPaused.value) {
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
		message.warning(`时长不足 5 秒，本次未保存（${formatDurationCN(result.durationSec)}）`);
	} else {
		message.warning("本次会话已结束，但没有写入记录");
	}
	await refreshRecords();
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
</script>

<template>
	<section class="timer-view">
		<div class="state-row">
			<NTag :type="tagType" size="small" round :bordered="false">{{ stateText }}</NTag>
		</div>

		<ClockDisplay :time="clockText" :state="(state?.state as TimerState) ?? 'idle'"
			:caption="todayCaption" />

		<div class="actions">
			<NButton class="action-primary" type="primary" size="large" :disabled="busy" :loading="busy"
				@click="onPrimary">
				<template #icon>
					<NIcon>
						<IconPause v-if="isRunning" />
						<IconPlay v-else />
					</NIcon>
				</template>
				{{ primaryLabel }}
			</NButton>

			<div class="actions-row">
				<NButton class="action-secondary" size="large" secondary :disabled="busy || isIdle" @click="onStop">
					<template #icon>
						<NIcon>
							<IconStop />
						</NIcon>
					</template>
					结束并保存
				</NButton>
				<NButton class="action-secondary" size="large" quaternary type="error" :disabled="busy || isIdle"
					@click="onDiscard">
					<template #icon>
						<NIcon>
							<IconDiscard />
						</NIcon>
					</template>
					放弃
				</NButton>
			</div>
		</div>

		<div class="stats">
			<div class="stat stat-today">
				<span class="stat-label">今日累计</span>
				<span class="stat-value">{{ formatDurationCN(todayTotalSec) }}</span>
			</div>
			<div class="stat-pair">
				<div class="stat">
					<span class="stat-label">本次</span>
					<span class="stat-value">{{ formatDurationCN(elapsedSec) }}</span>
				</div>
				<div class="stat">
					<span class="stat-label">今日场次</span>
					<span class="stat-value">{{ sessionCount }} 次</span>
				</div>
			</div>
		</div>

		<div class="reminder">
			<template v-if="reminderOff">
				<span class="reminder-off">休息提醒已关闭</span>
			</template>
			<template v-else>
				<div class="reminder-head">
					<span>{{ breakRemainText }}</span>
					<span class="reminder-interval">间隔 {{ breakSec.interval / 60 }} 分钟</span>
				</div>
				<NProgress class="reminder-bar" type="line" :percentage="breakPercent" :height="6"
					:show-indicator="false" :border-radius="3" />
			</template>
		</div>

		<p v-if="lastError" class="error-text">{{ lastError }}</p>
	</section>
</template>

<style scoped>
.timer-view {
	display: flex;
	flex-direction: column;
	gap: 18px;
}

.state-row {
	display: flex;
	justify-content: center;
}

.actions {
	display: flex;
	flex-direction: column;
	gap: 10px;
}

.action-primary {
	width: 100%;
}

.actions-row {
	display: flex;
	gap: 10px;
}

.action-secondary {
	flex: 1 1 0;
	min-width: 0;
}

.stats {
	display: flex;
	flex-direction: column;
	gap: 10px;
	padding: 14px 16px;
	border-radius: 12px;
	background: var(--gx-surface);
	border: 1px solid var(--gx-border);
}

.stat {
	display: flex;
	flex-direction: column;
	gap: 2px;
	min-width: 0;
}

.stat-today .stat-value {
	font-size: 26px;
	font-weight: 650;
	line-height: 1.2;
}

.stat-label {
	font-size: 12px;
	color: var(--gx-text-muted);
}

.stat-value {
	font-variant-numeric: tabular-nums;
}

.stat-pair {
	display: flex;
	gap: 12px;
}

.stat-pair .stat {
	flex: 1 1 0;
}

.stat-pair .stat-value {
	font-size: 16px;
	font-weight: 600;
}

.reminder {
	display: flex;
	flex-direction: column;
	gap: 8px;
	padding: 12px 14px;
	border-radius: 12px;
	background: var(--gx-surface);
	border: 1px solid var(--gx-border);
}

.reminder-off {
	font-size: 13px;
	color: var(--gx-text-muted);
}

.reminder-head {
	display: flex;
	justify-content: space-between;
	gap: 10px;
	font-size: 13px;
}

.reminder-interval {
	color: var(--gx-text-muted);
	white-space: nowrap;
}

.reminder-bar {
	width: 100%;
}

.error-text {
	margin: 0;
	font-size: 13px;
	color: var(--gx-danger, #d03050);
}
</style>
