package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 导入设置是整份替换:不确认就写盘,等于让用户用一次误点换掉全部配置。
// 这条测试守着"选完文件只进入确认态"。
func TestImportWaitsForConfirmation(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "ncreamer-settings.json")
	old := defaultSettings()
	old.AI.Model = "旧模型"
	if err := ExportSettingsTo(dst, old); err != nil {
		t.Fatal(err)
	}
	settingsFilePath = func() string { return dst }
	defer func() { settingsFilePath = settingsPath }()

	src := filepath.Join(dir, "import.json")
	next := defaultSettings()
	next.AI.Model = "新模型"
	next.Theme.Mode = "深色"
	if err := ExportSettingsTo(src, next); err != nil {
		t.Fatal(err)
	}

	a := &app{stepSel: -1, settings: old}
	a.dialogs.open = func(mygo.OpenDialogOptions) ([]string, error) { return []string{src}, nil }
	a.importSettings()

	if a.settings.AI.Model != "旧模型" {
		t.Fatalf("导入在确认前就改了设置: %q", a.settings.AI.Model)
	}
	if !a.importOpen || a.importPending == nil {
		t.Fatal("导入后应停在确认态")
	}
	// 磁盘上那份也必须还是旧的
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "旧模型") {
		t.Fatalf("确认前就写盘了: %s", b)
	}
}

// 确认之后:写新设置、并把原文件备份成 .bak。
func TestImportBacksUpBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "ncreamer-settings.json")
	old := defaultSettings()
	old.AI.Model = "旧模型"
	if err := ExportSettingsTo(dst, old); err != nil {
		t.Fatal(err)
	}
	settingsFilePath = func() string { return dst }
	defer func() { settingsFilePath = settingsPath }()

	next := defaultSettings()
	next.AI.Model = "新模型"

	a := &app{stepSel: -1, settings: old}
	a.importPending, a.importPath, a.importOpen = &next, filepath.Join(dir, "import.json"), true
	a.commitImport()

	if a.settings.AI.Model != "新模型" {
		t.Fatalf("确认后没换成新设置: %q", a.settings.AI.Model)
	}
	bak, err := os.ReadFile(dst + ".bak")
	if err != nil {
		t.Fatalf("没有备份原设置: %v", err)
	}
	if !strings.Contains(string(bak), "旧模型") {
		t.Fatalf("备份内容不对: %s", bak)
	}
	if a.importPending != nil || a.importOpen {
		t.Fatal("确认后应关掉确认态")
	}
}

// 确认框要说出会改掉什么,而不是只问一句"确定吗"。
func TestImportDialogListsWhatChanges(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "ncreamer-settings.json")
	old := defaultSettings()
	old.AI.Model = "旧模型"
	old.Theme.Mode = "浅色"
	if err := ExportSettingsTo(dst, old); err != nil {
		t.Fatal(err)
	}
	settingsFilePath = func() string { return dst }
	defer func() { settingsFilePath = settingsPath }()

	next := defaultSettings()
	next.AI.Model = "新模型"
	next.Theme.Mode = "深色"
	next.AI.Key = "sk-test"

	a := &app{stepSel: -1, settings: old, importOpen: true, importPending: &next,
		importPath: filepath.Join(dir, "import.json")}
	tt := ui.NewTester(a.view, 1100, 760)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	for _, want := range []string{"导入设置？", "导入并保存", "取消"} {
		if !tt.HasText(want) {
			t.Fatalf("确认框缺少 %q: %v", want, tt.Texts())
		}
	}
	joined := strings.Join(tt.Texts(), "\n")
	for _, want := range []string{"模型：旧模型 → 新模型", "外观模式：浅色 → 深色", "API Key：文件里带了一个"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("确认框没有列出 %q:\n%s", want, joined)
		}
	}
}

// 取消不能留下任何痕迹。
func TestImportCancelKeepsSettings(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "ncreamer-settings.json")
	old := defaultSettings()
	old.AI.Model = "旧模型"
	if err := ExportSettingsTo(dst, old); err != nil {
		t.Fatal(err)
	}
	settingsFilePath = func() string { return dst }
	defer func() { settingsFilePath = settingsPath }()

	next := defaultSettings()
	next.AI.Model = "新模型"
	a := &app{stepSel: -1, settings: old, importOpen: true, importPending: &next,
		importPath: filepath.Join(dir, "import.json")}
	tt := ui.NewTester(a.view, 1100, 760)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.importOpen || a.importPending != nil {
		t.Fatal("取消后确认态没关掉")
	}
	if a.settings.AI.Model != "旧模型" {
		t.Fatalf("取消改动了设置: %q", a.settings.AI.Model)
	}
	if _, err := os.Stat(dst + ".bak"); err == nil {
		t.Fatal("取消不该产生备份文件")
	}
}
