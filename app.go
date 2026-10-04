package main

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"goxuetimer/internal/apppath"
	"goxuetimer/internal/reminder"
	"goxuetimer/internal/settings"
	"goxuetimer/internal/store"
	"goxuetimer/internal/timer"
)

// backend 把后端各部件与它们的回调接线集中在一处，
// 让 main.go 只负责装配与启动。
//
// 设计要点：状态的唯一权威是 timer.Service；这里只做"汇总 + 广播"。
type backend struct {
	clock func() time.Time

	db       *store.DB
	timer    *timer.Service
	reminder *reminder.Reminder
	notifier *notifierService

	app    *application.App
	window *application.WebviewWindow
	tray   *trayController

	settingsMu sync.RWMutex
	settings   settings.Settings

	// reallyQuitting 用来区分"关闭窗口"和"真正退出"。
	// 托盘「退出」会置位，ShouldQuit 与关窗 hook 都看它。
	reallyQuitting atomic.Bool

	shutdown   chan struct{}
	shutdownOn sync.Once

	// frontendOnce 让"前端已连接"只记一次日志。
	//
	// 这条日志是端到端诊断的锚点：生产构建看不到控制台，
	// "界面白屏但进程正常"和"桥接根本没通"只能靠它区分。
	frontendOnce sync.Once

	// emitHook 是可注入的事件观察者，仅用于测试；生产下为 nil。
	emitHook func(name string, data any)
}

// emit 把事件广播给前端。
//
// app 尚未构造或为空时（单元测试）静默跳过，这样服务层可以脱离 Wails 测试。
func (b *backend) emit(name string, data any) {
	if b.emitHook != nil {
		b.emitHook(name, data)
	}
	if b.app != nil {
		b.app.Event.Emit(name, data)
	}
}

// setupBackend 打开数据库、载入设置与计时状态。
//
// 恢复判定（timer.Recover）故意不在这里做：它要发提示、发事件，
// 必须等 application、窗口、托盘与回调全部就位之后才能调用。
func setupBackend(clk func() time.Time) (*backend, error) {
	dbPath, err := apppath.DBPath()
	if err != nil {
		return nil, fmt.Errorf("确定数据目录失败: %w", err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	b, err := newBackend(db, clk)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	slog.Info("数据目录就绪", "db", dbPath, "settings", b.Settings())
	return b, nil
}

// newBackend 在一个已打开的库上装配后端。
//
// 与 setupBackend 分开是为了可测：测试可以直接喂一个临时库，
// 不必去碰真实的 %LOCALAPPDATA%。
func newBackend(db *store.DB, clk func() time.Time) (*backend, error) {
	if db == nil {
		return nil, fmt.Errorf("newBackend: db 不能为空")
	}
	if clk == nil {
		clk = time.Now
	}

	cfg, err := settings.Load(db)
	if err != nil {
		return nil, err
	}
	timerSvc, err := timer.New(db, clk)
	if err != nil {
		return nil, err
	}

	b := &backend{
		clock:    clk,
		db:       db,
		timer:    timerSvc,
		reminder: reminder.New(),
		notifier: newNotifierService(),
		settings: cfg,
		shutdown: make(chan struct{}),
	}
	b.reminder.Configure(cfg.BreakReminderEnabled, cfg.Interval())
	return b, nil
}

// DBPath 返回数据库路径，供错误日志与界面展示。
func (b *backend) DBPath() string { return b.db.Path() }

// Close 关闭数据库连接。
func (b *backend) Close() error { return b.db.Close() }

// runFromTray 执行一次托盘触发的操作。
//
// 失败时通过事件告知界面而不是静默吞掉——托盘的失效态很容易被忽略，
// 用户会遇到"点了没反应"却不知道原因。
func (b *backend) runFromTray(action func() (timer.Snapshot, error)) {
	if _, err := action(); err != nil {
		slog.Warn("托盘操作失败", "err", err)
		b.emitNotice(timer.NoticeWarning, err.Error())
	}
}

// stopFromTray 结束会话并保存。
func (b *backend) stopFromTray() {
	out, err := b.timer.Stop()
	if err != nil {
		slog.Warn("托盘结束计时失败", "err", err)
		b.emitNotice(timer.NoticeWarning, err.Error())
		return
	}
	if !out.Saved {
		b.emitNotice(timer.NoticeInfo, "本次时长过短，未记录")
		return
	}
	b.emitRecordsChanged("stopped")
}

// Settings 返回当前设置副本。
func (b *backend) Settings() settings.Settings {
	b.settingsMu.RLock()
	defer b.settingsMu.RUnlock()
	return b.settings
}

// updateSettings 先落库、再改内存、最后把新配置推给提醒计数器。
func (b *backend) updateSettings(next settings.Settings) error {
	next = next.Normalize()
	if err := next.Save(b.db); err != nil {
		return err
	}
	b.settingsMu.Lock()
	b.settings = next
	b.settingsMu.Unlock()
	b.reminder.Configure(next.BreakReminderEnabled, next.Interval())
	return nil
}

// noteFrontendConnected 记录前端第一次成功调用后端。
func (b *backend) noteFrontendConnected() {
	b.frontendOnce.Do(func() {
		slog.Info("前端已连接后端，Wails 桥接正常")
	})
}

// stateDTO 汇总前端渲染计时器所需的全部状态。
func (b *backend) stateDTO() (TimerStateDTO, error) {
	now := b.clock()
	snap := b.timer.Snapshot()
	sum, err := b.db.SummaryByDay(now.Format(layoutDateKey))
	if err != nil {
		return TimerStateDTO{}, err
	}
	elapsed, interval := b.reminder.Snapshot()
	return timerStateDTOFrom(snap, now, sum.TotalSec, elapsed, interval), nil
}

// emitState 把最新状态广播给前端与托盘。
func (b *backend) emitState() {
	dto, err := b.stateDTO()
	if err != nil {
		slog.Error("汇总计时状态失败", "err", err)
		return
	}
	b.emit(eventTimerState, dto)
	if b.tray != nil {
		b.tray.update(dto)
	}
}

func (b *backend) emitRecordsChanged(reason string) {
	b.emit(eventRecordsChange, RecordChangeDTO{Reason: reason})
}

func (b *backend) emitNotice(level timer.NoticeLevel, message string) {
	b.emit(eventNotice, NoticeDTO{Level: string(level), Message: message})
}

// attach 在 application.New 之后、app.Run 之前调用，把回调、托盘与心跳挂上。
func (b *backend) attach(app *application.App, window *application.WebviewWindow, tray *trayController) {
	b.app = app
	b.window = window
	b.tray = tray

	// 回调必须在 Run 之前注册好，Recover 才能把提示投递出去。
	b.timer.OnStateChange(func(snap timer.Snapshot) {
		// 会话结束（含被放弃）时把休息提醒的进度清零，
		// 下一次会话从零开始算连续学习。
		if snap.State == timer.StateIdle {
			b.reminder.Reset()
		}
		b.emitState()
	})
	b.timer.OnNotice(func(level timer.NoticeLevel, message string) {
		slog.Warn("计时提示", "message", message)
		b.emitNotice(level, message)
	})

	out, err := b.timer.Recover()
	if err != nil {
		slog.Error("恢复上次计时失败", "err", err)
		b.emitNotice(timer.NoticeWarning, "恢复上次计时失败："+err.Error())
	} else if out.Saved {
		b.emitRecordsChanged("recovered")
	}

	// Recover 无论做什么结论，界面和托盘都需要一份当前状态：
	// 它是 RecoveryNone 时也变了（比如脏数据被清成 idle）。
	b.emitState()

	go b.heartbeat()
}

// stop 结束心跳。OnShutdown 调用，可重复调用。
func (b *backend) stop() {
	b.shutdownOn.Do(func() { close(b.shutdown) })
}

// heartbeat 是唯一的心跳：先推进休息提醒，再结算计时时长。
//
// 顺序不能反。计时器结算后会通过 OnStateChange 发出一次 timer:state，
// 那份 DTO 里带着提醒的进度；如果先结算计时，这次事件携带的就是**上一拍**的
// 提醒进度，而 runningSince 已经前进了 30 秒，前端算出来的倒计时会一直偏大
// 约一个心跳周期，直到下次真正触发提醒才跳回来。
//
// 反过来先推进提醒再结算计时，这次事件里的两个时间戳就是自洽的。
//
// 提醒直接镜像计时器的权威秒数，所以"休眠只算一个心跳""暂停不算时长"
// 这些判定只需要在计时器里存在一份。
func (b *backend) heartbeat() {
	ticker := time.NewTicker(timer.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.shutdown:
			return
		case <-ticker.C:
			if b.reminder.Tick(b.timer.ElapsedSec()) {
				b.fireBreakReminder()
			}
			if _, err := b.timer.Tick(); err != nil {
				slog.Error("计时心跳失败", "err", err)
			}
		}
	}
}

// fireBreakReminder 发一次休息提醒：系统通知 + 应用内事件双通道。
//
// 双通道是有意的：系统通知可能被"专注助手"静音，应用内横幅不会。
func (b *backend) fireBreakReminder() {
	cfg := b.Settings()
	minutes := cfg.BreakIntervalMin

	body := fmt.Sprintf("已连续学习 %d 分钟，起来活动一下", minutes)
	sent := b.notifier.send(notifications.NotificationOptions{
		ID:    fmt.Sprintf("goxuetimer-break-%d", b.clock().Unix()),
		Title: "该休息了",
		Body:  body,
	})
	// 这条日志是"提醒链路真的走通了"的客观锚点：生产构建没有控制台，
	// 而系统通知本身可能被专注助手静音、无法从外部观察。
	slog.Info("已发出休息提醒", "minutes", minutes, "systemToast", sent)

	b.emit(eventReminderBreak, ReminderEventDTO{IntervalMin: minutes})
	b.emitState()
}

// showMainWindow 把主窗口叫回前台。
//
// 隐藏态的窗口必须先 Show 再 Focus，只 Restore 是不够的。
func (b *backend) showMainWindow() {
	if b.window == nil {
		return
	}
	b.window.Restore()
	b.window.Show()
	b.window.Focus()
}

// quit 真正退出应用。
func (b *backend) quit() {
	b.reallyQuitting.Store(true)
	b.app.Quit()
}
