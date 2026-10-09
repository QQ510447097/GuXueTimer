<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { NRadioButton, NRadioGroup, NSelect, NSwitch, useMessage } from "naive-ui";
import GlassPanel from "../components/GlassPanel.vue";
import StepNumber from "../components/StepNumber.vue";
import { usePersonalization } from "../composables/usePersonalization";
import { useSettings } from "../composables/useSettings";

/**
 * 个性化页。
 *
 * 装两件事：外观（主题 + 将来会有多张背景图）和计时页上出现哪些模块。
 * 主题与休息提醒都是从设置页搬过来的——设置页管的是"应用怎么工作"
 * （托盘、数据、关于），而"长什么样、页面上有什么"是另一回事，
 * 混在一页会让设置页变成什么都往里塞的杂物抽屉。
 *
 * 这一页刻意**不做预览、也不撑满窗口**：预览会把"用户即将看到的"和
 * "当前这一页"混在一起看，而且为了让它有意义还得把卡片撑到和计时页一样高，
 * 于是整页被一块预览占满。控件本身就是自解释的，下面留白就留白。
 */
const { settings, patch } = useSettings();
const { imageUrl, picking, error: imageError, load: loadBackground, pick, clear } =
	usePersonalization();
const message = useMessage();

onMounted(() => {
	void loadBackground();
});

/*
 * 底部区域的下拉选项。
 *
 * 「其他组件」是**故意留的占位**：位置先占住（这样以后加进来不用重排这一页），
 * 但用 disabled 挡住，选不中。naive-ui 的 disabled 选项点不动、也是灰的。
 */
const bottomOptions = [
	{ label: "留空", value: "none" },
	{ label: "图片", value: "image" },
	{ label: "其他组件（暂未支持）", value: "other", disabled: true },
];

/** 当前值：只认 none 与 image，别的一律当留空（与后端 Normalize 同一套规则）。 */
const cardMode = computed(() => (settings.value.cardMode === "image" ? "image" : "none"));

/** 有一张图、但当前不是图片模式时给个提示，免得以为图丢了。 */
const imageIdle = computed(() => cardMode.value !== "image" && imageUrl.value !== "");

/*
 * NRadioGroup / NSelect 的值是 string | number | boolean，
 * 收窄成我们要的枚举再落库。用显式处理函数而不是在模板里给 computed 赋值：
 * 后者依赖模板编译器把写操作改写成 .value，读起来要绕一层。
 */
function setTheme(value: string): void {
	void patch({ theme: value === "light" || value === "dark" ? value : "system" });
}

function setCardMode(value: string): void {
	// 只放行这两个值：万一把 disabled 的选项透传进来，也不会写进设置。
	if (value !== "none" && value !== "image") return;
	void patch({ cardMode: value });
}

/*
 * 提醒间隔不能每敲一键就提交：StepNumber 只在失焦/回车/加减时发 commit。
 * 一份本地草稿 + 被别处改动时同步，这套做法原来在设置页里。
 */
const intervalDraft = ref<number>(settings.value.breakIntervalMin);
watch(
	settings,
	(next) => {
		intervalDraft.value = next.breakIntervalMin;
	},
	{ immediate: true },
);

function commitInterval(): void {
	if (intervalDraft.value === settings.value.breakIntervalMin) return;
	void patch({ breakIntervalMin: intervalDraft.value });
}

async function onPickImage(): Promise<void> {
	const changed = await pick();
	if (changed) message.success("背景图已更新");
}

async function onClearImage(): Promise<void> {
	await clear();
	message.success("已移除背景图");
}
</script>

<template>
	<section class="personalize-view gx-col">
		<GlassPanel padding="16px 18px">
			<p class="gx-section-title">外观</p>
			<div class="rows">
				<div class="row">
					<div class="row-text">
						<span class="row-label">主题</span>
					</div>
					<NRadioGroup :value="settings.theme" size="small" @update:value="setTheme">
						<NRadioButton value="system">跟随系统</NRadioButton>
						<NRadioButton value="light">浅色</NRadioButton>
						<NRadioButton value="dark">深色</NRadioButton>
					</NRadioGroup>
				</div>

				<!-- 占位：多张背景图还没做（先只留位置，选项本身禁用） -->
				<div class="row">
					<div class="row-text">
						<span class="row-label">多张背景图</span>
					</div>
					<NSelect
						class="pick"
						size="small"
						disabled
						:value="null"
						placeholder="待定"
						:options="[{ label: '待定', value: 'pending' }]"
					/>
				</div>
			</div>
		</GlassPanel>

		<GlassPanel padding="16px 18px">
			<p class="gx-section-title">计时页</p>

			<div class="rows">
				<div class="row">
					<div class="row-text">
						<span class="row-label">底部区域</span>
					</div>
					<NSelect
						class="pick"
						size="small"
						data-bottom-select
						:data-bottom="cardMode"
						:value="cardMode"
						:options="bottomOptions"
						@update:value="setCardMode"
					/>
				</div>

				<!-- 选「图片」时才需要这些控件 -->
				<div v-if="cardMode === 'image'" class="row">
					<div class="row-text">
						<span class="row-label">背景图</span>
						<span class="row-hint gx-faint">支持 PNG / JPEG / GIF</span>
					</div>
					<div class="actions">
						<button
							type="button"
							class="mini gx-ghost gx-pill gx-lift"
							:disabled="picking"
							@click="onPickImage"
						>
							{{ picking ? "选择中…" : "选择图片" }}
						</button>
						<button
							v-if="imageUrl"
							type="button"
							class="mini gx-ghost gx-pill gx-lift"
							@click="onClearImage"
						>
							移除
						</button>
					</div>
				</div>
				<p v-if="imageIdle" class="note gx-faint">已保存一张背景图，选「图片」即可显示</p>
				<p v-if="imageError" class="error">{{ imageError }}</p>

				<div class="row">
					<div class="row-text">
						<span class="row-label">休息提醒</span>
					</div>
					<NSwitch
						:value="settings.breakReminderEnabled"
						@update:value="(value: boolean) => patch({ breakReminderEnabled: value })"
					/>
				</div>

				<div class="row">
					<div class="row-text">
						<span class="row-label">提醒间隔</span>
					</div>
					<StepNumber
						v-model="intervalDraft"
						:disabled="!settings.breakReminderEnabled"
						suffix="分钟"
						@commit="commitInterval"
					/>
				</div>
			</div>
		</GlassPanel>
	</section>
</template>

<style scoped>
.personalize-view {
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

/* 下拉不要占满整行：它只是这一项的参数，宽度固定更好对齐 */
.pick {
	width: 190px;
	flex: 0 0 auto;
}

.actions {
	display: flex;
	align-items: center;
	gap: 8px;
	flex-wrap: wrap;
}

.mini {
	padding: 5px 12px;
	font-size: 12.5px;
	font-family: inherit;
	cursor: pointer;
}

.note,
.error {
	font-size: 12.5px;
}

.error {
	color: var(--gx-danger);
}
</style>
