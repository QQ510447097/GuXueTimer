<script setup lang="ts">
import type { Component } from "vue";

/**
 * 左侧导航栏。
 *
 * data-view / data-active 不是装饰：tools/ui_probe.mjs 靠它们点视图并读出当前是哪一屏，
 * 截图脚本因此不需要猜测 DOM 结构。改这里时要一并保留。
 */
export interface NavItem {
	key: string;
	label: string;
	icon: Component;
}

defineProps<{
	items: NavItem[];
	active: string;
}>();

const emit = defineEmits<{
	(e: "select", key: string): void;
}>();
</script>

<template>
	<aside class="rail gx-glass">
		<div class="brand">
			<div class="brand-mark">雪</div>
			<span class="brand-name">孤雪</span>
		</div>

		<nav class="nav">
			<button v-for="item in items" :key="item.key" type="button" class="nav-item gx-lift"
				:class="{ 'is-active': active === item.key }" :data-view="item.key"
				:data-active="active === item.key ? 'true' : 'false'" :title="item.label"
				@click="emit('select', item.key)">
				<component :is="item.icon" class="nav-icon" />
				<span class="nav-label">{{ item.label }}</span>
			</button>
		</nav>
	</aside>
</template>

<style scoped>
.rail {
	/* 76px：够放下一个图标加两个字的标签，又不至于把内容区挤到没法放月历 */
	flex: 0 0 76px;
	display: flex;
	flex-direction: column;
	gap: 14px;
	padding: 16px 8px 14px;
	border-radius: 0 var(--gx-r-lg) var(--gx-r-lg) 0;
	border-left: none;
}

.brand {
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: 5px;
}

.brand-mark {
	display: flex;
	align-items: center;
	justify-content: center;
	width: 34px;
	height: 34px;
	border-radius: 12px;
	background: var(--gx-accent-grad);
	color: #fff;
	font-size: 17px;
	font-weight: 700;
	box-shadow: 0 6px 16px -6px rgba(59, 130, 246, 0.55);
}

.brand-name {
	font-size: 11px;
	color: var(--gx-text-2);
	letter-spacing: 0.06em;
}

.nav {
	display: flex;
	flex-direction: column;
	gap: 6px;
	flex: 1 1 auto;
}

.nav-item {
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: 4px;
	padding: 10px 0;
	border: none;
	background: transparent;
	color: var(--gx-text-2);
	font-size: 11px;
	border-radius: var(--gx-r-md);
	cursor: pointer;
}

.nav-item:hover {
	background: var(--gx-glass-2-bg);
	color: var(--gx-text);
}

.nav-item.is-active {
	background: var(--gx-accent-soft);
	color: var(--gx-accent);
	font-weight: 600;
	box-shadow: inset 0 0 0 1px var(--gx-accent-line);
}

.nav-icon {
	width: 19px;
	height: 19px;
}

.nav-label {
	line-height: 1;
	letter-spacing: 0.04em;
}
</style>
