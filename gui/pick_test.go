package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func pickApp(t *testing.T) *app {
	t.Helper()
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	a.steps = []guiRule{step("pinyin"), step("scope.name"), step("case.lower"), step("truncate")}
	for i := range a.steps {
		a.stepUID++
		a.steps[i].opUID = a.stepUID
	}
	return a
}

// TestCheckboxSitsBeforeStepNumber 勾选框必须在编号之前。
//
// 以前它在行尾:类别胶囊、范围色条之后。读一行流程("1. 全部转小写
// 小写 大小写")要在半路绕过那个方框,而多选开着的时候每一行都多一个
// 方框横在流程中间。
func TestCheckboxSitsBeforeStepNumber(t *testing.T) {
	a := pickApp(t)
	a.pickMode = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	box, ok := tt.Find("选中第 1 步")
	if !ok {
		t.Fatal("找不到第一行的勾选框")
	}
	num, ok := tt.Find("1.")
	if !ok {
		t.Fatal("找不到编号「1.」")
	}
	if box.X >= num.X {
		t.Errorf("勾选框 x=%.2f 在编号 x=%.2f 右边(应在编号之前)", box.X, num.X)
	}
	// 同一行的操作名必须还在编号右边,否则顺序就反了。操作名在左栏
	// 目录里也有同名条目,所以按"和编号同一行(y 相近)且在其右侧"来找。
	found := false
	for _, x := range tt.AccessAll() {
		if x.Role != ui.RoleText || x.Name != "文件名转拼音" {
			continue
		}
		if x.Bounds.Y < num.Y-2 || x.Bounds.Y > num.Y+num.H+2 {
			continue
		}
		found = true
		if x.Bounds.X <= num.X {
			t.Errorf("操作名 x=%.2f 没在编号 x=%.2f 右边", x.Bounds.X, num.X)
		}
	}
	if !found {
		t.Fatal("在编号那一行找不到操作名")
	}
}

// TestCheckboxSlotKeepsRowFromShifting 勾选一行不能让整行文字横移。
//
// 槽位必须在"多选模式开着"时就恒占着,而不是"选中了才画"。否则勾上
// 第一行的瞬间,那一行的编号与操作名整体右移 16 DIP——读流程时最刺眼
// 的就是这种跳动。
func TestCheckboxSlotKeepsRowFromShifting(t *testing.T) {
	a := pickApp(t)
	a.pickMode = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	before, ok := tt.Find("1.")
	if !ok {
		t.Fatal("找不到编号")
	}

	a.chosen.Add(a.steps[0].opUID)
	tt.Frame()
	after, ok := tt.Find("1.")
	if !ok {
		t.Fatal("勾选后找不到编号")
	}
	if d := after.X - before.X; d > 0.5 || d < -0.5 {
		t.Errorf("勾选后编号 x 从 %.2f 变成 %.2f(差 %.2f DIP),整行横移了", before.X, after.X, d)
	}
}

// TestBatchBarHasSelectAllAndInvert 批量操作里要有全选与反选。
//
// 只有"删除所选 + 全不选"时,要删中间几步只能一个个点;而"先全选再
// 点掉不要的"更接近人想做的事。
func TestBatchBarHasSelectAllAndInvert(t *testing.T) {
	a := pickApp(t)
	a.pickMode = true
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	// 一个都没选时也要看得到这两个按钮——"全选"最需要它的时刻就是现在。
	for _, want := range []string{"全选", "反选", "全不选", "已选 0 步"} {
		if !tt.HasText(want) {
			t.Errorf("多选面板缺 %q", want)
		}
	}
	if err := tt.Click("全选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.chosen.Len() != len(a.steps) {
		t.Fatalf("全选后应选 %d 步,实际 %d", len(a.steps), a.chosen.Len())
	}
	if err := tt.Click("反选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.chosen.Len() != 0 {
		t.Fatalf("全选后反选应清空,实际 %d", a.chosen.Len())
	}
	// 反选不是清空:再点两下应当是"选中的变没选、没选的变选中"。
	a.chosen.Add(a.steps[0].opUID)
	a.chosen.Add(a.steps[1].opUID)
	tt.Frame()
	if err := tt.Click("反选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.chosen.Len() != 2 || a.chosen.Has(a.steps[0].opUID) || !a.chosen.Has(a.steps[2].opUID) {
		t.Fatalf("反选结果不对: len=%d has0=%v has2=%v",
			a.chosen.Len(), a.chosen.Has(a.steps[0].opUID), a.chosen.Has(a.steps[2].opUID))
	}
}

// TestBatchBarHiddenWithoutPickMode 不开多选、也没选中项时不占地方。
func TestBatchBarHiddenWithoutPickMode(t *testing.T) {
	a := pickApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if tt.HasText("全选") || tt.HasText("已选 0 步") {
		t.Error("没开多选时不该出现批量操作条")
	}
}

// TestCheckboxOnlyWhenPicking 勾选框只在"确实在多选"时出现。
//
// 以前条件是"多选模式开着、或本行被悬停、或已选中"。悬停那一条让勾选框
// 在正常浏览时随机出现又消失:一列步骤里突然多出一个方框,读流程的人会
// 以为那一步有什么特殊状态。现在只剩"开了多选"或"已经有选中项"两种,
// 后者是边界兜底——选中项在模式关掉后仍留着时,得有地方取消它。
//
// 注意这里守的是"模式开关"这一半:悬停那一半守不住,因为无头 Tester 的
// Move 不产生悬停链(实测加回 row.Hovered() 后本测试仍 PASS)。悬停归
// 手动检查,别把这条当成它的守卫。
func TestCheckboxOnlyWhenPicking(t *testing.T) {
	a := pickApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if tt.HasText("选中第 1 步") {
		t.Error("没开多选、也没选中项时不该有勾选框")
	}

	a.pickMode = true
	tt.Frame()
	if !tt.HasText("选中第 1 步") {
		t.Error("开了多选就该有勾选框")
	}

	// 关掉多选但留着选中项:勾选框还在,否则那一项无从取消。
	a.pickMode = false
	a.chosen.Add(a.steps[0].opUID)
	tt.Frame()
	if !tt.HasText("选中第 1 步") {
		t.Error("有选中项时勾选框应当保留,否则取消不了")
	}
	a.chosen.Clear()
	tt.Frame()
	if tt.HasText("选中第 1 步") {
		t.Error("清空选中项后勾选框该消失")
	}
}
