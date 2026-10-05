//go:build windows

package main

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"

	"goxuetimer/internal/settings"
)

// syncCaptionTheme 把窗口标题栏的明暗钉到**应用**主题上。
//
// 为什么需要这一步：Win10 上 BackgroundTypeTranslucent 走的是
// SetWindowCompositionAttribute(WCA_ACCENT_POLICY, ACCENT_ENABLE_BLURBEHIND)，
// 它让整个窗口（含非客户区）的底色变成模糊的半透明。而标题栏**文字**按什么样式
// 绘制由窗口主题决定，跟应用自己的浅色界面无关：本机系统外观是深色
// （SystemUsesLightTheme=0）而应用是浅色，两者一错位，文字就按深色样式画出来——
// 白字 + 一块不透明的深色底，压在浅色的半透明标题栏上，
// 正是"标题文字被深色矩形块包围"。已用 tools/window_shot.ps1 实拍确认。
//
// SetTheme 会同时对齐 DWM 的沉浸式深色属性、SetWindowTheme 与
// AllowDarkModeForWindow，所以文字与底色一起跟随应用主题。
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
	// 而本函数也会从 ApplicationStarted 处理器里调用，那时**已经**在主线程上，
	// 主线程会一直等在 wg.Wait() 上——直接自锁（实测：日志停在 WebView2 初始化那一行，
	// 窗口再也不出现）。
	//
	// 这几个 API 本来就允许跨线程调用：窗口过程始终由属主线程执行，
	// 别的线程调用时 Windows 会代为转发，不需要我们手动切线程。
	w32.SetTheme(hwnd, dark)
	// SetTheme 内部只 InvalidateRect 了客户区，而标题栏是非客户区：
	// 不额外重画的话，改动要等窗口被移动或缩放才看得见。
	w32.SetWindowPos(w32.HWND(hwnd), 0, 0, 0, 0, 0,
		w32.SWP_FRAMECHANGED|w32.SWP_NOMOVE|w32.SWP_NOSIZE|w32.SWP_NOZORDER|w32.SWP_NOACTIVATE)
	w32.RedrawWindow(w32.HWND(hwnd), nil, 0,
		w32.RDW_FRAME|w32.RDW_INVALIDATE|w32.RDW_UPDATENOW)
	slog.Info("已对齐标题栏主题", "dark", dark, "theme", theme)
}

// windowHandle 取原生窗口句柄；窗口还没创建时返回 0。
//
// 窗口是在 app.Run() 里才真正创建的（NewWithOptions 只是登记），
// 所以 attach 阶段拿不到句柄，必须在 ApplicationStarted 之后取。
func windowHandle(window *application.WebviewWindow) uintptr {
	if window == nil {
		return 0
	}
	return uintptr(window.NativeWindow())
}
