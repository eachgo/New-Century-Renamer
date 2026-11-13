package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 目录溢出时靠底缘渐隐提示"下面还有",不再另写一行字——颜色层级
// 已经在说这件事,再挂一行"↓ 下面还有"就是同一句话说两遍。
// 这里量的是"渐隐只在真溢出时出现":视口底缘那一带的像素必须比
// 中间更接近底色,而目录全放得下时不该有这层遮罩。
func TestCatalogFadeOnlyWhenOverflowing(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 620)
	tt.Frame()
	tt.Frame()

	if a.palScroll.MaxY <= 0 {
		t.Fatalf("620 高下目录应当溢出,MaxY=%.1f", a.palScroll.MaxY)
	}
	if tt.HasText("↓ 下面还有") {
		t.Fatal("渐隐已经在提示了,不该再写一行字")
	}
	img := tt.Image()
	bottom := int(a.catRect.Y + a.catRect.H)
	if a.catRect.H <= 0 {
		t.Fatalf("没拿到目录视口位置: %+v", a.catRect)
	}
	near := bandDarkestLum(img, 20, 220, bottom-14, bottom-2)
	far := bandDarkestLum(img, 20, 220, bottom-60, bottom-40)
	if near <= far {
		t.Fatalf("视口底缘没有淡出:最下 14px 最暗 %d,上方 40px 处最暗 %d", near, far)
	}

	// 拉高到整份目录都放得下,遮罩必须消失。
	tt.SetSize(1100, 2600)
	tt.Frame()
	tt.Frame()
	if a.palScroll.MaxY > 0 {
		t.Fatalf("2600 高下目录不该再溢出,MaxY=%.1f", a.palScroll.MaxY)
	}
}
