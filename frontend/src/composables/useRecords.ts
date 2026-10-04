import { readonly, ref, type Ref } from "vue";
import * as RecordsService from "../../bindings/goxuetimer/recordsservice";
import type { DaySummaryDTO, SessionDTO } from "../../bindings/goxuetimer/models";
import { errText } from "./format";

const sessions = ref<SessionDTO[]>([]);
const summary = ref<DaySummaryDTO | null>(null);
const loading = ref(false);
const error = ref("");

/**
 * 记录与今日汇总同源，因此一次刷新同时更新两者——
 * 分开刷新必然出现「列表已删、汇总没变」的中间态。
 */
export async function refreshRecords(): Promise<void> {
	loading.value = true;
	error.value = "";
	try {
		// 空串 = 今天，由后端按本地时区判定，前端不自己算日期。
		const [list, sum] = await Promise.all([
			RecordsService.ListByDay(""),
			RecordsService.TodaySummary(),
		]);
		sessions.value = list ?? [];
		summary.value = sum;
	} catch (err) {
		error.value = errText(err);
	} finally {
		loading.value = false;
	}
}

export async function deleteSession(id: number): Promise<boolean> {
	error.value = "";
	loading.value = true;
	try {
		await RecordsService.DeleteSession(id);
		// 后端会再发 records:changed，但点了删除就该立刻看到结果，这里主动刷一次。
		await refreshRecords();
		return true;
	} catch (err) {
		error.value = errText(err);
		return false;
	} finally {
		loading.value = false;
	}
}

export function useRecords() {
	return {
		sessions: readonly(sessions) as Readonly<Ref<SessionDTO[]>>,
		summary: readonly(summary) as Readonly<Ref<DaySummaryDTO | null>>,
		loading: readonly(loading) as Readonly<Ref<boolean>>,
		error: readonly(error) as Readonly<Ref<string>>,
		refresh: refreshRecords,
		remove: deleteSession,
	};
}
