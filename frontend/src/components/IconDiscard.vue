<script setup lang="ts">
withDefaults(defineProps<{ size?: number }>(), { size: 16 });
</script>

<template>
	<svg
		:width="size"
		:height="size"
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		aria-hidden="true"
		focusable="false"
	>
		<path d="M6 6l12 12" />
		<path d="M18 6L6 18" />
	</svg>
</template>
