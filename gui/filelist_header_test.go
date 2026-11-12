package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 结果列离名字几百像素,没有表头的话右边那列只是"一个小圆点加两个字",
// 用户得自己猜那是结果。这个测试锁死两件事:表头在,且"结果"二字真的
// 落在结果列上方——不是随便飘在某处。
func TestFileListHasColumnHeader(t *testing.T) {
	root := t.TempDir()
	a := &app{stepSel: 0, settings: defaultSettings()}
	var paths []string
	for i := 1; i <= 3; i++ {
		p := filepath.Join(root, fmt.Sprintf("0%d. THE UNION.flac", i))
		writeTestFile(t, p, "x")
		paths = append(paths, p)
	}
	a.addPaths(paths)
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	a.preview()

	tt := ui.NewTester(a.view, 1100, 620)
	for i := 0; i < 6; i++ {
		tt.Frame()
	}
	hdr, ok := tt.Find("结果")
	if !ok {
		t.Fatal("结果列没有表头")
	}
	if _, ok := tt.Find("原名 → 新名"); !ok {
		t.Fatal("名字列没有表头")
	}
	// 表头要压在第一张卡片上面,而不是卡片下面
	first, ok := tt.Find("01. THE UNION")
	if !ok {
		t.Fatal("找不到第一张文件卡")
	}
	if hdr.Y >= first.Y {
		t.Fatalf("表头在文件卡下面: 表头 y=%.1f,首卡 y=%.1f", hdr.Y, first.Y)
	}
	// "结果"二字要与结果列的圆点同一竖线上(右对齐到那 64px 宽的槽)
	chip, ok := tt.Find("将更名")
	if !ok {
		t.Fatal("找不到结果文字")
	}
	if right(hdr) < right(chip)-1 {
		t.Fatalf("表头没有右对齐到结果列: 表头右缘 %.1f,结果右缘 %.1f", right(hdr), right(chip))
	}
}

// 空列表时不该有表头:一行"原名 → 新名 / 结果"配一个空窗口只是噪音。
func TestFileListHeaderHiddenWhenEmpty(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 620)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	if tt.HasText("结果") {
		t.Fatal("空列表不该显示列头")
	}
}

func right(r ui.Rect) float32 { return r.X + r.W }
