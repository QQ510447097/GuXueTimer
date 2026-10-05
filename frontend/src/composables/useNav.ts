import { readonly, ref, type Ref } from "vue";
import { Events } from "@wailsio/runtime";
import type { NavGotoDTO } from "../../bindings/goxuetimer/models";

/**
 * 当前页面（模块级单例）。
 *
 * 为什么从 App.vue 里抽出来：有两处"外部"需要切页——
 *   1. 后端发来的 nav:goto（托盘菜单的「设置」）
 *   2. 视图内部的跳转（计时页的"去个性化挑一张图"）
 * 状态放在 App.vue 的局部 ref 里，这两处都够不着；放在模块级单例里，
 * 组件只管读，谁来切都走同一个入口。
 *
 * 合法性不在这一层判断：有哪些页是 App.vue 的视图表说了算，这里通过
 * setViewValidator 把判断权交给它，避免"页面键"在两地各存一份而漂移。
 */

const VIEW_STORAGE_KEY = "gx.activeView";

const active = ref<string>(restore());

/** 页面键是否合法。默认一律拒绝：App.vue 装配之前不该有任何切换生效。 */
let isValidKey: (key: string) => boolean = () => false;

export function setViewValidator(fn: (key: string) => boolean): void {
	isValidKey = fn;
}

function restore(): string {
	const saved = localStorage.getItem(VIEW_STORAGE_KEY) ?? "";
	// 存过的键可能已经不存在（改过页面结构），这时回落到计时页。
	return saved;
}

/** 切换页面。非法键被忽略，所以调用方不必自己校验。 */
export function selectView(key: string): void {
	if (!isValidKey(key)) return;
	active.value = key;
	localStorage.setItem(VIEW_STORAGE_KEY, key);
}

/** 幂等订阅后端的切页请求。 */
function ensureBound(): void {
	Events.On("nav:goto", (ev) => {
		const payload = ev.data as NavGotoDTO;
		if (payload?.view) selectView(payload.view);
	});
}

ensureBound();

export function useNav() {
	return {
		active: readonly(active) as Readonly<Ref<string>>,
		selectView,
	};
}
