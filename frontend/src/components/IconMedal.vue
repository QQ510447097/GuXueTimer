<script setup lang="ts">
/**
 * 成就图标。
 *
 * 用奖章而不是星星或奖杯：奖章是"累计出来的一枚"，与"坚持了多少"的语义最贴。
 * 内圈那个符号随解锁状态换（未解锁是一个点），颜色由 tier 决定。
 */
withDefaults(
	defineProps<{
		size?: number;
		/** 是否已解锁：未解锁时内圈只留一个点，颜色也更淡。 */
		unlocked?: boolean;
	}>(),
	{ size: 24, unlocked: true },
);
</script>

<template>
	<svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor"
		stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
		<circle cx="12" cy="14" r="6.2" />
		<path d="M8.4 8.6L6.6 2.8h10.8l-1.8 5.8" />
		<path v-if="unlocked" d="M9.6 14l1.7 1.7 3.2-3.4" />
		<circle v-else cx="12" cy="14" r="1.2" fill="currentColor" stroke="none" />
	</svg>
</template>
