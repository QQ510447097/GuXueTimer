<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { NInput, NRadioButton, NRadioGroup, useMessage } from "naive-ui";
import GlassPanel from "../components/GlassPanel.vue";
import InspirationCard from "../components/InspirationCard.vue";
import { quotes, pickQuote, type Quote } from "../data/quotes";
import { todayKey } from "../composables/format";
import { usePersonalization } from "../composables/usePersonalization";
import { useSettings } from "../composables/useSettings";

/**
 * 个性化页。
 *
 * 分成两件事：外观（主题）与首页卡片。主题从设置页搬过来——设置页管的是
 * "应用怎么工作"（提醒、托盘、数据），而"长什么样"是另一回事，混在一页会让
 * 设置页变成什么都往里塞的杂物抽屉。
 */
const { settings, patch } = useSettings();
const { imageUrl, picking, error: imageError, load: loadBackground, pick, clear } =
	usePersonalization();
const message = useMessage();

onMounted(() => {
	void loadBackground();
});

// 渲染期取一次：跨过午夜还开着窗口属于边角情况，那时每日一句要等下次进页面才换。
const today = todayKey();

/** 生效中的那一句：自定义文字优先，其次是固定/每日一句。 */
const activeQuote = computed<Quote>(() => {
	const custom = settings.value.customQuote.trim();
	if (custom) return { text: custom, source: "" };
	return pickQuote(today, settings.value.quoteIndex);
});

const cardMode = computed(() => (settings.value.cardMode === "image" ? "image" : "quote"));

/*
 * NRadioGroup 的值是 string | number | boolean，收窄成我们要的枚举再落库。
 * 用显式的处理函数而不是在模板里给 computed 赋值：后者依赖模板编译器把写操作
 * 改写成 .value，读起来要绕一层，这里直接是"收到一个值、做一件事"。
 */
function setTheme(value: string): void {
	void patch({ theme: value === "light" || value === "dark" ? value : "system" });
}

function setCardMode(value: string): void {
	void patch({ cardMode: value === "image" ? "image" : "quote" });
}

/*
 * 自定义文字：不能每敲一个字就提交一次。
 * 用一份本地草稿，失焦或回车才写回设置；被别处改动时（例如点了「恢复每日一句」）
 * 再同步回来。
 */
const customDraft = ref(settings.value.customQuote);
watch(
	settings,
	(next) => {
		if (next.customQuote !== customDraft.value) customDraft.value = next.customQuote;
	},
);

function commitCustomQuote(): void {
	const text = customDraft.value.trim();
	if (text === settings.value.customQuote) return;
	void patch({ customQuote: text });
}

/** 换一句：把"每日一句"变成固定的某一条，每次往后挪一条。 */
function shuffleQuote(): void {
	const current = activeQuote.value;
	const index = quotes.findIndex((q) => q.text === current.text);
	// 找不到（说明当前用的是自定义文字）就从第一条开始。
	const next = index < 0 ? 1 : ((index + 1) % quotes.length) + 1;
	void patch({ customQuote: "", quoteIndex: next });
	customDraft.value = "";
}

/** 恢复"每日一句"：清掉固定选择与自定义文字。 */
function useDailyQuote(): void {
	void patch({ customQuote: "", quoteIndex: 0 });
	customDraft.value = "";
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
						<span class="row-hint gx-faint">窗口毛玻璃是常开的，没有开关</span>
					</div>
					<NRadioGroup :value="settings.theme" size="small" @update:value="setTheme">
						<NRadioButton value="system">跟随系统</NRadioButton>
						<NRadioButton value="light">浅色</NRadioButton>
						<NRadioButton value="dark">深色</NRadioButton>
					</NRadioGroup>
				</div>
			</div>
		</GlassPanel>

		<GlassPanel class="card-block" padding="16px 18px">
			<p class="gx-section-title">计时页卡片</p>

			<div class="rows">
				<div class="row">
					<div class="row-text">
						<span class="row-label">装饰方式</span>
						<span class="row-hint gx-faint">放在计时页下方那块空白里</span>
					</div>
					<NRadioGroup :value="cardMode" size="small" @update:value="setCardMode">
						<NRadioButton value="quote">语录</NRadioButton>
						<NRadioButton value="image">图片</NRadioButton>
					</NRadioGroup>
				</div>

				<!-- 语录：自定义文字 + 换一句 / 恢复每日一句 -->
				<template v-if="cardMode === 'quote'">
					<div class="row">
						<div class="row-text">
							<span class="row-label">自定义文字</span>
							<span class="row-hint gx-faint">留空则用内置语录（每天自动换）</span>
						</div>
						<NInput
							v-model:value="customDraft"
							class="quote-input"
							size="small"
							placeholder="写一句给自己的话"
							:maxlength="120"
							@blur="commitCustomQuote"
							@keyup.enter="commitCustomQuote"
						/>
					</div>
					<div class="actions">
						<button type="button" class="mini gx-ghost gx-pill gx-lift" @click="shuffleQuote">
							换一句
						</button>
						<button type="button" class="mini gx-ghost gx-pill gx-lift" @click="useDailyQuote">
							恢复每日一句
						</button>
						<span class="gx-faint tiny">
							{{ settings.quoteIndex > 0 ? `已固定第 ${settings.quoteIndex} 句` : "当前：每日一句" }}
						</span>
					</div>
				</template>

				<!-- 图片：选择 / 移除 -->
				<template v-else>
					<div class="row">
						<div class="row-text">
							<span class="row-label">背景图</span>
							<span class="row-hint gx-faint">
								支持 PNG / JPEG / GIF，会复制一份到数据目录
							</span>
						</div>
						<div class="pic-actions">
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
					<p v-if="imageError" class="error">{{ imageError }}</p>
				</template>
			</div>

			<p class="preview-label gx-faint">预览</p>
			<InspirationCard
				:mode="cardMode"
				:quote="activeQuote"
				:image-url="imageUrl"
				empty-hint="还没有选图，点上面的「选择图片」"
				@pick="onPickImage"
			/>
		</GlassPanel>
	</section>
</template>

<style scoped>
.personalize-view {
	gap: var(--gx-gap);
	/*
	 * 与计时页同理：让最后一块面板吸收剩余高度，页面下方不留一大片空白。
	 * 在这里它还多一个作用——预览会长到和计时页那张卡片一样高，
	 * 于是"预览"真的是预览，而不是一个缩小的示意。
	 */
	min-height: 100%;
}

/* 卡片那一块（含预览）吸收剩余高度 */
.card-block {
	display: flex;
	flex-direction: column;
	flex: 1 1 auto;
}

.card-block :deep(.inspiration) {
	flex: 1 1 auto;
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

/* 输入框不能占满整行：它不是这一页的主角，只是"语录"这一项的参数 */
.quote-input {
	width: 240px;
	flex: 0 0 auto;
}

.actions,
.pic-actions {
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

.tiny {
	font-size: 11.5px;
}

.preview-label {
	margin: 16px 0 8px;
	font-size: 11.5px;
}

.error {
	margin-top: 2px;
	font-size: 12.5px;
	color: var(--gx-danger);
}
</style>
