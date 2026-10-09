<script setup lang="ts">
import type { Component } from "vue";
import brandMark from "../assets/brand-mark.png";

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
	/** 品牌图标的悬浮说明。 */
	brandTitle?: string;
}>();

const emit = defineEmits<{
	(e: "select", key: string): void;
}>();

/*
 * 品牌图就是应用图标本身（frontend/src/assets/brand-mark.png），
 * 从 assets/ 里那张 1024 原图裁掉透明边、缩成 128px 得来的。
 *
 * 这里以前是白字「雪」+ CSS 渐变方块。换成真图标是为了"一个应用一个标识"：
 * 任务栏、标题栏、托盘、界面左上角应该是同一张脸。裁边是必须的——原图四周有
 * 约 14% 的透明留白，直接用的话可见主体只有 32px 上下，比它右边那排导航图标
 * 大不了一圈，就失去了"这里唯一一个标识物"的读法。
 */
</script>

<template>
	<aside class="rail gx-glass">
		<div class="brand" :title="brandTitle">
			<img class="brand-mark" :src="brandMark" alt="" />
		</div>

		<!-- 品牌区与导航之间加一条分割线：图标放大之后两者的视觉重量接近，
		     没有线会读成"图标也是第一个导航项"。 -->
		<div class="divider" aria-hidden="true"></div>

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
	/* 76px：够放下图标 + 两个字的标签，又不至于把内容区挤到没法放月历 */
	flex: 0 0 76px;
	display: flex;
	flex-direction: column;
	gap: 12px;
	padding: 16px 8px 14px;
	border-radius: 0 var(--gx-r-lg) var(--gx-r-lg) 0;
	border-left: none;
}

.brand {
	display: flex;
	justify-content: center;
}

/*
 * 品牌图标刻意比导航图标大一圈（44 vs 19）：它是这一栏里唯一的"标识"，
 * 和导航项等大时读起来就像"又一个按钮"。底下也不再挂文字——窗口标题栏
 * 已经写着应用名，竖排的两个字只会让这一栏显得挤。
 *
 * 图标自带圆角方块与投影（就是应用图标本身），所以这里不再画背景和 box-shadow：
 * 再叠一层方块会在图标透明的四角露出底色，读起来像套了两层框。
 */
.brand-mark {
	display: block;
	width: 44px;
	height: 44px;
	/* 拖拽/选中时不要出现蓝色高亮与图片拖影：它是标识，不是可交互内容 */
	user-select: none;
	-webkit-user-drag: none;
}

.divider {
	height: 1px;
	margin: 0 4px;
	background: var(--gx-line);
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
