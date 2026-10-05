//go:build !windows

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// syncCaptionTheme 只在 Windows 上有意义：标题栏主题错位是 Win10 的
// blur-behind 与窗口主题叠加出来的问题。其它平台留空实现，保证能编译。
func syncCaptionTheme(_ uintptr, _ string) {}

// windowHandle 在非 Windows 平台上没有对应的原生句柄概念。
func windowHandle(_ *application.WebviewWindow) uintptr { return 0 }
