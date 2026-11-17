package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Ctrl+D 复制当前步骤:鼠标的「复制」和键盘必须走同一条路,
// 否则两条路径迟早会长歪(一边复制参数、一边忘掉)。
func TestShortcutDuplicateStep(t *testing.T) {
	a := &app{stepSel: 0, settings: defaultSettings()}
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID // 测试辅助函数自己发号,和 app 的自增计数器对不上
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyD)
	tt.Frame()
	if len(a.steps) != 2 {
		t.Fatalf("Ctrl+D 没有复制步骤: %d 步", len(a.steps))
	}
	if a.stepSel != 1 {
		t.Fatalf("复制后应选中新步骤, stepSel=%d", a.stepSel)
	}
	if a.steps[0].opUID == a.steps[1].opUID {
		t.Fatal("复制的步骤与原件共用 opUID,多选会串")
	}
}

// Delete 删步骤:一次就删,删完可以撤回。键盘和鼠标必须一样——
// 以前两条路径各自维护一套二次确认,删单步改可撤销之后如果只改了
// 鼠标那条,键盘就成了"绕过防错"的捷径(或者反过来,键盘比鼠标难用)。
func TestShortcutDeleteDeletesAndCanUndo(t *testing.T) {
	a := &app{stepSel: 0, settings: defaultSettings()}
	a.steps = []guiRule{step("pinyin"), step("case.lower")}
	a.stepUID = a.steps[1].opUID
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Key(0, ui.KeyDelete)
	tt.Frame()
	if len(a.steps) != 1 || a.steps[0].opLabel != "全部转小写" {
		t.Fatalf("Delete 没删掉选中步骤: %d 步", len(a.steps))
	}
	if last, ok := a.lastStepUndo(); !ok || last.step.opLabel != "文件名转拼音" {
		t.Fatalf("撤回栈里记的不是被删的那一步: %+v", a.stepUndo)
	}
	a.undoDelete()
	if len(a.steps) != 2 || a.steps[0].opLabel != "文件名转拼音" {
		t.Fatalf("撤回没把步骤放回原位: %d 步", len(a.steps))
	}
}

// Alt+↑↓ 上下移动步骤。
func TestShortcutMoveStep(t *testing.T) {
	a := &app{stepSel: 1, settings: defaultSettings()}
	a.steps = []guiRule{step("pinyin"), step("case.lower")}
	a.stepUID = a.steps[1].opUID
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Key(ui.Alt, ui.KeyUp)
	tt.Frame()
	if a.steps[0].opLabel != "全部转小写" {
		t.Fatalf("Alt+↑ 没上移: 第 1 步是 %q", a.steps[0].opLabel)
	}
	if a.stepSel != 0 {
		t.Fatalf("上移后应仍选中同一步, stepSel=%d", a.stepSel)
	}
	tt.Key(ui.Alt, ui.KeyDown)
	tt.Frame()
	if a.steps[1].opLabel != "全部转小写" {
		t.Fatalf("Alt+↓ 没下移: 第 2 步是 %q", a.steps[1].opLabel)
	}
}

// Ctrl+O 打开添加文件对话框。用注入的对话框钩子,不摸真窗口。
func TestShortcutOpenFiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &app{stepSel: -1, settings: defaultSettings()}
	called := 0
	a.dialogs = dialogHooks{
		open: func(o mygo.OpenDialogOptions) ([]string, error) {
			called++
			return []string{p}, nil
		},
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyO)
	// addFilesDialog 在 goroutine 里跑,等它把文件加进来
	for i := 0; i < 200 && len(a.files) == 0; i++ {
		tt.Frame()
		time.Sleep(5 * time.Millisecond)
	}
	if called == 0 {
		t.Fatal("Ctrl+O 没有打开添加文件对话框")
	}
	if len(a.files) != 1 {
		t.Fatalf("Ctrl+O 添加后文件数 = %d, want 1", len(a.files))
	}
}

// 设置对话框开着时,底下的窗口快捷键不该还能动东西——
// 否则就是"看不见的那一层在后台响应"。
func TestShortcutIgnoredBehindSettings(t *testing.T) {
	a := &app{stepSel: 0, settings: defaultSettings()}
	a.steps = []guiRule{step("pinyin")}
	a.stepUID = a.steps[0].opUID
	a.settingsOpen = true
	a.syncSeedUI()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyD)
	tt.Frame()
	if len(a.steps) != 1 {
		t.Fatalf("设置对话框开着时 Ctrl+D 仍然复制了步骤: %d 步", len(a.steps))
	}
}

// 快捷键提示要跟着平台换修饰键符号:Mac 上写 Ctrl+O 是错的。
func TestShortcutTipPerPlatform(t *testing.T) {
	got := shortcutTip("Ctrl+O")
	if runtime.GOOS == "darwin" {
		if got != "⌘O" {
			t.Fatalf("Mac 上应显示 ⌘O, got %q", got)
		}
		return
	}
	if got != "Ctrl+O" {
		t.Fatalf("非 Mac 上应保持 Ctrl+O, got %q", got)
	}
}

// 快捷键要写在界面上:主操作按钮悬停后要能看到组合键。
// 不写出来,用户找不到就等于不存在。
func TestShortcutTipShowsOnHover(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "旧.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.files = []fileInfo{statFile(p)}
	a.steps = []guiRule{step("case.upper")}
	a.stepUID = a.steps[0].opUID
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	r, ok := tt.Find("执行更名")
	if !ok {
		t.Fatal("找不到主按钮")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(700 * time.Millisecond)
	tt.Frame()
	want := shortcutTip("Ctrl+Enter")
	for _, s := range tt.Texts() {
		if s == want {
			return
		}
	}
	t.Fatalf("主按钮上没有显示 %q 的快捷键提示", want)
}
