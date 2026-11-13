package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// 「回滚上次」不能是三个字的哑按钮:它得说清是哪一次、几个文件。
// 日志里本来就写着时间与完整清单,以前 GUI 一次都没读过。
func TestUndoButtonShowsWhatItWillUndo(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")

	var ops []core.RenameOp
	for _, n := range []string{"01. THE UNION.flac", "02. Ali Baba.flac", "03. NIGHT FLOWER.flac"} {
		p := filepath.Join(dir, n)
		writeTestFile(t, p, "x")
		ops = append(ops, core.RenameOp{Dir: dir, OldName: n, NewName: n, Applied: true})
	}
	if err := core.WriteJournal(a.settings.Journal, ops); err != nil {
		t.Fatal(err)
	}

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	if !tt.HasText("回滚上次（3 个）") {
		t.Fatalf("回滚按钮没说明要回滚几个文件: %v", tt.Texts())
	}
}

// 点了「回滚上次」不能立刻动磁盘:先挂确认态,再点一次才真回滚。
// 回滚和清空列表一样是"动磁盘"的动作,不该一键生效。
func TestUndoNeedsConfirm(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")

	old := filepath.Join(dir, "旧名.txt")
	writeTestFile(t, old, "x")
	op := core.RenameOp{Dir: dir, OldName: "旧名.txt", NewName: "新名.txt"}
	if err := core.ApplyPlan([]core.RenameOp{op}, a.settings.Journal); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "新名.txt")); err != nil {
		t.Fatalf("准备失败,改名没生效: %v", err)
	}

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	if err := tt.Click("回滚上次（1 个）"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, err := os.Stat(filepath.Join(dir, "新名.txt")); err != nil {
		t.Fatal("第一次点击就回滚了,应该只是进入确认态")
	}
	if !a.confirmUndo {
		t.Fatal("第一次点击未进入确认态")
	}
	if err := tt.Click("确认回滚 1 个?"); err != nil {
		t.Fatalf("找不到确认按钮: %v", err)
	}
	for i := 0; i < 20; i++ {
		tt.Frame()
	}
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("第二次点击没有真的回滚: %v", err)
	}
}

// 悬停提示要说清是哪一批、前几个文件叫什么——点之前就该知道要点掉什么。
func TestUndoTipNamesTheBatch(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	sum := core.JournalSummary{
		Time:  time.Date(2026, 11, 13, 1, 20, 0, 0, time.Local),
		Names: []string{"a.flac", "b.flac", "c.flac", "d.flac"},
	}
	tip := a.undoTip(sum)
	for _, want := range []string{"11-13 01:20", "a.flac", "b.flac", "c.flac", "…等 4 个文件"} {
		if !contains(tip, want) {
			t.Fatalf("提示里缺 %q:\n%s", want, tip)
		}
	}
	if contains(tip, "d.flac") {
		t.Fatalf("提示里不该列出第 4 个文件:\n%s", tip)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// 日志默认路径必须是绝对的:相对路径跟着进程工作目录走,
// 换个目录启动就"回滚上次"不可用,或者回滚到别处记录里的文件。
func TestDefaultJournalIsAbsolute(t *testing.T) {
	p := defaultJournalPath()
	if !filepath.IsAbs(p) {
		t.Fatalf("默认日志路径不是绝对路径: %q", p)
	}
}
