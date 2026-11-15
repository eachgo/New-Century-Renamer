package main

import "github.com/egoist/mygo/ui"

// dialogPanel 是三个弹窗共用的面板样式。
//
// 以前三处各写一遍:帮助弹窗用 t.Background(暗色下是 #031209,近纯黑),
// 设置用 t.Surface 并带投影,导入确认用 t.Surface 但没投影。同一个应用里
// 三个"浮在内容上面的面板"三种底色、两种有无投影,看起来像三个人各做了
// 一个。这里收成一处,改样式只改这里。
//
// 底色选 t.Surface 而不是 t.Background:t.Background 是"窗口最底层的桌面",
// 弹窗是"浮在内容之上的一张纸",它该和卡片同级(卡片的底色也是 Surface 系),
// 而不是比内容还深——暗色下 t.Background 近纯黑,弹窗会像挖了一个洞。
//
// 投影是"浮起"的唯一线索(底色和卡片太接近,不靠投影分不出层次),所以三个
// 都必须有,且同一个偏移与模糊。
func dialogPanel(c *ui.Context, dw, dh float32) ui.Element {
	t := c.Theme()
	return ui.Column(c).Width(dw).MaxHeight(dh).Gap(12).Padding(20).Radius(14).
		Background(t.Surface).
		Shadow(0, 12, 32, 0, ui.RGBA(0x10, 0x30, 0x1F, 0.10))
}
