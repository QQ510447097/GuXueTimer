<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NInputNumber, NRadioButton, NRadioGroup, NSwitch, useMessage } from "naive-ui";
import type { SettingsDTO } from "../../bindings/goxuetimer/models";
import * as SettingsService from "../../bindings/goxuetimer/settingsservice";
import GlassPanel from "../components/GlassPanel.vue";
import { errText } from "../composables/format";
import { useSettings } from "../composables/useSettings";

/**
 * 设置页。
 *
 * 表单是一份本地草稿，只有点"保存"才回传后端：开关类设置每动一下就写盘
 * 会让"改错了"变得不可撤销。
 *
 * （M4 会把这里改成实时生效、去掉保存按钮；窗口毛玻璃已经从设置里去掉了，
 * 它恒定开启，见 main.go 里的 translucentWindow。）
 */
const { settings, saving, loadError, saveError, save } = useSettings();
const message = useMessage();

const draft = ref<SettingsDTO>({ ...settings.value });

// 保存成功、或别处改了设置（托盘菜单）之后，表单要显示真正生效的值。
watch(
	settings,
	(next) => {
		draft.value = { ...next };
	},
	{ immediate: true },
);

/** NInputNumber 的 v-model 是 number | null，不能直接绑在 number 字段上。 */
const intervalMin = computed<number | null>({
	get: () => draft.value.breakIntervalMin,
	set: (value) => {
		draft.value.breakIntervalMin = value ?? 1;
	},
});

/** NRadioGroup 的值是 string | number | boolean，收窄成主题枚举。 */
const themeValue = computed<string>({
	get: () => draft.value.theme,
	set: (value) => {
		draft.value.theme = value === "light" || value === "dark" ? value : "system";
	},
});

// 数据目录由后端给出：真实路径是逐级降级算出来的，界面自己拼很可能不是那个。
const dataPath = ref("");
const openingDir = ref(false);

void SettingsService.DataDir()
	.then((dir) => {
		dataPath.value = dir || "";
	})
	.catch(() => {
		dataPath.value = "（读取数据目录失败）";
	});

async function openDataDir(): Promise<void> {
	if (openingDir.value) return;
	openingDir.value = true;
	try {
		await SettingsService.OpenDataDir();
	} catch (err) {
		message.error(errText(err));
	} finally {
		openingDir.value = false;
	}
}

async function onSave(): Promise<void> {
	const payload: SettingsDTO = {
		breakReminderEnabled: draft.value.breakReminderEnabled,
		breakIntervalMin: draft.value.breakIntervalMin,
		minimizeToTray: draft.value.minimizeToTray,
		theme: draft.value.theme,
		// 0 是"不表态"：当前计划只由 PlansService.SetCurrent 掌管，
		// 设置页回传任何具体值都可能把用户刚选的计划覆盖掉。
		currentPlanId: 0,
	};
	const saved = await save(payload);
	if (!saved) {
		message.error(saveError.value || "保存失败");
		return;
	}
	// 用返回值覆盖草稿：越界值会被后端归一化，界面要显示真正生效的值。
	draft.value = { ...saved };
	message.success("设置已保存");
}
</script>

<template>
	<section class="settings-view gx-col">
		<GlassPanel padding="16px 18px">
			<p class="gx-section-title">休息提醒</p>
			<div class="rows">
				<div class="row">
					<div class="row-text">
						<span class="row-label">开启休息提醒</span>
						<span class="row-hint gx-faint">连续学习到设定间隔时提醒你起来活动一下</span>
					</div>
					<NSwitch v-model:value="draft.breakReminderEnabled" />
				</div>
				<div class="row">
					<div class="row-text">
						<span class="row-label">提醒间隔</span>
						<span class="row-hint gx-faint">1 ~ 480 分钟，按已连续学习的时间计算</span>
					</div>
					<div class="inline">
						<NInputNumber
							v-model:value="intervalMin"
							class="num"
							:min="1"
							:max="480"
							:disabled="!draft.breakReminderEnabled"
						/>
						<span class="unit gx-muted">分钟</span>
					</div>
				</div>
			</div>
		</GlassPanel>

		<GlassPanel padding="16px 18px">
			<p class="gx-section-title">窗口行为</p>
			<div class="rows">
				<div class="row">
					<div class="row-text">
						<span class="row-label">关闭窗口时最小化到托盘</span>
						<span class="row-hint gx-faint">关闭后计时与休息提醒仍在后台运行</span>
					</div>
					<NSwitch v-model:value="draft.minimizeToTray" />
				</div>
			</div>
		</GlassPanel>

		<GlassPanel padding="16px 18px">
			<p class="gx-section-title">外观</p>
			<div class="rows">
				<div class="row">
					<div class="row-text">
						<span class="row-label">主题</span>
						<span class="row-hint gx-faint">窗口毛玻璃是常开的，没有开关</span>
					</div>
					<NRadioGroup v-model:value="themeValue" size="small">
						<NRadioButton value="system">跟随系统</NRadioButton>
						<NRadioButton value="light">浅色</NRadioButton>
						<NRadioButton value="dark">深色</NRadioButton>
					</NRadioGroup>
				</div>
			</div>
		</GlassPanel>

		<GlassPanel padding="16px 18px">
			<p class="gx-section-title">数据</p>
			<div class="path gx-mono gx-truncate" :title="dataPath">
				{{ dataPath || "读取中…" }}
			</div>
			<button
				type="button"
				class="ghost gx-ghost gx-pill"
				:disabled="openingDir"
				@click="openDataDir"
			>
				打开所在文件夹
			</button>
		</GlassPanel>

		<GlassPanel padding="16px 18px">
			<p class="gx-section-title">关于</p>
			<div class="about">
				<span class="about-name">孤雪凝时</span>
				<span class="gx-muted">一个把专注时间记下来的小工具：计时、记录、计划与休息提醒。</span>
				<span class="about-stack gx-mono gx-faint">Go + Wails v3 + Vue 3</span>
			</div>
		</GlassPanel>

		<div class="footer">
			<button type="button" class="save gx-accent-btn gx-pill" :disabled="saving" @click="onSave">
				{{ saving ? "保存中…" : "保存" }}
			</button>
			<p v-if="loadError" class="error">{{ loadError }}</p>
			<p v-if="saveError" class="error">{{ saveError }}</p>
		</div>
	</section>
</template>

<style scoped>
.settings-view {
	gap: var(--gx-gap);
}

.rows {
	display: flex;
	flex-direction: column;
	gap: 14px;
	margin-top: 12px;
}

.row {
	display: flex;
	align-items: center;
	gap: 12px;
	min-width: 0;
}

.row-text {
	display: flex;
	flex-direction: column;
	gap: 1px;
	flex: 1 1 auto;
	min-width: 0;
}

.row-label {
	font-size: 13.5px;
}

.row-hint {
	font-size: 11.5px;
}

.inline {
	display: flex;
	align-items: center;
	gap: 7px;
	flex: 0 0 auto;
}

.num {
	width: 104px;
}

.unit {
	font-size: 12.5px;
}

.path {
	padding: 9px 12px;
	margin: 12px 0 10px;
	border-radius: var(--gx-r-sm);
	background: var(--gx-glass-2-bg);
	border: 1px solid var(--gx-glass-2-border);
	font-size: 12px;
	color: var(--gx-text-2);
}

.ghost {
	padding: 6px 13px;
	font-size: 12.5px;
	font-family: inherit;
	cursor: pointer;
}

.about {
	display: flex;
	flex-direction: column;
	gap: 3px;
	margin-top: 10px;
	font-size: 13px;
}

.about-name {
	font-size: 15px;
	font-weight: 650;
}

.about-stack {
	font-size: 12px;
}

.footer {
	display: flex;
	flex-direction: column;
	gap: 8px;
	padding-bottom: 4px;
}

.save {
	width: 100%;
	padding: 11px 18px;
	font-size: 14.5px;
	font-weight: 600;
	font-family: inherit;
	cursor: pointer;
}

.save:disabled {
	cursor: not-allowed;
}

.error {
	font-size: 12.5px;
	color: var(--gx-danger);
}
</style>
