import { onBeforeUnmount, onMounted } from "vue";

/**
 * 全局键盘快捷键。
 *
 * 桌面工具该有的东西：不用把鼠标移到按钮上也能开始/暂停。同时它也是界面自动化的
 * 一条降级路径——拿不到调试端口时，还能靠按键把各个视图依次切出来截图。
 */
export interface HotkeyHandlers {
	/** 数字键 1..9，参数是 0 基的下标；越界的交给调用方忽略。 */
	onView?: (index: number) => void;
	/** 空格：开始 / 暂停 / 继续。 */
	onToggleTimer?: () => void;
	/** Escape：交给界面自己决定要不要取消（例如收起内联表单）。 */
	onCancel?: () => void;
}

/** 焦点在输入类控件里时不能抢键，否则用户打不出数字和空格。 */
function isTypingTarget(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	const tag = target.tagName;
	return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || target.isContentEditable;
}

/** 有弹窗打开时不响应，免得"确认放弃"的弹窗里按空格把计时也停了。 */
function hasOpenDialog(): boolean {
	return document.querySelector(".n-modal-container, .n-dialog, .n-modal") !== null;
}

export function useHotkeys(handlers: HotkeyHandlers): void {
	function onKeyDown(ev: KeyboardEvent): void {
		if (ev.defaultPrevented || ev.ctrlKey || ev.altKey || ev.metaKey) return;
		if (isTypingTarget(ev.target)) return;

		if (ev.key === "Escape") {
			handlers.onCancel?.();
			return;
		}

		if (hasOpenDialog()) return;

		if (ev.key >= "1" && ev.key <= "9") {
			handlers.onView?.(Number(ev.key) - 1);
			ev.preventDefault();
			return;
		}

		if (ev.key === " " || ev.code === "Space") {
			// 焦点在按钮上时，浏览器本来就会把空格当成"点击这个按钮"。
			// 再自己处理一次会让计时被切换两遍，所以让原生的走。
			if (document.activeElement instanceof HTMLButtonElement) return;
			handlers.onToggleTimer?.();
			ev.preventDefault();
		}
	}

	onMounted(() => window.addEventListener("keydown", onKeyDown));
	onBeforeUnmount(() => window.removeEventListener("keydown", onKeyDown));
}
