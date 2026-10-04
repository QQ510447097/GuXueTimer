<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { NAlert, NButton, NDivider, NInputNumber, NRadio, NRadioGroup, NSwitch, useMessage } from "naive-ui";
import type { SettingsDTO } from "../../bindings/goxuetimer/models";
import { useSettings, type ThemeChoice } from "../composables/useSettings";

const { settings, loading, saving, loadError, saveError, load, save } = useSettings();
const message = useMessage();

// 绑定生成器用 -i 产出的是接口（纯类型），clone 只能手写字段，不能 new。
function clone(dto: SettingsDTO): SettingsDTO {
	return {
		breakReminderEnabled: dto.breakReminderEnabled,
		breakIntervalMin: dto.breakIntervalMin,
		minimizeToTray: dto.minimizeToTray,
		theme: dto.theme,
	};
}

const draft = ref<SettingsDTO>(clone(settings.value));
const themeChoice = ref<ThemeChoice>("system");

function syncFrom(dto: SettingsDTO): void {
	draft.value = clone(dto);
	themeChoice.value =
		dto.theme === "light" || dto.theme === "dark" ? dto.theme : "system";
}

// 保存成功、或别处改了设置（比如托盘）之后，表单要显示真正生效的值。
watch(settings, (next) => syncFrom(next), { immediate: true });

async function onSave(): Promise<void> {
	const payload: SettingsDTO = {
		breakReminderEnabled: draft.value.breakReminderEnabled,
		breakIntervalMin: draft.value.breakIntervalMin ?? 45,
		minimizeToTray: draft.value.minimizeToTray,
		theme: themeChoice.value,
	};
	const saved = await save(payload);
	if (saved) {
		// 后端返回的是归一化后的结论，拿它覆盖表单，用户才知道实际生效值。
		syncFrom(saved);
		message.success("设置已保存");
		return;
	}
	message.error(saveError.value || "保存失败");
}

onMounted(() => {
	void load(true);
});
</script>

<template>
	<section class="settings-view">
		<h2 class="settings-title">设置</h2>

		<NAlert v-if="loadError" class="settings-alert" type="error" :bordered="false" title="读取设置失败">
			{{ loadError }}
		</NAlert>

		<div class="setting-group">
			<div class="setting-row">
				<div class="setting-label">
					<span>休息提醒</span>
					<span class="setting-desc">按固定间隔提醒你起来活动一下</span>
				</div>
				<NSwitch v-model:value="draft.breakReminderEnabled" />
			</div>

			<div class="setting-row" :class="{ 'is-dim': !draft.breakReminderEnabled }">
				<div class="setting-label">
					<span>提醒间隔</span>
					<span class="setting-desc">1 ~ 480 分钟，默认 45 分钟</span>
				</div>
				<div class="setting-control-inline">
					<NInputNumber v-model:value="draft.breakIntervalMin" :min="1" :max="480" :step="5" :disabled="!draft.breakReminderEnabled"
						class="interval-input" />
					<span class="unit">分钟</span>
				</div>
			</div>
		</div>

		<NDivider class="settings-divider" />

		<div class="setting-group">
			<div class="setting-row">
				<div class="setting-label">
					<span>关闭窗口时最小化到托盘</span>
					<span class="setting-desc">关闭后应用继续在托盘运行</span>
				</div>
				<NSwitch v-model:value="draft.minimizeToTray" />
			</div>

			<div class="setting-row setting-row-stack">
				<div class="setting-label">
					<span>主题</span>
					<span class="setting-desc">跟随系统会实时响应 Windows 的深浅色切换</span>
				</div>
				<NRadioGroup v-model:value="themeChoice" class="theme-group">
					<NRadio value="system">跟随系统</NRadio>
					<NRadio value="light">浅色</NRadio>
					<NRadio value="dark">深色</NRadio>
				</NRadioGroup>
			</div>
		</div>

		<NAlert v-if="saveError" class="settings-alert" type="error" :bordered="false" title="保存失败">
			{{ saveError }}
		</NAlert>

		<NButton class="settings-save" type="primary" size="large" block :loading="saving" :disabled="loading"
			@click="onSave">
			保存
		</NButton>
	</section>
</template>

<style scoped>
.settings-view {
	display: flex;
	flex-direction: column;
	min-height: 0;
}

.settings-title {
	margin: 0 0 12px;
	font-size: 16px;
	font-weight: 600;
}

.setting-group {
	display: flex;
	flex-direction: column;
	gap: 14px;
}

.setting-row {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 12px;
}

.setting-row-stack {
	flex-direction: column;
	align-items: stretch;
	gap: 8px;
}

.is-dim {
	opacity: 0.55;
}

.setting-label {
	display: flex;
	flex-direction: column;
	gap: 2px;
	min-width: 0;
	font-size: 14px;
}

.setting-desc {
	font-size: 12px;
	color: var(--gx-text-muted);
}

.setting-control-inline {
	display: flex;
	align-items: center;
	gap: 6px;
	flex: 0 0 auto;
}

.interval-input {
	width: 108px;
}

.unit {
	font-size: 13px;
	color: var(--gx-text-muted);
}

.theme-group {
	display: flex;
	flex-wrap: wrap;
	gap: 4px 14px;
}

.settings-divider {
	margin: 16px 0;
}

.settings-alert {
	margin-top: 14px;
}

.settings-save {
	margin-top: 18px;
}
</style>
