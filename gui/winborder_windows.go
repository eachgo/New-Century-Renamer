//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/egoist/mygo/ui"
)

// Windows 11 的 DWM 会给每个窗口画一圈自己的边框色:聚焦时用系统强调色,
// 失焦时是灰/白。我们自绘了整个界面(包括标题栏),这圈线就成了唯一一块
// "不是我们画的"像素。用户把桌面设成纯黑之后,这道 1px 的白线极其显眼
// (实测:窗口最外 1px 恒为 (255,255,255),左/右/下三边都有,顶部没有)。
//
// mygo 自己只处理了标题栏(internal/windows/window.go 的 plainCaption 把
// DWMWA_CAPTION_COLOR 置成 DWMWA_COLOR_NONE),边框色没管;那边也有
// dwmwaBorderColor 常量,但只用在菜单窗口上。Windows 10 上 mygo 是把客户区
// 外扩 1px 盖住那条线(frameBorder),Windows 11 的圆角边框则"left as it is"。
//
// 这里在应用侧补一次,但**不用 DWMWA_COLOR_NONE**:那个值在部分 Win11 build
// 上不生效,而且 mygo 每次主题变化都会重设 DWMWA_USE_IMMERSIVE_DARK_MODE,
// 这会把边框色一起打回默认。改成设成"我们自己的底色":DWM 爱画就画,画出来
// 和旁边的内容一模一样,等于没有。颜色随明暗/主题变,所以由调用侧在主题、
// 显示、焦点变化时重新调。
//
// 这个属性是 Windows 11(build 22000)才有的。老系统上调用会返回
// E_INVALIDARG,那时本来也没有这圈线(mygo 用 frameBorder 盖住了),失败静默。
const dwmwaBorderColor = 34

var (
	dwmapi         = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetAttr = dwmapi.NewProc("DwmSetWindowAttribute")
)

// setWindowBorder 让 DWM 的窗口边框色与内容底色一致,从而看不见。
//
// hwnd 为 0(窗口还没建好)时直接返回。失败不影响任何别的东西:这只是
// 一圈装饰线,画不出来最多是回到"系统替我们画"。
func setWindowBorderPlatform(hwnd uintptr, col ui.Color) {
	if hwnd == 0 || procDwmSetAttr.Find() != nil {
		return
	}
	// COLORREF 是 0x00BBGGRR:和 ui.Color 的 RGB 顺序相反。
	ref := uint32(col.R) | uint32(col.G)<<8 | uint32(col.B)<<16
	// 返回值是 HRESULT。不支持这个属性时是非零(Win10 上是 E_INVALIDARG),
	// 这里不需要区分原因,也不该为此报错打扰用户。
	_, _, _ = procDwmSetAttr.Call(hwnd, dwmwaBorderColor, uintptr(unsafe.Pointer(&ref)), 4)
}
