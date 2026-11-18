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

// proseLineHeight 是成段说明文字的行高倍数。
//
// 12px 的默认行高来自字体自身的度量(实测三行 48px,合每行 16px = 1.33 倍),
// 那是"单行标签"的排法:连续几行中文贴得太紧,读起来像一块色块。1.5 倍
// (每行 18px)是中文正文的常见起点,行与行之间看得出呼吸。
const proseLineHeight float32 = 1.5

// proseMaxWidth 是一段说明文字最宽能到多少 DIP。
//
// 一行太长,眼睛换行时会跳行:中文正文的舒适区大约 30–40 个字,12px 下
// 就是 360–480 DIP。弹窗自己是有宽度的,所以今天实测最宽的一段是 496
// DIP(帮助页,弹窗 520 减内边距)——刚好在边上。这条约束写下来的意义
// 不是改今天的观感,而是让"以后把弹窗加宽到 880"不会顺手把正文拉成
// 一行 70 个字。
const proseMaxWidth float32 = 480

// prose 是一段给人读的说明文字:12px、次要色、行高 1.5。
//
// 帮助页、设置面板里的解释都是一段段中文,原来和单行标签共用默认行高。
// 收成一个入口,是为了让"成段正文"这件事有唯一的写法——否则下一个人加
// 一段说明时,还是会顺手写 ui.Text,行高又回到 1.33。
func prose(c *ui.Context, s string) ui.Element {
	return proseIn(c, s, c.Theme().TextMuted)
}

// proseIn 是 prose 的换色版本(警示、强调)。行高与最大宽度仍由这里统一。
func proseIn(c *ui.Context, s string, col ui.Color) ui.Element {
	return ui.Text(c, s).FontSize(12).TextColor(col).
		LineHeight(proseLineHeight).MaxWidth(proseMaxWidth)
}
