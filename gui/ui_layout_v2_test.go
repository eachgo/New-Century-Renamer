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

// 分组头必须独立成行:一个组 n 个文件占 n+1 行。
// 以前头兼作该组第一行,那行文件卡永远不画——7 个文件只看到 5 行,
// 底栏说"7 个将更名"而屏幕上少两行,用户没法逐行核对。
func TestGroupHeaderDoesNotSwallowFirstFile(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	dir := t.TempDir()
	var files []fileInfo
	for _, g := range []string{"Album A", "Album B"} {
		gd := filepath.Join(dir, g)
		os.MkdirAll(gd, 0o755)
		for i := 0; i < 3; i++ {
			p := filepath.Join(gd, fmt.Sprintf("track%d", i+1)+".mp3")
			os.WriteFile(p, []byte("x"), 0o644)
			f := statFile(p)
			f.group = g
			files = append(files, f)
		}
	}
	a.files = files
	rows := buildRows(a.files)
	if len(rows) != 8 { // 2 头 + 6 文件
		t.Fatalf("行数 = %d, want 8 (2 个头 + 6 个文件)", len(rows))
	}
	heads, cards := 0, 0
	for _, r := range rows {
		if r.header {
			heads++
		} else {
			cards++
		}
	}
	if heads != 2 || cards != 6 {
		t.Fatalf("头 %d 个、卡 %d 张, want 2 / 6", heads, cards)
	}

	// 渲染层面确认:每个组的第一个文件都真的画出来了
	tt := ui.NewTester(a.view, 900, 600)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	for _, want := range []string{"📁 Album A", "📁 Album B", "track1", "track2", "track3"} {
		if _, ok := tt.Find(want); !ok {
			t.Errorf("应能看到 %q", want)
		}
	}
}

// 执行后与回滚后都必须保持"7 个文件 2 个头"——不能因为重建 fileInfo
// 丢掉 group 而让列表进入混合状态(部分有组名、部分空),那样 isHeader
// 的 ""→"A" 交界处会凭空多出组头,每个头又吞掉一行。
func TestGroupSurvivesApplyAndUndo(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	dir := t.TempDir()
	var files []fileInfo
	for _, g := range []string{"Album A", "Album B"} {
		gd := filepath.Join(dir, g)
		os.MkdirAll(gd, 0o755)
		for i := 0; i < 3; i++ {
			p := filepath.Join(gd, fmt.Sprintf("t%d.txt", i+1))
			os.WriteFile(p, []byte("x"), 0o644)
			f := statFile(p)
			f.group = g
			files = append(files, f)
		}
	}
	a.files = files
	a.settings.Journal = filepath.Join(dir, "journal.json")
	a.steps = []guiRule{step("case.lower")}
	a.stepSel = 0
	a.preview()

	if got := len(buildRows(a.files)); got != 8 {
		t.Fatalf("执行前 %d 行, want 8", got)
	}
	a.apply()
	if got := len(buildRows(a.files)); got != 8 {
		t.Fatalf("执行后 %d 行, want 8(丢 group 会变成 6 行 4 头)", got)
	}
	for i := range a.files {
		if a.files[i].group == "" {
			t.Errorf("执行后第 %d 个文件丢了 group", i)
		}
	}
	a.undo()
	if got := len(buildRows(a.files)); got != 8 {
		t.Fatalf("回滚后 %d 行, want 8", got)
	}
	for i := range a.files {
		if a.files[i].group == "" {
			t.Errorf("回滚后第 %d 个文件丢了 group", i)
		}
	}
}
