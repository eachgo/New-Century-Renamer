package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// 执行完之后要能说清"刚才发生了什么、还能不能退"。
// 以前只有左下角一行 12px 灰字,而改名是当天最重要的事。
func TestAppliedBannerShowsWhenAndHowMany(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	for _, n := range []string{"A.TXT", "B.TXT"} {
		writeTestFile(t, filepath.Join(dir, n), "x")
	}
	a.addPaths([]string{filepath.Join(dir, "A.TXT"), filepath.Join(dir, "B.TXT")})
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	// 先跑几帧让实时预览把 lastSpec 记下来,否则执行完的第一帧
	// 会以为"步骤变了"重新预览,把 Applied 抹掉
	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	a.apply()
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if !tt.HasText("已执行") {
		t.Fatalf("执行完没有横幅: %v", tt.Texts())
	}
	if !tt.HasText("2 个文件") {
		t.Fatalf("横幅没说清改了几个: %v", tt.Texts())
	}
	if !tt.HasText("撤销这次") {
		t.Fatalf("横幅上没有撤销入口: %v", tt.Texts())
	}
}

// 执行过的行说"已更名",不再说"成功":
// "成功"是结果,"已更名"才是这一行现在的状态,能跟"将更名"对上。
func TestAppliedRowSaysRenamed(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	writeTestFile(t, filepath.Join(dir, "A.TXT"), "x")
	a.addPaths([]string{filepath.Join(dir, "A.TXT")})
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	a.apply()
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if !tt.HasText("已更名") {
		t.Fatalf("执行过的行没说已更名: %v", tt.Texts())
	}
}

// 回滚之后横幅必须消失:那一批已经不存在了,
// 继续说"已执行 N 个文件"是假话。
func TestAppliedBannerClearsAfterUndo(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	writeTestFile(t, filepath.Join(dir, "A.TXT"), "x")
	a.addPaths([]string{filepath.Join(dir, "A.TXT")})
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	a.apply()
	if a.appliedCount == 0 {
		t.Fatal("执行后没有记下成功数")
	}

	a.undo()
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if tt.HasText("已执行") {
		t.Fatalf("回滚后横幅还在: %v", tt.Texts())
	}
	if a.appliedAt != (time.Time{}) || a.appliedCount != 0 {
		t.Fatalf("回滚后没清干净: at=%v count=%d", a.appliedAt, a.appliedCount)
	}
}
