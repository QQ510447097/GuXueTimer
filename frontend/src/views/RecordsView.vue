<script setup lang="ts">
import { computed, h, onMounted } from "vue";
import {
	NButton,
	NDataTable,
	NDivider,
	NEmpty,
	NTag,
	useDialog,
	useMessage,
	type DataTableColumns,
} from "naive-ui";
import type { SessionDTO } from "../../bindings/goxuetimer/models";
import { useRecords } from "../composables/useRecords";
import { formatClockTime, formatDurationCN, formatTodayLabel } from "../composables/format";
import { useTimer } from "../composables/useTimer";

const { sessions, summary, loading, error, refresh, remove } = useRecords();
// 取一下单例里的时钟，日期标签才会跟着跨天更新。
const { now } = useTimer();
const dialog = useDialog();
const message = useMessage();

const totalText = computed(() => formatDurationCN(summary.value?.totalSec ?? 0));
const countText = computed(() => `${summary.value?.count ?? 0} 条`);
const todayText = computed(() => formatTodayLabel(now.value));

const columns: DataTableColumns<SessionDTO> = [
	{
		key: "id",
		title: "#",
		width: 44,
		render: (row) => h("span", { class: "cell-muted" }, String(row.id)),
	},
	{
		key: "startedAt",
		title: "开始",
		width: 78,
		render: (row) =>
			h("span", { class: "cell-time", title: row.startedAt }, formatClockTime(row.startedAt)),
	},
	{
		key: "endedAt",
		title: "结束",
		width: 78,
		render: (row) =>
			h("span", { class: "cell-time", title: row.endedAt }, formatClockTime(row.endedAt)),
	},
	{
		key: "durationSec",
		title: "时长",
		width: 92,
		render: (row) => h("span", { class: "cell-duration" }, formatDurationCN(row.durationSec)),
	},
	{
		key: "source",
		title: "来源",
		width: 72,
		render: (row) =>
			h(
				NTag,
				{
					size: "small",
					type: row.source === "recovered" ? "warning" : "default",
					bordered: false,
					round: true,
				},
				{ default: () => (row.source === "recovered" ? "恢复" : "计时") },
			),
	},
	{
		key: "actions",
		title: "操作",
		width: 60,
		render: (row) =>
			h(
				NButton,
				{ size: "tiny", quaternary: true, type: "error", onClick: () => confirmRemove(row) },
				{ default: () => "删除" },
			),
	},
];

function confirmRemove(row: SessionDTO): void {
	dialog.warning({
		title: "确认删除",
		content: `删除 ${formatClockTime(row.startedAt)} 的这条记录（${formatDurationCN(row.durationSec)}）？删除后无法恢复。`,
		positiveText: "删除",
		negativeText: "取消",
		onPositiveClick: () => {
			void (async () => {
				const ok = await remove(row.id);
				if (ok) message.success("已删除");
				else message.error(error.value || "删除失败");
			})();
		},
	});
}

function rowKey(row: SessionDTO): number {
	return row.id;
}

onMounted(() => {
	// 窗口打开前数据可能已经变了，进来先拉一次。
	void refresh();
});
</script>

<template>
	<section class="records-view">
		<header class="records-header">
			<div class="records-title">
				<h2>今日记录</h2>
				<span class="records-date">{{ todayText }}</span>
			</div>
			<div class="records-totals">
				<span class="total-value">{{ totalText }}</span>
				<span class="total-count">{{ countText }}</span>
			</div>
		</header>

		<NDivider class="records-divider" />

		<p v-if="error" class="error-text">{{ error }}</p>

		<NDataTable v-if="sessions.length > 0" class="records-table" :columns="columns" :data="sessions"
			:loading="loading" :bordered="false" :single-line="false" size="small" :row-key="rowKey" :scroll-x="420" />

		<div v-else class="records-empty">
			<NEmpty description="今天还没有记录">
				<template #extra>
					<span class="empty-hint">回到「计时」开始一次学习吧</span>
				</template>
			</NEmpty>
		</div>
	</section>
</template>

<style scoped>
.records-view {
	display: flex;
	flex-direction: column;
	min-height: 0;
}

.records-header {
	display: flex;
	align-items: flex-end;
	justify-content: space-between;
	gap: 12px;
}

.records-title h2 {
	margin: 0;
	font-size: 16px;
	font-weight: 600;
}

.records-date {
	font-size: 12px;
	color: var(--gx-text-muted);
}

.records-totals {
	display: flex;
	flex-direction: column;
	align-items: flex-end;
}

.total-value {
	font-size: 20px;
	font-weight: 650;
	font-variant-numeric: tabular-nums;
}

.total-count {
	font-size: 12px;
	color: var(--gx-text-muted);
}

.records-divider {
	margin: 10px 0 12px;
}

.records-table {
	width: 100%;
}

.records-empty {
	padding: 32px 0;
}

.empty-hint {
	font-size: 12px;
	color: var(--gx-text-muted);
}

.error-text {
	margin: 0 0 10px;
	font-size: 13px;
	color: var(--gx-danger, #d03050);
}

:deep(.cell-muted) {
	color: var(--gx-text-muted);
	font-size: 12px;
}

:deep(.cell-time),
:deep(.cell-duration) {
	font-variant-numeric: tabular-nums;
	white-space: nowrap;
}
</style>
