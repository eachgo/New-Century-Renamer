//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// Windows 11 的 DWM 会给每个窗口画一圈自己的边框色,聚焦时用系统强调色。
// 我们自绘了整个界面(包括标题栏),这圈线就成了唯一一块"不是我们画的"像素:
// 系统主题是亮色、软件手动选暗色时,它被画成一道亮灰/白线,看上去像渲染 bug。
//
// mygo 自己只处理了标题栏(internal/windows/window.go 的 plainCaption 把
// DWMWA_CAPTION_COLOR 置成 DWMWA_COLOR_NONE),边框色没管;那边也有
// dwmwaBorderColor 常量,但只用在菜单窗口上。Windows 10 上 mygo 是把客户区
// 外扩 1px 盖住那条线(frameBorder),Windows 11 的圆角边框则"left as it is"
// ——所以这里在应用侧补一次:把 DWMWA_BORDER_COLOR 置成 DWMWA_COLOR_NONE,
// 让 DWM 一圈都不画,完全交给我们自绘的内容。
//
// 这个属性是 Windows 11(build 22000)才有的。老系统上调用会返回
// E_INVALIDARG,那时本来也没有这圈线,失败就静默放弃。
const (
	dwmwaBorderColor = 34
	dwmColorNone     = 0xFFFFFFFE // DWMWA_COLOR_NONE:不画,而不是"画成强调色"
)

var (
	dwmapi         = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetAttr = dwmapi.NewProc("DwmSetWindowAttribute")
)

// hideSystemBorder 让 DWM 不再给主窗口画边框色。
//
// hwnd 为 0(窗口还没建好)时直接返回。失败不影响任何别的东西:这只是
// 一圈装饰线,画不出来最多是回到"系统替我们画"。
func hideSystemBorder(hwnd uintptr) {
	if hwnd == 0 || procDwmSetAttr.Find() != nil {
		return
	}
	none := uint32(dwmColorNone)
	// 返回值是 HRESULT。不支持这个属性时是非零(Win10 上是 E_INVALIDARG),
	// 这里不需要区分原因,也不该为此报错打扰用户。
	_, _, _ = procDwmSetAttr.Call(hwnd, dwmwaBorderColor, uintptr(unsafe.Pointer(&none)), 4)
}
