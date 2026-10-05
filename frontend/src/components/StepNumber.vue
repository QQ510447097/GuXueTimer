<script setup lang="ts">
import { computed, ref, watch } from "vue";
import IconMinus from "./IconMinus.vue";
import IconPlus from "./IconPlus.vue";
import { MAX_BREAK_INTERVAL_MIN, MIN_BREAK_INTERVAL_MIN, stepBy10 } from "../composables/format";

/**
 * 带加减按钮的数字输入。
 *
 * 用它替换 naive-ui 的 NInputNumber 有两个原因：
 *
 *   1. 步进规则不是"±1"。提醒间隔要按**最近的整十**走（43 → 50 → 60 / 43 → 40 → 30），
 *      见 format.ts 的 stepBy10。NInputNumber 的 step 只能固定一个值，做不到。
 *   2. 它的加减按钮在玻璃面板上太小，而这两个键是这一项最常用的操作。
 *
 * 手输仍然允许（后端会夹到 1~480），但**打字期间不提交**——提交时机由调用方决定
 * （失焦或回车），否则每敲一个数字都会写一次盘。
 *
 * data-step / data-stepper-value 是给 tools/ui_probe.mjs 断言步进规则用的：
 * "43 -> 50 -> 60" 这条规则得能被真的点出来，而不是只靠读代码相信它。
 */
const props = withDefaults(
	defineProps<{
		modelValue: number;
		min?: number;
		max?: number;
		disabled?: boolean;
		suffix?: string;
	}>(),
	{
		min: MIN_BREAK_INTERVAL_MIN,
		max: MAX_BREAK_INTERVAL_MIN,
		disabled: false,
		suffix: "",
	},
);

const emit = defineEmits<{
	(e: "update:modelValue", value: number): void;
	(e: "commit", value: number): void;
}>();

/*
 * 本地保留一份"我最后发出去的值"，而不是每次都用 props.modelValue 计算。
 *
 * 原因是连点：父组件的更新要等下一个 tick 才回流到这个组件，所以第二次点击读到的
 * props 还是旧值——两次都从 43 算起，连点两下加号只会到 50 而不是 60。
 * 保存一份本地的值，让连点按"我已经发到 50 了"继续往上算。
 *
 * 外部把值改成别的（后端归一化、或别处改了设置）时，下面这个 watch 会把本地值同步过去。
 */
const local = ref(props.modelValue);
watch(
	() => props.modelValue,
	(value) => {
		if (value !== local.value) local.value = value;
	},
);

const canDecrease = computed(() => !props.disabled && local.value > props.min);
const canIncrease = computed(() => !props.disabled && local.value < props.max);

/** 输入里显示的始终是当前值；打字只改本地值，失焦/回车才 commit。 */
function onInput(event: Event): void {
	const raw = (event.target as HTMLInputElement).value.replace(/[^\d]/g, "").slice(0, 3);
	// 边打字边同步数值，但**不发 commit**：commit 由失焦/回车触发。
	const next = raw === "" ? props.min : Number(raw);
	const clamped = Math.min(props.max, Math.max(props.min, next));
	local.value = clamped;
	emit("update:modelValue", clamped);
}

function step(dir: 1 | -1): void {
	const next = stepBy10(local.value, dir);
	if (next === local.value) return;
	local.value = next;
	emit("update:modelValue", next);
	emit("commit", next);
}

function commitTyped(): void {
	emit("commit", local.value);
}
</script>

<template>
	<div class="step-number" :class="{ 'is-disabled': props.disabled }">
		<button
			type="button"
			class="step"
			data-step="-1"
			:disabled="!canDecrease"
			:aria-label="`减少到最近的整十（当前 ${local}）`"
			@click="step(-1)"
		>
			<IconMinus :size="15" />
		</button>

		<input
			class="value gx-tabular"
			type="text"
			inputmode="numeric"
			:value="local"
			:disabled="props.disabled"
			:data-stepper-value="local"
			@input="onInput"
			@blur="commitTyped"
			@keyup.enter="commitTyped"
		/>

		<button
			type="button"
			class="step"
			data-step="1"
			:disabled="!canIncrease"
			:aria-label="`增加到最近的整十（当前 ${local}）`"
			@click="step(1)"
		>
			<IconPlus :size="15" />
		</button>

		<span v-if="props.suffix" class="suffix gx-muted">{{ props.suffix }}</span>
	</div>
</template>

<style scoped>
.step-number {
	display: inline-flex;
	align-items: center;
	gap: 2px;
	padding: 3px 4px;
	border-radius: var(--gx-r-sm);
	background: var(--gx-glass-2-bg);
	border: 1px solid var(--gx-glass-2-border);
}

.step-number.is-disabled {
	opacity: 0.55;
}

.step {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	width: 26px;
	height: 26px;
	border: none;
	border-radius: var(--gx-r-xs);
	background: transparent;
	color: var(--gx-text-2);
	cursor: pointer;
	transition: background-color var(--gx-dur) var(--gx-ease), color var(--gx-dur) var(--gx-ease);
}

.step:hover:not(:disabled) {
	background: var(--gx-accent-soft);
	color: var(--gx-accent);
}

.step:disabled {
	cursor: not-allowed;
	color: var(--gx-text-3);
	opacity: 0.6;
}

.value {
	width: 46px;
	padding: 3px 2px;
	border: none;
	background: transparent;
	color: var(--gx-text);
	font-family: inherit;
	font-size: 13.5px;
	font-weight: 600;
	text-align: center;
	outline: none;
}

.value:disabled {
	cursor: not-allowed;
}

.suffix {
	font-size: 12.5px;
	padding-right: 2px;
}
</style>
