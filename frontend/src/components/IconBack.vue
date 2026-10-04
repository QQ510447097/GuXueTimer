<script setup lang="ts">
/** 恢复 / 返回：箭头向左。 */
withDefaults(defineProps<{ size?: number }>(), { size: 16 });
</script>

<template>
	<svg
		:width="size"
		:height="size"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="1.9"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
		focusable="false"
	>
		<path d="M19 12H5" />
		<path d="M11 6l-6 6 6 6" />
	</svg>
</template>
