package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// splitSettingsApp 起一个最小可跑的 app，并把设置文件重定向到临时目录，
// 免得测试把宽度写进真实的 ncreamer-settings.json。
func splitSettingsApp(t *testing.T, s Settings) *app {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "ncreamer-settings.json")
	settingsFilePath = func() string { return dst }
	t.Cleanup(func() { settingsFilePath = settingsPath })
	a := &app{stepSel: -1, fileSel: -1, settings: s, stepScrollTo: -1, previewErrStep: -1}
	return a
}

// 上一次运行拖过的分栏宽度要在启动时接回来。
//
// 宽度只活在内存里时，重启就回到 220/330——用户把左栏拖宽是为了看全
// 规则名，等于每次开机都要重拖一遍。
func TestSplitWidthsSurviveRestart(t *testing.T) {
	s := defaultSettings()
	s.SplitL, s.SplitM = 300, 420
	a := splitSettingsApp(t, s)

	tt := ui.NewTester(a.view, 1400, 800)
	tt.Frame()

	if a.splitL != 300 || a.splitM != 420 {
		t.Fatalf("启动时没接回上次的宽度: splitL=%v splitM=%v,想要 300/420", a.splitL, a.splitM)
	}
}

// 没有存过宽度时仍然用默认值，不能被 0 顶成"两栏都没有宽度"。
func TestSplitFallsBackToDefaults(t *testing.T) {
	a := splitSettingsApp(t, defaultSettings())

	tt := ui.NewTester(a.view, 1400, 800)
	tt.Frame()

	if a.splitL != 220 || a.splitM != 330 {
		t.Fatalf("没有存过宽度时应当用 220/330,实际 %v/%v", a.splitL, a.splitM)
	}
}

// 松手之后才写盘，而且写下去的值要能再读回来。
func TestSplitIsPersistedWhenDragEnds(t *testing.T) {
	a := splitSettingsApp(t, defaultSettings())
	tt := ui.NewTester(a.view, 1400, 800)
	tt.Frame()

	a.splitL, a.splitM = 260, 380
	a.persistSplit()

	b, err := os.ReadFile(settingsFilePath())
	if err != nil {
		t.Fatalf("拖动结束后没有写盘: %v", err)
	}
	var got Settings
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.SplitL != 260 || got.SplitM != 380 {
		t.Fatalf("写盘的值不对: %v/%v,想要 260/380", got.SplitL, got.SplitM)
	}
	// 再起一个 app 应当接回同一对宽度。
	next := &app{stepSel: -1, fileSel: -1, settings: got, stepScrollTo: -1, previewErrStep: -1}
	tt2 := ui.NewTester(next.view, 1400, 800)
	tt2.Frame()
	if next.splitL != 260 || next.splitM != 380 {
		t.Fatalf("重启后宽度没接回来: %v/%v", next.splitL, next.splitM)
	}
}
