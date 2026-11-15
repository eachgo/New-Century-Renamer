package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func helpApp(t *testing.T) *app {
	t.Helper()
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	a.openHelp()
	return a
}

// diagramRegion 返回主流程图在图像里的像素范围。
//
// 图是 Painter 画的,元素树里只有一个 Box,所以定位靠它上面那行小标题,
// 范围靠 flowDiagram 自己算出来的高度(与画图用同一份公式,不另抄一遍)。
func diagramRegion(t *testing.T, tt *ui.Tester, logicalW float32, title string, lanes int) (x0, x1, y0, y1 int) {
	t.Helper()
	r, ok := tt.Find(title)
	if !ok {
		t.Fatalf("找不到图的标题 %q", title)
	}
	img := tt.Image()
	scale := float64(img.Bounds().Dx()) / float64(logicalW)
	h := float32(lanes)*flowBoxH + float32(lanes-1)*(flowLaneGap-flowBoxH) + flowTail
	x0 = int(float64(r.X) * scale)
	x1 = int(float64(r.X+r.W) * scale)
	y0 = int(float64(r.Y+18) * scale)
	y1 = int(float64(r.Y+18+h) * scale)
	if y1 > img.Bounds().Dy() {
		y1 = img.Bounds().Dy()
	}
	return
}

// 图必须真的被画出来——不是"元素存在"就算数。
//
// Painter 画的字不在元素树里,tt.HasText 找不到它们,所以只能量像素:
// 取图那一带的区域,数出不同颜色的数量。只画一个空盒子的话,那里是
// 一整片纯色(弹窗底色),数出来 1。
func TestFlowDiagramDrawsSomething(t *testing.T) {
	a := helpApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	if !tt.HasText("流程图：") {
		t.Fatal("帮助页里没有流程图")
	}
	img := tt.Image()
	x0, x1, y0, y1 := diagramRegion(t, tt, 1280, "两条起手，最后是同一条路", 2)

	colors := map[uint32]bool{}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			colors[uint32(r>>8)<<16|uint32(g>>8)<<8|uint32(b>>8)] = true
		}
	}
	if len(colors) < 8 {
		t.Fatalf("图那一带只有 %d 种颜色,基本是空白——图没画出来", len(colors))
	}
}

// 两条泳道必须是"上下两条",不是一条直线。
//
// 做法:把图那一带按行切开,统计每行有多少非底色像素。两条泳道会给出
// 两段"有内容"的行,中间夹着一段几乎全空的行(泳道间距)。只画一条的话,
// 中间那段空行不存在。
func TestFlowDiagramHasTwoLanes(t *testing.T) {
	a := helpApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	img := tt.Image()
	x0, x1, y0, y1 := diagramRegion(t, tt, 1280, "两条起手，最后是同一条路", 2)

	at := func(x, y int) uint32 {
		r, g, b, _ := img.At(x, y).RGBA()
		return uint32(r>>8)<<16 | uint32(g>>8)<<8 | uint32(b>>8)
	}
	// 底色取四角出现最多的颜色:四角一定落在方框之外。
	counts := map[uint32]int{}
	for _, p := range [][2]int{{x0 + 1, y0 + 1}, {x1 - 2, y0 + 1}, {x0 + 1, y1 - 2}, {x1 - 2, y1 - 2}} {
		counts[at(p[0], p[1])]++
	}
	var bg uint32
	best := 0
	for c, n := range counts {
		if n > best {
			bg, best = c, n
		}
	}

	// 每行的"墨迹量"。宽 400 像素的一行里,方框边框 + 文字至少几十个
	// 非底色像素;纯泳道间距是 0。
	ink := make([]int, 0, y1-y0)
	for y := y0; y < y1; y++ {
		n := 0
		for x := x0; x < x1; x++ {
			if at(x, y) != bg {
				n++
			}
		}
		ink = append(ink, n)
	}

	// 压成"有内容 / 空白"的段,空白段至少 6 像素才算真的分开
	// (方框圆角之间会有几个像素的空隙,那不是泳道间距)。
	var segs []struct {
		ink   bool
		lines int
	}
	for _, n := range ink {
		dirty := n > 12
		if len(segs) > 0 && segs[len(segs)-1].ink == dirty {
			segs[len(segs)-1].lines++
		} else {
			segs = append(segs, struct {
				ink   bool
				lines int
			}{dirty, 1})
		}
	}
	var lanes, gaps []int
	for i, s := range segs {
		if s.ink && s.lines >= 8 {
			lanes = append(lanes, i)
		}
		if !s.ink && s.lines >= 6 && i > 0 && i < len(segs)-1 {
			gaps = append(gaps, s.lines)
		}
	}
	if len(lanes) < 2 {
		t.Fatalf("只找到 %d 段有内容的行,图退化成了单行(或没画)", len(lanes))
	}
	if len(gaps) < 1 {
		t.Fatalf("两条泳道之间没有空白间隔,挤在一起了")
	}
}

// 两条泳道各走六到七步,末尾都回到"调整步骤"——那个回环是"可以反复走"
// 这件事本身,所以它必须在图上(文字替代里)说得出。
func TestFlowDiagramShowsBothLanesEndingInUndo(t *testing.T) {
	a := helpApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	var label string
	for _, n := range tt.AccessAll() {
		if strings.HasPrefix(n.Name, "流程图：两条起手方式") {
			label = n.Name
			break
		}
	}
	if label == "" {
		t.Fatal("图上没有文字替代")
	}
	for _, want := range []string{
		"增加文件", "调整步骤", "预览", "执行更名", "撤回",
		"预设", "添加步骤", // 第二条路的起点
	} {
		if !strings.Contains(label, want) {
			t.Errorf("文字替代里没提到 %q: %q", want, label)
		}
	}
	// "撤回"必须出现在"执行更名"之后:先改才谈得上退。
	if i, j := strings.Index(label, "执行更名"), strings.Index(label, "撤回"); i >= 0 && j >= 0 && j < i {
		t.Errorf("文字替代里撤回排在执行更名之前: %q", label)
	}
}

// 存档是另一件事,得单独一张图、单独一句话。混进主流程会让"排步骤"
// 看起来像流程的第七步。
func TestHelpHasASeparateArchiveFlow(t *testing.T) {
	a := helpApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	var label string
	for _, n := range tt.AccessAll() {
		if strings.HasPrefix(n.Name, "流程图：存档") {
			label = n.Name
			break
		}
	}
	if label == "" {
		t.Fatal("帮助页里没有存档小流程")
	}
	for _, want := range []string{"存成流程", "导出设置", "导入设置"} {
		if !strings.Contains(label, want) {
			t.Errorf("存档流程里没提到 %q: %q", want, label)
		}
	}
}
