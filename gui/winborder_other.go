//go:build !windows

package main

// 非 Windows 平台没有 DWM 边框可关,这里是空实现,好让调用点不用写
// 平台判断。
func hideSystemBorder(hwnd uintptr) {}
