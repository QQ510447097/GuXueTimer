import { readonly, ref, type Ref } from "vue";
import { Events } from "@wailsio/runtime";
import * as AchievementsService from "../../bindings/goxuetimer/achievementsservice";
import type { AchievementsDTO } from "../../bindings/goxuetimer/models";
import { errText } from "./format";

/**
 * 成就的汇总数据（模块级单例）。
 *
 * 没有本地缓存与增量更新：这个页面的数据全部由后端从 sessions 表推导，
 * 每次读都是"当前事实"。数据量是几百行级别的聚合，重新算比维护
 * "哪次操作会影响哪个数字"便宜得多，也不会出现自相矛盾的画面。
 */
const empty = (): AchievementsDTO => ({
	unlocked: 0,
	total: 0,
	totalSec: 0,
	todaySec: 0,
	longestSec: 0,
	sessions: 0,
	activeDays: 0,
	currentStreak: 0,
	longestStreak: 0,
	lateNightCount: 0,
	earlyMorningCount: 0,
	planCount: 0,
	qualifyingDays: 0,
	level: {
		level: 1,
		xpTenths: 0,
		intoTenths: 0,
		needTenths: 1000,
		rateTenths: 10,
		hoursCounted: 0,
		carrySec: 0,
		medals: { week: 0, month: 0, quarter: 0, year: 0 },
	},
	items: [],
});

const summary = ref<AchievementsDTO>(empty());
const loading = ref(false);
const error = ref("");

export function refreshAchievements(): Promise<void> {
	loading.value = true;
	error.value = "";
	return AchievementsService.Summary()
		.then((dto) => {
			summary.value = dto;
		})
		.catch((err: unknown) => {
			error.value = errText(err);
		})
		.finally(() => {
			loading.value = false;
		});
}

/*
 * 记录一变就重新拉：成就完全由记录推导，所以"结束一次计时"可能刚好解锁一个徽章。
 * 订阅放在模块作用域（与 useTimer 的做法一致），不随组件卸载而失效。
 */
Events.On("records:changed", () => {
	void refreshAchievements();
});

export function useAchievements() {
	return {
		summary: readonly(summary) as Readonly<Ref<AchievementsDTO>>,
		loading: readonly(loading) as Readonly<Ref<boolean>>,
		error: readonly(error) as Readonly<Ref<string>>,
		refresh: refreshAchievements,
	};
}
