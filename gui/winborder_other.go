//go:build !windows

package main

import "github.com/egoist/mygo/ui"

// setWindowBorderPlatform 在非 Windows 平台上什么也不做:那里没有 DWM 的窗口边框色,
// 窗口四周由合成器按自己的规则画。
func setWindowBorderPlatform(hwnd uintptr, col ui.Color) {}
