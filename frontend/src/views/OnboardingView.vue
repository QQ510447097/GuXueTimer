<script setup lang="ts">
import { useMessage } from "naive-ui";
import type { PlanDTO } from "../../bindings/goxuetimer/models";
import GlassPanel from "../components/GlassPanel.vue";
import PlanForm from "../components/PlanForm.vue";
import { usePlans } from "../composables/usePlans";

/**
 * 引导页：一个计划都没有时，整个外壳换成这一屏。
 *
 * 表单本身是 PlanForm（计时页的选择器里也用它），这一屏只负责"外壳"：
 * 品牌、说明，以及创建完之后刷新计划列表。
 *
 * 刻意不发任何"创建完成"事件：外壳看的是 usePlans 的 activePlans，
 * 刷新之后它自己就会切到计时页。多一条事件就多一处可能不一致的状态。
 *
 * 它是外壳的兄弟节点（不在 .content 里），所以要自己撑满窗口并居中。
 */
const { refresh } = usePlans();
const message = useMessage();

async function onCreated(plan: PlanDTO): Promise<void> {
	// 第一个计划后端会自动设为当前计划，这里只要让外壳看到列表变了。
	await refresh();
	message.success(`已创建「${plan.name}」`);
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

			<PlanForm submit-label="创建并开始" autofocus @created="onCreated" />
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
</style>
