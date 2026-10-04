<script setup lang="ts">
/** 一格里的小统计块：标签 + 大数字 + 可选注释。 */
withDefaults(
	defineProps<{
		label: string;
		value: string;
		hint?: string;
		/** 用主色渐变强调数字，用于"今日累计"这类主指标。 */
		accent?: boolean;
	}>(),
	{ hint: "", accent: false },
);
</script>

<template>
	<div class="tile">
		<span class="tile-label">{{ label }}</span>
		<span class="tile-value gx-tabular" :class="{ 'is-accent': accent }">{{ value }}</span>
		<span v-if="hint" class="tile-hint gx-faint gx-truncate">{{ hint }}</span>
	</div>
</template>

<style scoped>
.tile {
	display: flex;
	flex-direction: column;
	gap: 2px;
	min-width: 0;
}

.tile-label {
	font-size: 12px;
	color: var(--gx-text-2);
}

.tile-value {
	font-size: 20px;
	font-weight: 650;
	line-height: 1.25;
	color: var(--gx-text);
}

.tile-value.is-accent {
	background: var(--gx-accent-grad);
	-webkit-background-clip: text;
	background-clip: text;
	color: transparent;
}

.tile-hint {
	font-size: 11px;
}
</style>
