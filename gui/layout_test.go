package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 三栏内容必须从标题栏下方开始,不能钻到标题栏区域里
func TestLayoutTitlebarNoOverlap(t *testing.T) {
	dir, paths := resetCopy(t)
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.addPaths(paths)
	a.steps = []guiRule{step("case.lower")}
	a.stepUID++
	a.steps[0].opUID = a.stepUID

	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	brand, ok := tt.Find("reamer")
	if !ok {
		t.Fatal("标题栏品牌文字缺失")
	}
	titleBottom := brand.Y + brand.H

	// 三栏各自的第一个内容元素都要在标题栏下方
	checks := []string{"功能 · 点击加入步骤", "任务步骤(1)", "文件"}
	for _, label := range checks {
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("找不到 %q", label)
		}
		if r.Y < titleBottom-1 {
			t.Errorf("%q 顶部 y=%.1f 落在标题栏内(标题栏底部 %.1f)", label, r.Y, titleBottom)
		}
	}

	// 文件列表第一行的文件卡也不能钻进标题栏
	first := a.ops[0]
	if r, ok := tt.Find(first.OldName); ok && r.Y < titleBottom-1 {
		t.Errorf("文件卡 y=%.1f 落入标题栏(底部 %.1f)", r.Y, titleBottom)
	}
	_ = dir
}

