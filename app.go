package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"goxuetimer/internal/apppath"
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

	db    *store.DB
	timer *timer.Service

	app    *application.App
	window *application.WebviewWindow
	tray   *trayController

	// hwnd 是原生窗口句柄，只在需要动窗口外观时用（标题栏主题）。
	//
	// 用 atomic 而不是普通字段：它是 ApplicationStarted 时在主线程写一次、
	// 之后在设置变更的任意 goroutine 里读。加锁不值当，原子读写最省事。
	hwnd atomic.Uintptr

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

	// dataDirFn 覆盖数据目录的解析，仅用于测试（把读写限制在临时目录里）。
	dataDirFn func() (string, error)
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
		settings: cfg,
		shutdown: make(chan struct{}),
	}
	return b, nil
}

// DBPath 返回数据库路径，供错误日志与界面展示。
func (b *backend) DBPath() string { return b.db.Path() }

// Close 关闭数据库连接。
func (b *backend) Close() error { return b.db.Close() }

// openSettings 从托盘打开设置页：把窗口叫回前台，再让界面切到设置。
//
// 两件事都要做：窗口可能藏在托盘后面，而界面可能停在别的页。
// 切页是前端的事，所以走事件而不是直接改状态——导航状态的唯一归属在前端。
func (b *backend) openSettings() {
	b.showMainWindow()
	b.emit(eventNavGoto, NavGotoDTO{View: "settings"})
}

// dataDir 返回数据目录的绝对路径。
//
// 逐级降级在 apppath 里，这里统一走一个入口，免得设置服务与个人化服务
// 各自算一遍、算出两个不同的目录。
//
// 做成可注入的字段是为了能测：资源中间件与背景图导入都要往这个目录里读写，
// 测试必须把它指到一个临时目录，绝不能碰用户真正的数据目录。
func (b *backend) dataDir() (string, error) {
	if b.dataDirFn != nil {
		return b.dataDirFn()
	}
	dir, err := apppath.EnsureDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(dir)
}

// backgroundState 汇总背景图的当前状态。
//
// 地址里带 ?v=<修改时间>：换图之后文件名不变（就是 background.png），
// 不带版本号浏览器会一直用缓存里的上一张——这个坑一定会踩到。
// 文件不存在时返回空状态（用户可能手动删了数据目录里的图），
// 免得给前端一个必然 404 的地址。
func (b *backend) backgroundState() PersonalizationDTO {
	name := b.Settings().BackgroundImage
	if name == "" {
		return PersonalizationDTO{}
	}
	dir, err := b.dataDir()
	if err != nil {
		slog.Warn("读取数据目录失败", "err", err)
		return PersonalizationDTO{}
	}
	info, err := os.Stat(filepath.Join(dir, filepath.Base(name)))
	if err != nil {
		slog.Warn("背景图文件不存在，按未设置处理", "image", name, "err", err)
		return PersonalizationDTO{}
	}
	return PersonalizationDTO{
		Image: name,
		URL:   "/user/" + url.PathEscape(name) + "?v=" + strconv.FormatInt(info.ModTime().Unix(), 10),
	}
}

// Settings 返回当前设置副本。
func (b *backend) Settings() settings.Settings {
	b.settingsMu.RLock()
	defer b.settingsMu.RUnlock()
	return b.settings
}

// settleResult 描述一次"顺手结算"的结果。
type settleResult struct {
	// Settled 表示确实有一次进行中的会话被结算掉了。
	Settled bool
	// Saved 表示那次结算写进了记录（时长过短时不会写）。
	Saved bool
	// Sec 是结算下来的秒数。
	Sec int64
	// PlanName 是那段时间原来挂在哪个计划上。
	PlanName string
}

// settleIfRunning 把正在进行的会话结束并落库，用于"切换计划 / 归档当前计划 /
// 删除当前计划"这些会让"这段时长算谁的"发生变化的前置动作。
//
// 空闲时是空操作（Settled=false），所以调用方不必先判断状态。
//
// 失败时返回错误且**状态不变**：调用方必须据此中止后续动作。
// 宁可这次操作没发生，也不能把用户的一段时间弄丢。
func (b *backend) settleIfRunning() (settleResult, error) {
	if !b.timerRunning() {
		return settleResult{}, nil
	}

	// 先记下"这段时长属于谁"：Stop 之后状态就回 idle 了，拿不到计划名。
	planID := b.activePlanIDQuiet()
	name, _ := b.planLabel(planID)

	out, err := b.timer.Stop()
	if err != nil {
		return settleResult{}, err
	}
	if out.Saved {
		// 记录已经变了，界面上的图表与列表要重新拉。
		b.emitRecordsChanged("switched")
	}
	return settleResult{
		Settled:  true,
		Saved:    out.Saved,
		Sec:      out.DurationSec,
		PlanName: name,
	}, nil
}

// activePlanIDQuiet 取当前会话所属的计划 id，取不到时返回 0。
//
// 与 activePlanID 的区别是不报错、不回落：结算路径上拿不到计划名不该让结算失败，
// 而"这段时长算谁的"要用会话**自己记着**的计划（snap.PlanID），
// 不是设置里那个可能已经变了的当前计划。
func (b *backend) activePlanIDQuiet() int64 {
	if id := b.timer.Snapshot().PlanID; id > 0 {
		return id
	}
	return b.currentPlanID()
}

// updateSettings 先落库、再改内存、最后把新配置推给窗口外壳。
func (b *backend) updateSettings(next settings.Settings) error {
	next = next.Normalize()
	if err := next.Save(b.db); err != nil {
		return err
	}
	b.settingsMu.Lock()
	b.settings = next
	b.settingsMu.Unlock()
	b.syncWindowBackground(next)
	return nil
}

// syncWindowBackground 让原生窗口底色与标题栏主题跟上应用主题。
//
// 两件事都要做，原因不同：
//   - 底色：调整窗口大小时，新露出来的那块会先显示原生底色、等 WebView 重绘才被盖住。
//     底色和主题不一致时，暗色主题下拖窗口会闪白。
//   - 标题栏主题：Win10 的 blur-behind 让标题栏底色变半透明，而文字样式由窗口主题
//     决定；不跟着应用主题对齐就会出现"白字压着深色方块"（详见 syncCaptionTheme）。
func (b *backend) syncWindowBackground(s settings.Settings) {
	b.refreshCaptionTheme(s)

	if b.window == nil {
		return
	}
	if s.Theme == settings.ThemeDark {
		b.window.SetBackgroundColour(application.NewRGB(10, 18, 32))
		return
	}
	b.window.SetBackgroundColour(application.NewRGB(238, 244, 255))
}

// refreshCaptionTheme 是平台无关的转发：真正的 Win32 调用在 caption_windows.go 里，
// 非 Windows 平台是空实现，这样 app.go 不必带任何平台相关的 import。
//
// 两件事一起做，都是"原生外壳"的一部分：
//   - ensureBackdrop：把毛玻璃换成能正常绘制标题栏的实现（幂等，第二次调用无害）
//   - syncCaptionTheme：让标题栏明暗跟随应用主题
func (b *backend) refreshCaptionTheme(s settings.Settings) {
	hwnd := b.hwnd.Load()
	ensureBackdrop(hwnd)
	syncCaptionTheme(hwnd, s.Theme)
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
	sum, err := b.db.SummaryByDay(now.Format(layoutDateKey), 0)
	if err != nil {
		return TimerStateDTO{}, err
	}

	// 空闲时报"当前计划"（也就是下一段会记到哪），进行中时报这次会话自己的计划。
	//
	// 两者在可达状态下是同一个值——计时中禁止切换计划——唯一的差别出现在
	// "恢复了一段属于已归档计划的会话"这种边角情况：那时该显示的是那段会话
	// 真正的归属，而不是现在的选择。
	planID := snap.PlanID
	if snap.State == timer.StateIdle {
		planID = b.currentPlanID()
	}
	planName, planColor := b.planLabel(planID)

	return timerStateDTOFrom(snap, now, sum.TotalSec, planID, planName, planColor), nil
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
	b.emit(eventRecordsChange, ChangedDTO{Reason: reason})
}

func (b *backend) emitPlansChanged(reason string) {
	b.emit(eventPlansChange, ChangedDTO{Reason: reason})
}

func (b *backend) emitNotice(level timer.NoticeLevel, message string) {
	b.emit(eventNotice, NoticeDTO{Level: string(level), Message: message})
}

// timerRunning 表示当前有一次会话正在进行（运行中或已暂停）。
func (b *backend) timerRunning() bool {
	return b.timer.Snapshot().State != timer.StateIdle
}

// currentPlanID 返回当前计划 id；0 表示还没有任何计划。
func (b *backend) currentPlanID() int64 {
	return b.Settings().CurrentPlanID
}

// activePlanID 返回一个**可用**的当前计划：存在且未归档。
//
// 只看 CurrentPlanID 不够：它可能在"计划被归档/删除时正好有会话在进行"的
// 空档里残留成一个失效值（这时 repairCurrentPlan 会刻意不动它）。
// 开始一次新会话前必须过这一关，否则会把时长记到一个已经不存在的计划上。
//
// 顺手纠正设置，所以调用方不需要再自己 repair。
func (b *backend) activePlanID() (int64, error) {
	cur := b.currentPlanID()
	if cur > 0 {
		if plan, err := b.db.GetPlan(cur); err == nil && !plan.Archived {
			return cur, nil
		}
	}
	id, ok, err := b.db.FirstActivePlanID()
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, ErrNoPlan
	}
	if err := b.setCurrentPlan(id); err != nil {
		return 0, err
	}
	return id, nil
}

// setCurrentPlan 只改"当前计划"这一个字段。
func (b *backend) setCurrentPlan(id int64) error {
	next := b.Settings()
	if next.CurrentPlanID == id {
		return nil
	}
	next.CurrentPlanID = id
	return b.updateSettings(next)
}

// repairCurrentPlan 确保"当前计划"指向一个存在的、未归档的计划。
//
// 三类情况都会走到这里：首次启动、当前计划被归档、当前计划被删除。
//
// 会话进行中不偷换：宁可让 currentPlanId 暂时指着一个已归档的计划，
// 也不要在用户不知情时改变"这段时长算谁的"。等这段会话结束、下一次
// 计划变更时自然会纠正。
func (b *backend) repairCurrentPlan() error {
	cur := b.currentPlanID()
	if cur > 0 {
		if plan, err := b.db.GetPlan(cur); err == nil && !plan.Archived {
			return nil
		}
	}
	if b.timerRunning() {
		return nil
	}
	id, ok, err := b.db.FirstActivePlanID()
	if err != nil {
		return err
	}
	if !ok {
		id = 0
	}
	return b.setCurrentPlan(id)
}

// planLabel 把计划 id 解析成给用户看的名字与颜色。
//
// 查不到（未分类、计划被删、id 为 0）时返回空串，由界面决定显示什么，
// 后端不替它编"未分类"这种文案。
func (b *backend) planLabel(id int64) (name string, color string) {
	if id <= 0 {
		return "", ""
	}
	plan, err := b.db.GetPlan(id)
	if err != nil {
		return "", ""
	}
	return plan.Name, plan.Color
}

// attach 在 application.New 之后、app.Run 之前调用，把回调、托盘与心跳挂上。
func (b *backend) attach(app *application.App, window *application.WebviewWindow, tray *trayController) {
	b.app = app
	b.window = window
	b.tray = tray

	// 窗口创建时用的底色只是初值，这里按落库的设置再校一次。
	b.syncWindowBackground(b.Settings())

	// 回调必须在 Run 之前注册好，Recover 才能把提示投递出去。
	b.timer.OnStateChange(func(snap timer.Snapshot) {
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

	// 当前计划可能指向一个已被归档/删除的计划（用户在上次运行里干的），
	// 启动时纠正一次，让界面一进来就显示对的那个。
	if err := b.repairCurrentPlan(); err != nil {
		slog.Warn("纠正当前计划失败", "err", err)
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

// heartbeat 是唯一的心跳：结算计时时长并落库。
//
// 心跳之所以存在（而不是纯靠渲染时算），是因为"暂停/休眠该怎么算""异常退出后
// 该怎么接回"这些判定必须有一份权威的、定期落盘的答案；前端只负责显示。
func (b *backend) heartbeat() {
	ticker := time.NewTicker(timer.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.shutdown:
			return
		case <-ticker.C:
			if _, err := b.timer.Tick(); err != nil {
				slog.Error("计时心跳失败", "err", err)
			}
		}
	}
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
