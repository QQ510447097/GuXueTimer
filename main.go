package main

import (
	"embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"goxuetimer/internal/apppath"
)

// appDisplayName 是给用户看的名字：窗口标题、托盘提示、关于页都用它。
//
// 注意它同时会成为 Windows 通知的 AppUserModelId —— Wails 用
// application.Options.Name 作为 AppID 注册到
// HKCU\Software\Classes\AppUserModelId\ 下。所以这里是整个项目里唯一一处
// 中文会进入系统标识的地方；冒烟测试用 "systemToast=true" 断言把关，
// 一旦那条挂了，就把这里退回成 ASCII 的 "GuXueTimer"，其余中文名不受影响。
const appDisplayName = "孤雪学习计时器"

// Wails 通过 embed 把前端产物打进二进制。
//
//go:embed all:frontend/dist
var assets embed.FS

// 托盘图标。32x32 PNG 即可：Windows 侧会解码成 HICON，不需要 .ico。
//
//go:embed assets/tray.png
var trayIcon []byte

func init() {
	// 注册事件的数据类型，绑定生成器据此产出强类型的 TS API。
	application.RegisterEvent[TimerStateDTO](eventTimerState)
	application.RegisterEvent[RecordChangeDTO](eventRecordsChange)
	application.RegisterEvent[ReminderEventDTO](eventReminderBreak)
	application.RegisterEvent[NoticeDTO](eventNotice)
}

func main() {
	logFile := setupLogging()
	if logFile != nil {
		defer func() { _ = logFile.Close() }()
	}

	b, err := setupBackend(nil)
	if err != nil {
		fatal("初始化失败", err)
	}
	defer func() {
		b.stop()
		if cerr := b.Close(); cerr != nil {
			slog.Warn("关闭数据库失败", "err", cerr)
		}
	}()

	app := application.New(application.Options{
		Name:        appDisplayName,
		Description: "专注学习计时器",
		Services: []application.Service{
			application.NewService(NewTimerService(b)),
			application.NewService(NewRecordsService(b)),
			application.NewService(NewSettingsService(b)),
			// 通知服务自己实现了 ServiceStartup/ServiceShutdown：
			// 注册之后它会在启动时完成 Windows 通知所需的注册。
			application.NewService(b.notifier),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Windows: application.WindowsOptions{
			// 关窗只是隐藏到托盘，不能因为"最后一个窗口关了"就退出进程。
			DisableQuitOnLastWindowClosed: true,
		},
		ShouldQuit: func() bool {
			// 只有托盘「退出」置位后才放行；否则拦掉一切退出请求。
			return b.reallyQuitting.Load() || !b.Settings().MinimizeToTray
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.guxue.goxuetimer",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				// 第二次启动：把已有窗口叫回来，而不是再开一个进程。
				b.showMainWindow()
			},
		},
		OnShutdown: func() {
			b.stop()
			slog.Info("应用正在退出")
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            appDisplayName,
		Width:            480,
		Height:           720,
		MinWidth:         400,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(16, 18, 27),
		URL:              "/",
	})

	// 关窗的处理：默认隐藏到托盘，用户关了"最小化到托盘"就真的退出。
	//
	// 必须用 RegisterHook：OnWindowEvent 注册的是纯监听器，在监听器里调用
	// Cancel 不会有任何效果——HandleWindowEvent 只检查 hook 写下的取消位，
	// hook 一旦取消就直接返回，不再派发关闭。
	//
	// 关窗即退出的分支必须显式调用 Quit：Wails 的 ShouldQuit 只在应用级
	// destroy 时被查询，而 DisableQuitOnLastWindowClosed 会拦住"最后一个窗口
	// 关闭就退出"的自动路径。少了这一步，窗口关了、进程还在、托盘也没了入口。
	onClosing := func(e *application.WindowEvent) {
		if b.reallyQuitting.Load() {
			return
		}
		if !b.Settings().MinimizeToTray {
			go b.quit()
			return
		}
		window.Hide()
		e.Cancel()
	}
	window.RegisterHook(events.Common.WindowClosing, onClosing)
	// Windows 会先发平台事件；两套都挂上，无论哪一套到达都能拦住关闭。
	if events.Windows.WindowClosing != events.Common.WindowClosing {
		window.RegisterHook(events.Windows.WindowClosing, onClosing)
	}

	tray := newTrayController(app, b, trayIcon)
	b.attach(app, window, tray)

	// 应用真正跑起来之后再刷一次托盘：Menu.Update 在 app.Run 之前是空操作。
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		slog.Info("应用已启动")
		b.emitState()
	})

	if err := app.Run(); err != nil {
		fatal("运行失败", err)
	}
}

// fanout 把同一份日志写给多个 sink，每个 sink 的失败都单独忽略。
//
// 不能用 io.MultiWriter：它在第一个写失败的 sink 上就 return，
// 后面的 sink 一个字节都拿不到。生产构建带 -H windowsgui，os.Stderr 是个
// 无效句柄，第一次写就失败——那样日志会全部丢失，文件恒为 0 字节。
type fanout struct{ sinks []io.Writer }

func (f fanout) Write(p []byte) (int, error) {
	for _, w := range f.sinks {
		_, _ = w.Write(p)
	}
	return len(p), nil
}

// setupLogging 把日志同时写到 stderr 和 <数据目录>/app.log。
//
// 生产构建带 -H windowsgui，没有控制台可看；没有日志文件的话，
// 用户遇到"点了没反应"就完全没有排查依据。日志文件失败不影响主流程。
func setupLogging() *os.File {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	dir, err := apppath.EnsureDir()
	if err != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(fanout{sinks: []io.Writer{f, os.Stderr}}, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
	return f
}

// fatal 记录致命错误后退出。
//
// 同样因为看不到控制台，这里额外写一份到数据目录下的 error.log。
func fatal(msg string, err error) {
	line := fmt.Sprintf("%s %s: %v\n", time.Now().Format(time.RFC3339), msg, err)
	fmt.Fprint(os.Stderr, line)

	if dir, derr := apppath.DataDir(); derr == nil {
		if f, ferr := os.OpenFile(filepath.Join(dir, "error.log"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); ferr == nil {
			_, _ = f.WriteString(line)
			_ = f.Close()
		}
	}
	os.Exit(1)
}
