package main

import "github.com/egoist/mygo/ui"

// splitPanes 把一栏分成左右两半，中间是一条**可拖拽的空白缝**，不画线。
//
// 为什么不用 ui.Split：mygo 的分隔条在 split.go 里把颜色写死成
// t.Border（悬停时 t.Accent），没有参数可传。而 t.Border 同时是输入框、
// 下拉框、普通按钮的边框色——为了不让输入框的边看不见，Border 必须
// 足够深；深了以后那两条 1 DIP 的分隔线就成了整屏最扎眼的线，比它们
// 分隔的内容还显眼。用户的原话是"出现的细线分割不对"。
//
// 这里改成留缝：三栏本来就是浮在 base 底色上的圆角卡片（见 view() 的
// 注释），中间露出 base 本身就完成了分隔，还能顺便呼吸。缝的宽度与
// 卡片四周的内边距同一个值，所以三栏的节奏是统一的。
//
// 内边距只在最外层加一次：嵌套调用（中/右栏那一层）传 pad=0，否则
// 第二层又缩进 8px，中/右栏的卡片顶边会比左栏低 8px——三栏就不再
// 站在同一条水平线上了。
func (a *app) splitPanes(c *ui.Context, size *float32, first, second func()) {
	a.splitPanesInset(c, size, 8, 0, first, second)
}

// splitPanesInset 里的 availW 是**这一层真正能用的宽度**。嵌套调用必须
// 由调用方算好传进来：c.Size() 返回的始终是整个窗口的尺寸，不是父级
// 那一栏的宽度。以前中/右栏那一层也拿窗口宽度做夹紧，于是把 splitM
// 一路拖到 330 时右栏只剩几十 DIP，卡片里的名字被压成一列竖排——
// 夹紧看上去生效了，其实上限比实际可用宽度大了两倍多。
// availW <= 0 表示"就用窗口宽度"（最外层）。
func (a *app) splitPanesInset(c *ui.Context, size *float32, pad, availW float32, first, second func()) {
	t := c.Theme()
	w, _ := c.Size()
	// 缝宽与内边距同值：拖拽热区只有 1 DIP 时几乎点不中，这里给足
	// 10 DIP，视觉上仍是"一条空缝"。
	gripW := float32(10)
	minPane := t.Space(32) // 再窄就放不下工具栏，宁可不让拖
	avail := w - 2*pad - gripW
	if availW > 0 {
		avail = availW - gripW
	}
	if maxSize := avail - minPane; maxSize > minPane {
		if *size < minPane {
			*size = minPane
		}
		if *size > maxSize {
			*size = maxSize
		}
	} else if *size < 0 {
		*size = 0
	}

	ui.Row(c).Grow(1).FillWidth().Padding(pad, pad, pad, pad).Children(func() {
		ui.Column(c).Width(*size).FillHeight().Clip().Children(first)

		// 缝本身是拖拽把手：整条竖带都能抓，光标提示可拖。
		grip := ui.Box(c).Width(gripW).FillHeight().Focusable().Cursor(ui.CursorResizeEW)
		if dx, _, held := grip.Dragged(); held && dx != 0 {
			*size = clampPane(*size+dx, minPane, avail-minPane)
		}
		// 键盘：缝拿到焦点后左右键推，和拖动等价（可访问性底线）。
		if grip.Focused() {
			if c.Shortcut(0, ui.KeyLeft) {
				*size = clampPane(*size-t.Space(2.5), minPane, avail-minPane)
			}
			if c.Shortcut(0, ui.KeyRight) {
				*size = clampPane(*size+t.Space(2.5), minPane, avail-minPane)
			}
		}

		ui.Column(c).Grow(1).FillHeight().Clip().Children(second)
	})
}

// clampPane 把分隔位置夹在 [lo, hi] 内。hi 小于 lo 时（窗口被拖得
// 极窄）返回 lo，让第一栏占满，第二栏自然被挤出去而不是画成负数宽度。
func clampPane(v, lo, hi float32) float32 {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
