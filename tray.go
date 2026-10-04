package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"goxuetimer/internal/timer"
)

// trayController 管理托盘图标、菜单，以及菜单状态与计时状态的同步。
type trayController struct {
	app     *application.App
	backend *backend
	tray    *application.SystemTray
	menu    *application.Menu

	statusItem *application.MenuItem
	startItem  *application.MenuItem
	pauseItem  *application.MenuItem
	resumeItem *application.MenuItem
	stopItem   *application.MenuItem
}

// newTrayController 创建托盘图标与菜单。
//
// 有意不使用 AttachWindow：那是"在托盘旁边弹一个小面板"的语义，
// 而我们要的是把真正的主窗口叫回来，所以左键点击走 OnClick 显式处理。
func newTrayController(app *application.App, b *backend, icon []byte) *trayController {
	t := &trayController{app: app, backend: b}

	tray := app.SystemTray.New()
	tray.SetIcon(icon)
	tray.SetTooltip(appDisplayName)
	// 左键：把主窗口叫回前台。
	tray.OnClick(b.showMainWindow)
	// 不用 SetTemplateIcon：它在 Windows 上是空实现。

	menu := app.NewMenu()
	t.statusItem = menu.Add("今日累计 --")
	t.statusItem.SetEnabled(false)
	menu.AddSeparator()

	t.startItem = menu.Add("开始")
	t.startItem.OnClick(func(*application.Context) { b.startFromTray() })

	t.pauseItem = menu.Add("暂停")
	t.pauseItem.OnClick(func(*application.Context) { b.runFromTray(b.timer.Pause) })

	t.resumeItem = menu.Add("继续")
	t.resumeItem.OnClick(func(*application.Context) { b.runFromTray(b.timer.Resume) })

	t.stopItem = menu.Add("结束并保存")
	t.stopItem.OnClick(func(*application.Context) { b.stopFromTray() })

	menu.AddSeparator()
	menu.Add("显示主窗口").OnClick(func(*application.Context) { b.showMainWindow() })
	menu.Add("退出").OnClick(func(*application.Context) { b.quit() })

	tray.SetMenu(menu)

	t.tray = tray
	t.menu = menu
	return t
}

// update 把最新状态同步到托盘。
//
// 即使状态没变也要刷新：今日累计一直在涨。
// SetLabel/SetEnabled 只改内存字段，真正的菜单在弹出时按这些字段重建，
// 所以在后台 goroutine 里调用是安全的。
func (t *trayController) update(dto TimerStateDTO) {
	total := dto.TodayTotalSec + dto.CurrentSec
	label := "今日累计 " + formatDurationCN(total)
	t.statusItem.SetLabel(label)

	// 托盘提示里带上当前计划：一眼就能确认"现在记的是哪一科"。
	tip := appDisplayName
	if dto.PlanName != "" {
		tip += " · " + dto.PlanName
	}
	t.tray.SetTooltip(tip + " · " + label)

	running := dto.State == string(timer.StateRunning)
	idle := dto.State == string(timer.StateIdle)

	t.startItem.SetEnabled(idle)
	t.pauseItem.SetEnabled(running)
	t.resumeItem.SetEnabled(!running && !idle)
	t.stopItem.SetEnabled(!idle)

	// 菜单状态改完必须 Update：应用未跑起来时它是空操作，
	// 所以 ApplicationStarted 之后还会再刷一次。
	t.menu.Update()
}
