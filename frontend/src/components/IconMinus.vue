<script setup lang="ts">
/** 减号。与 IconPlus 同一套线宽与端点，两个按钮并排时看着才是一对。 */
withDefaults(defineProps<{ size?: number }>(), { size: 16 });
</script>

<template>
	<svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor"
		stroke-width="2" stroke-linecap="round" aria-hidden="true">
		<path d="M5 12h14" />
	</svg>
</template>
