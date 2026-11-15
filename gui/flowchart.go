package main

import (
	"github.com/egoist/mygo/ui"
)

// 帮助页的流程图。
//
// 为什么画图而不是再写一段话:五步讲的是"操作顺序",而用户真正要建立的
// 是"这是个流水线"的心智模型——文件从一头进,步骤在中间排,另一头出结果。
// 这个模型是空间性的,用文字描述它,读者得在脑子里搭一遍;画出来就直接
// 看见了。原来那句"一句话说完"是这么写的,写完还是有人问"顺序重要吗",
// 因为答案("两条路都通")藏在第三句里。
//
// 用 mygo 的 Element.Draw + Painter 手画,没有引第三方图:流程图是本应用
// 自己的概念(两条入口、一个汇合点),SVG 里画死等于把中文标签也写死,
// 而标签要跟着主题色、要在深色下也读得清。
//
// 两条泳道各自从自己的起点出发,到"预览"之前汇合——汇合点是这句话的
// 全部重点:前两步谁先谁后都行,从预览开始两边就是同一条路了。

const (
	flowBoxH   = float32(30)
	flowBoxW   = float32(112)
	flowArrowW = float32(14)
	flowElbowW = float32(18)
	flowMid1W  = float32(58)
	flowMid2W  = float32(70)
)

// flowDiagram 画那两条泳道。高度固定,宽度铺满:它只在帮助弹窗里出现,
// 弹窗最窄也有四百多 DIP,够放下这五列。
func flowDiagram(c *ui.Context) ui.Element {
	t := c.Theme()
	// 手画的东西对辅助技术是隐形的:一个像素都没有的节点,读屏软件只看到
	// 一个空盒子。所以给它一句文字替代,把图里的信息说出来——这也是
	// 无头测试唯一能确认"图在这儿"的方式(图里的字是 Painter 画的,
	// 不在元素树里)。
	return ui.Box(c).FillWidth().Height(100).
		Label("流程图：拖入文件与排步骤两条入口，谁先谁后都行，到预览汇合，再执行更名").
		Draw(func(p *ui.Painter, r ui.Rect) {
			drawFlow(p, r, t)
		})
}

func drawFlow(p *ui.Painter, r ui.Rect, t *ui.Theme) {
	laneA := r.Y + 2
	laneB := r.Y + 2 + flowBoxH + 32
	midY := r.Y + 2 + flowBoxH + 16

	// 整体居中并铺开:弹窗宽度会变(窄窗时 dw 跟着窗口缩),固定宽度
	// 会让图要么挤在左边、要么溢出右边。先算需要的总宽,再按可用宽度
	// 决定方框能不能加宽。
	bw := flowBoxW
	need := 2*bw + 2*flowArrowW + flowElbowW + 8 + flowMid1W + flowArrowW + flowMid2W
	if r.W > need+40 {
		// 有余量就加宽前两个方框,把空白分掉,而不是全堆在右边。
		bw += (r.W - need - 40) / 2
		if bw > flowBoxW*1.5 {
			bw = flowBoxW * 1.5
		}
		need = 2*bw + 2*flowArrowW + flowElbowW + 8 + flowMid1W + flowArrowW + flowMid2W
	}
	x1 := r.X + (r.W-need)/2
	if x1 < r.X {
		x1 = r.X
	}
	x2 := x1 + bw + flowArrowW
	xEnd := x2 + bw + flowArrowW
	xj := xEnd + flowElbowW
	mid1x := xj + 8
	mid2x := mid1x + flowMid1W + flowArrowW

	// 起点用主色淡底:两条泳道的"入口"在图上要一眼看得出是入口,
	// 而不是随便一格。
	startFill := t.Accent.Alpha(0.12)
	boxFill := t.Surface
	boxStroke := t.Border
	arrowCol := t.TextMuted

	box := func(x, y, w float32, label string, fill, stroke, fg ui.Color) {
		p.Fill(ui.Rect{X: x, Y: y, W: w, H: flowBoxH}, fill, 8)
		p.Stroke(ui.Rect{X: x, Y: y, W: w, H: flowBoxH}, stroke, 8, 1)
		sw, sh := p.MeasureText(0, ui.Span{Text: label, Size: 12})
		p.Text(x+(w-sw)/2, y+(flowBoxH-sh)/2, label, 12, fg)
	}

	// 泳道 A:先放文件
	box(x1, laneA, bw, "拖入文件", startFill, t.Accent.Alpha(0.55), t.Accent)
	arrowRight(p, x1+bw, laneA+flowBoxH/2, x2, arrowCol)
	box(x2, laneA, bw, "排步骤", boxFill, boxStroke, t.Text)

	// 泳道 B:先排步骤
	box(x1, laneB, bw, "排步骤", startFill, t.Accent.Alpha(0.55), t.Accent)
	arrowRight(p, x1+bw, laneB+flowBoxH/2, x2, arrowCol)
	box(x2, laneB, bw, "拖入文件", boxFill, boxStroke, t.Text)

	// 汇合:两条泳道的出口拐到中线上,再一起进"预览"。
	for _, y := range []float32{laneA + flowBoxH/2, laneB + flowBoxH/2} {
		elbow := (&ui.Path{}).MoveTo(xEnd, y).LineTo(xj, y).LineTo(xj, midY+flowBoxH/2)
		p.StrokePath(elbow, 1.2, arrowCol)
	}
	arrowRight(p, xj, midY+flowBoxH/2, mid1x, arrowCol)

	box(mid1x, midY, flowMid1W, "预览", boxFill, boxStroke, t.Text)
	arrowRight(p, mid1x+flowMid1W, midY+flowBoxH/2, mid2x, arrowCol)
	box(mid2x, midY, flowMid2W, "执行更名", t.Accent, t.Accent, t.AccentText)
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
