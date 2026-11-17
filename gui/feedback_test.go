package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// ---- T0015:极窄右栏的结果点不能只靠颜色 ----

// 收掉状态词之后,结果列只剩一个标记。如果那个标记的形状不随状态变化,
// 色觉障碍用户看到的就是五个一样的小方块,灰度截图里也一样。
func TestResultMarkIsDistinctPerState(t *testing.T) {
	states := []rowState{rowPending, rowApplied, rowFailed, rowSkipped, rowUnchanged, rowCanceled}
	seen := map[string]rowState{}
	for _, s := range states {
		mark, label := resultMark(s)
		if mark == "" {
			t.Errorf("状态 %v 没有形状标记", s)
		}
		if label == "" {
			t.Errorf("状态 %v 没有说明文字", s)
		}
		if prev, dup := seen[mark]; dup {
			t.Errorf("状态 %v 与 %v 用了同一个形状 %q,灰度下分不出来", s, prev, mark)
		}
		seen[mark] = s
	}
}

// 有文字时不画形状标记(文字已经说明了状态),没文字时必须有。
//
// 不能用 tt.HasText:它搜的是整个窗口,底栏那句"0 个将更名"里也有
// "将更名",窄窗口下照样命中——这条断言就成了空转。改成直接读卡片
// 节点自己的可访问名,那才是这一行渲染出来的东西。
func TestResultMarkOnlyWhenTextIsHidden(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/j.json"
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	p := newTestFile(t, "ABC.TXT")
	a.files = []fileInfo{statFile(p)}
	a.preview()
	if len(a.ops) != 1 || a.ops[0].NewName == a.ops[0].OldName {
		t.Fatalf("夹具应当是一个会被改名的文件, ops=%d", len(a.ops))
	}
	// 文件卡片的可访问名是 "旧名 → 新名 · 状态",步骤行也是 RoleListItem,
	// 所以按"名字里含这个文件名"挑,不靠顺序。
	cardName := func(tt *ui.Tester) string {
		for _, n := range tt.AccessAll() {
			if n.Role == ui.RoleListItem && strings.Contains(n.Name, "ABC") {
				return n.Name
			}
		}
		return ""
	}
	// 宽窗口:卡片名里有「将更名」三个字
	wide := ui.NewTester(a.view, 1280, 800)
	wide.Frame()
	wide.Frame()
	if got := cardName(wide); !strings.Contains(got, "将更名") {
		t.Fatalf("宽窗口下结果列应有状态词,卡片名=%q", got)
	}
	// 极窄:状态词收起,改由形状承担。右栏内宽 = 900-16-10-splitL-10-splitM-20,
	// 要让 innerW<200 得让 splitL+splitM > 644。
	a.splitL, a.splitM = 500, 250
	narrow := ui.NewTester(a.view, 900, 800)
	narrow.Frame()
	narrow.Frame()
	got := cardName(narrow)
	if strings.Contains(got, "将更名") {
		t.Fatalf("极窄右栏下状态词应收起,卡片名=%q", got)
	}
	if !strings.Contains(got, "→") {
		t.Fatalf("极窄右栏下应有形状标记代替状态词,卡片名=%q", got)
	}
}

// ---- T0016:AI 步骤没配 Key 时要在步骤卡里说出来 ----

func TestAIStepSaysItNeedsConfig(t *testing.T) {
	t.Setenv("NCREAMER_AI_KEY", "")
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/j.json"
	a.steps = []guiRule{step("ai")}
	a.stepUID = a.steps[0].opUID
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("还没填 API Key") {
		t.Fatal("AI 步骤在没配 Key 时应就地说明,而不是等执行后才整列变红")
	}
	if !tt.HasText("去配置") {
		t.Fatal("应给出「去配置」入口")
	}
}

func TestAIStepHintGoesAwayOnceConfigured(t *testing.T) {
	t.Setenv("NCREAMER_AI_KEY", "sk-test")
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/j.json"
	a.steps = []guiRule{step("ai")}
	a.stepUID = a.steps[0].opUID
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	if tt.HasText("还没填 API Key") {
		t.Fatal("配好 Key 之后提示应消失")
	}
}

// 「去配置」必须真的打开设置面板,不是一个装饰性的按钮。
func TestAIStepConfigureButtonOpensSettings(t *testing.T) {
	t.Setenv("NCREAMER_AI_KEY", "")
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/j.json"
	a.steps = []guiRule{step("ai")}
	a.stepUID = a.steps[0].opUID
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	if err := tt.Click("去配置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.settingsOpen {
		t.Fatal("点「去配置」应打开设置面板")
	}
}

// ---- T0018:空 replace 步骤不该一出现就是红字 ----

func TestFreshReplaceStepIsNotAnErrorYet(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/j.json"
	a.steps = []guiRule{step("replace")}
	a.stepUID = a.steps[0].opUID
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	if tt.HasText("查找内容不能为空") {
		t.Fatal("刚建出来的替换步骤不该报红——用户什么都还没做")
	}
	if !tt.HasText("填上要查找的内容") {
		t.Fatal("未触碰时应给中性提示,说清这一步会怎么走")
	}
}

func TestClearedReplaceStepTurnsIntoAnError(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/j.json"
	a.steps = []guiRule{step("replace")}
	a.stepUID = a.steps[0].opUID
	a.steps[0].oldStr = "abc"
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	// 填过之后再清空:这时才是真的填错了
	a.steps[0].oldStr = ""
	tt.Frame()
	if !tt.HasText("查找内容不能为空") {
		t.Fatal("填过又清空应转成红色报错")
	}
}

// ---- T0019:整轮预览错误要指出是哪一步 ----

func TestPreviewErrorNamesTheStep(t *testing.T) {
	a := &app{stepSel: 1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/j.json"
	a.steps = []guiRule{step("case.lower"), step("tpl")}
	for i := range a.steps {
		a.stepUID++
		a.steps[i].opUID = a.stepUID
	}
	// template= 没有模板参数是 core 的硬错误(坏模板本身是合法的,
	// 未知名只在 Apply 时报)。用空模板触发整轮失败。
	a.steps[1].tpl = ""
	a.preview()
	if a.previewErr == "" {
		t.Fatal("坏模板应让整轮预览算不出来")
	}
	if !strings.Contains(a.previewErr, "第 2 步") {
		t.Fatalf("错误信息应点明是第几步,实际 %q", a.previewErr)
	}
	if !strings.Contains(a.previewErr, "自定义模板") {
		t.Fatalf("错误信息应点明是哪一步(步骤名),实际 %q", a.previewErr)
	}
	if strings.Contains(a.previewErr, "规则 2") {
		t.Fatalf("不该把 core 的规则号直接摆给用户: %q", a.previewErr)
	}
}

// 规则号会跳过空 spec 的步骤,翻译必须跟着跳——否则报的是错的那一行。
func TestPreviewErrorSkipsEmptySpecs(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/j.json"
	empty := step("case.lower")
	empty.kind = ""
	a.steps = []guiRule{empty, step("case.lower"), step("tpl")}
	for i := range a.steps {
		a.stepUID++
		a.steps[i].opUID = a.stepUID
	}
	a.steps[2].tpl = ""
	a.preview()
	if !strings.Contains(a.previewErr, "第 3 步") {
		t.Fatalf("空 spec 的步骤不该被算进规则号,应报第 3 步,实际 %q", a.previewErr)
	}
}

// ---- T0020:窄栏下起始/步长各带自己的标签 ----

// exact 按可访问名精确找,不用 HasText——HasText 是子串匹配,
// "起始 / 步长" 里也含 "起始",用它断言等于什么都没测。
func exact(tt *ui.Tester, name string) (ui.Rect, bool) {
	for _, n := range tt.AccessAll() {
		if n.Name == name {
			return n.Bounds, true
		}
	}
	return ui.Rect{}, false
}

func TestSeqRangeFieldsAreLabelledWhenStacked(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/j.json"
	a.steps = []guiRule{step("seq")}
	a.stepUID = a.steps[0].opUID
	// 150 DIP 一定落在 stacked 那一档
	a.splitL, a.splitM = 220, 150
	tt := ui.NewTester(a.view, 900, 720)
	tt.Frame()
	tt.Frame()
	if _, ok := exact(tt, "起始"); !ok {
		t.Fatal("窄栏下应有「起始」标签")
	}
	if _, ok := exact(tt, "步长"); !ok {
		t.Fatal("窄栏下应有「步长」标签")
	}
	if _, ok := exact(tt, "起始 / 步长"); ok {
		t.Fatal("窄栏下不该再用合在一起的标签——折行之后第二个框就没有说明了")
	}
	// 两个标签各占一行,不是并排
	start, _ := exact(tt, "起始")
	step, _ := exact(tt, "步长")
	if start.Y == step.Y {
		t.Fatalf("两个标签应各占一行,实际同一行 y=%.1f", start.Y)
	}
}

func TestSeqRangeFieldsStayOnOneLineWhenWide(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/j.json"
	a.steps = []guiRule{step("seq")}
	a.stepUID = a.steps[0].opUID
	a.splitL, a.splitM = 220, 330
	tt := ui.NewTester(a.view, 1280, 720)
	tt.Frame()
	tt.Frame()
	if _, ok := exact(tt, "起始 / 步长"); !ok {
		t.Fatal("宽栏下应保持一行两组,不必拆开")
	}
	if _, ok := exact(tt, "起始"); ok {
		t.Fatal("宽栏下不该拆成两个标签")
	}
}

// 顺带守一下:撤回槽过期后必须真的清掉,不能只是不画。
func TestUndoSlotIsClearedNotJustHidden(t *testing.T) {
	a := &app{stepSel: 0, settings: defaultSettings()}
	a.steps = []guiRule{step("pinyin"), step("case.lower")}
	for i := range a.steps {
		a.stepUID++
		a.steps[i].opUID = a.stepUID
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	a.deleteStep(0)
	last, _ := a.lastStepUndo()
	last.at = time.Now().Add(-undoDeleteWindow - time.Second)
	a.stepUndo[len(a.stepUndo)-1] = last
	tt.Frame()
	if len(a.stepUndo) != 0 {
		t.Fatal("过期后撤回栈该清空")
	}
	// 再删一步:上一次的残留不该被撤回回来
	a.deleteStep(0)
	if last, ok := a.lastStepUndo(); !ok || last.step.opLabel != "全部转小写" {
		t.Fatalf("撤回栈记错了: %+v", a.stepUndo)
	}
}
