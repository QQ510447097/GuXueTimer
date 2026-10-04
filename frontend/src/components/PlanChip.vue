<script setup lang="ts">
/**
 * 计划色块。
 *
 * 两种形态共用一个组件，是为了让"颜色"只有一处渲染逻辑：
 *   - variant="chip"：带色点的胶囊，用于筛选与标签
 *   - variant="swatch"：圆形色板，用于新建 / 编辑计划时选色
 *
 * 根节点必须**唯一**（这里是那个 button）：多根节点时 Vue 无法自动继承
 * 父组件传下来的 @click，会变成一句运行时警告且点击完全没反应。
 */
withDefaults(
	defineProps<{
		label?: string;
		color?: string;
		selected?: boolean;
		variant?: "chip" | "swatch";
	}>(),
	{ label: "", color: "var(--gx-text-3)", selected: false, variant: "chip" },
);
</script>

<template>
	<button
		type="button"
		:class="
			variant === 'swatch'
				? ['swatch', 'gx-lift', { 'is-selected': selected }]
				: ['chip', 'gx-ghost', 'gx-pill', { 'is-active': selected }]
		"
		:style="variant === 'swatch' ? { background: color } : undefined"
		:aria-pressed="selected"
		:aria-label="variant === 'swatch' ? label || '选择配色' : undefined"
	>
		<span v-if="variant === 'swatch'" class="swatch-core" aria-hidden="true"></span>
		<template v-else>
			<span class="gx-dot" :style="{ background: color, color: color }" aria-hidden="true"></span>
			<span class="chip-label gx-truncate">{{ label }}</span>
		</template>
	</button>
</template>

<style scoped>
.swatch {
	position: relative;
	width: 26px;
	height: 26px;
	padding: 0;
	border: none;
	border-radius: 50%;
	cursor: pointer;
	box-shadow: 0 2px 8px -2px rgba(15, 30, 56, 0.35);
}

/* 选中环用 outline：它不吃盒模型，不会把色板撑大导致整排重排 */
.swatch.is-selected {
	outline: 2px solid var(--gx-accent);
	outline-offset: 2px;
}

.swatch-core {
	position: absolute;
	inset: 0;
	border-radius: 50%;
	box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.35);
}

.chip {
	display: inline-flex;
	align-items: center;
	gap: 6px;
	max-width: 100%;
	padding: 4px 10px 4px 8px;
	font-size: 12.5px;
	line-height: 1.5;
	cursor: pointer;
}

.chip-label {
	min-width: 0;
}
</style>
