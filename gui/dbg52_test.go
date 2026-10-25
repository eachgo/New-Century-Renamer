package main

import (
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDbgMulti(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.files = []fileInfo{statFile(newTestFile(t, "x.flac"))}
	tt := ui.NewTester(a.view, 1280, 800)
	// add three steps
	for _, l := range []string{"文件名转拼音", "选取文件名", "全部转小写"} {
		if err := tt.Click(l); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	// turn on multi-select
	if err := tt.Click("多选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// click two step rows to select them (List with Selection: rows choose on click)
	if err := tt.Click("1. 文件名转拼音"); err != nil {
		t.Log("click row1:", err)
	}
	tt.Frame()
	if err := tt.Click("2. 选取文件名"); err != nil {
		t.Log("click row2:", err)
	}
	tt.Frame()
	t.Logf("chosen=%d", a.chosen.Len())
	img := tt.Image()
	crop := image.NewRGBA(image.Rect(0, 0, 360, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 360; x++ {
			crop.Set(x, y, img.At(215+x, 28+y))
		}
	}
	f, _ := os.Create("dbg-multi.png")
	png.Encode(f, crop)
	f.Close()
}
