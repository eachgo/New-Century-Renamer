package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 目录条目之间必须有缝。以前每条 Padding(0,0) 紧贴,32 条挤成一片:
// 悬停底色和相邻条目连成一块,眼睛没法判断一行到哪结束。
func TestCatalogItemsHaveGaps(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 900)
	tt.Frame()
	tt.Frame()

	a1, ok1 := tt.Find("筛选文件")
	a2, ok2 := tt.Find("排除文件")
	if !ok1 || !ok2 {
		t.Fatal("找不到相邻的两条目录条目")
	}
	gap := a2.Y - (a1.Y + a1.H)
	if gap < 2 {
		t.Fatalf("目录条目之间只有 %.1fpx 缝,挤在一起了", gap)
	}
	if gap > 10 {
		t.Fatalf("目录条目之间 %.1fpx 缝,太散", gap)
	}
}

// 目录里每一条都得有图标。icon 字段写错名字时 opIcon 返回 nil,
// 界面上就是一片没有图标的文字——32 条里少一个没人会发现。
func TestEveryCatalogEntryHasAnIcon(t *testing.T) {
	for _, d := range catalog {
		if opIcon(d.icon) == nil {
			t.Errorf("功能 %q 的图标 %q 在 flowbite 图标表里不存在", d.label, d.icon)
		}
	}
}
