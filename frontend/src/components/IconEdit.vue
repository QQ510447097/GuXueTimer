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
		<path d="M14.5 4.9l4.6 4.6" />
		<path d="M16.2 3.2a1.9 1.9 0 0 1 2.7 0l1.9 1.9a1.9 1.9 0 0 1 0 2.7L8.6 19.9 4 21l1.1-4.6z" />
	</svg>
</template>
