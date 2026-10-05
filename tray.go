package main

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// trayController 管理托盘图标与右键菜单。
type trayController struct {
	app     *application.App
	backend *backend
	tray    *application.SystemTray
	menu    *application.Menu
}

// newTrayController 创建托盘图标与菜单。
//
// 菜单只有三项：显示主窗口 / 设置 / 退出。
//
// 开始、暂停、继续、结束这些**功能**刻意不放这里（早先版本放过）：
// 托盘菜单是个"入口"，不是控制面板——同一个动作在应用内和托盘各有一套入口，
// 只会让人犹豫该点哪个，而托盘那套还看不见当前状态（今日累计、当前计划）。
// 想操作就左键点开窗口，那里有完整的状态与反馈。
//
// 有意不使用 AttachWindow：那是"在托盘旁边弹一个小面板"的语义，
// 而我们要的是把真正的主窗口叫回来，所以左键点击走 OnClick 显式处理。
func newTrayController(app *application.App, b *backend, icon []byte) *trayController {
	t := &trayController{app: app, backend: b}

	tray := app.SystemTray.New()
	tray.SetIcon(icon)
	tray.SetTooltip(appDisplayName)
	// 左键：把主窗口叫回前台。这是托盘的主要交互。
	tray.OnClick(b.showMainWindow)
	// 不用 SetTemplateIcon：它在 Windows 上是空实现。

	menu := app.NewMenu()
	menu.Add("显示主窗口").OnClick(func(*application.Context) { b.showMainWindow() })
	menu.Add("设置").OnClick(func(*application.Context) { b.openSettings() })
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) { b.quit() })

	tray.SetMenu(menu)

	t.tray = tray
	t.menu = menu
	return t
}

// destroy 摘掉托盘图标。
//
// 正常退出时它会由 OnShutdown 调用一次；如果退出路径绕过了 Wails 的关闭流程
// （os.Exit / panic），则由 main.go 里的 trayCleanup 兜底调用。
// 重复调用是安全的：SystemTray.Destroy 会把托盘从应用的托盘表里摘掉，
// 之后 Wails 遍历那张表时已经没有它了。
//
// 这里刻意打一条日志，而且用词固定：它是"退出时到底有没有删掉托盘图标"
// 唯一可靠的观测点。数通知区域里的图标个数是不可靠的——explorer 的托盘工具条
// 删除按钮是惰性的，数出来的变化说明不了问题。
func (t *trayController) destroy() {
	if t == nil || t.tray == nil {
		return
	}
	t.tray.Destroy()
	slog.Info("已摘掉托盘图标")
}

// update 把最新状态同步到托盘。
//
// 菜单里已经没有随状态变化的东西了，所以这里只剩提示文字——它是唯一的常驻反馈
// （不开窗口也能看到"今天记了多久、记在哪一科"）。菜单项自己不带状态，
// 也就不需要在每次状态变化时重建菜单。
func (t *trayController) update(dto TimerStateDTO) {
	total := dto.TodayTotalSec + dto.CurrentSec
	tip := appDisplayName
	if dto.PlanName != "" {
		tip += " · " + dto.PlanName
	}
	t.tray.SetTooltip(tip + " · 今日 " + formatDurationCN(total))
}
