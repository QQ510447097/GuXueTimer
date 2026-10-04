import { computed, readonly, ref, type ComputedRef, type Ref } from "vue";
import { Events } from "@wailsio/runtime";
import * as PlansService from "../../bindings/goxuetimer/plansservice";
import type { PlanDTO } from "../../bindings/goxuetimer/models";
import { errText } from "./format";

/**
 * 学习计划的状态仓库（模块级单例）。
 *
 * 计划被三处需要：外壳的引导门禁、计时页的当前计划、记录页的名字与颜色查表。
 * 各自拉一次接口会出现"计时页已经切了、记录页还按旧计划筛"这种不一致，
 * 所以统一放这里，并订阅 plans:changed 自动刷新。
 */

const plans = ref<PlanDTO[]>([]);
const palette = ref<string[]>([]);
const loading = ref(false);
const error = ref("");
const loaded = ref(false);

let subscribed = false;

export function errOfPlans(): string {
	return error.value;
}

async function fetchAll(): Promise<void> {
	loading.value = true;
	error.value = "";
	try {
		// 含已归档：记录页要靠它把历史记录里的计划名与颜色查出来，
		// 归档计划的历史不该因此变成"未分类"。
		const [list, colors] = await Promise.all([
			PlansService.List(true),
			PlansService.Palette(),
		]);
		plans.value = list ?? [];
		palette.value = colors ?? [];
		loaded.value = true;
	} catch (err) {
		error.value = errText(err);
	} finally {
		loading.value = false;
	}
}

/** 计划变更后由后端事件触发，也可以手动调。 */
export function refreshPlans(): Promise<void> {
	return fetchAll();
}

/** 幂等订阅：多次调用 usePlans 不会注册多个监听。 */
function ensureSubscribed(): void {
	if (subscribed) return;
	subscribed = true;
	Events.On("plans:changed", () => {
		void fetchAll();
	});
}

export function usePlans() {
	ensureSubscribed();
	if (!loaded.value && !loading.value) {
		void fetchAll();
	}

	const activePlans = computed(() =>
		plans.value.filter((p) => !p.archived).sort((a, b) => a.sortOrder - b.sortOrder),
	);
	const archivedPlans = computed(() => plans.value.filter((p) => p.archived));

	/** 计划 id -> 计划，用于把记录上的 planId 解析成名字与颜色。 */
	const byId = computed(() => {
		const map: Record<number, PlanDTO> = {};
		for (const p of plans.value) map[p.id] = p;
		return map;
	});

	/** 一条记录的计划名；查不到就退回"未分类"。 */
	function nameOf(planId: number): string {
		if (!planId) return "未分类";
		return byId.value[planId]?.name ?? "未分类";
	}

	/** 一条记录的计划颜色；查不到给中性色。 */
	function colorOf(planId: number): string {
		if (!planId) return "var(--gx-text-3)";
		return byId.value[planId]?.color ?? "var(--gx-text-3)";
	}

	return {
		plans: readonly(plans) as Readonly<Ref<PlanDTO[]>>,
		activePlans,
		archivedPlans,
		palette: readonly(palette) as Readonly<Ref<string[]>>,
		byId,
		loading: readonly(loading) as Readonly<Ref<boolean>>,
		error: readonly(error) as Readonly<Ref<string>>,
		loaded: readonly(loaded) as Readonly<Ref<boolean>>,
		nameOf,
		colorOf,
		refresh: refreshPlans,
	};
}

export type UsePlansReturn = ReturnType<typeof usePlans>;
export type { PlanDTO };
export type PlansList = ComputedRef<PlanDTO[]>;
