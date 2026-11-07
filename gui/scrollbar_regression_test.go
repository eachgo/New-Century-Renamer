package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestFileListScrolls 复现并锁死"文件列表拖入大量文件后没有滚动条/滚不动"的回归。
// 根因:steps 与 files 之间的内层 Split 缺少 .Grow(1).FillWidth(),文件列表
// 所在的窗格高度塌成内容高度,List 永远不会成为滚动视口(内容不溢出→没有
// 滚动条,滚轮/拖拽都无效)。修法是给内层 Split 加上纵向填充。
//
// 判定:往文件列表里加 40 个文件后,List 的高度必须小于内容高度,即可滚动;
// 通过把 List 滚到底,Visible() 的区间必须真的移动。
func TestFileListScrolls(t *testing.T) {
	root := t.TempDir()
	a := &app{stepSel: -1, settings: defaultSettings()}
	var paths []string
	for i := 0; i < 40; i++ {
		p := filepath.Join(root, fmt.Sprintf("第 %02d 个文件.txt", i))
		writeTestFile(t, p, "x")
		paths = append(paths, p)
	}
	a.addPaths(paths)

	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 6; i++ {
		tt.Frame()
	}
	firstTop, lastTop := a.table.Visible()
	if lastTop < 0 {
		t.Fatal("文件列表没有被布局")
	}
	// 列表必须被高度限制:可见行数应远小于 40
	if lastTop-firstTop+1 >= len(a.files) {
		t.Fatalf("文件列表高度没有受限:可见 %d..%d,共 %d 行;列表不会出现滚动条",
			firstTop, lastTop, len(a.files))
	}
	// 而且要真的能滚动:滚到底后可见区间必须移动
	a.table.ScrollToEnd()
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	firstEnd, _ := a.table.Visible()
	if firstEnd == firstTop {
		t.Fatalf("把列表滚到底后可见起始行仍是 %d,列表滚不动", firstEnd)
	}
}
