<script setup lang="ts">
/** 计划：清单 + 勾选，比靶心更贴近"学习计划"的语义。 */
withDefaults(defineProps<{ size?: number }>(), { size: 16 });
</script>

<template>
	<svg
		:width="size"
		:height="size"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
		focusable="false"
	>
		<path d="M9.5 6.5h10M9.5 12h10M9.5 17.5h10" />
		<path d="M4 6.2l1.3 1.3L7.8 5" />
		<path d="M4 16.7l1.3 1.3L7.8 15.5" />
		<path d="M6.6 11.2h1.2" />
	</svg>
</template>
