<script setup lang="ts">
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
		<rect x="6" y="5" width="4" height="14" rx="1" />
		<rect x="14" y="5" width="4" height="14" rx="1" />
	</svg>
</template>
