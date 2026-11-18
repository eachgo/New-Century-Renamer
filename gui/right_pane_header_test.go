package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 右栏标题「文件」不许和工具栏的任何一颗按钮挨上。
//
// 报告量到的是 686 vs 685.9——他们说"重叠",当时我复测 1280 宽没复现,
// 就降级成了"待复测"。真正的触发条件是**窄窗口**:paneTitle 里的 Row
// 没写 Shrink(0),flex 会把它压到比文字还窄,于是后面的工具栏从标题
// 文字中间开始排。实测 900 宽:标题右缘 616,「添加顺序」左缘 612,
// 压进去 4 DIP——不是贴死,是压上了。
func TestFilesHeaderNeverOverlapsItsToolbar(t *testing.T) {
	labels := []string{"添加顺序", "添加文件…", "导出清单…", "导入清单…", "清空列表"}
	for _, w := range []int{900, 960, 1000, 1100, 1280, 1600} {
		a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
		a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
		dir := t.TempDir()
		var paths []string
		for _, n := range []string{"a.mp3", "b.flac"} {
			p := filepath.Join(dir, n)
			writeTestFile(t, p, "x")
			paths = append(paths, p)
		}
		a.addPaths(paths)

		tt := ui.NewTester(a.view, w, 700)
		tt.Frame()
		tt.Frame()

		title, ok := tt.Find("文件")
		if !ok {
			t.Fatalf("w=%d:找不到右栏标题", w)
		}
		for _, label := range labels {
			for _, ai := range tt.AccessAll() {
				if ai.Name != label {
					continue
				}
				b := ai.Bounds
				// 只查同一行的:折到下一行的按钮和标题没有水平竞争关系。
				if b.Y >= title.Y+title.H || b.Y+b.H <= title.Y {
					continue
				}
				if b.X < title.X+title.W+4 {
					t.Errorf("w=%d:「%s」左缘 %.1f 压进了标题(右缘 %.1f)——工具栏该从标题右边 %d 处开始",
						w, label, b.X, title.X+title.W, 8)
				}
			}
		}
	}
}

// 标题不许被工具栏挤扁:文字必须整串画得出来。
func TestFilesHeaderKeepsItsText(t *testing.T) {
	for _, w := range []int{900, 1280} {
		a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
		a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
		tt := ui.NewTester(a.view, w, 700)
		tt.Frame()
		tt.Frame()
		r, ok := tt.Find("文件")
		if !ok {
			t.Fatalf("w=%d:找不到右栏标题", w)
		}
		// 两个中文字 14px,实测宽 28;被压扁时会小于 26。
		if r.W < 26 {
			t.Errorf("w=%d:标题被压成 %.1f 宽,两个字画不下了", w, r.W)
		}
	}
}

// 像素层面的同一件事:「文件」两个字的右边不许有按钮底/按钮边框。
func TestFilesHeaderHasClearSpaceAfterIt(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	dir := t.TempDir()
	var paths []string
	for _, n := range []string{"a.mp3"} {
		p := filepath.Join(dir, n)
		writeTestFile(t, p, "x")
		paths = append(paths, p)
	}
	a.addPaths(paths)

	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	tt.Frame()

	title, ok := tt.Find("文件")
	if !ok {
		t.Fatal("找不到右栏标题")
	}
	img := tt.Image()
	ground := pix(img, int(title.X)-4, int(title.Y+title.H/2))
	// 标题右侧 1..6 DIP 必须还是栏底:那里若有按钮底或按钮边框,
	// 说明工具栏压上来了。
	for dx := 1; dx <= 6; dx++ {
		x := int(title.X+title.W) + dx
		y := int(title.Y + title.H/2)
		if got := pix(img, x, y); got != ground {
			t.Fatalf("标题右缘外 %d DIP 处像素是 %v,栏底是 %v——工具栏压上来了", dx, got, ground)
		}
	}
}
