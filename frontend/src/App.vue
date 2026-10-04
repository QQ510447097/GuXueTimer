<script setup lang="ts">
import { computed, defineComponent, markRaw, onBeforeUnmount, onMounted, ref, watchEffect, type Component } from "vue";
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
import IconCalendar from "./components/IconCalendar.vue";
import IconPlan from "./components/IconPlan.vue";
import IconPlay from "./components/IconPlay.vue";
import IconSettings from "./components/IconSettings.vue";
import NavRail, { type NavItem } from "./components/NavRail.vue";
import { useHotkeys } from "./composables/useHotkeys";
import { usePlans } from "./composables/usePlans";
import { useSettings } from "./composables/useSettings";
import { pauseTimer, pullState, resumeTimer, startTimer, useTimer } from "./composables/useTimer";
import OnboardingView from "./views/OnboardingView.vue";
import PlansView from "./views/PlansView.vue";
import RecordsView from "./views/RecordsView.vue";
import SettingsView from "./views/SettingsView.vue";
import TimerView from "./views/TimerView.vue";

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

const { naiveTheme, themeOverrides, isDark, load: loadSettings } = useSettings();
const { activePlans, loaded: plansLoaded } = usePlans();
const { state } = useTimer();
void loadSettings();

// 深色切换挂到 <html> 上而不是组件根节点：CSS 变量定义在 :root，
// 只有挂在 html 上，body 底色和 naive-ui 的浮层才会一起跟着换。
watchEffect(() => {
	const root = document.documentElement;
	root.classList.toggle("theme-dark", isDark.value);
});

const navItems: NavItem[] = [
	{ key: "timer", label: "计时", icon: markRaw(IconPlay) },
	{ key: "records", label: "记录", icon: markRaw(IconCalendar) },
	{ key: "plans", label: "计划", icon: markRaw(IconPlan) },
	{ key: "settings", label: "设置", icon: markRaw(IconSettings) },
];

const views: Record<string, Component> = {
	timer: markRaw(TimerView),
	records: markRaw(RecordsView),
	plans: markRaw(PlansView),
	settings: markRaw(SettingsView),
};

const VIEW_STORAGE_KEY = "gx.activeView";

function restoreView(): string {
	const saved = localStorage.getItem(VIEW_STORAGE_KEY) ?? "";
	return saved in views ? saved : "timer";
}

const active = ref(restoreView());
const currentView = computed(() => views[active.value] ?? views.timer);

function selectView(key: string): void {
	if (!(key in views)) return;
	active.value = key;
	localStorage.setItem(VIEW_STORAGE_KEY, key);
}

// 键盘 1-4 切视图，空格开始/暂停。桌面工具该有，也让截图脚本多一条降级路径。
useHotkeys({
	onView: (index) => {
		const item = navItems[index];
		if (item) selectView(item.key);
	},
	onToggleTimer: () => {
		// 空格只在计时页生效。全局绑定的后果是：用户在记录页翻图表时误按一下空格，
		// 就会悄悄开始一次计时（实测确实踩到过——数据库里多出一条 paused 状态）。
		// 开始计时是有副作用的动作，不该在任何一屏都能被误触。
		if (active.value !== "timer") return;
		const s = state.value?.state ?? "idle";
		if (s === "running") void pauseTimer();
		else if (s === "paused") void resumeTimer();
		else void startTimer();
	},
});

// 还没有任何在用的计划时，整个外壳换成引导页：先建计划，才谈得上计时。
const needsOnboarding = computed(() => plansLoaded.value && activePlans.value.length === 0);

onMounted(() => {
	void pullState();
});
</script>

<template>
	<NConfigProvider class="app-root" :theme="naiveTheme" :theme-overrides="themeOverrides">
		<NGlobalStyle />
		<NMessageProvider placement="top">
			<NDialogProvider>
				<NNotificationProvider placement="bottom-right" :max="3">
					<NoticeBridge />

					<!-- 背景层：渐变 + 三处色斑 + 极淡噪点。
					     玻璃面板的 backdrop-filter 就靠它才有东西可模糊：纯渐变是低频信号，
					     模糊了也看不出来，噪点提供了高频细节。 -->
					<div class="gx-backdrop" aria-hidden="true">
						<span class="blob blob-1"></span>
						<span class="blob blob-2"></span>
						<span class="blob blob-3"></span>
						<span class="blob blob-4"></span>
						<span class="gx-grain"></span>
					</div>

					<OnboardingView v-if="needsOnboarding" />

					<div v-else class="shell">
						<NavRail :items="navItems" :active="active" @select="selectView" />
						<main class="content gx-scroll">
							<component :is="currentView" />
						</main>
					</div>
				</NNotificationProvider>
			</NDialogProvider>
		</NMessageProvider>
	</NConfigProvider>
</template>

<style scoped>
.gx-backdrop {
	position: fixed;
	inset: 0;
	z-index: 0;
	overflow: hidden;
	background: var(--gx-bg-gradient);
	pointer-events: none;
}

.blob {
	position: absolute;
	border-radius: 50%;
	/* 40px 而不是 60px：色斑保留一点边界，玻璃面板压在上面时才看得出"糊了一下" */
	filter: blur(40px);
}

/* 四处色斑的位置经过挑选：左上最亮、右上偏青、右下偏靛、左下补一点绿松，
   内容区四角都有色彩变化，不至于整片灰白。 */
.blob-1 {
	top: -16%;
	left: -12%;
	width: 58%;
	height: 48%;
	background: var(--gx-blob-1);
}

.blob-2 {
	top: 2%;
	right: -18%;
	width: 50%;
	height: 44%;
	background: var(--gx-blob-2);
}

.blob-3 {
	bottom: -22%;
	left: 22%;
	width: 56%;
	height: 46%;
	background: var(--gx-blob-3);
}

.blob-4 {
	bottom: -14%;
	right: -14%;
	width: 46%;
	height: 40%;
	background: var(--gx-blob-4);
}

.shell {
	position: relative;
	z-index: 1;
	display: flex;
	width: 100%;
	height: 100%;
	min-height: 0;
}

.content {
	flex: 1 1 auto;
	min-width: 0;
	min-height: 0;
	padding: 16px 18px 20px;
}
</style>
