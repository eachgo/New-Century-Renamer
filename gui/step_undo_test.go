package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// undoApp 造一个装了 n 步的 app。步骤的 opUID 由 step() 自己发,所以这里
// 只需要保证选中项指向第一行。
func undoApp(t *testing.T, ids ...string) *app {
	t.Helper()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	for _, id := range ids {
		a.steps = append(a.steps, step(id))
	}
	return a
}

// 连着删两步,两次都得能撤回——原来只有一个槽,第一步永远回不来。
func TestConsecutiveDeletesAreAllUndoable(t *testing.T) {
	a := undoApp(t, "pinyin", "case.lower", "case.upper")
	a.deleteStep(0) // 删「文件名转拼音」
	a.deleteStep(0) // 删「全部转小写」
	if len(a.steps) != 1 || a.steps[0].opLabel != "全部转大写" {
		t.Fatalf("删两步之后应只剩「全部转大写」,实际 %d 步: %+v", len(a.steps), labels(a.steps))
	}
	if len(a.stepUndo) != 2 {
		t.Fatalf("撤回栈应有 2 项,实际 %d", len(a.stepUndo))
	}

	a.undoDelete()
	if len(a.steps) != 2 || a.steps[0].opLabel != "全部转小写" {
		t.Fatalf("第一次撤回应放回「全部转小写」在第 1 位,实际 %+v", labels(a.steps))
	}
	a.undoDelete()
	if len(a.steps) != 3 {
		t.Fatalf("第二次撤回应放回全部 3 步,实际 %d 步: %+v", len(a.steps), labels(a.steps))
	}
	if a.steps[0].opLabel != "文件名转拼音" {
		t.Fatalf("「文件名转拼音」应回到第 1 位,实际首位是 %q", a.steps[0].opLabel)
	}
	if len(a.stepUndo) != 0 {
		t.Fatal("全部撤回之后栈该空")
	}
}

// 「全部撤回」一次把连删的几步都放回去,且顺序/位置都对。
func TestUndoAllDeletedRestoresOrder(t *testing.T) {
	a := undoApp(t, "pinyin", "case.lower", "case.upper", "ext")
	a.deleteStep(1) // 删「全部转小写」(idx 1)
	a.deleteStep(1) // 删「全部转大写」(现在在 idx 1)
	if len(a.steps) != 2 {
		t.Fatalf("应剩 2 步,实际 %+v", labels(a.steps))
	}
	a.undoAllDeleted()
	want := []string{"文件名转拼音", "全部转小写", "全部转大写", "修改扩展名"}
	got := labels(a.steps)
	if len(got) != len(want) {
		t.Fatalf("全部撤回后应有 %d 步,实际 %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 步应是 %q,实际 %q(整体 %v)", i+1, want[i], got[i], got)
		}
	}
}

// 批量删除也必须能撤回:31 步里删错两个,原来没有任何退路。
func TestDeleteChosenIsUndoable(t *testing.T) {
	a := undoApp(t, "pinyin", "case.lower", "case.upper", "ext")
	a.chosen.Add(a.steps[1].opUID)
	a.chosen.Add(a.steps[2].opUID)
	a.deleteChosen()
	if len(a.steps) != 2 {
		t.Fatalf("批量删两步后应剩 2 步,实际 %+v", labels(a.steps))
	}
	if len(a.stepUndo) != 2 {
		t.Fatalf("批量删除应把两步都压进撤回栈,实际 %d 项", len(a.stepUndo))
	}
	a.undoAllDeleted()
	want := []string{"文件名转拼音", "全部转小写", "全部转大写", "修改扩展名"}
	got := labels(a.steps)
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("批量撤回后顺序不对: 想要 %v,实际 %v", want, got)
		}
	}
}

// 撤回条在屏幕上要说出"还有几步",并给出「全部撤回」。
func TestUndoStripOffersUndoAll(t *testing.T) {
	a := undoApp(t, "pinyin", "case.lower")
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	a.deleteStep(0)
	a.deleteStep(0)
	tt.Frame()
	if !tt.HasText("等 2 步") {
		t.Fatal("连删两步时撤回条应说出总数")
	}
	if err := tt.Click("全部撤回（2）"); err != nil {
		t.Fatalf("点不到「全部撤回（2）」: %v", err)
	}
	tt.Frame()
	if len(a.steps) != 2 {
		t.Fatalf("点「全部撤回」后应恢复 2 步,实际 %d", len(a.steps))
	}
	if tt.HasText("全部撤回") {
		t.Fatal("撤回栈空之后不该还留着「全部撤回」")
	}
}

// 把最后一步删掉之后,撤回条必须还在。
//
// 撤回条原来画在"有步骤"那个分支里:删光步骤的同时,退路也一起消失了
// ——而那正是最需要退路的时刻(中栏换成意图卡,用户看到的是一片空白)。
func TestUndoStripSurvivesDeletingTheLastStep(t *testing.T) {
	a := undoApp(t, "pinyin")
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	a.deleteStep(0)
	tt.Frame()
	if len(a.steps) != 0 {
		t.Fatalf("应删光步骤,实际还剩 %d 步", len(a.steps))
	}
	if !tt.HasText("已删除「文件名转拼音」") {
		t.Fatal("删光步骤后撤回条不见了,退路跟着一起消失")
	}
	if err := tt.Click("撤回"); err != nil {
		t.Fatalf("删光步骤后点不到撤回: %v", err)
	}
	tt.Frame()
	if len(a.steps) != 1 || a.steps[0].opLabel != "文件名转拼音" {
		t.Fatalf("撤回后应恢复那一步,实际 %+v", labels(a.steps))
	}
}

// 清空列表要能撤回,而且恢复的是整份快照:文件、顺序、排序依据。
func TestClearListIsUndoable(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = dir + "/journal.json"
	a.addPaths([]string{
		newTestFile(t, "a.txt"),
		newTestFile(t, "b.txt"),
		newTestFile(t, "c.txt"),
	})
	a.sortBy, a.sortDesc = "name", true
	before := len(a.files)
	if before != 3 {
		t.Fatalf("前置条件不成立,应有 3 个文件,实际 %d", before)
	}

	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	// 走真实路径:点「清空列表」→ 点「确认清空?」
	if err := tt.Click("清空列表"); err != nil {
		t.Fatalf("点不到「清空列表」: %v", err)
	}
	tt.Frame()
	if err := tt.Click("确认清空?"); err != nil {
		t.Fatalf("点不到「确认清空?」: %v", err)
	}
	tt.Frame()
	if len(a.files) != 0 {
		t.Fatalf("确认后清单应为空,实际 %d 个", len(a.files))
	}
	if len(a.clearedFiles) != before {
		t.Fatalf("清空时该存下 %d 个文件的快照,实际 %d", before, len(a.clearedFiles))
	}
	if !tt.HasText("撤回") {
		t.Fatal("清空之后应给出撤回入口")
	}
	if err := tt.Click("撤回"); err != nil {
		t.Fatalf("点不到撤回: %v", err)
	}
	tt.Frame()
	if len(a.files) != before {
		t.Fatalf("撤回后应恢复 %d 个文件,实际 %d", before, len(a.files))
	}
	if a.sortBy != "name" || !a.sortDesc {
		t.Fatalf("撤回后排序依据应一起恢复,实际 sortBy=%q sortDesc=%v", a.sortBy, a.sortDesc)
	}
	if a.nextOrder != 3 {
		t.Fatalf("撤回后序号计数器应恢复成 3,实际 %d", a.nextOrder)
	}
}

// 清空之后又加了新文件:旧的快照必须丢掉,否则撤回会把新加的文件整批盖掉。
func TestAddingFilesDropsStaleClearUndo(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.addPaths([]string{newTestFile(t, "a.txt")})
	a.clearedFiles = a.files
	a.clearedAt = time.Now()
	a.files, a.ops = nil, nil

	a.addPaths([]string{newTestFile(t, "b.txt")})
	if len(a.clearedFiles) != 0 {
		t.Fatalf("加入新文件后旧的清空快照该丢掉,实际还剩 %d 个", len(a.clearedFiles))
	}
}

// 清空撤回也有窗口:过期后快照要丢掉,不能一直占着。
func TestClearUndoExpires(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.addPaths([]string{newTestFile(t, "a.txt")})
	a.clearedFiles = a.files
	a.clearedOrder = a.nextOrder
	a.files, a.ops = nil, nil
	a.clearedAt = time.Now().Add(-undoDeleteWindow - time.Second)

	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if len(a.clearedFiles) != 0 {
		t.Fatal("过期后快照该丢掉")
	}
	if tt.HasText("已清空") {
		t.Fatal("过期后不该还画着撤回条")
	}
}

func labels(steps []guiRule) []string {
	out := make([]string, len(steps))
	for i := range steps {
		out[i] = steps[i].opLabel
	}
	return out
}
