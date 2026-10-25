package main

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDbgChips(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.files = []fileInfo{statFile(newTestFile(t, "蓝莲花.MP3"))}
	tt := ui.NewTester(a.view, 1280, 1250)
	for _, l := range []string{"文件名转拼音", "选取文件名", "全部转小写", "排除文件", "追加序号", "清除筛选", "恢复整体"} {
		if err := tt.Click(l); err != nil {
			t.Fatal(l, err)
		}
		tt.Frame()
	}
	// configure 排除文件 pattern via direct state (tester typing is fiddly)
	a.steps[3].pattern = "*.tmp"
	tt.Frame()
	f, _ := os.Create("dbg-chips.png")
	png.Encode(f, tt.Image())
	f.Close()
}
