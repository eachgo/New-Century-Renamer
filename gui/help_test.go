package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 帮助入口必须在标题栏、任何窗口高度都点得到:34 项功能、9 种模板变量、
// NFO 编号、CSV 回填,靠试是学不会的,而学不会的入口等于没有。
func TestHelpReachableAtAnyHeight(t *testing.T) {
	for _, h := range []int{480, 720, 1080} {
		a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
		tt := ui.NewTester(a.view, 1280, h)
		tt.Frame()
		tt.Frame()

		r, ok := tt.Find("?")
		if !ok {
			t.Fatalf("h=%d 标题栏里没有帮助入口", h)
		}
		if r.Y > 60 {
			t.Fatalf("h=%d 帮助入口在 y=%.1f,不在标题栏", h, r.Y)
		}
		if err := tt.Click("?"); err != nil {
			t.Fatalf("h=%d 点不开帮助: %v", h, err)
		}
		tt.Frame()
		if !a.helpOpen {
			t.Fatalf("h=%d 点了帮助但对话框没开", h)
		}
	}
}

// F1 也要能开,而且 Esc 要能关——开了关不掉的模态是最糟的。
func TestHelpOpensWithF1AndClosesWithEsc(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	tt.Key(0, ui.KeyF1)
	tt.Frame()
	if !a.helpOpen {
		t.Fatal("F1 没有打开帮助")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.helpOpen {
		t.Fatal("Esc 没有关掉帮助")
	}
}

// 帮助内容得真的覆盖两条路和模板变量:写一个空壳对话框等于没做。
//
// 以前这里断言的是五条编号步骤。那五条已经删了——图和它下面那几句
// 把同一件事说完了,再列一遍编号步骤,读者会在两套说法之间对不上号。
// 现在断言的是"两条路都走到了撤回"这件事:图里画的字不在元素树里,
// 所以靠图的文字替代来验,那是读屏软件读到的同一句。
func TestHelpExplainsTheWholeFlow(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	tt.Frame()
	tt.Key(0, ui.KeyF1)
	tt.Frame()

	var label string
	for _, n := range tt.AccessAll() {
		if strings.HasPrefix(n.Name, "流程图：两条起手方式") {
			label = n.Name
			break
		}
	}
	if label == "" {
		t.Fatal("帮助里没有主流程图")
	}
	// 第一条路:先加文件。第二条路:先排步骤。两条都要有。
	for _, want := range []string{"增加文件", "调整步骤", "预览", "执行更名", "撤回"} {
		if !strings.Contains(label, want) {
			t.Errorf("主流程图的文字替代里没提到 %q: %q", want, label)
		}
	}
	if !strings.Contains(label, "预设") && !strings.Contains(label, "添加步骤") {
		t.Errorf("主流程图没说第二条路从预设/添加步骤起手: %q", label)
	}

	// 存档那条小流程与主流程是两件事,得分开说。
	for _, want := range []string{"这套排法可以带走", "存成流程", "导出设置"} {
		if !tt.HasText(want) {
			t.Errorf("帮助里没有 %q", want)
		}
	}
	for _, want := range []string{"<META:title>", "<NFO:episode>"} {
		if !tt.HasText(want) {
			t.Errorf("帮助里没有 %q", want)
		}
	}
	// 五步文字删掉之后不能再冒出来。
	for _, gone := range []string{"五步就够", "先放文件，或者先排步骤", "改坏了就回滚"} {
		if tt.HasText(gone) {
			t.Errorf("五步文字还在: %q", gone)
		}
	}
}

// 模板框旁边的"插入变量"要真的把变量写进模板,否则它只是个装饰。
func TestVarPickerInsertsIntoTemplate(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	tt.Frame()

	if err := tt.Click("用文件里的标题改名"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	before := a.steps[0].tpl

	if err := tt.Click("插入变量"); err != nil {
		t.Fatalf("插入变量按钮: %v", err)
	}
	tt.Frame()
	if len(tt.Menu()) == 0 {
		t.Fatal("插入变量没有弹出菜单")
	}
	if err := tt.ChooseMenuItem("<META:artist>  歌手"); err != nil {
		t.Fatalf("选变量: %v", err)
	}
	tt.Frame()
	if a.steps[0].tpl != before+"<META:artist>" {
		t.Fatalf("变量没插进去: %q → %q", before, a.steps[0].tpl)
	}
}

// AI 变量插进去的必须是能直接用的指示,不能是一个省略号。
func TestVarPickerInsertsUsableAIPlaceholder(t *testing.T) {
	var got string
	for _, v := range templateVars {
		if v.name == "<AI:…>" {
			got = v.insert
		}
	}
	if got == "" {
		t.Fatal("模板变量表里没有 AI 变量")
	}
	if got == "<AI:…>" {
		t.Fatalf("插进模板的是省略号,用户还得自己猜要写什么: %q", got)
	}
}

// 重新打开要从顶部开始:接着上次滚到的位置会让人以为内容变了。
func TestHelpReopensAtTop(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1000, 480)
	tt.Frame()
	tt.Frame()

	tt.Key(0, ui.KeyF1)
	tt.Frame()
	for i := 0; i < 4; i++ {
		tt.Scroll(500, 300, 0, 200)
		tt.Frame()
	}
	if a.helpScroll.Y == 0 {
		t.Fatal("测试没滚到底,后面的断言没有意义")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	tt.Key(0, ui.KeyF1)
	tt.Frame()
	if a.helpScroll.Y != 0 {
		t.Fatalf("重新打开时停在 y=%.1f,应当回到顶部", a.helpScroll.Y)
	}
}

// 小窗口里内容放不下时得能滚到最底下那行快捷键,否则等于没写。
func TestHelpScrollsToShortcutLine(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1000, 480)
	tt.Frame()
	tt.Frame()
	tt.Key(0, ui.KeyF1)
	tt.Frame()

	if a.helpScroll.MaxY <= 0 {
		t.Fatal("480 高的窗口里帮助内容应当溢出,否则这条测试没在测滚动")
	}
	// 快捷键那几行越写越长(键盘导航的说明也在里面),滚 4 屏不一定够。
	// 一直滚到 Scroll 的 MaxY 不再变化为止,再断言内容可见——测的是
	// "能不能滚到底",不是"滚几次能到底"。
	for i := 0; i < 20; i++ {
		last := a.helpScroll.Y
		tt.Scroll(500, 300, 0, 200)
		tt.Frame()
		if a.helpScroll.Y == last {
			break
		}
	}
	// 断言用 shortcutTip 拼出来的片段,不写死 "Ctrl+"。帮助页在 macOS 上
	// 显示的是 ⌘/⌥,把 Ctrl 写进断言等于把那个 bug 钉成契约——真去修
	// 跨平台显示时,这条测试会在苹果机器上先红。
	for _, want := range []string{
		shortcutTip("Ctrl+D") + " 复制步骤",
		shortcutTip("Alt+↑↓"),
		shortcutTip("Ctrl+,") + " 设置",
		"F1 帮助",
	} {
		if !tt.HasText(want) {
			t.Fatalf("滚到底也看不到快捷键说明里的 %q(实际修饰键按本平台换算)", want)
		}
	}
}
