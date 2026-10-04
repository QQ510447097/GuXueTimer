<script setup lang="ts">
/**
 * 玻璃面板。
 *
 * 单独抽一个组件而不是到处写 class：全应用的面板必须是同一套底、同一套圆角与阴影，
 * 散落在各处的 .gx-glass 迟早会漂移出六七种灰白。ui_probe 也靠这个类统计面板数量。
 */
withDefaults(
	defineProps<{
		padding?: string;
		radius?: string;
		/** 悬停时轻抬，用在可点击的卡片上。 */
		interactive?: boolean;
	}>(),
	{
		padding: "16px",
		radius: "var(--gx-r-lg)",
		interactive: false,
	},
);
</script>

<template>
	<section class="gx-glass panel" :class="{ 'gx-lift': interactive }"
		:style="{ padding, borderRadius: radius }">
		<slot />
	</section>
</template>

<style scoped>
.panel {
	min-width: 0;
}
</style>
