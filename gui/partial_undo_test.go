package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// 回滚只退得回一半时,界面必须跟着磁盘走:退回去的那行显示旧名,
// 没退成的那行继续显示新名(它确实还叫新名字)。
// 以前是一律把所有 Applied 的行改成旧名,于是没退成的文件在界面里
// 变成了旧名,而磁盘上它还是新名——用户按界面去找文件会找不到。
func TestPartialUndoKeepsFailedRowOnNewName(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")

	writeTestFile(t, filepath.Join(dir, "a.txt"), "a")
	writeTestFile(t, filepath.Join(dir, "b.txt"), "b")
	plan := []core.RenameOp{
		{Dir: dir, OldName: "a.txt", NewName: "A.txt"},
		{Dir: dir, OldName: "b.txt", NewName: "B.txt"},
	}
	if _, err := core.ApplyPlanReport(plan, a.settings.Journal, nil); err != nil {
		t.Fatal(err)
	}
	// 把 B.txt 挪走,让回滚时 B.txt → b.txt 必然失败
	if err := os.Rename(filepath.Join(dir, "B.txt"), filepath.Join(dir, "B-elsewhere.txt")); err != nil {
		t.Fatal(err)
	}

	a.files = []fileInfo{
		{path: filepath.Join(dir, "A.txt"), size: 1, typ: "文本"},
		{path: filepath.Join(dir, "B-elsewhere.txt"), size: 1, typ: "文本"},
	}
	a.ops = []core.RenameOp{
		{Dir: dir, OldName: "a.txt", NewName: "A.txt", Applied: true},
		{Dir: dir, OldName: "b.txt", NewName: "B.txt", Applied: true},
	}
	a.applied = true
	a.undo()

	if a.ops[0].Applied {
		t.Fatal("退回去的那行还标着已更名")
	}
	if filepath.Base(a.files[0].path) != "a.txt" {
		t.Fatalf("退回去的行没换回旧名: %s", a.files[0].path)
	}
	if !a.ops[1].Applied {
		t.Fatal("没退成的那行被清掉了 Applied,界面会说它已经退回")
	}
	if filepath.Base(a.files[1].path) != "B-elsewhere.txt" {
		t.Fatalf("没退成的那行被改成了 %s", a.files[1].path)
	}
	// 状态里必须说清是部分完成,而不是"回滚失败"或"已回滚 2 个"
	if want := "回滚部分完成"; !strings.Contains(a.status, want) {
		t.Fatalf("status = %q, 应含 %q", a.status, want)
	}
}

// 横幅里的「撤销这次」和底栏的「回滚上次」必须是同一套防错:
// 同一个动作有两套强度,用户迟早会在弱的那一侧误触。
func TestBannerUndoAlsoNeedsConfirm(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	writeTestFile(t, filepath.Join(dir, "旧.txt"), "x")
	plan := []core.RenameOp{{Dir: dir, OldName: "旧.txt", NewName: "新.txt"}}
	if _, err := core.ApplyPlanReport(plan, a.settings.Journal, nil); err != nil {
		t.Fatal(err)
	}
	// 横幅长在文件列表里:空列表显示的是拖放区,那里没有横幅。
	a.files = []fileInfo{{path: filepath.Join(dir, "新.txt"), size: 1, typ: "文本"}}
	a.ops = []core.RenameOp{{Dir: dir, OldName: "旧.txt", NewName: "新.txt", Applied: true}}
	a.applied = true
	a.appliedCount = 1
	a.appliedAt = time.Now() // 横幅只在"刚才这一批"有时间时出现

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	if err := tt.Click("撤销这次"); err != nil {
		t.Fatalf("横幅里找不到撤销按钮: %v", err)
	}
	tt.Frame()
	if _, err := os.Stat(filepath.Join(dir, "新.txt")); err != nil {
		t.Fatal("第一次点击就回滚了,横幅应该只是进入确认态")
	}
	if !a.confirmUndo {
		t.Fatal("横幅的撤销没有走确认态")
	}
	if err := tt.Click("确认回滚 1 个?"); err != nil {
		t.Fatalf("横幅里找不到确认按钮: %v", err)
	}
	for i := 0; i < 20; i++ {
		tt.Frame()
	}
	if _, err := os.Stat(filepath.Join(dir, "旧.txt")); err != nil {
		t.Fatalf("确认后没有回滚: %v", err)
	}
}
