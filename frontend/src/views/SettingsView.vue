<script setup lang="ts">
import { ref, watch } from "vue";
import { NInputNumber, NSwitch, useMessage } from "naive-ui";
import * as SettingsService from "../../bindings/goxuetimer/settingsservice";
import GlassPanel from "../components/GlassPanel.vue";
import { errText } from "../composables/format";
import { useSettings } from "../composables/useSettings";

/**
 * 设置页。
 *
 * 没有保存按钮：改一下就立即生效。
 *
 * 早先的版本是一份本地草稿 + 一个"保存"按钮，理由是"改错了不可撤销"。但实测下来
 * 这个理由站不住：这里的每一项都是所见即所得的显示偏好（提醒开关、间隔、托盘行为），
 * 看着不合适再改回来就是了，反倒是"改完忘了点保存、以为已经生效"更容易真的踩到。
 * 现在唯一需要"提交时机"的是数字输入框——它每敲一个键都会发 update:value，
 * 所以打字期间只改本地值，失焦或回车才提交（见 commitInterval）。
 *
 * 外观（主题）搬去了「个性化」页，那是它更该在的地方。
 */
const { settings, saving, loadError, saveError, patch } = useSettings();
const message = useMessage();

/** 数字输入不能每敲一键就提交：它每敲一键都会发 update:value。 */
const intervalDraft = ref<number | null>(settings.value.breakIntervalMin);
watch(
	settings,
	(next) => {
		intervalDraft.value = next.breakIntervalMin;
	},
	{ immediate: true },
);

function commitInterval(): void {
	const value = intervalDraft.value;
	if (value === null || value === settings.value.breakIntervalMin) return;
	void patch({ breakIntervalMin: value });
}

/** 开关类设置：动一下就提交，失败已经在 saveError 里。 */
function commit(partial: Parameters<typeof patch>[0]): void {
	void patch(partial);
}

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
					<NSwitch
						:value="settings.breakReminderEnabled"
						@update:value="(value: boolean) => commit({ breakReminderEnabled: value })"
					/>
				</div>
				<div class="row">
					<div class="row-text">
						<span class="row-label">提醒间隔</span>
						<span class="row-hint gx-faint">1 ~ 480 分钟，按已连续学习的时间计算</span>
					</div>
					<div class="inline">
						<NInputNumber
							v-model:value="intervalDraft"
							class="num"
							:min="1"
							:max="480"
							:disabled="!settings.breakReminderEnabled"
							@blur="commitInterval"
							@keyup.enter="commitInterval"
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
					<NSwitch
						:value="settings.minimizeToTray"
						@update:value="(value: boolean) => commit({ minimizeToTray: value })"
					/>
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

		<!-- 没有保存按钮，所以这里只留一行状态：正在写、写失败、或提示"改了就算数" -->
		<div class="footer">
			<p v-if="loadError" class="error">{{ loadError }}</p>
			<p v-else-if="saveError" class="error">{{ saveError }}</p>
			<p v-else class="ok gx-faint">{{ saving ? "保存中…" : "改动会立即生效" }}</p>
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
	padding: 2px 2px 4px;
}

.ok {
	font-size: 12.5px;
}

.error {
	font-size: 12.5px;
	color: var(--gx-danger);
}
</style>
