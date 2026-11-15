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

	// 先确认图在那儿(文字替代),再量它下面那片区域。
	if !tt.HasText("流程图：") {
		t.Fatal("帮助页里没有流程图")
	}
	img := tt.Image()
	scale := float64(img.Bounds().Dx()) / 1280

	// 图所在那一带:从"它是这么一条线"往下约 100 DIP。
	title, ok := tt.Find("它是这么一条线")
	if !ok {
		t.Fatal("找不到图的标题")
	}
	x0 := int((title.X) * float32(scale))
	x1 := int((title.X + 400) * float32(scale))
	y0 := int((title.Y + 18) * float32(scale))
	y1 := int((title.Y + 100) * float32(scale))

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
// 做法:逐行找"很长的绿色横线"(方框的上下边)。一条泳道贡献两条边,
// 两条泳道就是两簇、中间隔着三十多 DIP。只有一簇说明图退化成了单行。
func TestFlowDiagramHasTwoLanes(t *testing.T) {
	a := helpApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	img := tt.Image()
	scale := float64(img.Bounds().Dx()) / 1280
	title, ok := tt.Find("它是这么一条线")
	if !ok {
		t.Fatal("找不到图的标题")
	}
	// 起点描边是主色 55% 叠在弹窗底上,混完不等于主色本身,所以判据是
	// "绿色明显压过红蓝",不是"等于主色"。
	acc := a.currentTheme().light.Accent
	margin := (int(acc.G) - int(acc.R)) / 2
	if margin < 8 {
		margin = 8
	}

	x0 := int(title.X * float32(scale))
	x1 := int((title.X + 300) * float32(scale))
	y0 := int((title.Y + 16) * float32(scale))
	y1 := int((title.Y + 150) * float32(scale))

	// 每行数"最长连续绿色段":方框的上下边是连续的百来个像素,
	// 侧边只有两三个,文字更短。
	var edgeRows []int
	for y := y0; y < y1; y++ {
		run, best := 0, 0
		for x := x0; x < x1; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			rr, gg, bb := int(r>>8), int(g>>8), int(b>>8)
			// 只比绿红差:描边混色后蓝分量还偏高(底色本身就带绿),
			// 加上 gg-bb 的条件会把整条边排除掉。
			if gg-rr >= margin && gg >= bb {
				run++
				if run > best {
					best = run
				}
			} else {
				run = 0
			}
		}
		if best >= 60 {
			edgeRows = append(edgeRows, y)
		}
	}
	if len(edgeRows) < 4 {
		t.Fatalf("只找到 %d 条长横边,期望至少 4 条(两个起点方框各有上下边): %v", len(edgeRows), edgeRows)
	}

	// 压成簇,再要求"至少两簇、且彼此隔开"——那正是上下两条泳道。
	var bands [][2]int
	start, prev := edgeRows[0], edgeRows[0]
	for _, y := range edgeRows[1:] {
		if y-prev > 6 {
			bands = append(bands, [2]int{start, prev})
			start = y
		}
		prev = y
	}
	bands = append(bands, [2]int{start, prev})
	if len(bands) < 2 {
		t.Fatalf("只有 %d 簇横边,图退化成了单行: %v", len(bands), bands)
	}
	if gap := bands[1][0] - bands[0][1]; gap < 20 {
		t.Errorf("两条泳道只隔 %d DIP,挤在一起了", gap)
	}
}

// 文字替代要把两条入口都说出来,而不只是"有个流程图"。
func TestFlowDiagramLabelExplainsBothEntries(t *testing.T) {
	a := helpApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	var label string
	for _, n := range tt.AccessAll() {
		if strings.HasPrefix(n.Name, "流程图：") {
			label = n.Name
			break
		}
	}
	if label == "" {
		t.Fatal("图上没有文字替代")
	}
	for _, want := range []string{"拖入文件", "排步骤", "预览", "执行更名"} {
		if !strings.Contains(label, want) {
			t.Errorf("文字替代里没提到 %q: %q", want, label)
		}
	}
}
