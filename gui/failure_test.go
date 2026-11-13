package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 失败原因里只放文件名,不放绝对路径。
// 绝对路径在一行里放不下,省略号从右边吃掉——被截掉的恰好是文件名,
// 于是错误信息等于没写(图20 是"⚠ 目标已存在: C:"后面一长串临时目录)。
func TestFailureErrorNamesFileNotPath(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("tpl")}
	a.steps[0].tpl = "同名<.EXT>"
	a.stepUID = a.steps[0].opUID
	for _, n := range []string{"a.txt", "b.txt", "同名.txt"} {
		writeTestFile(t, filepath.Join(dir, n), "x")
	}
	a.addPaths([]string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")})

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	msg := firstErrText(a.ops)
	if msg == "" {
		t.Fatal("冲突没有报错")
	}
	if strings.Contains(msg, dir) {
		t.Fatalf("错误信息里带了绝对路径 %q: %q", dir, msg)
	}
}

// 60 个文件里 5 个失败时,逐行扫描找红字是不可能完成的任务:
// 列表顶部要说一句"有几个没法改"。
func TestFailureCountAggregatedOnTop(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("tpl")}
	a.steps[0].tpl = "同名<.EXT>"
	a.stepUID = a.steps[0].opUID
	for _, n := range []string{"a.txt", "b.txt", "同名.txt"} {
		writeTestFile(t, filepath.Join(dir, n), "x")
	}
	a.addPaths([]string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")})

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if !tt.HasText("2 个文件无法更名") {
		t.Fatalf("列表顶部没有失败计数: %v", tt.Texts())
	}
}

// 失败的行要能一眼从"将更名"里挑出来。整卡降饱和太狠会连文件名
// 都读不清,所以只加一道危险色描边。
func TestFailedRowHasDangerBorder(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("tpl")}
	a.steps[0].tpl = "同名<.EXT>"
	a.stepUID = a.steps[0].opUID
	for _, n := range []string{"a.txt", "b.txt", "同名.txt"} {
		writeTestFile(t, filepath.Join(dir, n), "x")
	}
	a.addPaths([]string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")})

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if countErrs(a.ops) != 2 {
		t.Fatalf("应有 2 个失败, got %d", countErrs(a.ops))
	}
	// 错误文本本身现在允许折两行(11px 单行 + 省略号会吃掉后半句)
	if !tt.HasText("批量执行会互相覆盖") {
		t.Fatalf("失败原因被截断了: %v", tt.Texts())
	}
}
