package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 200 个文件里只有 3 个不想改时,以前唯一的办法是清空整表重来。
// 现在能单个移除,而且移除后预览必须重算:新名字可能依赖序号、
// 分组计数这类上下文,删掉一行后面每一行的预览都可能变。
func TestRemoveOneFileRecalculatesPreview(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("seq")}
	a.stepUID = a.steps[0].opUID
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		writeTestFile(t, filepath.Join(dir, n), "x")
	}
	a.addPaths([]string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt"), filepath.Join(dir, "c.txt")})
	if len(a.files) != 3 || len(a.ops) != 3 {
		t.Fatalf("准备失败: files=%d ops=%d", len(a.files), len(a.ops))
	}
	before := a.ops[2].NewName

	a.removeFile(1) // 删中间那个
	if len(a.files) != 2 || len(a.ops) != 2 {
		t.Fatalf("移除后 files=%d ops=%d, want 2/2", len(a.files), len(a.ops))
	}
	// files 与 ops 是按下标对齐的两个切片,错位一行比不删更糟
	for i := range a.files {
		if filepath.Base(a.files[i].path) != a.ops[i].OldName {
			t.Fatalf("第 %d 行 files 与 ops 错位: %q vs %q",
				i, filepath.Base(a.files[i].path), a.ops[i].OldName)
		}
	}
	if a.ops[1].NewName == before {
		t.Fatalf("删掉一行后序号没重算,第三行新名字仍是 %q", before)
	}
}

// 列表里文件卡与分组头混在同一个 rows 序列里,行号不能直接当文件号用。
func TestRowFileSkipsHeaders(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "Album")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"1.txt", "2.txt"} {
		writeTestFile(t, filepath.Join(sub, n), "x")
	}
	writeTestFile(t, filepath.Join(dir, "loose.txt"), "x")

	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.addPaths([]string{sub, filepath.Join(dir, "loose.txt")})

	rows := buildRows(a.files)
	if len(rows) != len(a.files)+1 {
		t.Fatalf("rows=%d, want files+1=%d", len(rows), len(a.files)+1)
	}
	for ri := range rows {
		got := a.rowFile(ri)
		if rows[ri].header {
			if got != -1 {
				t.Fatalf("第 %d 行是分组头,rowFile 应返回 -1, got %d", ri, got)
			}
			continue
		}
		if got != rows[ri].file {
			t.Fatalf("第 %d 行 rowFile=%d, want %d", ri, got, rows[ri].file)
		}
	}
}

// Delete 键在文件列表拿着焦点时删文件,在步骤列表拿着焦点时删步骤。
// 同一个键管两件事,靠焦点区分,不靠用户记"现在删的是什么"。
func TestDeleteKeyRemovesFocusedFile(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("case.upper")}
	a.stepUID = a.steps[0].opUID
	// 故意用无扩展名的文件:带扩展名时卡片把名字拆成"主名 . 扩展名"
	// 三个独立标签,测试按文字点会找不到
	for _, n := range []string{"alpha", "beta"} {
		writeTestFile(t, filepath.Join(dir, n), "x")
	}
	a.addPaths([]string{filepath.Join(dir, "alpha"), filepath.Join(dir, "beta")})

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	// 点第一张文件卡:列表拿到焦点,fileSel 指向它
	if err := tt.Click("alpha"); err != nil {
		t.Fatalf("点不到文件卡: %v", err)
	}
	tt.Frame()
	if a.fileSel < 0 {
		t.Fatal("点文件卡后 fileSel 仍是 -1")
	}
	// mygo 的快捷键注册表每帧交换一次:焦点是在上一帧的构建里才设上的,
	// 所以要多跑一帧让 handleShortcuts 看到"列表已聚焦"并注册 Delete
	tt.Frame()
	tt.Key(0, ui.KeyDelete)
	tt.Frame()
	if len(a.files) != 1 || filepath.Base(a.files[0].path) != "beta" {
		t.Fatalf("Delete 没删掉选中的文件: %d 个", len(a.files))
	}
	if len(a.steps) != 1 {
		t.Fatalf("文件列表拿着焦点时 Delete 误删了步骤: %d 步", len(a.steps))
	}
}

// 右键菜单四项都要在,而且"复制新名字"要真把新名字写进剪贴板。
func TestFileContextMenu(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("case.upper")}
	a.stepUID = a.steps[0].opUID
	p := filepath.Join(dir, "oldname")
	writeTestFile(t, p, "x")
	a.addPaths([]string{p})

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if err := tt.RightClick("oldname"); err != nil {
		t.Fatalf("右键点不到文件卡: %v", err)
	}
	tt.Frame()
	menu := tt.Menu()
	want := []string{"从列表移除", "-", "在文件管理器中显示", "复制新名字", "-", "复制原文件名"}
	if len(menu) != len(want) {
		t.Fatalf("右键菜单项不对: %v", menu)
	}
	for i := range want {
		if menu[i] != want[i] {
			t.Fatalf("右键菜单第 %d 项 = %q, want %q (全部: %v)", i, menu[i], want[i], menu)
		}
	}

	if err := tt.ChooseMenuItem("复制新名字"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := tt.Clipboard(); got != "OLDNAME" {
		t.Fatalf("剪贴板 = %q, want 预览里的新名字 %q", got, "OLDNAME")
	}
}

// 右键菜单里"从列表移除"也要真删。
func TestFileContextMenuRemoves(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	for _, n := range []string{"alpha", "beta"} {
		writeTestFile(t, filepath.Join(dir, n), "x")
	}
	a.addPaths([]string{filepath.Join(dir, "alpha"), filepath.Join(dir, "beta")})

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if err := tt.RightClick("alpha"); err != nil {
		t.Fatalf("右键点不到文件卡: %v", err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("从列表移除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.files) != 1 || filepath.Base(a.files[0].path) != "beta" {
		t.Fatalf("右键移除没生效: %d 个文件", len(a.files))
	}
}
