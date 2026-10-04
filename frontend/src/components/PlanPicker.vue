<script setup lang="ts">
import { computed, ref } from "vue";
import { NPopover } from "naive-ui";
import type { PlanDTO } from "../../bindings/goxuetimer/models";
import IconDown from "./IconDown.vue";

/**
 * 当前计划选择器。
 *
 * 计时中会被禁用（后端也拒绝切换）：这段时长算谁的不能在用户没察觉时改变，
 * 所以禁用原因要写在旁边，而不是只把按钮变灰了事。
 *
 * 用 NPopover 而不是 NDropdown：选项要带计划色点，自定义列表比
 * 往下拉菜单里塞渲染函数更好读，也更好控制玻璃观感。
 */
const props = withDefaults(
	defineProps<{
		plans: PlanDTO[];
		currentId: number;
		disabled?: boolean;
		hint?: string;
	}>(),
	{ disabled: false, hint: "" },
);

const emit = defineEmits<{ (e: "select", id: number): void }>();

const open = ref(false);

const current = computed(() => props.plans.find((p) => p.id === props.currentId) ?? null);
const currentName = computed(() => current.value?.name ?? "未选择计划");
const currentColor = computed(() => current.value?.color ?? "var(--gx-text-3)");

function choose(id: number): void {
	open.value = false;
	if (id !== props.currentId) emit("select", id);
}
</script>

<template>
	<div class="picker">
		<NPopover
			v-model:show="open"
			trigger="click"
			placement="bottom-start"
			:disabled="disabled || plans.length === 0"
			:show-arrow="false"
		>
			<template #trigger>
				<button
					type="button"
					class="trigger gx-glass-2 gx-pill gx-lift"
					:class="{ 'is-disabled': disabled }"
					:disabled="disabled || plans.length === 0"
					:title="disabled ? hint : '切换当前计划'"
				>
					<span class="gx-dot" :style="{ background: currentColor, color: currentColor }"
						aria-hidden="true"></span>
					<span class="name gx-truncate">{{ currentName }}</span>
					<IconDown :size="14" class="chev" />
				</button>
			</template>

			<div class="menu">
				<button
					v-for="plan in plans"
					:key="plan.id"
					type="button"
					class="item"
					:class="{ 'is-current': plan.id === currentId }"
					@click="choose(plan.id)"
				>
					<span class="gx-dot" :style="{ background: plan.color, color: plan.color }"
						aria-hidden="true"></span>
					<span class="item-name gx-truncate">{{ plan.name }}</span>
					<span v-if="plan.id === currentId" class="check" aria-hidden="true">✓</span>
				</button>
			</div>
		</NPopover>

		<span v-if="disabled && hint" class="hint gx-faint">{{ hint }}</span>
	</div>
</template>

<style scoped>
.picker {
	display: flex;
	align-items: center;
	gap: 8px;
	min-width: 0;
}

.trigger {
	display: inline-flex;
	align-items: center;
	gap: 7px;
	max-width: 100%;
	padding: 6px 12px;
	font-size: 13px;
	font-family: inherit;
	color: var(--gx-text);
	cursor: pointer;
}

.trigger.is-disabled {
	cursor: not-allowed;
	opacity: 0.62;
}

.name {
	font-weight: 600;
	min-width: 0;
}

.chev {
	color: var(--gx-text-3);
	flex: 0 0 auto;
}

.hint {
	font-size: 12px;
	white-space: nowrap;
}

.menu {
	display: flex;
	flex-direction: column;
	gap: 2px;
	min-width: 168px;
	max-width: 240px;
	padding: 4px;
}

.item {
	display: flex;
	align-items: center;
	gap: 8px;
	width: 100%;
	padding: 7px 9px;
	border: none;
	border-radius: var(--gx-r-xs);
	background: transparent;
	color: var(--gx-text);
	font-size: 13px;
	font-family: inherit;
	text-align: left;
	cursor: pointer;
	transition: background-color var(--gx-dur) var(--gx-ease);
}

.item:hover {
	background: var(--gx-accent-soft);
}

.item.is-current {
	color: var(--gx-accent);
	font-weight: 600;
}

.item-name {
	min-width: 0;
	flex: 1 1 auto;
}

.check {
	flex: 0 0 auto;
	color: var(--gx-accent);
}
</style>
