package main

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// notifierService 给 Wails 的通知服务加一层"失败不致命"的保护。
//
// Wails 的 notifications.NotificationService 在 ServiceStartup 里往
// HKCU\Software\Classes 写注册表来完成 Windows 通知注册（AppUserModelId +
// COM 激活器）。按 Wails 的约定，服务启动返回非 nil 错误会**中止整个启动流程**，
// app.Run 直接把错误抛出来。
//
// "系统通知注册不上"是个环境问题（受限账户、损坏的用户配置），它不该让一个
// 学习计时器完全打不开。所以这里吞掉错误、退化成只用应用内提醒，并留下日志。
type notifierService struct {
	inner *notifications.NotificationService
	ready atomic.Bool
}

func newNotifierService() *notifierService {
	return &notifierService{inner: notifications.New()}
}

// ServiceStartup 实现 application.ServiceStartup。
func (n *notifierService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	if err := n.inner.ServiceStartup(ctx, options); err != nil {
		slog.Warn("系统通知注册失败，本次运行只使用应用内提醒", "err", err)
		return nil
	}
	n.ready.Store(true)
	slog.Info("系统通知已就绪")
	return nil
}

// ServiceShutdown 实现 application.ServiceShutdown。
func (n *notifierService) ServiceShutdown() error {
	if !n.ready.Load() {
		// 启动没成功就不要再走关闭流程，避免在未初始化的实现上操作。
		return nil
	}
	return n.inner.ServiceShutdown()
}

// ServiceName 让 Wails 的日志用这个名字而不是类型名。
func (n *notifierService) ServiceName() string { return "GoXueTimerNotifications" }

// Available 报告系统通知是否可用，界面据此决定要不要提示用户。
func (n *notifierService) Available() bool { return n.ready.Load() }

// send 发一条系统通知，返回是否真的发出去了。
// 刻意不导出：导出的话会把 notifications.NotificationOptions 拖进前端绑定。
func (n *notifierService) send(opts notifications.NotificationOptions) bool {
	if !n.ready.Load() {
		return false
	}
	if err := n.inner.SendNotification(opts); err != nil {
		slog.Warn("发送系统通知失败", "err", err)
		return false
	}
	return true
}
