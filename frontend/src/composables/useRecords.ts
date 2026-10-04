import { computed, readonly, ref, type Ref } from "vue";
import * as RecordsService from "../../bindings/goxuetimer/recordsservice";
import type { DaySummaryDTO, SessionDTO } from "../../bindings/goxuetimer/models";
import { errText, todayKey } from "./format";
import { refreshStats, useStats } from "./useStats";

/**
 * 选中日的记录列表与当天汇总（模块级单例）。
 *
 * 记录与汇总同源，一次刷新同时更新两者——分开刷必然出现
 * 「列表已删、汇总没变」的中间态。
 *
 * "选中哪天、筛哪个计划"不在这里存：唯一的真相在 useStats。
 * 这里只负责按那个组合去查，所以只提供 refresh()，不接受 planID 参数——
 * 两个地方都能改筛选条件的话，列表和图迟早会各说各话。
 */

const sessions = ref<SessionDTO[]>([]);
const summary = ref<DaySummaryDTO | null>(null);
const loading = ref(false);
const error = ref("");

/** 已经装进 sessions 的那次查询的键；用来判断当前列表是不是"过期数据"。 */
const loadedKey = ref("");
let seq = 0;

function keyOf(date: string, planID: number): string {
	return `${date}|${planID}`;
}

function selection(): { date: string; planID: number; key: string } {
	const stats = useStats();
	const date = stats.selectedDate.value;
	const planID = stats.filterPlanId.value;
	return { date, planID, key: keyOf(date, planID) };
}

async function fetchDay(date: string, planID: number): Promise<void> {
	const mine = ++seq;
	loading.value = true;
	error.value = "";

	/*
	 * 选中的就是今天时传空串，让后端按本地时区判定"今天"。
	 * 应用跨过午夜还开着的话，前端缓存的日期键会停在昨天，空串不会。
	 */
	const queryDate = date === todayKey() ? "" : date;

	try {
		const [list, sum] = await Promise.all([
			RecordsService.ListByDay(queryDate, planID),
			RecordsService.DaySummary(queryDate, planID),
		]);
		if (mine !== seq) return;
		sessions.value = list ?? [];
		summary.value = sum;
		loadedKey.value = keyOf(date, planID);
	} catch (err) {
		if (mine === seq) error.value = errText(err);
	} finally {
		if (mine === seq) loading.value = false;
	}
}

/** 按当前"选中日 + 筛选计划"重新拉一次。 */
export function refreshRecords(): Promise<void> {
	const { date, planID } = selection();
	return fetchDay(date, planID);
}

/** 记录变动后整体刷新：记录列表 + 区间/月历统计。 */
async function refreshAll(): Promise<void> {
	const { date, planID } = selection();
	await Promise.all([fetchDay(date, planID), refreshStats()]);
}

async function removeSession(id: number): Promise<boolean> {
	error.value = "";
	try {
		await RecordsService.DeleteSession(id);
		// 后端也会发 records:changed，但点了删除就该立刻看到结果。
		await refreshAll();
		return true;
	} catch (err) {
		error.value = errText(err);
		return false;
	}
}

export function useRecords() {
	const stats = useStats();
	return {
		sessions: readonly(sessions) as Readonly<Ref<SessionDTO[]>>,
		summary: readonly(summary) as Readonly<Ref<DaySummaryDTO | null>>,
		loading: readonly(loading) as Readonly<Ref<boolean>>,
		error: readonly(error) as Readonly<Ref<string>>,
		/** 列表还不是当前选择的数据（切日期后到新数据回来之前为 true）。 */
		stale: computed(() => loadedKey.value !== selection().key),
		refresh: refreshAll,
		remove: removeSession,
		selectedDate: stats.selectedDate,
		setSelectedDate: stats.setSelectedDate,
	};
}

export type UseRecordsReturn = ReturnType<typeof useRecords>;
