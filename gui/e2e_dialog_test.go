package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
)

// 对话框可注入后,添加/导出/导入 CSV、导入导出设置这些"摸窗口"的路径
// 也能无头跑完整链路。
func TestE2EDialogHooks(t *testing.T) {
	dir, paths := resetCopy(t)
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, ".journal.json")

	csvPath := filepath.Join(dir, "list.csv")
	setPath := filepath.Join(dir, "settings-out.json")
	a.dialogs = dialogHooks{
		open: func(o mygo.OpenDialogOptions) ([]string, error) {
			switch {
			case strings.Contains(o.Title, "设置"):
				return []string{setPath}, nil
			case strings.Contains(o.Title, "CSV"):
				return []string{csvPath}, nil
			}
			return paths, nil
		},
		save: func(o mygo.SaveDialogOptions) (string, error) {
			if strings.Contains(o.Title, "设置") {
				return setPath, nil
			}
			return csvPath, nil
		},
	}

	// 添加文件(走对话框路径)
	a.addFilesDialog()
	if len(a.files) != len(paths) {
		t.Fatalf("addFilesDialog loaded %d of %d", len(a.files), len(paths))
	}

	// 导出 CSV
	a.exportCSV()
	if _, err := os.Stat(csvPath); err != nil {
		t.Fatalf("CSV 未生成: %v", err)
	}

	// 清空后从 CSV 导回
	a.files = nil
	a.importCSV()
	if len(a.files) != len(paths) {
		t.Fatalf("importCSV loaded %d of %d", len(a.files), len(paths))
	}

	// 设置导出/导入
	a.settings.AI.Model = "test-model"
	a.exportSettings()
	if _, err := os.Stat(setPath); err != nil {
		t.Fatalf("设置未导出: %v", err)
	}
	a.settings.AI.Model = ""
	a.importSettings()
	if a.settings.AI.Model != "test-model" {
		t.Fatalf("设置导入未生效: %+v", a.settings.AI)
	}
}
