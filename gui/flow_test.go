package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// flowApp 造一个带三条步骤的 app:预置、带参数、跨分组各一条。
func flowApp(t *testing.T) *app {
	t.Helper()
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	a.steps = stepsFromIDs("del.re", "seq.prefix", "case.lower")
	for i := range a.steps {
		a.stepUID++
		a.steps[i].opUID = a.stepUID
	}
	a.steps[0].oldStr = `（[^）]*）`
	a.steps[1].seqWidth = 3
	return a
}

// 存下来再读回来,必须一模一样——这是"用户可保存的流程"的全部意义。
func TestFlowRoundTripsThroughSettings(t *testing.T) {
	a := flowApp(t)
	a.saveFlow("我的常用流程")

	if len(a.settings.Flows) != 1 {
		t.Fatalf("存了 %d 条流程,期望 1", len(a.settings.Flows))
	}
	f := a.settings.Flows[0]
	if f.Name != "我的常用流程" {
		t.Errorf("流程名 = %q", f.Name)
	}
	if len(f.Steps) != 3 {
		t.Fatalf("流程里有 %d 步,期望 3", len(f.Steps))
	}
	// 参数被改过的那两步要带着参数,没改的第三步不该写冗余键。
	if f.Steps[0].Args["oldStr"] != `（[^）]*）` {
		t.Errorf("替换步骤的查找内容没存下来: %v", f.Steps[0].Args)
	}
	if f.Steps[1].Args["seqWidth"] != "3" {
		t.Errorf("编号步骤的补零位数没存下来: %v", f.Steps[1].Args)
	}
	if len(f.Steps[2].Args) != 0 {
		t.Errorf("没改过的步骤不该写参数,实际 %v", f.Steps[2].Args)
	}

	// 读回来:清空当前步骤,套用流程,参数必须还在。
	a.steps = nil
	a.applyFlow(f, true)
	if len(a.steps) != 3 {
		t.Fatalf("套用后有 %d 步,期望 3", len(a.steps))
	}
	if a.steps[0].oldStr != `（[^）]*）` {
		t.Errorf("套用后查找内容 = %q", a.steps[0].oldStr)
	}
	if a.steps[1].seqWidth != 3 {
		t.Errorf("套用后补零位数 = %v,期望 3", a.steps[1].seqWidth)
	}
	// opID 与界面信息也要回来,否则步骤行会显示成空标签。
	if a.steps[1].opID != "seq.prefix" || a.steps[1].opLabel == "" {
		t.Errorf("套用后步骤身份丢了: id=%q label=%q", a.steps[1].opID, a.steps[1].opLabel)
	}
}

// 同名再存一次是覆盖,不是攒第二条。
func TestSavingSameNameOverwrites(t *testing.T) {
	a := flowApp(t)
	a.saveFlow("常用")
	a.steps = a.steps[:1]
	a.saveFlow("常用")

	if len(a.settings.Flows) != 1 {
		t.Fatalf("同名保存后 %d 条,期望 1", len(a.settings.Flows))
	}
	if n := len(a.settings.Flows[0].Steps); n != 1 {
		t.Errorf("覆盖后是 %d 步,期望 1(取最新的那次)", n)
	}
}

// 内置预设不进设置文件,否则升级后预设就成了旧快照。
func TestBuiltinFlowsAreNotStored(t *testing.T) {
	a := flowApp(t)
	a.saveFlow("只有这条该进设置")

	for _, f := range a.settings.Flows {
		if f.Builtin {
			t.Errorf("内置预设被写进了设置文件: %q", f.Name)
		}
	}
	// 但列表里必须看得到它们。
	all := a.allFlows()
	builtin := 0
	for _, f := range all {
		if f.Builtin {
			builtin++
		}
	}
	if builtin != len(intents) {
		t.Errorf("流程列表里有 %d 条预设,期望 %d", builtin, len(intents))
	}
	if len(all) != len(intents)+1 {
		t.Errorf("流程列表 %d 条,期望 %d", len(all), len(intents)+1)
	}
}

// 排到一半也能打开流程弹窗——这是这条反馈的核心。
func TestFlowDialogOpensWithStepsAlreadyThere(t *testing.T) {
	a := flowApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	// 有步骤时,中栏不再是空态,意图卡不在。
	if tt.HasText("想改成什么样？点一张，我把步骤配好") {
		t.Fatal("有步骤时不该还摆着空态意图卡")
	}

	// 左栏"高级"组的"自定义流程"必须能点开弹窗。用搜索把它顶到第一条:
	// 直接点要先把左栏滚到"高级"组,而那条路本身另有测试守着。
	a.palQuery = "自定义流程"
	tt.Frame()
	if err := tt.Click("自定义流程"); err != nil {
		t.Fatalf("点「自定义流程」: %v", err)
	}
	tt.Frame()
	if !a.flowOpen {
		t.Fatal("点了「自定义流程」但弹窗没打开")
	}
	if !tt.HasText("流程") {
		t.Error("弹窗里没有标题")
	}
	// 弹窗里要能看到"存成流程"的入口和至少一条预设。
	if !tt.HasText("把现在的步骤存成流程") {
		t.Error("弹窗里没有保存入口")
	}
	if !tt.HasText(intents[0].label) {
		t.Errorf("弹窗里没有预设 %q", intents[0].label)
	}
}

// 套用会替换当前步骤;追加模式接在后面。
func TestApplyFlowReplacesOrAppends(t *testing.T) {
	a := flowApp(t)
	f := flowFromRules("两步", stepsFromIDs("case.lower", "truncate"))

	a.applyFlow(f, true)
	if len(a.steps) != 2 {
		t.Errorf("替换后有 %d 步,期望 2", len(a.steps))
	}

	a.applyFlow(f, false)
	if len(a.steps) != 4 {
		t.Errorf("追加后有 %d 步,期望 4", len(a.steps))
	}
	// 追加不能把已有步骤的身份搞乱:opUID 必须全不同。
	seen := map[int]bool{}
	for _, s := range a.steps {
		if s.opUID == 0 || seen[s.opUID] {
			t.Fatalf("步骤 opUID 重复或为零: %d", s.opUID)
		}
		seen[s.opUID] = true
	}
}

// 流程里出现已下线的 opID 时,跳过它而不是让整条流程作废。
func TestUnknownStepIsSkippedNotFatal(t *testing.T) {
	f := flow{
		Name: "含已下线步骤",
		Steps: []flowStep{
			{ID: "case.lower"},
			{ID: "op.that.no.longer.exists"},
			{ID: "truncate"},
		},
	}
	rules := f.rules()
	if len(rules) != 2 {
		t.Fatalf("还原出 %d 步,期望 2(跳过未知的)", len(rules))
	}
	if rules[0].opID != "case.lower" || rules[1].opID != "truncate" {
		t.Errorf("顺序错了: %q, %q", rules[0].opID, rules[1].opID)
	}
}

// 流程摘要要读得懂:步骤名按顺序连起来。
func TestFlowSummaryListsStepsInOrder(t *testing.T) {
	f := flowFromRules("x", stepsFromIDs("case.lower", "truncate"))
	got := flowSummary(f)
	if !strings.Contains(got, "→") {
		t.Errorf("摘要里没有箭头: %q", got)
	}
	lower := strings.Index(got, "全部转小写")
	trunc := strings.Index(got, "截断")
	if lower < 0 || trunc < 0 || lower > trunc {
		t.Errorf("摘要顺序不对: %q", got)
	}
}

// 删除只对用户存的流程开放;内置的删了下次还会回来,所以根本不该有按钮。
func TestDeleteFlowOnlyAffectsUserFlows(t *testing.T) {
	a := flowApp(t)
	a.saveFlow("A")
	a.saveFlow("B")
	if len(a.settings.Flows) != 2 {
		t.Fatalf("存了 %d 条", len(a.settings.Flows))
	}
	a.deleteFlow("A")
	if len(a.settings.Flows) != 1 || a.settings.Flows[0].Name != "B" {
		t.Errorf("删完之后: %+v", a.settings.Flows)
	}
	// 删一个内置预设的名字,不该动到用户流程。
	a.deleteFlow(intents[0].label)
	if len(a.settings.Flows) != 1 {
		t.Errorf("删预设名不该影响用户流程,实际 %d 条", len(a.settings.Flows))
	}
}
