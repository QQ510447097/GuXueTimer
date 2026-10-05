<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NInput, useMessage } from "naive-ui";
import * as PlansService from "../../bindings/goxuetimer/plansservice";
import GlassPanel from "../components/GlassPanel.vue";
import PlanChip from "../components/PlanChip.vue";
import { errText } from "../composables/format";
import { usePlans } from "../composables/usePlans";

/**
 * 引导页：一个计划都没有时，整个外壳换成这一屏。
 *
 * 这里刻意不发任何"创建完成"事件：外壳看的是 usePlans 的 activePlans，
 * 刷新之后它自己就会切到计时页。多一条事件就多一处可能不一致的状态。
 *
 * 它是外壳的兄弟节点（不在 .content 里），所以要自己撑满窗口并居中。
 */
const { palette, refresh } = usePlans();
const message = useMessage();

const name = ref("");
const color = ref("");
const busy = ref(false);
const error = ref("");

// 配色白名单来自后端；首帧还没回来时不能瞎猜，等它到了再默认选中第一个。
watch(
	palette,
	(colors) => {
		if (!color.value && colors.length > 0) color.value = colors[0];
	},
	{ immediate: true },
);

const canSubmit = computed(
	() => name.value.trim().length > 0 && color.value.length > 0 && !busy.value,
);

async function submit(): Promise<void> {
	const planName = name.value.trim();
	if (!planName || busy.value) return;
	if (!color.value) {
		error.value = "还没有拿到可用配色，请稍后再试";
		return;
	}
	busy.value = true;
	error.value = "";
	try {
		// 第一个计划后端会自动设为当前计划。
		await PlansService.Create(planName, color.value);
		await refresh();
		message.success(`已创建「${planName}」`);
	} catch (err) {
		error.value = errText(err);
		message.error(error.value);
	} finally {
		busy.value = false;
	}
}
</script>

<template>
	<div class="onboarding">
		<GlassPanel class="card" padding="28px 26px">
			<div class="brand">
				<span class="mark" aria-hidden="true">雪</span>
				<div class="titles">
					<h1 class="title">孤雪凝时</h1>
					<p class="sub gx-muted">
						先建一个计划。之后每一段专注都会记在它名下，可以随时改名、归档。
					</p>
				</div>
			</div>

			<div class="field">
				<span class="gx-section-title">计划名称</span>
				<!-- 回车提交挂在外层 div：NInput 的包裹元素一定能收到内部 input 冒泡的 keyup -->
				<div @keyup.enter="submit">
					<NInput
						v-model:value="name"
						size="large"
						placeholder="例如：考研数学"
						:maxlength="24"
						autofocus
					/>
				</div>
			</div>

			<div class="field">
				<span class="gx-section-title">选择一个配色</span>
				<div class="swatches">
					<PlanChip
						v-for="option in palette"
						:key="option"
						variant="swatch"
						:color="option"
						:selected="color === option"
						@click="color = option"
					/>
					<span v-if="palette.length === 0" class="gx-faint">正在加载配色…</span>
				</div>
			</div>

			<button type="button" class="submit gx-accent-btn gx-pill" :disabled="!canSubmit" @click="submit">
				{{ busy ? "创建中…" : "创建并开始" }}
			</button>

			<p v-if="error" class="error">{{ error }}</p>
		</GlassPanel>
	</div>
</template>

<style scoped>
.onboarding {
	/* 背景层是 fixed + z-index 0，这一屏要自己浮在它上面 */
	position: relative;
	z-index: 1;
	display: flex;
	align-items: center;
	justify-content: center;
	width: 100%;
	height: 100%;
	padding: 28px 22px;
	overflow-y: auto;
}

.card {
	width: 100%;
	max-width: 420px;
}

.brand {
	display: flex;
	align-items: center;
	gap: 12px;
}

.mark {
	display: flex;
	align-items: center;
	justify-content: center;
	width: 42px;
	height: 42px;
	flex: 0 0 auto;
	border-radius: 14px;
	background: var(--gx-accent-soft);
	color: var(--gx-accent);
	font-size: 21px;
	font-weight: 700;
}

.titles {
	display: flex;
	flex-direction: column;
	gap: 3px;
	min-width: 0;
}

.title {
	font-size: 19px;
	font-weight: 700;
	letter-spacing: 0.01em;
}

.sub {
	font-size: 12.5px;
	line-height: 1.5;
}

.field {
	display: flex;
	flex-direction: column;
	gap: 7px;
	margin-top: 20px;
}

.swatches {
	display: flex;
	flex-wrap: wrap;
	gap: 10px;
	/* 选中环是 outline + 2px offset，留出内边距免得被父容器裁掉 */
	padding: 3px 4px;
}

.submit {
	width: 100%;
	margin-top: 22px;
	padding: 12px 18px;
	font-size: 15px;
	font-weight: 600;
	font-family: inherit;
	cursor: pointer;
}

.submit:disabled {
	cursor: not-allowed;
}

.error {
	margin-top: 10px;
	font-size: 12.5px;
	color: var(--gx-danger);
}
</style>
