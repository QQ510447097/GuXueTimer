<script setup lang="ts">
/**
 * 大号等宽时钟。
 *
 * 用 CSS 固定宽度的数字槽而不是靠字体本身的等宽：字重变化或字体回落时
 * 数字宽度不一致会让整块时钟左右抖动。
 */
defineProps<{
	time: string;
	state: "idle" | "running" | "paused";
	caption: string;
}>();
</script>

<template>
	<div class="clock" :data-state="state">
		<div class="clock-time">{{ time }}</div>
		<div class="clock-caption">{{ caption }}</div>
	</div>
</template>

<style scoped>
.clock {
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: 4px;
	width: 100%;
}

.clock-time {
	display: flex;
	justify-content: center;
	max-width: 100%;
	padding: 0 4px;
	font-family: "Cascadia Mono", Consolas, "Courier New", ui-monospace, monospace;
	font-size: clamp(40px, 12vw, 60px);
	font-weight: 600;
	line-height: 1.12;
	letter-spacing: 0.01em;
	white-space: nowrap;
	/* 等宽数字：不同数字占用同一宽度 */
	font-variant-numeric: tabular-nums;
	font-feature-settings: "tnum" 1;
	transition: color 0.2s ease;
}

.clock[data-state="running"] .clock-time {
	color: var(--gx-clock-running);
}

.clock[data-state="paused"] .clock-time {
	color: var(--gx-clock-paused);
}

.clock[data-state="idle"] .clock-time {
	color: var(--gx-clock-idle);
}

.clock-caption {
	font-size: 12px;
	color: var(--gx-text-muted);
}
</style>
