package main

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 默认的 1280×720 下,设置里的四张卡必须全部看得见,而且不用滚。
// 之前是单栏排下来总高 890,「主题」「设置文件」整个落在视口之外,滚也
// 没有提示——用户以为这些功能不存在。
func TestSettingsCardsVisibleAtDefaultSize(t *testing.T) {
	for _, sz := range [][2]int{{1280, 720}, {1280, 800}, {1440, 900}} {
		a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
		tt := ui.NewTester(a.view, sz[0], sz[1])
		tt.Frame()
		tt.Frame()
		a.openSettings()
		tt.Frame()
		tt.Frame()

		if a.setScroll.MaxY > 0 {
			t.Errorf("%dx%d 下设置不该需要滚动,MaxY=%.1f", sz[0], sz[1], a.setScroll.MaxY)
		}
		for _, name := range []string{"AI 接口（OpenAI 兼容）", "文件", "主题", "设置文件"} {
			r, ok := tt.Find(name)
			if !ok {
				t.Errorf("%dx%d 下找不到卡片 %q", sz[0], sz[1], name)
				continue
			}
			if r.H <= 0 {
				t.Errorf("%dx%d 下卡片 %q 被裁成 0 高", sz[0], sz[1], name)
			}
		}
	}
}

// 窄窗口退回单栏,表单可以滚,但底边要淡出:硬切在一条直线上看起来像
// 对话框坏了,而不是"还能往下滚"。
func TestSettingsNarrowFallsBackAndFades(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 900, 480)
	tt.Frame()
	tt.Frame()
	a.openSettings()
	tt.Frame()
	tt.Frame()

	if a.setScroll.MaxY <= 0 {
		t.Fatal("900×480 下设置应当溢出并可以滚动")
	}
	if _, ok := tt.Find("AI 接口（OpenAI 兼容）"); !ok {
		t.Fatal("窄窗口下第一张卡应当可见")
	}
	// 滚到底之后四张卡都要能到。
	for i := 0; i < 12; i++ {
		tt.Scroll(450, 250, 0, 120)
		tt.Frame()
	}
	for _, name := range []string{"主题", "设置文件"} {
		r, ok := tt.Find(name)
		if !ok || r.H <= 0 {
			t.Errorf("滚到底也看不到卡片 %q", name)
		}
	}
}

// 窄窗口下滚动区底边要淡出:被裁掉的那半行字硬切在一条直线上,看起来
// 像对话框坏了而不是"还能往下滚"。量法:沿纵向取每一行的最暗像素,底边
// 往上 20px 之内应当逐行变浅(文字被渐变的底色盖住)。
func TestSettingsFadesAtScrollEdge(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 900, 480)
	tt.Frame()
	tt.Frame()
	a.openSettings()
	tt.Frame()
	tt.Frame()

	if a.setScroll.MaxY <= 0 {
		t.Fatal("900×480 下设置应当溢出并可以滚动")
	}
	img := tt.Image()
	r := a.setBodyRect
	if r.W <= 0 || r.H <= 0 {
		t.Fatal("没有记录到滚动视口的位置")
	}
	// 遮罩在视口底边之上 20px,渐变到最底一行正好是底色。所以最下 6px
	// 里不该再出现文字那种深度的像素——没有遮罩时,被裁的那半行字会
	// 一路暗到底边(实测最暗 97,有遮罩时最暗 213)。
	bottom := int(r.Y + r.H)
	x0, x1 := int(r.X)+2, int(r.X+r.W)-2
	if v := bandDarkestLum(img, x0, x1, bottom-6, bottom-1); v < 170 {
		t.Fatalf("滚动区底缘没有淡出:最下 6px 最暗 %d(有遮罩时应当 >170)", v)
	}
}

func bandDarkestLum(img *image.RGBA, x0, x1, y0, y1 int) int {
	best := 999
	for y := y0; y <= y1; y++ {
		if v := rowDarkestLum(img, x0, x1, y); v < best {
			best = v
		}
	}
	return best
}

func rowDarkestLum(img *image.RGBA, x0, x1, y int) int {
	best := 999
	for x := x0; x < x1; x++ {
		r, g, b, _ := img.At(x, y).RGBA()
		l := int(r>>8)*299/1000 + int(g>>8)*587/1000 + int(b>>8)*114/1000
		if l < best {
			best = l
		}
	}
	return best
}
