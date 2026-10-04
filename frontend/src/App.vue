<script setup lang="ts">
import { computed, defineComponent, markRaw, onBeforeUnmount, onMounted, ref, type Component } from "vue";
import { Events } from "@wailsio/runtime";
import {
	NConfigProvider,
	NDialogProvider,
	NGlobalStyle,
	NMessageProvider,
	NNotificationProvider,
	useMessage,
	useNotification,
} from "naive-ui";
import type { NoticeDTO, ReminderEventDTO } from "../bindings/goxuetimer/models";
import RecordsView from "./views/RecordsView.vue";
import SettingsView from "./views/SettingsView.vue";
import TimerView from "./views/TimerView.vue";
import { useSettings } from "./composables/useSettings";
import { useTimer } from "./composables/useTimer";

type NavKey = "timer" | "records" | "settings";

/**
 * 事件桥必须是 provider 的子组件：useMessage / useNotification 取的是祖先注入，
 * 在 App 自己的 setup 里调用会拿不到（provider 就在本组件的模板里）。
 */
const NoticeBridge = defineComponent({
	name: "NoticeBridge",
	setup() {
		const message = useMessage();
		const notification = useNotification();
		let offNotice: (() => void) | null = null;
		let offReminder: (() => void) | null = null;

		onMounted(() => {
			offNotice = Events.On("notice", (ev) => {
				const payload = ev.data as NoticeDTO;
				if (!payload || !payload.message) return;
				if (payload.level === "warning") message.warning(payload.message);
				else message.info(payload.message);
			});

			// 后端另外会发 Windows 通知；这里是应用内的兜底提醒。
			offReminder = Events.On("reminder:break", (ev) => {
				const payload = ev.data as ReminderEventDTO;
				const minutes = payload && payload.intervalMin > 0 ? payload.intervalMin : 45;
				notification.info({
					title: "该休息了",
					content: `已连续学习 ${minutes} 分钟，起来活动一下`,
					duration: 0,
					keepAliveOnHover: true,
					closable: true,
				});
			});
		});

		onBeforeUnmount(() => {
			offNotice?.();
			offReminder?.();
			offNotice = null;
			offReminder = null;
		});

		return () => null;
	},
});

const { naiveTheme, themeClass } = useSettings();
// 计时单例在这里初始化：事件订阅与本地时钟只做一次，切视图不会重复注册。
const { state, stateText } = useTimer();

const active = ref<NavKey>("timer");

// markRaw：组件对象不需要响应式代理，包一层只会白费开销。
const views: Record<NavKey, Component> = {
	timer: markRaw(TimerView),
	records: markRaw(RecordsView),
	settings: markRaw(SettingsView),
};

const navItems: { key: NavKey; label: string }[] = [
	{ key: "timer", label: "计时" },
	{ key: "records", label: "记录" },
	{ key: "settings", label: "设置" },
];

const currentView = computed(() => views[active.value]);

const stateDotClass = computed(() => {
	switch (state.value?.state) {
		case "running":
			return "dot is-running";
		case "paused":
			return "dot is-paused";
		default:
			return "dot is-idle";
	}
});
</script>

<template>
	<NConfigProvider :theme="naiveTheme" class="app-root" :class="themeClass">
		<NGlobalStyle />
		<NMessageProvider placement="top">
			<NDialogProvider>
				<NNotificationProvider placement="bottom-right" :max="3">
					<NoticeBridge />
					<div class="shell">
						<aside class="sidebar">
							<div class="brand">
								<span class="brand-mark">古</span>
								<span class="brand-name">古学计时</span>
							</div>
							<nav class="nav">
								<button v-for="item in navItems" :key="item.key" type="button" class="nav-item"
									:class="{ 'is-active': active === item.key }" @click="active = item.key">
									{{ item.label }}
								</button>
							</nav>
							<div class="sidebar-foot">
								<span :class="stateDotClass" aria-hidden="true"></span>
								<span class="foot-text">{{ stateText }}</span>
							</div>
						</aside>
						<main class="content">
							<component :is="currentView" />
						</main>
					</div>
				</NNotificationProvider>
			</NDialogProvider>
		</NMessageProvider>
	</NConfigProvider>
</template>

<style scoped>
.shell {
	display: flex;
	width: 100%;
	height: 100%;
	min-height: 0;
	background: var(--gx-bg);
}

.sidebar {
	flex: 0 0 88px;
	display: flex;
	flex-direction: column;
	justify-content: space-between;
	gap: 12px;
	padding: 14px 8px 12px;
	background: var(--gx-surface);
	border-right: 1px solid var(--gx-border);
}

.brand {
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: 4px;
}

.brand-mark {
	display: flex;
	align-items: center;
	justify-content: center;
	width: 30px;
	height: 30px;
	border-radius: 9px;
	background: var(--gx-accent-soft);
	color: var(--gx-accent);
	font-size: 16px;
	font-weight: 700;
}

.brand-name {
	font-size: 11px;
	color: var(--gx-text-muted);
	letter-spacing: 0.04em;
}

.nav {
	display: flex;
	flex-direction: column;
	gap: 6px;
	flex: 1 1 auto;
}

.nav-item {
	appearance: none;
	border: none;
	background: transparent;
	color: var(--gx-text-muted);
	font-size: 13px;
	font-family: inherit;
	padding: 9px 0;
	border-radius: 8px;
	cursor: pointer;
	transition: background-color 0.15s ease, color 0.15s ease;
}

.nav-item:hover {
	background: var(--gx-surface-hover);
	color: var(--gx-text);
}

.nav-item.is-active {
	background: var(--gx-accent-soft);
	color: var(--gx-accent);
	font-weight: 600;
}

.sidebar-foot {
	display: flex;
	flex-direction: column;
	align-items: center;
	gap: 4px;
}

.foot-text {
	font-size: 11px;
	color: var(--gx-text-muted);
}

.dot {
	width: 8px;
	height: 8px;
	border-radius: 50%;
	background: var(--gx-text-muted);
}

.dot.is-running {
	background: #18a058;
	box-shadow: 0 0 0 3px rgba(24, 160, 88, 0.18);
}

.dot.is-paused {
	background: #f0a020;
	box-shadow: 0 0 0 3px rgba(240, 160, 32, 0.18);
}

.dot.is-idle {
	background: var(--gx-text-muted);
	opacity: 0.5;
}

.content {
	flex: 1 1 auto;
	min-width: 0;
	min-height: 0;
	overflow-y: auto;
	overflow-x: hidden;
	padding: 16px 14px 18px;
}
</style>
