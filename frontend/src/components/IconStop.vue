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
		<rect x="5" y="5" width="14" height="14" rx="2" />
	</svg>
</template>
