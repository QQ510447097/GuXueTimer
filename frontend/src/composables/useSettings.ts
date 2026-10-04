import { computed, readonly, ref, type ComputedRef, type Ref } from "vue";
import { darkTheme } from "naive-ui";
import type { GlobalTheme } from "naive-ui";
import * as SettingsService from "../../bindings/goxuetimer/settingsservice";
import type { SettingsDTO } from "../../bindings/goxuetimer/models";
import { errText } from "./format";

export type ThemeChoice = "system" | "light" | "dark";

export const defaultBreakIntervalMin = 45;

// 与后端 internal/settings 的 Defaults() 保持一致。
// 绑定生成器用 -i 产出的是接口（纯类型），不能 new，所以直接给字面量。
const defaults = (): SettingsDTO => ({
	breakReminderEnabled: true,
	breakIntervalMin: defaultBreakIntervalMin,
	minimizeToTray: true,
	theme: "system",
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
 * 保存后必须用返回值覆盖本地草稿：后端返回的是"结论"，
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
		themeClass,
		load: loadSettings,
		save: saveSettings,
	};
}

export type UseSettingsReturn = ReturnType<typeof useSettings>;
