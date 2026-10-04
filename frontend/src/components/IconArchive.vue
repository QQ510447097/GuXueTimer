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
		stroke-width="1.8"
		stroke-linecap="round"
		stroke-linejoin="round"
		aria-hidden="true"
		focusable="false"
	>
		<rect x="3.5" y="4.5" width="17" height="4" rx="1.6" />
		<path d="M5.3 8.5v9.2a2 2 0 0 0 2 2h9.4a2 2 0 0 0 2-2V8.5" />
		<path d="M10 12.5h4" />
	</svg>
</template>
