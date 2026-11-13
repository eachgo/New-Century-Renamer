package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 目录滚不动时不该说"下面还有"——假提示比没有提示更糟,它训练用户
// 忽略这行字。
func TestCatalogCueOnlyWhenOverflowing(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 620)
	tt.Frame()
	tt.Frame()

	if a.palScroll.MaxY <= 0 {
		t.Fatalf("620 高下目录应当溢出,MaxY=%.1f", a.palScroll.MaxY)
	}
	if !tt.HasText("↓ 下面还有") {
		t.Fatal("目录溢出时应当提示下面还有内容")
	}

	// 拉高到整份目录都放得下
	tt.SetSize(1100, 1700)
	tt.Frame()
	tt.Frame()
	if a.palScroll.MaxY > 0 {
		t.Fatalf("1700 高下目录不该再溢出,MaxY=%.1f", a.palScroll.MaxY)
	}
	if tt.HasText("↓ 下面还有") {
		t.Fatal("目录全部可见时不该还提示下面还有")
	}
}

// 滚到底之后提示要消失:否则用户会一直以为下面还有。
func TestCatalogCueGoneAtBottom(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 620)
	tt.Frame()
	tt.Frame()

	a.palScroll.Y = a.palScroll.MaxY
	tt.Frame()
	if tt.HasText("↓ 下面还有") {
		t.Fatal("滚到底之后不该还提示下面还有")
	}
}
