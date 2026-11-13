package main

import (
	"image"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 三栏之间应当是空缝，不是一条深色竖线。
//
// 之前用的是 ui.Split，它把分隔条颜色写死成 Theme.Border（见 mygo
// split.go 的 div.Draw）。而 Border 同时是输入框的边框色，为了输入框
// 看得见就必须够深；深了以后那两条 1 DIP 的线成了整屏最扎眼的东西，
// 比它们分隔的内容还显眼。现在改成留 10 DIP 的空缝。
func TestPaneGapIsEmptyNotALine(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	var paths []string
	for _, n := range []string{"01. THE UNION.flac", "02. Ali Baba.flac"} {
		p := filepath.Join(dir, n)
		writeTestFile(t, p, "x")
		paths = append(paths, p)
	}
	a.addPaths(paths)
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	a.stepSel = 0
	a.preview()

	tt := ui.NewTester(a.view, 1100, 700)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	img := tt.Image()

	// 判据：某一列上有一大段连续的深色像素——那是贯穿整栏的竖线。
	// 文字只占十来像素高，不会在整列上连成一片。
	for x := 0; x < 1100; x++ {
		run := 0
		for y := 100; y < 600; y++ {
			if darkPixel(img, x, y) {
				run++
				if run > 300 {
					t.Fatalf("x=%d 有一条贯穿的深色竖线,三栏之间应当只剩空缝", x)
				}
			} else {
				run = 0
			}
		}
	}
}

// darkPixel 判断一个像素是否"比周围明显深"——用它找分隔线，
// 不依赖具体的主题色值。
func darkPixel(img *image.RGBA, x, y int) bool {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return false
	}
	r, g, b, _ := img.At(x, y).RGBA()
	lum := (r>>8)*299/1000 + (g>>8)*587/1000 + (b>>8)*114/1000
	return lum < 200
}

// 拖拽缝能改变栏宽；键盘也能。
func TestPaneGapDrags(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 700)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	before := a.splitL
	if before <= 0 {
		t.Fatalf("初始栏宽没设上: %v", before)
	}
	// 缝在左栏右缘：x = 8(padding) + splitL，取它中间
	gx := float32(8) + before + 5
	tt.Press(gx, 400)
	tt.Move(gx+60, 400)
	tt.Frame()
	tt.Release(gx+60, 400)
	tt.Frame()
	if a.splitL <= before {
		t.Fatalf("拖动之后左栏没有变宽: %v → %v", before, a.splitL)
	}
}

// 卡片：内边距必须小于卡间间距，否则一组卡片读起来像一张散开的表。
func TestFileCardSpacingIsOuterGreaterThanInner(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	var paths []string
	for _, n := range []string{"01. THE UNION.flac", "02. Ali Baba.flac", "03. NIGHT FLOWER.flac"} {
		p := filepath.Join(dir, n)
		writeTestFile(t, p, "x")
		paths = append(paths, p)
	}
	a.addPaths(paths)
	a.preview()

	tt := ui.NewTester(a.view, 1100, 700)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	img := tt.Image()

	// 在右栏取一条干净竖列（避开文字），量卡片底色的连续段：
	// 卡片底色是 card，缝是 right 底色，两者不同。
	// 列位置不能写死：栏宽会随窗口与内边距变，硬编码的 x 一旦落到
	// 栏底色上就一张卡也量不到。逐列试，取第一列能量出三张卡的。
	type run struct{ start, end int }
	var runs []run
	for _, x := range []int{1000, 990, 980, 900, 700, 650, 620, 610} {
		runs = runs[:0]
		inCard := false
		cur := run{}
		for y := 60; y < 600; y++ {
			card := isCardPixel(img, x, y)
			if card && !inCard {
				inCard, cur = true, run{start: y}
			} else if !card && inCard {
				inCard, cur.end = false, y-1
				if cur.end-cur.start > 20 {
					runs = append(runs, cur)
				}
			}
		}
		if len(runs) >= 3 {
			break
		}
	}
	if len(runs) < 3 {
		t.Fatalf("没量到三张卡片: %v", runs)
	}
	cardH := float32(runs[0].end - runs[0].start + 1)
	gap := float32(runs[1].start - runs[0].end - 1)
	// 卡内单边内边距是 8（Padding(8, 10) 的第一个值）。
	// 判据：卡间缝要比单边内边距大，两组卡才会读成"两张卡"而不是
	// "一张卡里换了个小节"。
	if gap <= 8 {
		t.Fatalf("卡间缝 %.0fpx 不大于卡内单边内边距 8px,分组感不成立(卡高 %.0f)", gap, cardH)
	}
	// 上界：缝不能大到把一组卡片拆成互不相干的两堆。
	if gap > 20 {
		t.Fatalf("卡间缝 %.0fpx 太大,一组卡片读不成一组(卡高 %.0f)", gap, cardH)
	}
}

// isCardPixel 判断像素是不是文件卡片底：比右栏底色亮。
func isCardPixel(img *image.RGBA, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	// 卡片底 (252,254,253)，右栏底 (236,242,238)
	return r>>8 >= 248 && g>>8 >= 250 && b>>8 >= 248
}
