package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// 帮助页的流程图。
//
// 为什么画图而不是再写一段话:用户要建立的是"这是个流水线"的心智模型——
// 文件从一头进,步骤在中间排,另一头出结果,改坏了还能退回来接着改。
// 这个模型是空间性的,用文字描述它,读者得在脑子里搭一遍;画出来就直接
// 看见了。而且"顺序重要吗""改坏了怎么办"这两个最常被问的问题,在图上
// 分别由"两条起手"和"虚线回环"直接回答了,不用再解释。
//
// 用 mygo 的 Element.Draw + Painter 手画,没有引第三方图:流程图是本应用
// 自己的概念,而标签要跟着主题色、要在深色下也读得清。画出来的字不在
// 元素树里,所以每个图都带一句 Label 作为文字替代(读屏软件与无头测试
// 都靠它)。
//
// 每条泳道末尾的"撤回"用虚线回环指回第一个"调整步骤":那是"再来一轮",
// 不是下一步,所以线型也要不一样。

const (
	flowBoxH     = float32(28) // 单行标签的方框高
	flowBoxHTall = float32(38) // 有标签要折成两行时的方框高
	flowArrowW   = float32(12)
	flowLaneGap  = float32(46) // 相邻两条泳道的基线距离(含给回环线留的空)
	flowMaxBoxW  = float32(96)
	flowMinBoxW  = float32(38)
	// flowTail 是最后一条泳道底下的余量:回环线画在方框下方,不留这一块
	// 会被裁掉半条。
	flowTail = float32(16)
	// flowFoldMark 是"这个标签可以折成两行"的分隔符。
	flowFoldMark = "/"
)

// boxHeight 返回这批泳道该用的方框高。
//
// 只要有一个标签带折行标记,整张图的方框就一起加高——高度在布局期就得
// 定下来(元素要占位),而那时还不知道实际宽度。按"可能需要两行"取高,
// 顶多多占十 DIP;按单行取,则窄窗下第二行一定溢出下边框。
func boxHeight(lanes [][]string) float32 {
	for _, lane := range lanes {
		for _, label := range lane {
			if strings.Contains(label, flowFoldMark) {
				return flowBoxHTall
			}
		}
	}
	return flowBoxH
}

// diagramHeight 是图占的高度。测试也用它,免得两处各算一遍。
func diagramHeight(lanes [][]string) float32 {
	bh := boxHeight(lanes)
	return float32(len(lanes))*bh +
		float32(len(lanes)-1)*(flowLaneGap-bh) + flowTail
}

// flowDiagram 把 lanes 画成若干条横向泳道,每条是一个方框序列。
//
// 宽度铺满:它只在帮助弹窗里出现,而弹窗宽度跟着窗口变,所以方框宽度由
// "最长的那条泳道"反推,每条泳道各自居中。
func flowDiagram(c *ui.Context, lanes [][]string, label string) ui.Element {
	t := c.Theme()
	return ui.Box(c).FillWidth().Height(diagramHeight(lanes)).Label(label).
		Draw(func(p *ui.Painter, r ui.Rect) {
			drawLanes(p, r, t, lanes)
		})
}

func drawLanes(p *ui.Painter, r ui.Rect, t *ui.Theme, lanes [][]string) {
	nMax := 0
	for _, l := range lanes {
		if len(l) > nMax {
			nMax = len(l)
		}
	}
	if nMax == 0 {
		return
	}
	bh := boxHeight(lanes)
	bw := (r.W - float32(nMax-1)*flowArrowW) / float32(nMax)
	if bw > flowMaxBoxW {
		bw = flowMaxBoxW
	}
	if bw < flowMinBoxW {
		bw = flowMinBoxW
	}

	// 字号与折行要一起定:先挑最大的字号,让每个标签要么整串放得下、
	// 要么(带折行标记时)按最长那一半放得下。放不放得进必须用
	// MeasureText 真量——中英文混排的宽度差得很远,按字数估会错。
	foldWidth := func(label string, size float32) (float32, bool) {
		head, tail, ok := strings.Cut(label, flowFoldMark)
		if !ok {
			w, _ := p.MeasureText(0, ui.Span{Text: label, Size: size})
			return w, false
		}
		hw, _ := p.MeasureText(0, ui.Span{Text: head, Size: size})
		tw, _ := p.MeasureText(0, ui.Span{Text: tail, Size: size})
		if tw > hw {
			hw = tw
		}
		return hw, true
	}
	size := float32(10)
	for _, cand := range []float32{12, 11, 10} {
		all := true
		for _, lane := range lanes {
			for _, label := range lane {
				pad := float32(8)
				if strings.Contains(label, flowFoldMark) {
					pad = 6 // 折行的标签左右留窄一点,否则最长那一半也放不进
				}
				if w, _ := foldWidth(label, cand); w > bw-pad {
					all = false
				}
			}
		}
		size = cand
		if all {
			break
		}
	}

	// 起点用主色淡底:两条泳道的"入口"在图上要一眼看得出是入口,而不是
	// 随便一格。终点"执行更名"用实心主色——那是唯一真的动磁盘的一步。
	startFill := t.Accent.Alpha(0.12)
	boxFill := t.Surface
	boxStroke := t.Border
	arrowCol := t.TextMuted

	box := func(x, y, w float32, label string, fill, stroke, fg ui.Color) {
		rect := ui.Rect{X: x, Y: y, W: w, H: bh}
		p.Fill(rect, fill, 8)
		p.Stroke(rect, stroke, 8, 1)
		head, tail, ok := strings.Cut(label, flowFoldMark)
		if !ok {
			sw, sh := p.MeasureText(0, ui.Span{Text: label, Size: size})
			p.Text(x+(w-sw)/2, y+(bh-sh)/2, label, size, fg)
			return
		}
		// 两行的字号再单独算一次:方框虽然加高了,但两行 12 号字加上
		// 行距仍可能顶出去,所以逐档往下试到装得下为止。
		ts := size
		for ; ts > 8; ts-- {
			_, hh := p.MeasureText(0, ui.Span{Text: head, Size: ts})
			_, th := p.MeasureText(0, ui.Span{Text: tail, Size: ts})
			if hh+th <= bh-6 {
				break
			}
		}
		hw, hh := p.MeasureText(0, ui.Span{Text: head, Size: ts})
		tw, th := p.MeasureText(0, ui.Span{Text: tail, Size: ts})
		top := y + (bh-hh-th)/2
		p.Text(x+(w-hw)/2, top, head, ts, fg)
		p.Text(x+(w-tw)/2, top+hh, tail, ts, fg)
	}

	for li, lane := range lanes {
		y := r.Y + 2 + float32(li)*flowLaneGap
		width := float32(len(lane))*bw + float32(len(lane)-1)*flowArrowW
		x0 := r.X + (r.W-width)/2
		cy := y + bh/2

		adjCx, undoCx := float32(-1), float32(-1)
		for i, label := range lane {
			x := x0 + float32(i)*(bw+flowArrowW)
			fill, stroke, fg := boxFill, boxStroke, t.Text
			switch {
			case i == 0:
				fill, stroke, fg = startFill, t.Accent.Alpha(0.55), t.Accent
			case label == "执行更名":
				fill, stroke, fg = t.Accent, t.Accent, t.AccentText
			case label == "撤回":
				// 撤回是"安全出口",不是又一步操作:底色留白,只用主色描边,
				// 免得和实心的"执行更名"抢同一种重量。
				stroke, fg = t.Accent, t.Accent
			}
			box(x, y, bw, label, fill, stroke, fg)
			if i > 0 {
				arrowRight(p, x-flowArrowW, cy, x, arrowCol)
			}
			switch label {
			case "调整步骤":
				// 记第一个"调整步骤",不是最后一个。回环要指回"你动手调的
				// 那一步",指回紧挨着的那个盒子等于画了一段十二像素的短线,
				// 看着像箭头画歪了,读不出"再来一轮"。
				if adjCx < 0 {
					adjCx = x + bw/2
				}
			case "撤回":
				undoCx = x + bw/2
			}
		}

		// 回环:撤回 → 调整步骤。虚线,因为它是"再来一轮"而不是下一步。
		if undoCx > 0 && adjCx > 0 && undoCx > adjCx {
			const head = float32(5)
			retY := y + bh + 9
			loop := (&ui.Path{}).
				MoveTo(undoCx, y+bh).
				LineTo(undoCx, retY).
				LineTo(adjCx, retY).
				LineTo(adjCx, y+bh+head)
			loop.Dash(0, 3, 3)
			p.StrokePath(loop, 1.2, arrowCol)
			p.FillPath((&ui.Path{}).
				MoveTo(adjCx, y+bh).
				LineTo(adjCx-3.2, y+bh+head).
				LineTo(adjCx+3.2, y+bh+head).
				Close(), arrowCol)
		}
	}
}

// arrowRight 画一段水平箭头:线 + 三角头。头用填充路径而不是描边,
// 因为描边的三角在 1 像素宽时只剩一个点。
func arrowRight(p *ui.Painter, x0, y, x1 float32, c ui.Color) {
	if x1 <= x0 {
		return
	}
	const head = float32(5)
	p.Line(x0, y, x1-head, y, 1.2, c)
	tip := (&ui.Path{}).
		MoveTo(x1, y).
		LineTo(x1-head, y-3.2).
		LineTo(x1-head, y+3.2).
		Close()
	p.FillPath(tip, c)
}
