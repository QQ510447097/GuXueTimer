<script setup lang="ts">
import { ref } from "vue";
import { NSwitch, useMessage } from "naive-ui";
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
 * 这个理由站不住：这里的每一项都是所见即所得的偏好（托盘行为、数据目录），
 * 看着不合适再改回来就是了，反倒是"改完忘了点保存、以为已经生效"更容易真的踩到。
 *
 * 主题与休息提醒都在「个性化」里——它们决定界面长什么样、计时页上有什么。
 * 这一页只留 窗口行为 / 数据 / 关于。
 */
const { settings, loadError, saveError, patch } = useSettings();
const message = useMessage();

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

// 版本号由后端给：唯一真相是 main.go 的 appVersion，exe 的版本信息在 build/config.yml。
const version = ref("");
void SettingsService.Version()
	.then((v) => {
		version.value = v || "";
	})
	.catch(() => {
		version.value = "";
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
			<p class="gx-section-title">窗口行为</p>
			<div class="rows">
				<div class="row">
					<div class="row-text">
						<span class="row-label">关闭窗口时最小化到托盘</span>
						<span class="row-hint gx-faint">关闭后计时仍在后台运行</span>
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
				<span class="about-head">
					<span class="about-name">孤雪凝时</span>
					<span v-if="version" class="about-version gx-faint">v{{ version }}</span>
				</span>
				<span class="gx-muted">雪落无声，专注有痕。一个记录时间足迹的工具。</span>
				<span class="about-stack gx-mono gx-faint">Go + Wails v3 + Vue 3</span>
			</div>
		</GlassPanel>

		<!-- 没有保存按钮，所以这里只在真的出错时说话：顺利保存是理所当然的，不必报喜 -->
		<div v-if="loadError || saveError" class="footer">
			<p class="error">{{ loadError || saveError }}</p>
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

/* 名字与版本号同一行：版本号跟着名字走，保持灰色小字 */
.about-head {
	display: flex;
	align-items: baseline;
	gap: 6px;
}

.about-name {
	font-size: 15px;
	font-weight: 650;
}

.about-version {
	font-size: 12px;
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

.error {
	font-size: 12.5px;
	color: var(--gx-danger);
}
</style>
