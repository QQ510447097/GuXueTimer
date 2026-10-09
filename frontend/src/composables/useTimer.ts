import { computed, readonly, ref, type Ref } from "vue";
import { Events } from "@wailsio/runtime";
import * as TimerService from "../../bindings/goxuetimer/timerservice";
import type { StopResultDTO, TimerStateDTO } from "../../bindings/goxuetimer/models";
import { errText } from "./format";
import { refreshStats } from "./useStats";

/**
 * 计时状态是模块级单例：三个视图拿到的是同一份数据，事件也只在模块作用域订阅一次。
 */

export type TimerState = "idle" | "running" | "paused";

/** 时钟与提醒进度都靠这里的 now 平滑推进。 */
const clockTickMs = 250;

const state = ref<TimerStateDTO | null>(null);
const now = ref(Date.now());
const busy = ref(false);
const lastError = ref("");

let ticker: ReturnType<typeof setInterval> | null = null;
let subscribed = false;
let initialized = false;

function applyState(dto: TimerStateDTO): void {
	state.value = dto;
	now.value = Date.now();
}

/** 当前运行段已经走过的时间。非 running、时间戳不可解析或时钟回拨时为 0。 */
function runningDeltaSec(anchor: string, stamp: number): number {
	if (!anchor) return 0;
	const parsed = Date.parse(anchor);
	if (Number.isNaN(parsed)) return 0;
	// 系统时间被往回调时 delta 为负，直接当作 0，免得时钟倒退或进度条回卷。
	const delta = (stamp - parsed) / 1000;
	return delta > 0 ? delta : 0;
}

async function run(fn: () => Promise<TimerStateDTO>): Promise<void> {
	busy.value = true;
	lastError.value = "";
	try {
		applyState(await fn());
	} catch (err) {
		lastError.value = errText(err);
	} finally {
		busy.value = false;
	}
}

export function pullState(): Promise<void> {
	return run(() => TimerService.GetState());
}

export function startTimer(): Promise<void> {
	return run(() => TimerService.Start());
}

export function pauseTimer(): Promise<void> {
	return run(() => TimerService.Pause());
}

export function resumeTimer(): Promise<void> {
	return run(() => TimerService.Resume());
}

export async function stopTimer(): Promise<StopResultDTO | null> {
	busy.value = true;
	lastError.value = "";
	try {
		const result = await TimerService.Stop();
		// Stop 的返回值不含最新状态，重新拉一次。
		await pullState();
		return result;
	} catch (err) {
		lastError.value = errText(err);
		return null;
	} finally {
		busy.value = false;
	}
}

export function discardTimer(): Promise<void> {
	return run(() => TimerService.Discard());
}

export function useTimer() {
	if (!subscribed) {
		subscribed = true;

		// 后端每次状态迁移与 30 秒心跳都会发这个事件，永远以它为准覆盖本地状态。
		Events.On("timer:state", (ev) => {
			applyState(ev.data as TimerStateDTO);
		});

		Events.On("records:changed", () => {
			// 落库/删除都会改变区间统计与月历，图表也要跟着重算。
			void refreshStats();
			// 「今日已落库时长」也变了，顺手拉一次权威状态。
			void pullState();
		});
	}

	if (!initialized) {
		initialized = true;
		if (ticker === null) {
			/*
			 * 后端心跳 30 秒一次，直接用它驱动界面会让秒数一顿一顿；
			 * 250ms 既够顺滑，又不会像 rAF 那样在后台窗口空转。
			 */
			ticker = setInterval(() => {
				now.value = Date.now();
			}, clockTickMs);
		}
		void pullState();
	}

	const elapsedSec = computed(() => {
		const s = state.value;
		if (!s) return 0;
		let sec = s.accumulatedSec;
		if (s.state === "running" && s.runningSince) {
			sec += runningDeltaSec(s.runningSince, now.value);
		}
		return Math.floor(sec);
	});

	// 今日累计 = 已落库的部分 + 正在走的这一段，否则数字要等 30 秒心跳才跳。
	const todayTotalSec = computed(() => {
		const s = state.value;
		if (!s) return 0;
		return s.todayTotalSec + elapsedSec.value;
	});

	const stateText = computed(() => {
		switch (state.value?.state) {
			case "running":
				return "运行中";
			case "paused":
				return "已暂停";
			default:
				return "未开始";
		}
	});

	return {
		state: readonly(state) as Readonly<Ref<TimerStateDTO | null>>,
		now: readonly(now) as Readonly<Ref<number>>,
		busy: readonly(busy) as Readonly<Ref<boolean>>,
		lastError: readonly(lastError) as Readonly<Ref<string>>,
		elapsedSec,
		todayTotalSec,
		stateText,
	};
}

// 时钟文案统一由 format.ts 提供；这里转出一次，免得调用方多 import 一个模块。
export { formatClock } from "./format";
