package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// stepsSettingsApp 起一个最小 app，并把设置文件重定向到临时目录，
// 免得测试把步骤写进真实的 ncreamer-settings.json。
func stepsSettingsApp(t *testing.T) (*app, string) {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "ncreamer-settings.json")
	settingsFilePath = func() string { return dst }
	t.Cleanup(func() { settingsFilePath = settingsPath })
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings(), stepScrollTo: -1, previewErrStep: -1}
	return a, dst
}

// 上次退出时排着的步骤要在启动时接回来。
//
// 步骤是用户一点点调出来的：模板、正则、编号起始值。关一次窗口就全没了，
// 等于每次开机从头排一遍。
func TestStepsSurviveRestart(t *testing.T) {
	a, dst := stepsSettingsApp(t)

	// 排三步，并且把参数改得和默认值不一样。
	lower := step("case.lower")
	re := step("replace.re")
	re.oldStr = `\d+`
	re.newStr = "N"
	re.regex = true
	seq := step("seq")
	seq.seqStart = 5
	seq.seqWidth = 3
	a.steps = []guiRule{lower, re, seq}

	a.persistSteps()

	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("退出时没有写盘: %v", err)
	}
	var saved Settings
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Steps) != 3 {
		t.Fatalf("应当存下 3 步,实际 %d: %+v", len(saved.Steps), saved.Steps)
	}

	// 起一个"新进程"：设置从盘上读回来，再 restoreSteps。
	next := &app{stepSel: -1, fileSel: -1, settings: saved, stepScrollTo: -1, previewErrStep: -1}
	next.restoreSteps()

	if len(next.steps) != 3 {
		t.Fatalf("重启后应当有 3 步,实际 %d", len(next.steps))
	}
	if got := next.steps[0].opID; got != "case.lower" {
		t.Errorf("第 1 步应是 case.lower,实际 %q", got)
	}
	if next.steps[1].oldStr != `\d+` || next.steps[1].newStr != "N" || !next.steps[1].regex {
		t.Errorf("正则参数没还原: oldStr=%q newStr=%q regex=%v",
			next.steps[1].oldStr, next.steps[1].newStr, next.steps[1].regex)
	}
	if next.steps[2].seqStart != 5 || next.steps[2].seqWidth != 3 {
		t.Errorf("编号参数没还原: start=%v width=%v", next.steps[2].seqStart, next.steps[2].seqWidth)
	}
}

// 只存"和默认值不同"的参数：设置文件是给人看也给人改的，把每一步的
// 全部字段都抄一遍会变成几十行噪声。
func TestPersistedStepsOnlyCarryChangedArgs(t *testing.T) {
	a, _ := stepsSettingsApp(t)
	a.steps = []guiRule{step("case.lower")}
	a.persistSteps()

	if len(a.settings.Steps) != 1 {
		t.Fatalf("应当存下 1 步,实际 %d", len(a.settings.Steps))
	}
	if n := len(a.settings.Steps[0].Args); n != 0 {
		t.Fatalf("没有改过参数时不该存任何 Args,实际 %d 项: %+v", n, a.settings.Steps[0].Args)
	}
}

// 没有存过步骤时不能凭空造出一步，也不能把 stepSel 指到一个不存在的位置。
func TestRestoreStepsOnEmptySettings(t *testing.T) {
	a, _ := stepsSettingsApp(t)
	a.restoreSteps()

	if len(a.steps) != 0 {
		t.Fatalf("没有存过步骤时应当是空的,实际 %d 步", len(a.steps))
	}
	if a.stepSel != -1 {
		t.Fatalf("stepSel 应当是 -1,实际 %d", a.stepSel)
	}
}

// 认不出的步骤 id（比如条目在新版本里改了名）只跳过它自己，不能让整串
// 步骤作废——剩下的那些仍然能用。
func TestRestoreStepsSkipsUnknownIDs(t *testing.T) {
	a, _ := stepsSettingsApp(t)
	a.settings.Steps = []flowStep{
		{ID: "case.lower"},
		{ID: "no.such.op"},
		{ID: "case.upper"},
	}
	a.restoreSteps()

	if len(a.steps) != 2 {
		t.Fatalf("认不出的那一步该被跳过,剩下 2 步,实际 %d", len(a.steps))
	}
	if a.steps[0].opID != "case.lower" || a.steps[1].opID != "case.upper" {
		t.Fatalf("剩下的步骤顺序不对: %q %q", a.steps[0].opID, a.steps[1].opID)
	}
}

// 还原回来的步骤必须立刻算出预览：右栏一打开就该有新名字，而不是先
// 空着等用户动一下。
func TestRestoredStepsProduceAPreview(t *testing.T) {
	a, _ := stepsSettingsApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "Hello.TXT")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.files = []fileInfo{statFile(p)}
	a.settings.Steps = []flowStep{{ID: "case.lower", Args: map[string]string{"caseMode": "小写"}}}
	a.restoreSteps()

	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	if len(a.ops) != 1 {
		t.Fatalf("还原步骤后应当算出 1 项计划,实际 %d", len(a.ops))
	}
	if a.ops[0].NewName != "hello.txt" {
		t.Fatalf("预览结果不对: %q", a.ops[0].NewName)
	}
}

// 退出钩子必须真的接在 persistSteps 上。
//
// 只写函数不接线是最容易漏的一步：函数有了、测试也能调，但程序退出时
// 没人叫它，用户看到的仍然是"步骤又没了"。所以这里读源码确认 main()
// 里注册了 OnBeforeQuit 并调用 persistSteps。
func TestQuitHookPersistsSteps(t *testing.T) {
	src := string(mustRead(t, "main.go"))
	if !strings.Contains(src, "mygo.App.OnBeforeQuit(") {
		t.Fatal("没有注册 OnBeforeQuit:退出时不会保存步骤")
	}
	i := strings.Index(src, "mygo.App.OnBeforeQuit(")
	tail := src[i:]
	end := strings.Index(tail, "})")
	if end < 0 {
		t.Fatal("OnBeforeQuit 的注册语句不完整")
	}
	if !strings.Contains(tail[:end], "a.persistSteps()") {
		t.Fatalf("OnBeforeQuit 里没有调用 persistSteps: %s", tail[:end])
	}
}
