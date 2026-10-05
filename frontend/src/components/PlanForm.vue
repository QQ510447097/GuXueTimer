<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NInput } from "naive-ui";
import * as PlansService from "../../bindings/goxuetimer/plansservice";
import type { PlanDTO } from "../../bindings/goxuetimer/models";
import PlanChip from "./PlanChip.vue";
import { errText } from "../composables/format";
import { usePlans } from "../composables/usePlans";

/**
 * 新建计划表单。
 *
 * 抽成组件是因为它出现在两个地方：没有任何计划时的引导页，以及计时页计划选择器
 * 底部的「＋ 新建计划」。两处长得不一样（一个是大号引导，一个是弹出层里的紧凑形态），
 * 但规则完全一样——名字长度、配色白名单、重名报错都来自后端，前端不该各写一遍。
 *
 * 它只管"创建"，不管创建之后干什么：引导页要刷新外壳（外壳看 activePlans 自己会切走），
 * 选择器要把它设为当前计划。那些由 created 事件的订阅方决定。
 */
const props = withDefaults(
	defineProps<{
		submitLabel?: string;
		/** 紧凑形态：用在弹出层里，输入框小一号、色板更密。 */
		compact?: boolean;
		autofocus?: boolean;
		/** 是否显示"取消"（弹出层里需要，引导页里没有可取消的东西）。 */
		cancelable?: boolean;
	}>(),
	{
		submitLabel: "创建",
		compact: false,
		autofocus: false,
		cancelable: false,
	},
);

const emit = defineEmits<{
	(e: "created", plan: PlanDTO): void;
	(e: "cancel"): void;
}>();

const { palette } = usePlans();

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
		const plan = await PlansService.Create(planName, color.value);
		// 清空名字好让"连着建两个"不用手删：色板保留上一次的选择，
		// 连着建同一科的多个细分计划时少点几下。
		name.value = "";
		emit("created", plan);
	} catch (err) {
		error.value = errText(err);
	} finally {
		busy.value = false;
	}
}
</script>

<template>
	<div class="plan-form" :class="{ 'is-compact': props.compact }">
		<div class="field">
			<span v-if="!props.compact" class="gx-section-title">计划名称</span>
			<!-- 回车提交挂在外层 div：NInput 的包裹元素一定能收到内部 input 冒泡的 keyup -->
			<div @keyup.enter="submit">
				<NInput
					v-model:value="name"
					:size="props.compact ? 'small' : 'large'"
					:placeholder="props.compact ? '新计划名称' : '例如：考研数学'"
					:maxlength="24"
					:autofocus="props.autofocus"
				/>
			</div>
		</div>

		<div class="field">
			<span v-if="!props.compact" class="gx-section-title">选择一个配色</span>
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

		<div class="actions">
			<button
				type="button"
				class="submit gx-accent-btn gx-pill"
				:class="{ 'is-compact': props.compact }"
				:disabled="!canSubmit"
				@click="submit"
			>
				{{ busy ? "创建中…" : props.submitLabel }}
			</button>
			<button
				v-if="props.cancelable"
				type="button"
				class="cancel gx-ghost gx-pill"
				@click="emit('cancel')"
			>
				取消
			</button>
		</div>

		<p v-if="error" class="error">{{ error }}</p>
	</div>
</template>

<style scoped>
.plan-form {
	display: flex;
	flex-direction: column;
	min-width: 0;
}

.field {
	display: flex;
	flex-direction: column;
	gap: 7px;
	margin-top: 20px;
}

.is-compact .field {
	gap: 6px;
	margin-top: 8px;
}

.swatches {
	display: flex;
	flex-wrap: wrap;
	gap: 10px;
	/* 选中环是 outline + 2px offset，留出内边距免得被父容器裁掉 */
	padding: 3px 4px;
}

.is-compact .swatches {
	gap: 7px;
	padding: 2px 3px;
}

.actions {
	display: flex;
	align-items: center;
	gap: 8px;
	margin-top: 22px;
}

.is-compact .actions {
	margin-top: 10px;
}

.submit {
	width: 100%;
	padding: 12px 18px;
	font-size: 15px;
	font-weight: 600;
	font-family: inherit;
	cursor: pointer;
}

.submit.is-compact {
	padding: 7px 14px;
	font-size: 13px;
}

.submit:disabled {
	cursor: not-allowed;
}

.cancel {
	flex: 0 0 auto;
	padding: 7px 14px;
	font-size: 13px;
	font-family: inherit;
	cursor: pointer;
}

.error {
	margin-top: 10px;
	font-size: 12.5px;
	color: var(--gx-danger);
}
</style>
