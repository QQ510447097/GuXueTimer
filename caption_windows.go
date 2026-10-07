//go:build windows

package main

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"

	"goxuetimer/internal/settings"
)

/*
 * 这个文件负责窗口的"原生外壳"：毛玻璃与标题栏。
 *
 * ## 毛玻璃为什么要从 blur-behind 换成亚克力
 *
 * Win10 上 Wails 的 BackgroundTypeTranslucent 走的是
 *     SetWindowCompositionAttribute(WCA_ACCENT_POLICY, ACCENT_ENABLE_BLURBEHIND)
 * （AccentFlags 与 GradientColor 都是 0）。它让**整个窗口**——包括非客户区——
 * 的底色变成模糊的半透明。
 *
 * 问题出在标题栏：底色变半透明之后，标题文字仍然由系统按"毛玻璃上的文字"绘制，
 * 于是一块不透明的深色底垫在字下面。窗口**非活动**时看不出（那种状态走的是扁平绘制），
 * 一激活就露出来——用户看到的正是"孤雪凝时"四个字压着一个深色方块。
 *
 * 用 tools/accent_probe.ps1 从外部逐个改 AccentState 做对照，实测：
 *   - 0 关掉       → 方块消失，但窗口完全不透明，桌面透不进来
 *   - 3 blur-behind→ 方块出现（Wails 的默认状态）
 *   - 4 亚克力     → 方块消失，且客户区仍然透出并模糊桌面  ← 选它
 *   - 5 hostbackdrop→ 在本机（19045）与 4 渲染完全一致
 *
 * 着色值实测对客户区没有任何可见影响（用 0xCCFF0000 强红试过，界面照旧），
 * 因为应用自己的背景层本来就不透明；所以传 0 保持与 Wails 一致的"不着色"。
 *
 * 如果某台机器上亚克力出现拖动卡顿或画面拖影（Win10 上这是未公开 API 的已知风险），
 * 把 backdropMode 改成 backdropWailsDefault 即可退回原状态——代价是标题栏那个方块会回来，
 * 那时候更该考虑的是把 translucentWindow 关掉（见 main.go）。
 */

// 毛玻璃的实现方式。
const (
	// backdropAcrylic 用 ACCENT_ENABLE_ACRYLICBLURBEHIND，标题栏正常。
	backdropAcrylic = iota
	// backdropWailsDefault 保持 Wails 设的 blur-behind（标题栏会有深色方块）。
	backdropWailsDefault
)

// backdropMode 是唯一的开关，见文件头的说明。
const backdropMode = backdropAcrylic

// ensureBackdrop 让窗口的毛玻璃用能正常绘制标题栏的那种实现。
//
// 必须在 Wails 设完 backdrop 之后调用（也就是窗口创建之后），否则会被它覆盖。
// 它是幂等的，会被多个窗口事件重复调用（显示/恢复/取消最小化都可能重置合成状态），
// 所以日志只打第一次——否则每次唤回窗口都要往日志里多写几行。
func ensureBackdrop(hwnd uintptr) {
	if hwnd == 0 || backdropMode != backdropAcrylic {
		return
	}
	accent := w32.ACCENT_POLICY{
		AccentState: w32.ACCENT_ENABLE_ACRYLICBLURBEHIND,
	}
	data := w32.WINDOWCOMPOSITIONATTRIBDATA{
		Attrib: w32.WCA_ACCENT_POLICY,
		PvData: w32.PVOID(&accent),
		CbData: unsafe.Sizeof(accent),
	}
	w32.SetWindowCompositionAttribute(w32.HWND(hwnd), &data)
	backdropLogged.Do(func() {
		slog.Info("已把窗口毛玻璃切换为亚克力（标题栏文字不再有深色底）")
	})
}

// backdropLogged 让上面那条日志只出现一次。
var backdropLogged sync.Once

// syncCaptionTheme 让标题栏的明暗跟随**应用**主题，而不是系统外观。
//
// 这一步与上面那个方块问题**不是**同一件事：换成亚克力之后标题栏就已经正常了。
// 这里管的是另一种错位——应用是深色而系统外观是浅色（或反过来）时，
// 标题栏文字与底色的明暗不至于拧着。Win10 上标题栏底色本身跟的是系统外观
// （DWMWA_CAPTION_COLOR 要 Win11），所以能做的是把文字样式对齐到应用主题。
func syncCaptionTheme(hwnd uintptr, theme string) {
	if hwnd == 0 {
		return
	}

	dark := false
	switch theme {
	case settings.ThemeDark:
		dark = true
	case settings.ThemeLight:
		dark = false
	default:
		// 跟随系统：标题栏本来也该跟随系统外观。
		dark = w32.IsCurrentlyDarkMode()
	}

	// 不套 application.InvokeSync：它内部是 dispatchOnMainThread + wg.Wait()，
	// 而本函数也会从主线程上的事件处理器里调用，那时主线程会一直等在 wg.Wait() 上——
	// 直接自锁（实测：日志停在 WebView2 初始化那一行，窗口再也不出现）。
	//
	// 这几个 API 本来就允许跨线程调用：窗口过程始终由属主线程执行，
	// 别的线程调用时 Windows 会代为转发，不需要我们手动切线程。
	w32.SetTheme(hwnd, dark)
	// SetTheme 内部只 InvalidateRect 了客户区，而标题栏是非客户区：
	// 不额外重画的话，改动要等窗口被移动或缩放才看得见。
	redrawFrame(hwnd)

	// 只在"明暗真的变了"时记一行：这个函数会被多个窗口事件重复调用，
	// 每次都记的话，光是唤回一次窗口就要往日志里多写三行。
	want := int32(0)
	if dark {
		want = 1
	}
	if lastCaptionDark.Swap(want) != want {
		slog.Info("已对齐标题栏主题", "dark", dark, "theme", theme)
	}
}

// lastCaptionDark 记录上一次应用的标题栏明暗（-1 表示还没应用过）。
// 用 atomic 是因为这个函数可能从不同 goroutine 被调用。
var lastCaptionDark atomic.Int32

func init() { lastCaptionDark.Store(-1) }

// redrawFrame 强制重画非客户区。
func redrawFrame(hwnd uintptr) {
	w32.SetWindowPos(w32.HWND(hwnd), 0, 0, 0, 0, 0,
		w32.SWP_FRAMECHANGED|w32.SWP_NOMOVE|w32.SWP_NOSIZE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE)
	w32.RedrawWindow(w32.HWND(hwnd), nil, 0,
		w32.RDW_FRAME|w32.RDW_INVALIDATE|w32.RDW_UPDATENOW)
}

// windowHandle 取原生窗口句柄；窗口还没创建时返回 0。
//
// 窗口是在 app.Run() 里才真正创建的（NewWithOptions 只是登记），
// 所以 attach 阶段拿不到句柄，必须等窗口就绪的事件。
func windowHandle(window *application.WebviewWindow) uintptr {
	if window == nil {
		return 0
	}
	return uintptr(window.NativeWindow())
}
