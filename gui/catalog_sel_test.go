package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 左栏目录的高亮必须跟着"当前选中的那一步",不是"最后点过的目录条目"。
//
// 原来是两套状态:点步骤行只改 stepSel,左栏 palSel 还停在很久以前加进来
// 的那条上——同一屏里两处高亮说着两件不同的事,用户没法从左边读出"我
// 正在改的是哪条规则"。
func TestCatalogHighlightFollowsSelectedStep(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("case.lower"), step("case.upper")}

	if got := a.catalogSel(); got != "case.lower" {
		t.Fatalf("选中第 1 步时应高亮 case.lower,实际 %q", got)
	}
	a.stepSel = 1
	if got := a.catalogSel(); got != "case.upper" {
		t.Fatalf("选中第 2 步时应高亮 case.upper,实际 %q", got)
	}
	// 没有选中项时一条都不该亮:此刻没有哪条规则"正在被改"。
	a.stepSel = -1
	if got := a.catalogSel(); got != "" {
		t.Fatalf("没有选中步骤时不该高亮任何条目,实际 %q", got)
	}
}

// 屏幕上也要跟着变:换选中的步骤,左栏的选中底色跟着换行。
//
// 用「筛选/范围」组里的两条(默认展开,不用先点开分组),量的是条目
// 中段的底色:文字与图标都在左半,取横向中点不会踩到它们。
func TestCatalogHighlightMovesOnScreen(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("filter"), step("filter.out")}

	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	selBg, _ := a.selColors()

	rowBg := func(label string) ui.Color {
		b, ok := tt.Find(label)
		if !ok {
			t.Fatalf("左栏找不到「%s」", label)
		}
		x := int(b.X + b.W/2)
		y := int(b.Y + b.H/2)
		img := tt.Image()
		r, g, bl, _ := img.At(x, y).RGBA()
		return ui.Color{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: 255}
	}

	if got := rowBg("筛选文件"); got != selBg {
		t.Fatalf("选中第 1 步时「筛选文件」应是选中底色 %v,实际 %v", selBg, got)
	}
	if got := rowBg("排除文件"); got == selBg {
		t.Fatalf("选中第 1 步时「排除文件」不该是选中底色,实际 %v", got)
	}

	// 换到第 2 步:高亮必须跟着走,不能留在原来那条上。
	a.stepSel = 1
	tt.Frame()
	if got := rowBg("排除文件"); got != selBg {
		t.Fatalf("选中第 2 步时「排除文件」应是选中底色 %v,实际 %v", selBg, got)
	}
	if got := rowBg("筛选文件"); got == selBg {
		t.Fatal("选中第 2 步后「筛选文件」还亮着,高亮没跟着走")
	}
}
