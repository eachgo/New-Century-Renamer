package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 实心主按钮上的字必须读得出来。
//
// 主色的明度被夹在 0.55–0.68,白字在浅色相(黄、青、亮绿)上只有
// 2.76–2.89:1——「执行更名」四个字糊成一片。AccentText 现在按
// "白字与深墨取高者"算,全色域最差 4.56:1。
func TestAccentTextReadableOnEverySeed(t *testing.T) {
	seeds := []string{
		"#2F7A52", "#F2703C", "#3140A8", "#8A2BE2", "#E22B5C",
		"#F5C518", "#7FBF3F", "#00BCD4", "#000000", "#FFFFFF",
	}
	for _, hex := range seeds {
		for _, dark := range []bool{false, true} {
			th := themeFor(ui.Hex(hex)).light
			if dark {
				th = themeFor(ui.Hex(hex)).dark
			}
			if c := contrastRatio(th.Accent, th.AccentText); c < 4.5 {
				t.Fatalf("%s(dark=%v) 实心主色上的字只有 %.2f:1,应 ≥4.5", hex, dark, c)
			}
		}
	}
}

// 全色域扫一遍:任何色相/彩度/明度下,主色上的字都要读得出来。
func TestAccentTextReadableAcrossGamut(t *testing.T) {
	worst, worstDesc := 99.0, ""
	for h := 0.0; h < 360; h += 10 {
		for _, c := range []float64{0.0, 0.06, 0.12, 0.2, 0.3} {
			for _, l := range []float64{0.55, 0.60, 0.65, 0.68} {
				th := themeFor(oklchColor(l, c, h)).light
				got := contrastRatio(th.Accent, th.AccentText)
				if got < worst {
					worst = got
					worstDesc = fmt.Sprintf("H=%.0f C=%.2f L=%.2f", h, c, l)
				}
			}
		}
	}
	if worst < 4.5 {
		t.Fatalf("全色域最差 %.2f:1(%s),应 ≥4.5", worst, worstDesc)
	}
	t.Logf("全色域最差 %.2f:1(%s)", worst, worstDesc)
}

// 禁用的主按钮也要读得出来。
//
// 原来只是 .Disabled(true),mygo 会把整块(连文字)乘 0.5 不透明度。
// 实心主色本来就饱和,乘完实测只有 1.37–1.55:1——没文件可改的时候
// 「执行更名」等于看不见,而它恰恰是这时候最需要"看得见但知道点不了"的。
func TestDisabledPrimaryButtonStaysReadable(t *testing.T) {
	for _, hex := range []string{"#2F7A52", "#F2703C", "#F5C518", "#00BCD4", "#FFFFFF", "#000000"} {
		dt := themeFor(ui.Hex(hex))
		th := dt.light
		bar := dt.panes(false).left
		a := &app{settings: defaultSettings()}
		a.settings.Theme.Seed = hex
		tt := ui.NewTester(a.view, 1280, 720)
		for i := 0; i < 3; i++ {
			tt.Frame()
		}
		if !tt.HasText("执行更名") {
			t.Fatalf("%s: 空列表时也必须有「执行更名」按钮", hex)
		}
		_ = th
		_ = bar
		// 量真实像素,不量主题里的颜色。只比对主题色的话,把按钮改回
		// 裸 .Disabled(true)(mygo 会把整块连文字乘 0.5)这条也照样通过
		// ——实测就是这样漏过去的。
		r, ok := tt.Find("执行更名")
		if !ok {
			t.Fatalf("%s: 找不到「执行更名」的位置", hex)
		}
		got := contrastInRect(t, tt, r)
		if got < 4.5 {
			t.Fatalf("%s: 禁用的主按钮实测对比度只有 %.2f:1,应 ≥4.5", hex, got)
		}
	}
}

// 预览整体算不出来时,界面必须说清是"算不出来",而不是装作没有预览。
func TestPreviewErrorIsStatedNotHidden(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = dir + "/j.json"
	// 正则少了右括号:core.ParseRules 会拒绝整条规则,于是这一轮
	// 一个名字都算不出来——正是 previewErr 要覆盖的情形。
	a.steps = []guiRule{step("del.re")}
	a.steps[0].oldStr = "("
	a.stepUID = a.steps[0].opUID
	writeTestFile(t, dir+"/a.mp3", "x")
	a.addPaths([]string{dir + "/a.mp3"})

	if a.previewErr == "" {
		t.Fatal("正则写错时应当记下预览错误")
	}
	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if !tt.HasText("预览算不出来") {
		t.Fatalf("没有说明预览算不出来: %v", tt.Texts())
	}
	if !tt.HasText("等步骤修好") {
		t.Fatalf("新名字该占位说明,不能凭空消失: %v", tt.Texts())
	}
	if !tt.HasText("算不出") {
		t.Fatalf("结果列该占位,不能整片消失: %v", tt.Texts())
	}
}

// contrastInRect 量一块渲染区域内"最常见的颜色"与"离它最远的文字色"
// 之间的对比度。用于断言按钮上的字真的读得出来——包括 mygo 自己
// 施加的透明度,那是主题色里看不出来的。
func contrastInRect(t *testing.T, tt *ui.Tester, r ui.Rect) float64 {
	t.Helper()
	img := tt.Image()
	// 探针按 1× 渲染,DIP 与像素一一对应;万一将来改了缩放,
	// 用图宽除以逻辑宽(1280)兜住。
	scale := float64(img.Bounds().Dx()) / 1280
	counts := map[uint32]int{}
	type px struct{ x, y int }
	var all []px
	var colors []ui.Color
	for y := int(float64(r.Y) * scale); y < int(float64(r.Y+r.H)*scale); y++ {
		for x := int(float64(r.X) * scale); x < int(float64(r.X+r.W)*scale); x++ {
			c := img.At(x, y)
			cr, cg, cb, _ := c.RGBA()
			key := (cr>>8)<<16 | (cg>>8)<<8 | cb>>8
			counts[key]++
			all = append(all, px{x, y})
			colors = append(colors, ui.RGB(uint8(cr>>8), uint8(cg>>8), uint8(cb>>8)))
		}
	}
	// 众数 = 底色
	best, bgKey := 0, uint32(0)
	for k, v := range counts {
		if v > best {
			best, bgKey = v, k
		}
	}
	bg := ui.RGB(uint8(bgKey>>16), uint8(bgKey>>8), uint8(bgKey))
	// 字色取"和底色差得最远的那个"——笔画中心。不能取最小值:
	// 字形边缘的反锯齿像素本来就贴近底色,最小值恒为 1.00,
	// 那样这条测试永远失败、等于没写。
	worst := 0.0
	for _, c := range colors {
		if v := contrastRatio(c, bg); v > worst {
			worst = v
		}
	}
	return worst
}
