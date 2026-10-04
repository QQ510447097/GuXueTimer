<script setup lang="ts">
/** 内联 SVG：不引图标库，避免多一个依赖。 */
withDefaults(defineProps<{ size?: number }>(), { size: 16 });
</script>

<template>
	<svg
		:width="size"
		:height="size"
		viewBox="0 0 24 24"
		fill="currentColor"
		aria-hidden="true"
		focusable="false"
	>
		<path d="M8 5.14v13.72a1 1 0 0 0 1.53.85l10.2-6.86a1 1 0 0 0 0-1.7L9.53 4.29A1 1 0 0 0 8 5.14z" />
	</svg>
</template>
