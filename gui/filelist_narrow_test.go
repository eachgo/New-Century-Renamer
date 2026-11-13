package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// 900 宽是程序自己声明的最小窗口(WindowOptions.MinWidth)。
// 以前在这个宽度下,文件名胶囊被压到 30px,只剩一个「…」——
// 名字是这个软件唯一要看的东西,压缩到读不出来等于没有这一列。
// 现在的做法:名字行允许换行(Wrap)+ 胶囊保底 70px,
// 宁可卡片长高,也要让「原名 → 新名」都读得出来。
func TestFileNameReadableAtMinimumWidth(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	dir := t.TempDir()
	a.files = []fileInfo{
		{path: filepath.Join(dir, "01. THE UNION.flac"), size: 12 << 20,
			modTime: time.Now(), realType: "FLAC", typ: "FLAC 音频", group: "Album"},
	}
	a.steps = append(a.steps, step("case.lower"))
	a.preview()

	tt := ui.NewTester(a.view, 900, 480)
	tt.Frame()
	tt.Frame()

	old, ok := tt.Find("01. THE UNION")
	if !ok {
		t.Fatal("900 宽下找不到原文件名")
	}
	if old.W < 70 {
		t.Errorf("原文件名胶囊被压到 %.0fpx,读不出名字(保底 70px)", old.W)
	}
	nw, ok := tt.Find("01. the union")
	if !ok {
		t.Fatal("900 宽下找不到新文件名")
	}
	if nw.W < 70 {
		t.Errorf("新文件名胶囊被压到 %.0fpx,读不出名字(保底 70px)", nw.W)
	}
	// 换行后箭头要跟着新名字走,不能孤零零留在上一行行尾
	arrow, ok := tt.Find("→")
	if !ok {
		t.Fatal("900 宽下找不到箭头")
	}
	if arrow.Y+arrow.H < nw.Y {
		t.Errorf("箭头留在上一行(y=%.0f),新名字在下一行(y=%.0f)——箭头应当跟着新名字", arrow.Y, nw.Y)
	}
	// 结果列必须在窗口内
	res, ok := tt.Find("将更名")
	if !ok {
		t.Fatal("900 宽下找不到结果列")
	}
	if right(res) > 900 {
		t.Errorf("结果列右缘 %.0f 超出窗口 900", right(res))
	}
}
