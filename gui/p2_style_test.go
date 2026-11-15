package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// P2 七条的回归测试。每条都对着"以前长什么样"写断言——只断言"现在是这样"
// 的测试挡不住有人改回去。

// 三个弹窗必须是同一个面板:同一个底色、同一处圆角、都有投影。
//
// 以前帮助弹窗用 t.Background(暗色下近纯黑)、设置用 t.Surface 带投影、
// 导入确认用 t.Surface 不带投影。同一个应用里三个"浮在内容上面"的面板
// 三种样子,看起来像三个人各做了一个。
func TestDialogsShareOnePanelStyle(t *testing.T) {
	src, err := os.ReadFile("dialog_style.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, want := range []string{"Background(t.Surface)", "Shadow(", "Radius(14)", "Padding(20)"} {
		if !strings.Contains(body, want) {
			t.Errorf("dialogPanel 缺少 %s——三个弹窗的统一样式必须收在这一处", want)
		}
	}
	// 三个弹窗都必须走它,不许再自己拼一个 Column。
	for _, f := range []string{"help_ui.go", "settings_ui.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if n := strings.Count(s, "dialogPanel(c, dw, dh)"); n == 0 {
			t.Errorf("%s 没有用 dialogPanel,弹窗样式又分叉了", f)
		}
		if strings.Contains(s, ".Background(t.Background)") {
			t.Errorf("%s 又把弹窗底色写成 t.Background(暗色下近纯黑)", f)
		}
	}
}

// 按钮只剩三种:实心主色(primaryButton)、主色淡底(tonalButton)、描边
// (outlinedButton,含危险动作的悬停变体)。
//
// 第四种是 dangerButton 自己一档——平时就红着。工具栏里"清空列表"和
// "删除"一进场就是两个红框,一个还没发生的动作喊得比当前要做的事还响。
func TestDangerButtonIsAnOutlinedVariant(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "func dangerButton(")
	if start < 0 {
		t.Fatal("找不到 dangerButton")
	}
	fn := body[start:]
	if end := strings.Index(fn[1:], "\nfunc "); end > 0 {
		fn = fn[:end+1]
	}
	if !strings.Contains(fn, "outlinedButton(c, label, enabled)") {
		t.Error("dangerButton 应当复用 outlinedButton,而不是自己拼一种按钮")
	}
	// 平时不能红:错误色只能在悬停那一支里出现。
	hoverAt := strings.Index(fn, "Hovered()")
	if hoverAt < 0 {
		t.Fatal("dangerButton 应当在悬停时才变成危险色")
	}
	before := fn[:hoverAt]
	if strings.Contains(before, "t.Danger") {
		t.Error("dangerButton 平时就用了错误色——危险该在手要落下去时才显现")
	}
}

// 数字框必须是"一条外轮廓",不是两个各自描边的框并排。
func TestNumberFieldIsOneBorderedBox(t *testing.T) {
	src, err := os.ReadFile("rules_editor.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, name := range []string{"func numField(", "func intField("} {
		start := strings.Index(body, name)
		if start < 0 {
			t.Fatalf("找不到 %s", name)
		}
		fn := body[start:]
		if end := strings.Index(fn[1:], "\nfunc "); end > 0 {
			fn = fn[:end+1]
		}
		if !strings.Contains(fn, ".Border(1, t.Border)") {
			t.Errorf("%s 应当在容器上画唯一的边框", name)
		}
		// 内部两个子控件都必须去掉自己的边框,否则相接处又出现双线。
		if n := strings.Count(fn, "Border(0, ui.Color{})"); n < 2 {
			t.Errorf("%s 里去掉边框的子控件只有 %d 个(应为 2:输入框与步进器)", name, n)
		}
	}
}

// 空态的主按钮必须是实心主色,不是主色淡底。
func TestEmptyStateCTAIsSolid(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "j.json")
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	tt.Frame()
	// 工具栏里也有一个「添加文件…」。空态那颗是"唯一的主动作",尺寸
	// 明显更大——按宽度挑,不能靠 tt.Find 的返回顺序(那是构建顺序)。
	var r ui.Rect
	found := false
	for _, n := range tt.AccessAll() {
		if n.Name == "添加文件…" && n.Bounds.W > 100 {
			r = n.Bounds
			found = true
		}
	}
	if !found {
		t.Fatal("空态应当有一颗大号的「添加文件…」")
	}
	th := a.currentTheme().light
	if a.isDark() {
		th = a.currentTheme().dark
	}
	img := tt.Image()
	scale := float64(img.Bounds().Dx()) / 1100
	// 取按钮内左侧一点(避开字形:字形有反锯齿,取到边缘像素会得到混色)。
	px := int((float64(r.X) + 5) * scale)
	py := int((float64(r.Y) + float64(r.H)/2) * scale)
	got := img.RGBAAt(px, py)
	want := th.Accent
	dr, dg, db := int(got.R)-int(want.R), int(got.G)-int(want.G), int(got.B)-int(want.B)
	if dr*dr+dg*dg+db*db > 900 {
		t.Errorf("空态主按钮底色是 #%02X%02X%02X,不是实心主色 #%02X%02X%02X——"+
			"淡底按钮和工具栏里的次要按钮长得几乎一样", got.R, got.G, got.B, want.R, want.G, want.B)
	}
}

// 范围色条:三根都对右栏底 ≥3:1,且「仅文件名」与「仅扩展名」彼此
// ≥1.45:1。
//
// 以前三个色相共用明度 0.62,「仅文件名」与「仅扩展名」实测只有
// 1.01–1.02——数值上就是同一个颜色,读的人只能靠猜。
func TestRangeBarsAreDistinguishable(t *testing.T) {
	seeds := []ui.Color{
		ui.RGB(0x2F, 0x7A, 0x52), ui.RGB(0x31, 0x40, 0xA8), ui.RGB(0x8A, 0x2B, 0xE2),
		ui.RGB(0xF5, 0xC5, 0x18), ui.RGB(0, 0, 0), ui.RGB(255, 255, 255),
	}
	for _, seed := range seeds {
		dt := buildTheme(seed)
		for _, dark := range []bool{false, true} {
			p := dt.panes(dark)
			rp := dt.ranges(dark)
			for _, c := range []struct {
				name string
				v    ui.Color
			}{{"筛选", rp.filter}, {"仅文件名", rp.name}, {"仅扩展名", rp.ext}} {
				if got := contrastRatio(c.v, p.right); got < 3 {
					t.Errorf("%s/%v: %s 色条对右栏底只有 %.2f:1,应 ≥3", hexc6(seed), dark, c.name, got)
				}
			}
			// 两个子档必须分得开。筛选在另一个槽位、含义也不同,不要求
			// 与它们靠颜色区分——它俩永远不会并排出现。
			if got := contrastRatio(rp.name, rp.ext); got < 1.45 {
				t.Errorf("%s/%v: 「仅文件名」与「仅扩展名」只差 %.2f:1,看不出切换", hexc6(seed), dark, got)
			}
		}
	}
}

func hexc6(c ui.Color) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }
