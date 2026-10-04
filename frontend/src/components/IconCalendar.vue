<script setup lang="ts">
/**
 * 内联 SVG：不引图标库。
 *
 * 只暴露 size，颜色继承 currentColor，class 由 Vue 自动落到根 svg 上——
 * 导航栏就是靠外部 class 控制尺寸的。
 */
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
		<rect x="3.5" y="5" width="17" height="15.5" rx="3" />
		<path d="M3.5 10h17" />
		<path d="M8 3.5V6.5M16 3.5V6.5" />
		<path d="M7.5 13.5h2M11 13.5h2M14.5 13.5h2M7.5 17h2M11 17h2" />
	</svg>
</template>
