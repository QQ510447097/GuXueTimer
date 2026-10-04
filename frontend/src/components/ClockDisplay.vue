<script setup lang="ts">
/**
 * 大号等宽时钟。
 *
 * 数字宽度靠 font-variant-numeric: tabular-nums 固定，而不是给每个数字做等宽槽：
 * 秒数每 250ms 变一次，任何一点宽度差都会变成整块时钟的左右抖动。
 *
 * 光晕用 --gx-clock-glow（一个 box-shadow 值）画在文字背后的伪元素上：
 * 它是 box-shadow 而不是 text-shadow，直接加到文字上只会画出一个方块阴影。
 */
defineProps<{
	time: string;
	state: "idle" | "running" | "paused";
	caption: string;
}>();
</script>

<template>
	<div class="clock" :data-state="state">
		<div class="clock-stage">
			<span class="clock-glow" aria-hidden="true"></span>
			<span class="clock-time gx-mono gx-tabular">{{ time }}</span>
		</div>
		<div class="clock-caption gx-muted">{{ caption }}</div>
	</div>
</template>

<style scoped>
.clock {
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: 6px;
	width: 100%;
	min-width: 0;
}

.clock-stage {
	position: relative;
	display: flex;
	align-items: center;
	justify-content: center;
	max-width: 100%;
	padding: 4px 8px;
}

.clock-glow {
	position: absolute;
	inset: 16% 3%;
	border-radius: var(--gx-r-pill);
	box-shadow: var(--gx-clock-glow);
	opacity: 0.35;
	transition: opacity var(--gx-dur) var(--gx-ease);
}

.clock-time {
	position: relative;
	font-family: var(--gx-mono);
	font-size: clamp(42px, 12vw, 60px);
	font-weight: 600;
	line-height: 1.1;
	letter-spacing: 0.02em;
	color: var(--gx-clock);
	white-space: nowrap;
}

.clock-caption {
	font-size: 12px;
}

.clock[data-state="running"] .clock-glow {
	opacity: 0.62;
	animation: clock-breathe 3.4s var(--gx-ease) infinite;
}

.clock[data-state="paused"] .clock-glow {
	opacity: 0.5;
}

.clock[data-state="paused"] .clock-time {
	opacity: 0.82;
}

@keyframes clock-breathe {
	0%,
	100% {
		opacity: 0.42;
	}
	50% {
		opacity: 0.72;
	}
}
</style>
