<script setup lang="ts">
import { computed, ref } from "vue";
import { NPopover } from "naive-ui";
import type { PlanDTO } from "../../bindings/goxuetimer/models";
import IconDown from "./IconDown.vue";
import PlanForm from "./PlanForm.vue";

/**
 * 当前计划选择器。
 *
 * 计时中也能切：后端会先把这段会话结算到原来那个计划上，再切过去
 * （早先是拒绝切换，理由是"这段时长算谁的不能在用户没察觉时改变"；但那个做法
 * 要求用户先手动结束一次，多两步还容易忘，而现在归属并没有丢）。
 *
 * 底部带「＋ 新建计划」内联表单：想记一科新内容时不必先跑去计划页。
 *
 * 用 NPopover 而不是 NDropdown：选项要带计划色点，自定义列表比
 * 往下拉菜单里塞渲染函数更好读，也更好控制玻璃观感。
 */
const props = withDefaults(
	defineProps<{
		plans: PlanDTO[];
		currentId: number;
	}>(),
	{},
);

const emit = defineEmits<{
	(e: "select", id: number): void;
	(e: "created", plan: PlanDTO): void;
}>();

const open = ref(false);
const creating = ref(false);

const current = computed(() => props.plans.find((p) => p.id === props.currentId) ?? null);
const currentName = computed(() => current.value?.name ?? "未选择计划");
const currentColor = computed(() => current.value?.color ?? "var(--gx-text-3)");

function choose(id: number): void {
	open.value = false;
	creating.value = false;
	if (id !== props.currentId) emit("select", id);
}

/** 新建成功：关掉弹出层并把新计划交给父组件（它要负责设为当前）。 */
function onCreated(plan: PlanDTO): void {
	creating.value = false;
	open.value = false;
	emit("created", plan);
}

function close(): void {
	creating.value = false;
}
</script>

<template>
	<div class="picker">
		<NPopover
			v-model:show="open"
			trigger="click"
			placement="bottom-start"
			:disabled="plans.length === 0 && !creating"
			:show-arrow="false"
		>
			<template #trigger>
				<button
					type="button"
					class="trigger gx-glass-2 gx-pill gx-lift"
					:disabled="plans.length === 0 && !creating"
					title="切换当前计划"
				>
					<span class="gx-dot" :style="{ background: currentColor, color: currentColor }"
						aria-hidden="true"></span>
					<span class="name gx-truncate">{{ currentName }}</span>
					<IconDown :size="14" class="chev" />
				</button>
			</template>

			<div class="menu">
				<template v-if="!creating">
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

					<div class="sep" aria-hidden="true"></div>
					<button type="button" class="item new" @click="creating = true">
						<span class="plus" aria-hidden="true">＋</span>
						<span class="item-name">新建计划</span>
					</button>
				</template>

				<PlanForm v-else compact cancelable submit-label="创建并设为当前"
					@created="onCreated" @cancel="close" />
			</div>
		</NPopover>
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

.name {
	font-weight: 600;
	min-width: 0;
}

.chev {
	color: var(--gx-text-3);
	flex: 0 0 auto;
}

.menu {
	display: flex;
	flex-direction: column;
	gap: 2px;
	min-width: 168px;
	max-width: 260px;
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

.sep {
	height: 1px;
	margin: 4px 2px;
	background: var(--gx-line);
}

/* 「＋ 新建计划」：比普通项淡一点，读起来是"动作"而不是"另一个计划" */
.item.new {
	color: var(--gx-text-2);
}

.item.new:hover {
	color: var(--gx-accent);
}

.plus {
	flex: 0 0 auto;
	width: 8px;
	text-align: center;
	font-size: 13px;
}
</style>
