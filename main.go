package main

import (
	"embed"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"goxuetimer/internal/apppath"
	"goxuetimer/internal/settings"
)

// appDisplayName 是给用户看的名字：窗口标题、托盘提示、关于页都用它。
//
// 注意它同时会成为 Windows 通知的 AppUserModelId —— Wails 用
// application.Options.Name 作为 AppID 注册到
// HKCU\Software\Classes\AppUserModelId\ 下。所以这里是整个项目里唯一一处
// 中文会进入系统标识的地方；冒烟测试用 "systemToast=true" 断言把关，
// 一旦那条挂了，就把这里退回成 ASCII 的 "GuXueTimer"，其余中文名不受影响。
const appDisplayName = "孤雪凝时"

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
	application.RegisterEvent[ChangedDTO](eventRecordsChange)
	application.RegisterEvent[ChangedDTO](eventPlansChange)
	application.RegisterEvent[NavGotoDTO](eventNavGoto)
	application.RegisterEvent[ReminderEventDTO](eventReminderBreak)
	application.RegisterEvent[NoticeDTO](eventNotice)
}

func main() {
	logFile := setupLogging()
	if logFile != nil {
		defer func() { _ = logFile.Close() }()
	}

	// 异常退出时的兜底。
	//
	// 正常退出会走 app.Quit() -> Wails 的关闭流程 -> 逐个 systray.destroy()
	// （内部就是 NIM_DELETE），托盘图标会被摘掉。但下面两条路径绕过了那套流程：
	//
	//   - app.Run() 返回错误后走 fatal()，里面是 os.Exit
	//   - main 里发生 panic
	//
	// 两者都会让进程直接消失，而托盘图标不显式删除就会变成通知区域里的
	// "残留图标"——直到鼠标扫过才被系统清掉，也就是用户看到的那种"图标越堆越多"。
	// 用一个可替换的清理函数记下当时的托盘，供这两条路径调用。
	trayCleanup := func() {}
	defer func() {
		if r := recover(); r != nil {
			trayCleanup()
			fatal("发生未预期的错误", fmt.Errorf("%v", r))
		}
	}()

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
			application.NewService(NewPlansService(b)),
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
			AdditionalBrowserArgs:         debugBrowserArgs(),
		},
		ShouldQuit: func() bool {
			// 只有托盘「退出」置位后才放行；否则拦掉一切退出请求。
			return b.reallyQuitting.Load() || !b.Settings().MinimizeToTray
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.guduxue.guxuetimer",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				// 支持 `GuXueTimer.exe --quit`：让"优雅退出已有实例"变成一个可以从
				// 命令行或脚本触发的动作。
				//
				// 这不只是给开发工具用的。走这个开关和走托盘菜单的「退出」是同一条
				// 路径（b.quit -> app.Quit -> Wails 关闭流程 -> NIM_DELETE），而
				// "结束任务"式的强杀不是——强杀会留下残留图标。有了这个开关，
				// 脚本和快捷方式就不必再用强杀，也就不会再往通知区域里堆东西。
				if hasQuitFlag(data.Args) {
					slog.Info("收到 --quit，正在退出已有实例")
					b.quit()
					return
				}
				// 第二次启动：把已有窗口叫回来，而不是再开一个进程。
				b.showMainWindow()
			},
		},
		OnShutdown: func() {
			b.stop()
			// 显式摘掉托盘再让 Wails 收尾。两点考虑：
			//
			//   1. 可观测：我们自己调 destroy，日志里就一定有"已摘掉托盘图标"。
			//      "退出时删没删图标"这件事因此可以用日志断言，而不必去数通知
			//      区域里那几个图标——那件事数不准（explorer 删除按钮是惰性的）。
			//   2. 幂等：Destroy 会把托盘从应用的托盘表里摘掉，Wails 随后的
			//      关闭流程遍历那张表时已经没有它，不会重复删除。
			trayCleanup()
			slog.Info("应用正在退出")
		},
	})

	// 原生底色取当前主题的底色，避免启动瞬间闪一下另一种颜色。
	// "跟随系统"在这里按浅色处理，理由见 backend.syncWindowBackground。
	//
	// 开着窗口毛玻璃时这一层也被模糊过的桌面盖住，但仍要设：拖动/缩放窗口时
	// 新露出来的那块会先显示原生底色，底色和主题不一致就会闪一下。
	cfg := b.Settings()
	backdrop := application.NewRGB(238, 244, 255)
	if cfg.Theme == settings.ThemeDark {
		backdrop = application.NewRGB(10, 18, 32)
	}
	// 窗口尺寸是量出来的，不是估的：把设置页与记录页的内容高度量出来之后，
	// 960 高（内容区约 921）刚好让两页都不用滚。宽度收到 620——
	// 记录页的月历七列仍有约 67px 一格，够放日期数字。
	windowOpts := application.WebviewWindowOptions{
		Title:  appDisplayName,
		Width:  620,
		Height: 960,
		// 最小尺寸下页面会滚动，这是应有的行为（内容本来就超出一屏），
		// 但不许出现横向滚动条。
		MinWidth:         540,
		MinHeight:        720,
		BackgroundColour: backdrop,
		URL:              "/",
	}
	// 窗口级毛玻璃：让桌面透出来并模糊。
	//
	// 恒定开启，界面里没有开关——这个效果是这套外观的一半，做成可选项只会让人
	// 在"要不要开"上耗神，而且开了之后背景层也要跟着改半透明才看得出差别。
	//
	// 实现上：Windows 上 Wails 走 SetWindowCompositionAttribute（Win10 回退路径）
	// 或 DWM 的 Mica/Acrylic（Win11 22621+）。前者是未公开 API，个别显卡/驱动上
	// 可能出现缩放卡顿或整窗发黑；真遇到了，把下面这行的 true 改成 false
	// 并重新构建即可回到不透明窗口（应用内的玻璃观感不受影响）。
	const translucentWindow = true
	if translucentWindow {
		windowOpts.BackgroundType = application.BackgroundTypeTranslucent
		// 官方文档说 Acrylic 需要 Win11 22621+；Win10 上 Wails 会回退到
		// SetWindowCompositionAttribute 的 blur-behind。
		windowOpts.Windows.BackdropType = application.Acrylic
	}
	window := app.Window.NewWithOptions(windowOpts)

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
	// 记下清理动作：此后任何绕过 app.Quit() 的退出路径都能先把托盘摘掉。
	trayCleanup = tray.destroy
	b.attach(app, window, tray)

	// 应用启动后刷一次状态；Menu.Update 在 app.Run 之前是空操作。
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		slog.Info("应用已启动")
		b.emitState()
	})

	// 拿窗口句柄的时机要晚于 ApplicationStarted：那个事件触发时窗口还没创建
	// （实测日志顺序是"应用已启动"早于 WebView2 初始化），句柄是 0。
	// WindowRuntimeReady 是窗口与 webview 都就绪之后，句柄一定有效。
	//
	// 取到句柄后按应用主题对齐标题栏：Win10 的 blur-behind 让标题栏底色变半透明，
	// 而文字样式由窗口主题决定，不跟着应用主题走就会出现"白字压着深色方块"。
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		b.hwnd.Store(windowHandle(window))
		b.syncWindowBackground(b.Settings())
	})

	if err := app.Run(); err != nil {
		// Run 出错时托盘可能已经注册过了，先摘掉再退出：fatal 走的是 os.Exit，
		// 不会经过 Wails 的关闭流程。
		trayCleanup()
		fatal("运行失败", err)
	}
}

// hasQuitFlag 判断第二次启动是否带着"让已有实例退出"的意图。
//
// 三种写法都接受：Windows 上习惯用 /quit，跨平台脚本常用 --quit。
func hasQuitFlag(args []string) bool {
	for _, arg := range args {
		switch strings.ToLower(strings.TrimSpace(arg)) {
		case "--quit", "-quit", "/quit":
			return true
		}
	}
	return false
}

// debugBrowserArgs 在设置了 GX_DEBUG_PORT 时给 WebView2 开远程调试端口。
//
// 只为开发期一件具体的事存在：用 CDP 驱动界面截图并断言布局。
// "这个界面到底长什么样"靠读代码是判断不出来的，唯一可靠的办法是让渲染进程
// 自己吐一张图出来。不设这个环境变量时返回 nil，发布构建的行为完全不受影响。
func debugBrowserArgs() []string {
	port := strings.TrimSpace(os.Getenv("GX_DEBUG_PORT"))
	if port == "" {
		return nil
	}
	slog.Info("已开启 WebView2 远程调试端口", "port", port)
	return []string{"--remote-debugging-port=" + port}
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
