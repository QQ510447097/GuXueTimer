package main

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"goxuetimer/internal/settings"
	"goxuetimer/internal/store"
	"goxuetimer/internal/timer"
)

// 这一组是服务层的集成测试：直接用临时库跑真实的 SQL 与状态机，
// 只是绕开 Wails 的 application（事件走注入的钩子）。
// 它验证的是"用户点下去会发生什么"，而不是内部实现细节。

var t0 = time.Date(2026, 5, 1, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock(t time.Time) *clock { return &clock{now: t} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type eventSpy struct {
	mu     sync.Mutex
	names  []string
	states []TimerStateDTO
}

func (s *eventSpy) record(name string, data any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = append(s.names, name)
	if dto, ok := data.(TimerStateDTO); ok {
		s.states = append(s.states, dto)
	}
}

func (s *eventSpy) count(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, got := range s.names {
		if got == name {
			n++
		}
	}
	return n
}

func (s *eventSpy) lastState(t *testing.T) TimerStateDTO {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.states) == 0 {
		t.Fatal("没有收到任何 timer:state 事件")
	}
	return s.states[len(s.states)-1]
}

// harness 把一套后端 + 三个服务装好，模拟应用启动后的可用状态。
type harness struct {
	b        *backend
	clock    *clock
	spy      *eventSpy
	timer    *TimerService
	records  *RecordsService
	settings *SettingsService
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open() 失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	clk := newClock(t0)
	b, err := newBackend(db, clk.Now)
	if err != nil {
		t.Fatalf("newBackend() 失败: %v", err)
	}
	spy := &eventSpy{}
	b.emitHook = spy.record

	// 模拟 attach 里的回调注册与恢复流程（不含窗口/托盘）。
	b.timer.OnStateChange(func(snap timer.Snapshot) {
		if snap.State == timer.StateIdle {
			b.reminder.Reset()
		}
		b.emitState()
	})
	b.timer.OnNotice(func(level timer.NoticeLevel, message string) {
		b.emitNotice(level, message)
	})
	if _, err := b.timer.Recover(); err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}

	return &harness{
		b:        b,
		clock:    clk,
		spy:      spy,
		timer:    NewTimerService(b),
		records:  NewRecordsService(b),
		settings: NewSettingsService(b),
	}
}

// ---------------------------------------------------------------- 完整计时流程

func TestService_FullTimerFlow(t *testing.T) {
	h := newHarness(t)

	state, err := h.timer.GetState()
	if err != nil {
		t.Fatalf("GetState() 失败: %v", err)
	}
	if state.State != string(timer.StateIdle) || state.CurrentSec != 0 || state.TodayTotalSec != 0 {
		t.Fatalf("初始状态 = %+v, 期望 idle/0/0", state)
	}

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(25 * time.Minute)
	if _, err := h.timer.Pause(); err != nil {
		t.Fatalf("Pause() 失败: %v", err)
	}
	h.clock.Advance(5 * time.Minute) // 休息，不该计入
	if _, err := h.timer.Resume(); err != nil {
		t.Fatalf("Resume() 失败: %v", err)
	}
	h.clock.Advance(5 * time.Minute)

	stopped, err := h.timer.Stop()
	if err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}
	const want = 30 * 60
	if !stopped.Saved {
		t.Fatalf("Stop() 未保存: %+v", stopped)
	}
	if stopped.DurationSec != want {
		t.Errorf("DurationSec = %d, 期望 %d（暂停的 5 分钟必须排除）", stopped.DurationSec, want)
	}
	if stopped.Session == nil || stopped.Session.StartedAt != "2026-05-01T09:00:00+08:00" {
		t.Errorf("记录 = %+v, StartedAt 不符", stopped.Session)
	}

	// 落库之后，今日累计与列表都要能看到它。
	sum, err := h.records.TodaySummary()
	if err != nil {
		t.Fatalf("TodaySummary() 失败: %v", err)
	}
	if sum.TotalSec != want || sum.Count != 1 {
		t.Errorf("今日汇总 = %+v, 期望 total=%d count=1", sum, want)
	}

	list, err := h.records.ListByDay("")
	if err != nil {
		t.Fatalf("ListByDay(\"\") 失败: %v", err)
	}
	if len(list) != 1 || list[0].DurationSec != want {
		t.Errorf("列表 = %+v, 期望 1 条 %ds", list, want)
	}

	// 状态回到 idle，且今日累计里含刚保存的这一条。
	final, err := h.timer.GetState()
	if err != nil {
		t.Fatalf("GetState() 失败: %v", err)
	}
	if final.State != string(timer.StateIdle) || final.CurrentSec != 0 {
		t.Errorf("结束后的状态 = %+v, 期望 idle/0", final)
	}
	if final.TodayTotalSec != want {
		t.Errorf("结束后 TodayTotalSec = %d, 期望 %d", final.TodayTotalSec, want)
	}

	// 事件：timer:state 与 records:changed 都发过。
	for _, name := range []string{eventTimerState, eventRecordsChange} {
		if h.spy.count(name) == 0 {
			t.Errorf("没有收到 %s 事件", name)
		}
	}
}

func TestService_StartTwiceReturnsError(t *testing.T) {
	h := newHarness(t)

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("首次 Start() 失败: %v", err)
	}
	if _, err := h.timer.Start(); err == nil {
		t.Fatal("第二次 Start() 应报错")
	} else if !strings.Contains(err.Error(), "已有正在进行的计时") {
		t.Errorf("错误信息 = %q, 期望包含中文说明", err.Error())
	}
}

// 时长过短：不落库，但状态必须回到 idle，且界面能看懂原因。
func TestService_StopTooShortReportsReason(t *testing.T) {
	h := newHarness(t)

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(2 * time.Second)

	out, err := h.timer.Stop()
	if err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}
	if out.Saved || out.Reason != "too-short" || out.Session != nil {
		t.Errorf("Stop() = %+v, 期望未保存 + too-short", out)
	}

	sum, err := h.records.TodaySummary()
	if err != nil {
		t.Fatalf("TodaySummary() 失败: %v", err)
	}
	if sum.Count != 0 {
		t.Errorf("记录条数 = %d, 期望 0", sum.Count)
	}
	state, err := h.timer.GetState()
	if err != nil {
		t.Fatalf("GetState() 失败: %v", err)
	}
	if state.State != string(timer.StateIdle) {
		t.Errorf("状态 = %s, 期望 idle", state.State)
	}
}

func TestService_DiscardLeavesNothing(t *testing.T) {
	h := newHarness(t)

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(10 * time.Minute)
	state, err := h.timer.Discard()
	if err != nil {
		t.Fatalf("Discard() 失败: %v", err)
	}
	if state.State != string(timer.StateIdle) {
		t.Errorf("状态 = %s, 期望 idle", state.State)
	}
	sum, err := h.records.TodaySummary()
	if err != nil {
		t.Fatalf("TodaySummary() 失败: %v", err)
	}
	if sum.Count != 0 {
		t.Errorf("放弃后记录条数 = %d, 期望 0", sum.Count)
	}
}

func TestService_DeleteSession(t *testing.T) {
	h := newHarness(t)

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(time.Minute)
	out, err := h.timer.Stop()
	if err != nil {
		t.Fatalf("Stop() 失败: %v", err)
	}

	if err := h.records.DeleteSession(out.Session.ID); err != nil {
		t.Fatalf("DeleteSession() 失败: %v", err)
	}
	sum, err := h.records.TodaySummary()
	if err != nil {
		t.Fatalf("TodaySummary() 失败: %v", err)
	}
	if sum.Count != 0 {
		t.Errorf("删除后记录条数 = %d, 期望 0", sum.Count)
	}

	if err := h.records.DeleteSession(out.Session.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("重复删除 error = %v, 期望 ErrNotFound", err)
	}
	if err := h.records.DeleteSession(0); err == nil {
		t.Error("DeleteSession(0) 应报错")
	}
}

func TestService_ListByDayRejectsBadDate(t *testing.T) {
	h := newHarness(t)

	list, err := h.records.ListByDay("")
	if err != nil {
		t.Fatalf("ListByDay(空) 失败: %v", err)
	}
	if list == nil {
		t.Fatal("ListByDay 返回了 nil 切片，会被序列化成 null")
	}
	if len(list) != 0 {
		t.Errorf("空日列表长度 = %d, 期望 0", len(list))
	}

	if _, err := h.records.ListByDay("2026/05/01"); err == nil {
		t.Error("错误格式的日期应报错")
	}
	if _, err := h.records.ListByDay("2026-5-1"); err == nil {
		t.Error("非零填充的日期应报错")
	}
	if _, err := h.records.ListByDay("2026-05-01"); err != nil {
		t.Errorf("合法日期报错: %v", err)
	}
}

// ---------------------------------------------------------------- 设置

func TestService_SettingsRoundTripAndNormalize(t *testing.T) {
	h := newHarness(t)

	got, err := h.settings.Get()
	if err != nil {
		t.Fatalf("Get() 失败: %v", err)
	}
	if got.BreakIntervalMin != settings.DefaultBreakIntervalMin || got.Theme != settings.ThemeSystem {
		t.Errorf("默认设置 = %+v", got)
	}

	saved, err := h.settings.Save(SettingsDTO{
		BreakReminderEnabled: false,
		BreakIntervalMin:     25,
		MinimizeToTray:       false,
		Theme:                settings.ThemeDark,
	})
	if err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}
	if saved.BreakIntervalMin != 25 || saved.Theme != settings.ThemeDark || saved.MinimizeToTray {
		t.Errorf("Save() 回读 = %+v, 与传入不一致", saved)
	}

	// 落库之后重新读一次（模拟下次启动）。
	reloaded, err := settings.Load(h.b.db)
	if err != nil {
		t.Fatalf("settings.Load() 失败: %v", err)
	}
	if reloaded != settingsFromDTO(saved) {
		t.Errorf("重新载入 = %+v, 期望 %+v", reloaded, settingsFromDTO(saved))
	}

	// 越界值应被拒绝，而不是静默写进去。
	if _, err := h.settings.Save(SettingsDTO{BreakIntervalMin: 0, Theme: settings.ThemeSystem}); err == nil {
		t.Error("间隔为 0 应被拒绝")
	}
	if _, err := h.settings.Save(SettingsDTO{BreakIntervalMin: 45, Theme: "rainbow"}); err == nil {
		t.Error("未知主题应被拒绝")
	}
}

// 改设置必须立刻影响提醒进度：关闭后 intervalSec 归零。
func TestService_SettingsAffectReminder(t *testing.T) {
	h := newHarness(t)

	state, err := h.timer.GetState()
	if err != nil {
		t.Fatalf("GetState() 失败: %v", err)
	}
	if state.BreakIntervalSec != int64(settings.DefaultBreakIntervalMin)*60 {
		t.Errorf("BreakIntervalSec = %d, 期望 %d",
			state.BreakIntervalSec, settings.DefaultBreakIntervalMin*60)
	}

	if _, err := h.settings.Save(SettingsDTO{
		BreakReminderEnabled: false,
		BreakIntervalMin:     15,
		MinimizeToTray:       true,
		Theme:                settings.ThemeSystem,
	}); err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}

	state, err = h.timer.GetState()
	if err != nil {
		t.Fatalf("GetState() 失败: %v", err)
	}
	// 关闭提醒时 Configure 把 intervalSec 设为 0，前端据此隐藏进度条。
	if state.BreakIntervalSec != 0 {
		t.Errorf("关闭提醒后 BreakIntervalSec = %d, 期望 0", state.BreakIntervalSec)
	}
}

// ---------------------------------------------------------------- 心跳与恢复

// 心跳按配置的间隔驱动休息提醒，并且提醒进度与计时锚点必须自洽。
//
// 这里复刻 heartbeat 的循环体但不真的发系统通知：通知服务需要
// application 启动后才可用，那部分只能实跑验证。
func TestService_HeartbeatDrivesBreakReminder(t *testing.T) {
	h := newHarness(t)

	if _, err := h.settings.Save(SettingsDTO{
		BreakReminderEnabled: true,
		BreakIntervalMin:     1,
		MinimizeToTray:       true,
		Theme:                settings.ThemeSystem,
	}); err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}

	// 顺序必须和 backend.heartbeat 一致：先提醒、后结算计时。
	fires := 0
	for i := 0; i < 6; i++ {
		h.clock.Advance(timer.TickInterval)
		if h.b.reminder.Tick(h.b.timer.ElapsedSec()) {
			fires++
		}
		if _, err := h.b.timer.Tick(); err != nil {
			t.Fatalf("第 %d 次 Tick() 失败: %v", i+1, err)
		}
	}

	// 6 拍 = 180s，间隔 60s。第一拍只用来对齐基准，所以在第 3 拍（90s）
	// 和第 5 拍（150s）各触发一次；这是"观察到的学习时长"的量化粒度。
	if fires != 2 {
		t.Errorf("触发次数 = %d, 期望 2", fires)
	}

	// 计时器发出来的那份状态里，提醒进度必须是"这一拍之后"的值。
	// 如果 heartbeat 里先结算计时再推进提醒，事件携带的就是上一拍的值，
	// 前端按它算出来的倒计时会一直偏大约一个心跳周期。
	reminderElapsed, _ := h.b.reminder.Snapshot()
	if got := h.spy.lastState(t).BreakElapsedSec; got != reminderElapsed {
		t.Errorf("事件里的 BreakElapsedSec = %d, 提醒实际进度 = %d（说明事件携带了过期的提醒进度）",
			got, reminderElapsed)
	}

	// 关掉提醒后不应再触发。
	if _, err := h.settings.Save(SettingsDTO{
		BreakReminderEnabled: false,
		BreakIntervalMin:     1,
		MinimizeToTray:       true,
		Theme:                settings.ThemeSystem,
	}); err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}
	for i := 0; i < 4; i++ {
		h.clock.Advance(timer.TickInterval)
		if h.b.reminder.Tick(h.b.timer.ElapsedSec()) {
			t.Fatal("提醒已关闭，不应触发")
		}
		if _, err := h.b.timer.Tick(); err != nil {
			t.Fatalf("Tick() 失败: %v", err)
		}
	}
}

// 心跳把时长结算进状态，并且事件里带上正确的当前时长。
func TestService_HeartbeatSettles(t *testing.T) {
	h := newHarness(t)

	if _, err := h.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	h.clock.Advance(timer.TickInterval)
	if _, err := h.b.timer.Tick(); err != nil {
		t.Fatalf("Tick() 失败: %v", err)
	}

	got := h.spy.lastState(t)
	if got.AccumulatedSec != int64(timer.TickInterval/time.Second) {
		t.Errorf("AccumulatedSec = %d, 期望 %d", got.AccumulatedSec, int64(timer.TickInterval/time.Second))
	}
	if got.State != string(timer.StateRunning) {
		t.Errorf("State = %s, 期望 running", got.State)
	}
}

// 重启后自动结算：记录进库，事件通知界面刷新。
func TestService_RecoveryAutoStopEmitsRecordsChanged(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open() 失败: %v", err)
	}
	defer func() { _ = db.Close() }()

	clk := newClock(t0)

	// 第一次运行：开始计时并跑一次心跳，然后"崩溃"。
	first, err := newBackend(db, clk.Now)
	if err != nil {
		t.Fatalf("newBackend() 失败: %v", err)
	}
	if _, err := first.timer.Start(); err != nil {
		t.Fatalf("Start() 失败: %v", err)
	}
	clk.Advance(timer.TickInterval)
	if _, err := first.timer.Tick(); err != nil {
		t.Fatalf("Tick() 失败: %v", err)
	}

	// 过夜后重启。
	clk.Advance(10 * time.Hour)

	second, err := newBackend(db, clk.Now)
	if err != nil {
		t.Fatalf("newBackend() 失败: %v", err)
	}
	spy := &eventSpy{}
	second.emitHook = spy.record
	second.timer.OnStateChange(func(snap timer.Snapshot) { second.emitState() })

	out, err := second.timer.Recover()
	if err != nil {
		t.Fatalf("Recover() 失败: %v", err)
	}
	if out.Action != timer.RecoveryAutoStopped || !out.Saved {
		t.Fatalf("Recover() = %+v, 期望自动结算并保存", out)
	}
	second.emitRecordsChanged("recovered")
	second.emitState()

	if spy.count(eventRecordsChange) == 0 {
		t.Error("恢复结算后应发出 records:changed")
	}
	sum, err := second.db.SummaryByDay(t0.Format(layoutDateKey))
	if err != nil {
		t.Fatalf("SummaryByDay() 失败: %v", err)
	}
	if sum.Count != 1 || sum.TotalSec != int64(timer.TickInterval/time.Second) {
		t.Errorf("汇总 = %+v, 期望 1 条 %ds", sum, int64(timer.TickInterval/time.Second))
	}
}

// 通知服务没启动成功时必须安静降级，而不是抛错或 panic。
//
// 这里刻意不调用真正的 ServiceStartup：它会往 HKCU\Software\Classes 写注册表，
// 不该在单元测试里发生。测的是"未就绪"这个状态下所有入口的行为。
func TestNotifier_DegradesWhenNotReady(t *testing.T) {
	n := newNotifierService()

	if n.Available() {
		t.Error("未启动时 Available() 应为 false")
	}
	if n.ServiceName() == "" {
		t.Error("ServiceName() 不应为空")
	}
	if n.send(notifications.NotificationOptions{ID: "x", Title: "t", Body: "b"}) {
		t.Error("未就绪时 send 应返回 false")
	}
	if err := n.ServiceShutdown(); err != nil {
		t.Errorf("未就绪时 ServiceShutdown() 应返回 nil, 实际 %v", err)
	}
}

func TestFormatDurationCN(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 分钟"},
		{-5, "0 分钟"},
		{30, "30 秒"},
		{60, "1 分钟"},
		{3599, "59 分钟"},
		{3600, "1 小时"},
		{3660, "1 小时 1 分"},
		{4980, "1 小时 23 分"},
	}
	for _, c := range cases {
		if got := formatDurationCN(c.in); got != c.want {
			t.Errorf("formatDurationCN(%d) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

func TestSettingsFromDTORoundTrip(t *testing.T) {
	in := SettingsDTO{
		BreakReminderEnabled: true,
		BreakIntervalMin:     50,
		MinimizeToTray:       false,
		Theme:                settings.ThemeLight,
	}
	if got := settingsDTOFrom(settingsFromDTO(in)); got != in {
		t.Errorf("往返 = %+v, 期望 %+v", got, in)
	}
}
