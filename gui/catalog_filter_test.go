package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 目录有 34 项 11 组,任何窗口高度都要滚;输入一个词就能立刻缩到几项。
func TestCatalogFilterNarrowsList(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()

	// 没过滤时目录里所有条目都在,只是大部分被滚出可视区
	before := catalogLabelsShown(tt)
	if before < 20 {
		t.Fatalf("未过滤时应当有整份目录,实际只渲染了 %d 项", before)
	}

	a.palQuery = "nfo"
	tt.Frame()
	if !tt.HasText("按 NFO 编号命名") {
		t.Fatal("输入 nfo 之后应当能筛到「按 NFO 编号命名」")
	}
	after := catalogLabelsShown(tt)
	if after >= before {
		t.Fatalf("过滤后条目数应当减少: %d → %d", before, after)
	}
	// 无关条目必须消失
	if tt.HasText("全部转小写") {
		t.Fatal("过滤后仍显示无关条目")
	}
}

// 组名也参与匹配,并且匹配不到时给出人话,而不是一片空白。
func TestCatalogFilterMatchesGroupAndExplainsEmpty(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 720)

	a.palQuery = "筛选"
	tt.Frame()
	if !tt.HasText("筛选文件") || !tt.HasText("排除文件") {
		t.Fatal("按组名「筛选」应当留下整组")
	}

	a.palQuery = "zzz-没有这个"
	tt.Frame()
	if !tt.HasText("没有匹配的功能") {
		t.Fatal("筛不到时应当说一句,而不是给一片空白")
	}
	if tt.HasText("筛选文件") {
		t.Fatal("筛不到时不该还留着条目")
	}
}

// 过滤只是缩小视野,不能改变"点一下就加步骤"这件事。
func TestCatalogFilterStillAddsStep(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 720)
	a.palQuery = "小写"
	tt.Frame()

	if err := tt.Click("全部转小写"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.steps) != 1 {
		t.Fatalf("过滤状态下点击应正常加入步骤,实际 %d 步", len(a.steps))
	}
}

// catalogLabelsShown counts how many catalog entries the current frame renders.
func catalogLabelsShown(tt *ui.Tester) int {
	texts := map[string]bool{}
	for _, s := range tt.Texts() {
		texts[s] = true
	}
	n := 0
	for _, d := range catalog {
		if texts[d.label] {
			n++
		}
	}
	return n
}
