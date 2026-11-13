package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// 批量执行时底栏要给出"第几个/共几个":一个转圈说不出还要多久,
// 而 500 个文件能跑十几秒。
func TestBusyStatusShowsCount(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.busy = true
	a.busyStart = time.Now()
	a.busyDone, a.busyTotal = 37, 120

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 2; i++ {
		tt.Frame()
	}
	if !tt.HasText("37/120") {
		t.Fatalf("底栏没有进度计数: %v", tt.Texts())
	}
}

// 执行完之后必须把计数清掉,否则底栏会永远停在上一次的 "120/120"。
func TestBusyCountClearedAfterApply(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	for i := 0; i < 3; i++ {
		writeTestFile(t, filepath.Join(dir, fmt.Sprintf("F%d.TXT", i)), "x")
	}
	a.addPaths([]string{
		filepath.Join(dir, "F0.TXT"), filepath.Join(dir, "F1.TXT"), filepath.Join(dir, "F2.TXT"),
	})
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	a.preview()

	a.apply()

	if a.busyDone != 0 || a.busyTotal != 0 {
		t.Fatalf("执行完计数没清: %d/%d", a.busyDone, a.busyTotal)
	}
	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 2; i++ {
		tt.Frame()
	}
	if tt.HasText("3/3") {
		t.Fatalf("执行完底栏还留着进度: %v", tt.Texts())
	}
}
