<script setup lang="ts">
/** 设置：齿轮。外圈用八段短齿，比复杂路径在小尺寸下更清晰。 */
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
		<circle cx="12" cy="12" r="3.2" />
		<path d="M12 2.8v2.6M12 18.6v2.6M4.5 12H2M22 12h-2.5" />
		<path d="M6.7 6.7L4.9 4.9M19.1 19.1l-1.8-1.8M17.3 6.7l1.8-1.8M4.9 19.1l1.8-1.8" />
	</svg>
</template>
