package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 预览算不出来时,界面要能直接把人送到出问题的那一步。
//
// 原来只有一句「第 3 步「正则替换」：…」——用户读完还得回列表里数到
// 第三行,再点开它。步骤多的时候这一步比读报错还费劲。
func TestPreviewErrorOffersToJumpToTheStep(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("case.lower"), step("replace.re"), step("case.upper")}
	a.steps[1].oldStr = "[a-z"
	a.steps[1].regex = true
	a.stepSel = 2 // 故意停在别处:跳转才有意义
	// 红条长在右栏文件区里,所以得有一个文件才看得见。
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	writeTestFile(t, src, "x")
	a.addPaths([]string{src})

	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()
	a.preview()
	tt.Frame()

	if a.previewErr == "" {
		t.Fatal("坏正则应当让整轮预览算不出来")
	}
	if a.previewErrStep != 1 {
		t.Fatalf("出错步骤应是第 2 步(下标 1),实际 %d", a.previewErrStep)
	}
	// 报错要指到具体那一步,而不是只说"规则 2"。
	if !tt.HasText("第 2 步") {
		t.Fatalf("报错里应指出是第 2 步,屏幕上只有: %v", tt.Texts())
	}
	if !tt.HasText("去修这一步") {
		t.Fatal("认得出是哪一步时,该给一个「去修这一步」的入口")
	}

	if err := tt.Click("去修这一步"); err != nil {
		t.Fatalf("点不到「去修这一步」: %v", err)
	}
	tt.Frame()
	if a.stepSel != 1 {
		t.Fatalf("点完应当选中出问题的那一步(下标 1),实际 %d", a.stepSel)
	}
}

// 认不出是哪一步时不许瞎猜:不给跳转按钮,报错原样显示。
func TestPreviewErrorWithoutAStepStillShowsText(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings(), previewErrStep: -1}
	a.steps = []guiRule{step("case.lower")}
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	writeTestFile(t, src, "x")
	a.addPaths([]string{src})
	tt := ui.NewTester(a.view, 1400, 900)
	tt.Frame()

	a.previewErr = "规则 9：未知规则 \"nope\""
	a.previewErrStep = -1
	tt.Frame()

	if !tt.HasText("预览算不出来") {
		t.Fatal("整轮算不出来时该有红条")
	}
	if tt.HasText("去修这一步") {
		t.Fatal("认不出是哪一步时不该给跳转按钮")
	}
}
