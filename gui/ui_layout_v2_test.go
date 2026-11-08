package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 文件分组用吸顶分组头:滚动到某个分组时,该分组的文件夹名固定在列表顶部,
// 文件夹再也不会"滚没了就不知道现在在哪个文件夹"。
func TestFolderHeaderSticksWhileScroll(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	dir := t.TempDir()
	var files []fileInfo
	for _, g := range []string{"Album A", "Album B"} {
		gd := filepath.Join(dir, g)
		os.MkdirAll(gd, 0o755)
		for i := 0; i < 10; i++ {
			p := filepath.Join(gd, fmt.Sprintf("%s track %02d.mp3", g, i))
			os.WriteFile(p, []byte("x"), 0o644)
			f := statFile(p)
			f.group = g
			files = append(files, f)
		}
	}
	a.files = files
	tt := ui.NewTester(a.view, 900, 400)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	if _, ok := tt.Find("📁 Album A"); !ok {
		t.Fatal("初始应能看到第一个文件夹头 Album A")
	}
	// 记下列表顶部(工具栏下方)的 y,吸顶头应钉在这里附近
	listTop := func() float32 {
		if r, ok := tt.Find("📁 Album A"); ok {
			return r.Y
		}
		return 90
	}()
	// 滚进 Album B 深处:Album B 的头应被顶到列表顶部并固定在那里
	a.table.ScrollTo(18, 0) // Album B 的中段
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	r, ok := tt.Find("📁 Album B")
	if !ok {
		t.Fatal("滚进 Album B 深处后,其文件夹头仍应固定在顶部可见")
	}
	if r.Y > listTop+4 {
		t.Errorf("Album B 的吸顶头应钉在列表顶部(y≈%.1f),实际 y=%.1f", listTop, r.Y)
	}
}

// 主次分工:执行更名/回滚上次属于"整个软件"的主操作,放在底部状态栏;
// 添加文件/导出/清空属于"文件列表"的操作,留在右栏顶部。
func TestPrimaryActionInBottomBar(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.files = []fileInfo{statFile(newTestFile(t, "a.txt"))}
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	apply, ok := tt.Find("执行更名")
	if !ok {
		t.Fatal("找不到 执行更名 主按钮")
	}
	// 执行更名应在窗口底部(状态栏),而不是文件列表顶部
	if apply.Y < 640 {
		t.Errorf("执行更名应在底部状态栏(y≥640),实际 y=%.1f", apply.Y)
	}
	undo, ok := tt.Find("回滚上次")
	if !ok {
		t.Fatal("找不到 回滚上次 按钮")
	}
	if undo.Y < 640 {
		t.Errorf("回滚上次应在底部状态栏,实际 y=%.1f", undo.Y)
	}
}
