<script setup lang="ts">
import { computed } from "vue";
import { useDialog } from "naive-ui";
import type { SessionDTO } from "../../bindings/goxuetimer/models";
import IconTrash from "./IconTrash.vue";
import { formatClockHM, formatDurationCN } from "../composables/format";
import { usePlans } from "../composables/usePlans";

/**
 * 一条学习记录。
 *
 * 内容分两行排：计划名可能很长，时间、时长、删除按钮又都不能省，
 * 塞成一行在 448px 的最小内容宽度下必然溢出（或把计划名压成一个字）。
 *
 * 删除在这里确认，而不是把弹窗塞给上层：确认文案需要计划名与时长，
 * 它们本来就在这里。
 */
const props = defineProps<{ session: SessionDTO }>();
const emit = defineEmits<{ (e: "remove", id: number): void }>();

const dialog = useDialog();
const { nameOf, colorOf } = usePlans();

const planName = computed(() => nameOf(props.session.planId));
const planColor = computed(() => colorOf(props.session.planId));
const timeRange = computed(
	() => `${formatClockHM(props.session.startedAt)} – ${formatClockHM(props.session.endedAt)}`,
);
const recovered = computed(() => props.session.source === "recovered");

function confirmRemove(): void {
	dialog.warning({
		title: "删除这条记录",
		content: `${planName.value} · ${formatDurationCN(props.session.durationSec)}，删除后无法恢复。`,
		positiveText: "删除",
		negativeText: "取消",
		onPositiveClick: () => {
			emit("remove", props.session.id);
		},
	});
}
</script>

<template>
	<div class="row gx-glass-2 gx-lift">
		<div class="line">
			<span class="gx-dot" :style="{ background: planColor, color: planColor }"
				aria-hidden="true"></span>
			<span class="name gx-truncate">{{ planName }}</span>
			<span class="duration gx-mono gx-tabular">{{ formatDurationCN(session.durationSec) }}</span>
		</div>
		<div class="line line-2">
			<span class="time gx-mono gx-tabular">{{ timeRange }}</span>
			<span v-if="recovered" class="recovered">恢复</span>
			<span class="gx-spacer"></span>
			<button type="button" class="remove" title="删除这条记录" @click="confirmRemove">
				<IconTrash :size="14" />
			</button>
		</div>
	</div>
</template>

<style scoped>
.row {
	display: flex;
	flex-direction: column;
	gap: 3px;
	padding: 9px 12px;
}

.line {
	display: flex;
	align-items: center;
	gap: 8px;
	min-width: 0;
}

.name {
	flex: 1 1 auto;
	font-size: 13.5px;
	font-weight: 600;
}

.duration {
	flex: 0 0 auto;
	font-size: 13px;
	color: var(--gx-text-2);
}

.line-2 {
	gap: 7px;
}

.time {
	font-size: 12px;
	color: var(--gx-text-3);
}

.recovered {
	flex: 0 0 auto;
	padding: 0 7px;
	border-radius: var(--gx-r-pill);
	background: var(--gx-accent-soft);
	color: var(--gx-accent);
	font-size: 11px;
	line-height: 17px;
}

.remove {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	width: 24px;
	height: 24px;
	padding: 0;
	border: none;
	border-radius: 50%;
	background: transparent;
	color: var(--gx-text-3);
	cursor: pointer;
	transition: color var(--gx-dur) var(--gx-ease), background-color var(--gx-dur) var(--gx-ease);
}

.remove:hover {
	color: var(--gx-danger);
	background: var(--gx-accent-soft);
}
</style>
