package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 默认只展开前两组:11 组 32 条全摊开时一屏只放得下前三四组,用户既
// 不知道下面还有多少,也不知道该从哪儿找起。但也不能全折——全折之后
// 左栏只剩 11 行组名,第一次打开的人看不出这里装的是能点的规则。
func TestCatalogGroupsStartPartiallyOpen(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	open, shut := 0, 0
	for _, g := range stepGroups {
		if tt.HasText("▾ " + g) {
			open++
		}
		if tt.HasText("▸ " + g) {
			shut++
		}
	}
	if open != defaultOpenGroups {
		t.Fatalf("默认应当只展开 %d 组,实际 %d 组", defaultOpenGroups, open)
	}
	if shut != len(stepGroups)-defaultOpenGroups {
		t.Fatalf("其余 %d 组应当折起,实际 %d 组", len(stepGroups)-defaultOpenGroups, shut)
	}
	// 展开的那两组里的条目要真的在
	if !tt.HasText("查找替换") || !tt.HasText("文件名前添加") {
		t.Fatal("展开的组里应当看得见条目")
	}
	// 折起的组里条目不该占位
	if tt.HasText("全部转小写") {
		t.Fatal("折起的组里条目不该还在")
	}
}

// 折起来之后组里的条目要真的消失,而且组标题还留着——否则用户没有
// 办法再展开。
func TestCatalogGroupCollapses(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	// 编号组默认是折起的,先展开它
	if err := tt.Click("▸ 编号"); err != nil {
		t.Fatalf("组标题应当可点: %v", err)
	}
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

	// 编号默认折起,先展开再折回,确认只影响自己
	if err := tt.Click("▸ 编号"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("▾ 编号"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// 元数据组默认也是折起的(它排在第 8 位,不在前两组里),所以这里
	// 断言的是"它没有被连带展开/折起",不是"它是开的"。
	if !tt.HasText("▸ 元数据") {
		t.Fatal("折起编号组不该改变元数据组的状态")
	}
	// 展开它,条目应当在;再折起编号组,它不该跟着变
	if err := tt.Click("▸ 元数据"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("用标题命名") {
		t.Fatal("元数据组展开后条目应当可见")
	}
}
