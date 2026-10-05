<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { NInput, useDialog, useMessage } from "naive-ui";
import * as PlansService from "../../bindings/goxuetimer/plansservice";
import type { PlanDTO } from "../../bindings/goxuetimer/models";
import GlassPanel from "../components/GlassPanel.vue";
import IconArchive from "../components/IconArchive.vue";
import IconBack from "../components/IconBack.vue";
import IconDown from "../components/IconDown.vue";
import IconEdit from "../components/IconEdit.vue";
import IconPlus from "../components/IconPlus.vue";
import IconTrash from "../components/IconTrash.vue";
import IconUp from "../components/IconUp.vue";
import PlanChip from "../components/PlanChip.vue";
import { errText, formatDurationCN } from "../composables/format";
import { usePlans } from "../composables/usePlans";
import { pullState, useTimer } from "../composables/useTimer";

/**
 * 计划管理页。
 *
 * 每次改动都重新拉一次 usePlans().refresh()：后端的返回只带被改的那一条，
 * 而排序、当前计划标记、统计都会连带变化，局部更新反而更容易出错。
 */
const { activePlans, archivedPlans, palette, refresh } = usePlans();
const { state } = useTimer();
const dialog = useDialog();
const message = useMessage();

const busy = ref(false);
const showCreate = ref(false);
const newName = ref("");
const newColor = ref("");
const editingId = ref(0);
const editName = ref("");
const editColor = ref("");

/** 当前计划以后端状态为准：它同时被计时页的选择器和托盘菜单改。 */
const currentPlanId = computed(() => state.value?.planId ?? 0);

/** 在用的计划 + 已归档的计划排成一列，已归档的那一段前面插一条分隔标题。 */
const list = computed(() => [...activePlans.value, ...archivedPlans.value]);
const archivedStart = computed(() => activePlans.value.length);

// 配色白名单来自后端，可能比首个计划晚到；到了就默认选中第一个。
watch(
	palette,
	(colors) => {
		if (!newColor.value && colors.length > 0) newColor.value = colors[0];
	},
	{ immediate: true },
);

onMounted(() => {
	void pullState();
});

async function run(action: () => Promise<unknown>, successText = ""): Promise<boolean> {
	busy.value = true;
	try {
		await action();
		await refresh();
		if (successText) message.success(successText);
		return true;
	} catch (err) {
		// 后端的报错就是写给用户看的（"这个计划下有 3 条学习记录，不能删除；请改成「归档」"），原样展示。
		message.error(errText(err));
		return false;
	} finally {
		busy.value = false;
	}
}

async function createPlan(): Promise<void> {
	const name = newName.value.trim();
	if (!name) {
		message.warning("先给计划起个名字");
		return;
	}
	if (!newColor.value) {
		message.warning("还没有拿到可用配色，请稍后再试");
		return;
	}
	if (await run(() => PlansService.Create(name, newColor.value), `已创建「${name}」`)) {
		newName.value = "";
		showCreate.value = false;
	}
}

function startEdit(plan: PlanDTO): void {
	editingId.value = plan.id;
	editName.value = plan.name;
	editColor.value = plan.color;
}

async function saveEdit(plan: PlanDTO): Promise<void> {
	const name = editName.value.trim();
	if (!name) {
		message.warning("计划名不能为空");
		return;
	}
	const color = editColor.value || plan.color;
	if (await run(() => PlansService.Update(plan.id, name, color), "已保存")) {
		editingId.value = 0;
	}
}

async function move(index: number, delta: number): Promise<void> {
	const ordered = activePlans.value.slice();
	const target = index + delta;
	if (target < 0 || target >= ordered.length) return;
	const [moved] = ordered.splice(index, 1);
	ordered.splice(target, 0, moved);
	// 只传在用的计划：后端对没提到的计划保持原有排序，归档计划不必参与。
	await run(() => PlansService.Reorder(ordered.map((p) => p.id)));
}

async function setCurrent(plan: PlanDTO): Promise<void> {
	if (await run(() => PlansService.SetCurrent(plan.id), `已切换到「${plan.name}」`)) {
		await pullState();
	}
}

function archive(plan: PlanDTO): void {
	dialog.warning({
		title: "归档计划",
		content: `「${plan.name}」会从计划选择器里隐藏，历史记录与统计都保留。`,
		positiveText: "归档",
		negativeText: "取消",
		onPositiveClick: () => {
			void run(() => PlansService.SetArchived(plan.id, true), "已归档");
		},
	});
}

function restore(plan: PlanDTO): void {
	void run(() => PlansService.SetArchived(plan.id, false), `已恢复「${plan.name}」`);
}

function remove(plan: PlanDTO): void {
	dialog.error({
		title: "删除计划",
		content: `「${plan.name}」将被永久删除，此操作无法撤销。`,
		positiveText: "删除",
		negativeText: "取消",
		onPositiveClick: () => {
			void run(() => PlansService.Delete(plan.id), "已删除");
		},
	});
}
</script>

<template>
	<section class="plans-view gx-col">
		<GlassPanel padding="14px 16px">
			<div class="create-head">
				<span class="gx-section-title">计划</span>
				<span class="gx-spacer"></span>
				<button type="button" class="act gx-ghost gx-pill" @click="showCreate = !showCreate">
					<IconPlus :size="13" />
					<span>新建计划</span>
				</button>
			</div>

			<div v-if="showCreate" class="create-form">
				<!-- 回车提交挂在外层 div 上：NInput 会把未知监听器透传到自己的包裹元素，
				     而原生 div 上的监听一定能收到内部 input 冒泡上来的 keyup -->
				<div class="field" @keyup.enter="createPlan">
					<NInput v-model:value="newName" placeholder="计划名称，例如：考研数学" :maxlength="24"
						autofocus />
				</div>
				<div class="swatches">
					<PlanChip v-for="color in palette" :key="color" variant="swatch" :color="color"
						:selected="newColor === color" @click="newColor = color" />
					<span v-if="palette.length === 0" class="gx-faint">正在加载配色…</span>
				</div>
				<div class="create-actions">
					<button type="button" class="act solid gx-accent-btn gx-pill" :disabled="busy"
						@click="createPlan">
						创建
					</button>
					<button type="button" class="act gx-ghost gx-pill" @click="showCreate = false">取消</button>
				</div>
			</div>

			<p v-if="list.length === 0" class="gx-faint empty-hint">还没有计划，先新建一个吧。</p>
		</GlassPanel>

		<div class="list">
			<template v-for="(plan, index) in list" :key="plan.id">
				<div v-if="plan.archived && index === archivedStart" class="divider-head">
					<span class="gx-section-title">已归档</span>
					<span class="gx-faint">历史记录仍然计入统计</span>
				</div>

				<!-- 行用次级玻璃而不是 GlassPanel：这是"一行"，不是一张卡片。
				     面板数也要克制，整页只有顶部那一个玻璃面板。 -->
				<div class="plan-row gx-glass-2 gx-lift">
					<div v-if="editingId === plan.id" class="edit-form">
						<div class="field">
							<NInput v-model:value="editName" size="small" :maxlength="24" />
						</div>
						<div class="swatches">
							<PlanChip v-for="color in palette" :key="color" variant="swatch" :color="color"
								:selected="editColor === color" @click="editColor = color" />
						</div>
						<div class="row-actions">
							<button type="button" class="act solid gx-accent-btn gx-pill"
								:disabled="busy" @click="saveEdit(plan)">
								保存
							</button>
							<button type="button" class="act gx-ghost gx-pill"
								@click="editingId = 0">
								取消
							</button>
						</div>
					</div>

					<template v-else>
						<div class="row-head">
							<span class="gx-dot"
								:style="{ background: plan.color, color: plan.color }"></span>
							<span class="name gx-truncate">{{ plan.name }}</span>
							<span v-if="!plan.archived && plan.id === currentPlanId" class="badge">当前</span>
							<span v-if="plan.archived" class="badge is-muted">已归档</span>
							<span class="gx-spacer"></span>
							<template v-if="!plan.archived">
								<button type="button" class="icon-btn" title="上移"
									:disabled="index === 0 || busy" @click="move(index, -1)">
									<IconUp :size="13" />
								</button>
								<button type="button" class="icon-btn" title="下移"
									:disabled="index === archivedStart - 1 || busy"
									@click="move(index, 1)">
									<IconDown :size="13" />
								</button>
							</template>
						</div>

						<p class="stats gx-faint gx-tabular">
							今日 {{ formatDurationCN(plan.todaySec) }} · 累计
							{{ formatDurationCN(plan.totalSec) }} · {{ plan.sessions }} 次
						</p>

						<div class="row-actions">
							<button v-if="!plan.archived && plan.id !== currentPlanId" type="button"
								class="act gx-ghost gx-pill" :disabled="busy"
								@click="setCurrent(plan)">
								设为当前
							</button>
							<button v-if="!plan.archived" type="button" class="act gx-ghost gx-pill"
								:disabled="busy" @click="startEdit(plan)">
								<IconEdit :size="13" />
								<span>编辑</span>
							</button>
							<button v-if="!plan.archived" type="button" class="act gx-ghost gx-pill"
								:disabled="busy" @click="archive(plan)">
								<IconArchive :size="13" />
								<span>归档</span>
							</button>
							<button v-if="plan.archived" type="button" class="act gx-ghost gx-pill"
								:disabled="busy" @click="restore(plan)">
								<IconBack :size="13" />
								<span>恢复</span>
							</button>
							<!-- 有记录的计划后端会拒绝删除，这里干脆不给入口；真被拒了也有后端文案 -->
							<button v-if="plan.sessions === 0" type="button"
								class="act gx-ghost gx-pill is-danger" :disabled="busy"
								@click="remove(plan)">
								<IconTrash :size="13" />
								<span>删除</span>
							</button>
						</div>
					</template>
				</div>
			</template>
		</div>
	</section>
</template>

<style scoped>
.plans-view {
	gap: var(--gx-gap);
}

.create-head {
	display: flex;
	align-items: center;
	gap: 8px;
	min-width: 0;
}

.create-form,
.edit-form {
	display: flex;
	flex-direction: column;
	gap: 10px;
	margin-top: 12px;
}

.field {
	min-width: 0;
}

.swatches {
	display: flex;
	flex-wrap: wrap;
	gap: 9px;
	/* 选中环用 outline + offset，靠 padding 把首尾的环留在容器内 */
	padding: 3px 4px;
}

.create-actions,
.row-actions {
	display: flex;
	flex-wrap: wrap;
	gap: 6px;
}

.list {
	display: flex;
	flex-direction: column;
	gap: 9px;
}

.divider-head {
	display: flex;
	align-items: baseline;
	gap: 8px;
	margin: 6px 2px 0;
	font-size: 11.5px;
}

.plan-row {
	padding: 12px 14px;
	min-width: 0;
}

.row-head {
	display: flex;
	align-items: center;
	gap: 8px;
	min-width: 0;
}

.name {
	flex: 0 1 auto;
	min-width: 0;
	font-size: 14px;
	font-weight: 650;
}

.badge {
	flex: 0 0 auto;
	padding: 1px 8px;
	border-radius: var(--gx-r-pill);
	background: var(--gx-accent-soft);
	color: var(--gx-accent);
	font-size: 11px;
}

.badge.is-muted {
	background: var(--gx-glass-2-bg);
	color: var(--gx-text-3);
}

.stats {
	margin: 6px 0 9px;
	font-size: 12px;
}

.act {
	display: inline-flex;
	align-items: center;
	gap: 5px;
	padding: 5px 11px;
	font-size: 12.5px;
	font-family: inherit;
	cursor: pointer;
}

.act:disabled {
	opacity: 0.45;
	cursor: not-allowed;
}

.act.is-danger:hover:not(:disabled) {
	color: var(--gx-danger);
	border-color: var(--gx-danger);
}

.act.solid {
	color: #fff;
	font-weight: 600;
}

.icon-btn {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	width: 24px;
	height: 24px;
	padding: 0;
	flex: 0 0 auto;
	border: 1px solid var(--gx-glass-2-border);
	border-radius: 50%;
	background: var(--gx-glass-2-bg);
	color: var(--gx-text-2);
	cursor: pointer;
	transition: color var(--gx-dur) var(--gx-ease), border-color var(--gx-dur) var(--gx-ease);
}

.icon-btn:hover:not(:disabled) {
	color: var(--gx-accent);
	border-color: var(--gx-accent-line);
}

.icon-btn:disabled {
	opacity: 0.35;
	cursor: not-allowed;
}

.empty-hint {
	margin-top: 10px;
	font-size: 12.5px;
}
</style>
