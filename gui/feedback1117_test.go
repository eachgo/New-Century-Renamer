package main

// 1117 反馈第一梯队（P0，T0021–T0025）的回归测试。
//
// 这五条病灶在报告里被列成 P0，但它们原本连一条测试都没有——报告附的
// 七个探针名（TestTrunkTest 等）在仓库里 grep 全为 0，是对着更早的快照
// 写的。这里按当前代码重写等价断言，防止它们再退回去。

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// T0021：套用流程会替换现有步骤，必须先确认；确认后被替换的旧步骤要能
// 整批撤回。修复前：一次点击 3 步→1 步，无确认、无恢复通道。
func TestFlowReplaceNeedsConfirmAndIsRecoverable(t *testing.T) {
	a := flowApp(t) // 3 步
	f := flowFromRules("两步", stepsFromIDs("case.lower", "truncate"))

	a.applyFlow(f, true)
	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if len(a.steps) != 3 {
		t.Fatalf("还没确认就把步骤换掉了:%d 步", len(a.steps))
	}

	a.applyFlow(f, true) // 第二次 = 确认
	if len(a.steps) != 2 {
		t.Fatalf("确认后应有 2 步,实际 %d", len(a.steps))
	}
	if len(a.recoveredSteps) != 3 {
		t.Fatalf("被替换的 3 步应进撤回通道,实际 %d", len(a.recoveredSteps))
	}

	a.undoFlowReplace()
	if len(a.steps) != 3 {
		t.Fatalf("撤回后应回到 3 步,实际 %d", len(a.steps))
	}
}

// T0022：目录条目的说明必须挂在可聚焦的外层按钮上，键盘/读屏才拿得到。
// 修复前说明挂在内层 Text 上，外层按钮 Description 为空。
func TestCatalogItemTooltipReachesKeyboard(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	tt.Frame()

	var want string
	for _, d := range catalog {
		if d.id == "replace" {
			want = d.desc
		}
	}
	if want == "" {
		t.Fatal("测试前提错误:replace 条目没有说明")
	}
	found := false
	for _, n := range tt.AccessAll() {
		if n.Role == ui.RoleButton && n.Name == "查找替换" {
			found = true
			if n.Description != want {
				t.Errorf("目录按钮的说明为空或不对: %q,期望 %q", n.Description, want)
			}
			if !n.Focusable {
				t.Error("目录条目应当可聚焦,否则键盘拿不到说明")
			}
		}
	}
	if !found {
		t.Fatal("找不到「查找替换」按钮节点")
	}
}

// T0023：空态的四张意图卡必须是可聚焦的按钮，而不只是 Column。
// 修复前它们是 ui.Column，Tab 序列里 0 站、Enter 也按不动。
func TestIntentCardsAreFocusableButtons(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	label := intents[0].label
	hits := 0
	for _, n := range tt.AccessAll() {
		if n.Role == ui.RoleButton && len(n.Name) >= len(label) && n.Name[:len(label)] == label {
			hits++
			if !n.Focusable {
				t.Errorf("意图卡「%s」应当可聚焦", label)
			}
		}
	}
	if hits == 0 {
		t.Fatalf("意图卡「%s」不是按钮(Tab 到不了)", label)
	}
	// 点一下要真的加步骤。
	if err := tt.Click(label); err != nil {
		t.Fatalf("点意图卡失败: %v", err)
	}
	tt.Frame()
	if len(a.steps) == 0 {
		t.Fatal("点意图卡没有加入任何步骤")
	}
}

// T0024：执行更名要两次点击。修复前一次点击直接落盘。
func TestApplyNeedsSecondClick(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "OLD.TXT")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := step("case.lower") // OLD.TXT -> old.txt
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings(),
		files: []fileInfo{statFile(p)}, steps: []guiRule{d}}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	if !a.hasChanges() {
		t.Fatal("测试前提错误:应当有待更名的文件")
	}
	if err := tt.Click("执行更名"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, err := os.Stat(p); err != nil {
		t.Fatal("第一次点击就把文件改了,应该只是进入确认态")
	}
	if !a.confirmApply {
		t.Fatal("第一次点击未进入确认态")
	}
	if err := tt.Click("确认执行?"); err != nil {
		t.Fatalf("确认按钮不存在: %v", err)
	}
	// 落盘是异步的,等它跑完。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "old.txt")); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("确认后文件没有被更名")
}

// T0025：默认展开的必须是最高频的两组。修复前按书写顺序取前两组
// （筛选/范围、转换），把最冷门的两组展开了。
func TestDefaultOpenGroupsAreMostUsed(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	want := stepGroups[:defaultOpenGroups]
	for _, g := range want {
		if !tt.HasText("▾ " + g) {
			t.Errorf("默认应当展开高频组「%s」,实际没展开", g)
		}
	}
	for _, g := range stepGroups[defaultOpenGroups:] {
		if tt.HasText("▾ " + g) {
			t.Errorf("低频组「%s」不该默认展开", g)
		}
	}
	// 前两组至少要是"替换""结构"这一类改名高频组,而不是历史书写顺序的头部。
	if len(want) < 2 || want[0] != "替换" || want[1] != "结构" {
		t.Fatalf("默认展开组不对: %v", want)
	}
}
