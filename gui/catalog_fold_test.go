package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 折叠是给"我已经知道要什么、只是嫌长"的人用的:默认必须全展开,
// 否则第一次打开就把 26 项规则藏进 11 次点击后面,等于换个方式让人
// 找不到。
func TestCatalogGroupsStartExpanded(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	if !tt.HasText("▾ 编号") {
		t.Fatal("分组默认应当是展开的（▾）")
	}
	if !tt.HasText("追加序号") {
		t.Fatal("默认展开时应当看得见组里的条目")
	}
}

// 折起来之后组里的条目要真的消失,而且组标题还留着——否则用户没有
// 办法再展开。
func TestCatalogGroupCollapses(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	if err := tt.Click("▾ 编号"); err != nil {
		t.Fatalf("组标题应当可点: %v", err)
	}
	tt.Frame()
	if !tt.HasText("▸ 编号") {
		t.Fatal("点过之后组标题应当变成折起状态（▸）")
	}
	if tt.HasText("追加序号") {
		t.Fatal("折起之后组里的条目不该还在")
	}

	// 再点一次展开回来
	if err := tt.Click("▸ 编号"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("追加序号") {
		t.Fatal("再点一次应当展开回原样")
	}
}

// 折叠状态跟着组走,不是"全局折起":折一个组不该影响别的组。
func TestCatalogCollapseIsPerGroup(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	if err := tt.Click("▾ 编号"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("▾ 元数据") {
		t.Fatal("折起编号组不该把元数据组也折起来")
	}
	if !tt.HasText("用标题命名") {
		t.Fatal("没被折起的组里条目应当照旧可见")
	}
}
