package main

import (
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

// 帮助内容得真的覆盖五个步骤和模板变量:写一个空壳对话框等于没做。
func TestHelpExplainsTheWholeFlow(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	tt.Frame()
	tt.Key(0, ui.KeyF1)
	tt.Frame()

	for _, want := range []string{
		"五步就够",
		"把文件拖进来",
		"说清想改成什么",
		"调参数，看右栏",
		"确认没问题再执行",
		"改坏了就回滚",
		"<META:title>",
		"<NFO:episode>",
	} {
		if !tt.HasText(want) {
			t.Fatalf("帮助里没有 %q", want)
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
