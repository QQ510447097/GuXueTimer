import { computed, readonly, ref, type ComputedRef, type Ref } from "vue";
import { darkTheme } from "naive-ui";
import type { GlobalTheme } from "naive-ui";
import * as SettingsService from "../../bindings/goxuetimer/settingsservice";
import type { SettingsDTO } from "../../bindings/goxuetimer/models";
import { buildThemeOverrides } from "../theme/naiveTheme";
import { errText } from "./format";

export type ThemeChoice = "system" | "light" | "dark";

export const defaultBreakIntervalMin = 45;

// 与后端 internal/settings 的 Defaults() 保持一致。
// 绑定生成器用 -i 产出的是接口（纯类型），不能 new，所以直接给字面量。
//
// 默认浅色而不是跟随系统：软件的主色调是蓝白，第一眼就该是那个样子；
// 真加载完之后会被后端返回的值覆盖。
const defaults = (): SettingsDTO => ({
	breakReminderEnabled: true,
	breakIntervalMin: defaultBreakIntervalMin,
	minimizeToTray: true,
	theme: "light",
	currentPlanId: 0,
	// 计时页最下方那块区域：留空。后端 internal/settings 的 Defaults() 同此。
	cardMode: "none",
});

const settings = ref<SettingsDTO>(defaults());
const loading = ref(false);
const saving = ref(false);
const loadError = ref("");
const saveError = ref("");
const systemDark = ref(false);

let mediaBound = false;
let loaded = false;

function watchSystemTheme(): void {
	if (mediaBound || typeof window === "undefined" || !window.matchMedia) return;
	mediaBound = true;
	const query = window.matchMedia("(prefers-color-scheme: dark)");
	systemDark.value = query.matches;
	// 跟随系统时必须是"实时"的：用户在 Windows 里切换主题，窗口要立刻跟着变。
	query.addEventListener("change", (ev) => {
		systemDark.value = ev.matches;
	});
}

watchSystemTheme();

const themeChoice = computed<ThemeChoice>(() => {
	const raw = settings.value.theme;
	return raw === "light" || raw === "dark" ? raw : "system";
});

/** 实际生效的深浅色。system 时跟随 matchMedia，其他取值由用户显式决定。 */
const isDark = computed(() => {
	if (themeChoice.value === "system") return systemDark.value;
	return themeChoice.value === "dark";
});

const naiveTheme: ComputedRef<GlobalTheme | null> = computed(() =>
	isDark.value ? darkTheme : null,
);
/** naive-ui 的配色与圆角覆盖，跟随深浅色切换。 */
const themeOverrides = computed(() => buildThemeOverrides(isDark.value));
/** 供 CSS 变量用：naive-ui 的组件主题之外，页面底色也要跟着换。 */
const themeClass = computed(() => (isDark.value ? "theme-dark" : "theme-light"));

export function loadSettings(force = false): Promise<void> {
	if (loaded && !force) return Promise.resolve();
	loaded = true;
	loading.value = true;
	loadError.value = "";
	return SettingsService.Get()
		.then((dto) => {
			settings.value = dto;
		})
		.catch((err: unknown) => {
			loadError.value = errText(err);
		})
		.finally(() => {
			loading.value = false;
		});
}

/**
 * 保存后必须用返回值覆盖本地设置：后端返回的是"结论"，
 * 越界值会被它归一化，界面要立刻显示真正生效的值。
 */
export async function saveSettings(draft: SettingsDTO): Promise<SettingsDTO | null> {
	saving.value = true;
	saveError.value = "";
	try {
		const saved = await SettingsService.Save(draft);
		settings.value = saved;
		return saved;
	} catch (err) {
		saveError.value = errText(err);
		return null;
	} finally {
		saving.value = false;
	}
}

/*
 * 实时生效的保存队列。
 *
 * 设置页没有保存按钮，改一下就提交一次；用户手快时（连点两个开关、拖数字输入）
 * 会有多次提交重叠。如果让它们并行，后到的响应可能覆盖先到的，
 * 界面就停在"最后一次响应"而不是"最后一次操作"的状态上。
 * 所以把提交串成一条链：每一次都等前一次结束，再按**当时**的设置合成请求。
 *
 * "按当时的设置合成"很关键：patch 里读的是 settings.value 的最新值，
 * 而它已经被前一次保存的返回值覆盖过，所以排队中的改动是叠加而不是互相丢弃。
 */
let saveQueue: Promise<SettingsDTO | null> = Promise.resolve(null);

/** 改哪一项就只传哪一项；其余字段取当前已生效的值。 */
export function patchSettings(partial: Partial<SettingsDTO>): Promise<SettingsDTO | null> {
	const run = (): Promise<SettingsDTO | null> => {
		const payload: SettingsDTO = {
			...settings.value,
			...partial,
			// 当前计划只由 PlansService.SetCurrent 掌管，这里一律回 0（"不表态"）。
			// 设置页里没有这个字段，回传具体值会把用户刚选的计划覆盖掉。
			currentPlanId: 0,
		};
		return saveSettings(payload);
	};
	// 前一次无论成功还是失败，都要继续跑下一次（失败已经落到 saveError 里了）。
	saveQueue = saveQueue.then(run, run);
	return saveQueue;
}

export function useSettings() {
	return {
		settings: readonly(settings) as Readonly<Ref<SettingsDTO>>,
		loading: readonly(loading) as Readonly<Ref<boolean>>,
		saving: readonly(saving) as Readonly<Ref<boolean>>,
		loadError: readonly(loadError) as Readonly<Ref<string>>,
		saveError: readonly(saveError) as Readonly<Ref<string>>,
		themeChoice,
		isDark,
		naiveTheme,
		themeOverrides,
		themeClass,
		load: loadSettings,
		save: saveSettings,
		patch: patchSettings,
	};
}

export type UseSettingsReturn = ReturnType<typeof useSettings>;
