package main

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 目录滚不动时不该说"下面还有"——假提示比没有提示更糟,它训练用户
// 忽略这行字。
func TestCatalogCueOnlyWhenOverflowing(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 620)
	tt.Frame()
	tt.Frame()

	if a.palScroll.MaxY <= 0 {
		t.Fatalf("620 高下目录应当溢出,MaxY=%.1f", a.palScroll.MaxY)
	}
	if !tt.HasText("↓ 下面还有") {
		t.Fatal("目录溢出时应当提示下面还有内容")
	}

	// 拉高到整份目录都放得下。条目是两行(名称 + 说明),所以这里
	// 要拉到 2600 才真的一屏放得下 32 条。
	tt.SetSize(1100, 2600)
	tt.Frame()
	tt.Frame()
	if a.palScroll.MaxY > 0 {
		t.Fatalf("2600 高下目录不该再溢出,MaxY=%.1f", a.palScroll.MaxY)
	}
	if tt.HasText("↓ 下面还有") {
		t.Fatal("目录全部可见时不该还提示下面还有")
	}
}

// 滚到底之后提示要消失:否则用户会一直以为下面还有。
func TestCatalogCueGoneAtBottom(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 620)
	tt.Frame()
	tt.Frame()

	a.palScroll.Y = a.palScroll.MaxY
	tt.Frame()
	if tt.HasText("↓ 下面还有") {
		t.Fatal("滚到底之后不该还提示下面还有")
	}
}

// 被视口裁掉的那半行字要淡出,不能硬切在一条直线上——硬切看起来像
// 渲染坏了,而"下面还有"这行字本身不足以说明它被裁了。
// 量法:提示行上方 18px 带里的最暗像素,应当比再往上 40px 的带子浅。
func TestCatalogFadesAtBottomEdge(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 620)
	tt.Frame()
	tt.Frame()

	cue, ok := tt.Find("↓ 下面还有")
	if !ok {
		t.Fatal("620 高下目录应当溢出并提示下面还有")
	}
	img := tt.Image()
	top := int(cue.Y) - 18
	near := darkestLum(img, 10, top, 230, int(cue.Y)-2)
	far := darkestLum(img, 10, top-46, 230, top-10)
	if near <= far {
		t.Fatalf("视口底缘没有淡出:最下 16px 最暗 %d,上方 36px 处最暗 %d", near, far)
	}
}

func darkestLum(img *image.RGBA, x0, y0, x1, y1 int) int {
	best := 999
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			l := int(r>>8)*299/1000 + int(g>>8)*587/1000 + int(b>>8)*114/1000
			if l < best {
				best = l
			}
		}
	}
	return best
}
