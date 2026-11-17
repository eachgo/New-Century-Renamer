package main

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 全软件只有一条"能不能点"的视觉规则:边框带主题色 = 能点,边框纯灰 = 不能。
//
// 以前两者都是 Theme.Border(彩度 0.004–0.018,肉眼是灰),于是"按不动"和
// "按得动"长得一样;更糟的是那些灰按钮根本没被禁用,Clicked() 照样返回
// true,只是动作跑起来无事发生——用户看到的就是"点了没反应"。

func lookSeeds() []ui.Color {
	return []ui.Color{
		ui.RGB(0x2F, 0x7A, 0x52), ui.RGB(0xF2, 0x70, 0x3C), ui.RGB(0xF5, 0xC5, 0x18),
		ui.RGB(0x00, 0xBC, 0xD4), ui.RGB(0x8A, 0x2B, 0xE2), ui.RGB(0xE2, 0x2B, 0x5C),
		ui.RGB(0x31, 0x40, 0xA8), ui.RGB(0x7F, 0xBF, 0x3F), ui.RGB(0x00, 0x00, 0x00),
		ui.RGB(0xFF, 0xFF, 0xFF),
	}
}

// 能点的框:带主题色的彩度 + 对每种底都 ≥3:1(WCAG 1.4.11 的控件边界)。
func TestLiveBorderCarriesTheThemeHueAndStaysReadable(t *testing.T) {
	for _, seed := range lookSeeds() {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			th, p := dt.light, dt.lightPane
			if dark {
				th, p = dt.dark, dt.darkPane
			}
			if _, c, _ := colorToOklch(th.Border); c < 0.010 {
				t.Errorf("%s dark=%v: 能点的边框彩度只有 %.4f——纯灰分不出能不能点",
					colorHex(seed), dark, c)
			}
			for name, g := range map[string]ui.Color{
				"base": p.base, "left": p.left, "mid": p.mid,
				"right": p.right, "card": p.card, "chip": p.chip,
			} {
				if v := contrastRatio(th.Border, g); v < 3.0 {
					t.Errorf("%s dark=%v: 能点的边框对 %s 只有 %.2f:1,应 ≥3:1",
						colorHex(seed), dark, name, v)
				}
			}
		}
	}
}

// 不能点的框:零彩度,而且必须比能点的淡(但还没淡到看不见)。
//
// 明暗两侧都算:亮色下"更淡"是往下压明度,暗色下是往上抬——直接比数值
// 会在暗色里反过来。
func TestInertBorderIsNeutralAndQuieter(t *testing.T) {
	for _, seed := range lookSeeds() {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			th, p := dt.light, dt.lightPane
			if dark {
				th, p = dt.dark, dt.darkPane
			}
			inert := inertBorder(th)
			if _, c, _ := colorToOklch(inert); c > 0.002 {
				t.Errorf("%s dark=%v: 不能点的边框彩度 %.4f 不是零——它必须是纯灰",
					colorHex(seed), dark, c)
			}
			// 对每种底都比能点的淡,同时看得见(≥1.4:1)。
			for name, g := range map[string]ui.Color{
				"base": p.base, "left": p.left, "mid": p.mid, "card": p.card,
			} {
				live := contrastRatio(th.Border, g)
				dead := contrastRatio(inert, g)
				if dead >= live {
					t.Errorf("%s dark=%v: 不能点的边框对 %s 有 %.2f:1,不比能点的 %.2f 淡——两者分不开",
						colorHex(seed), dark, name, dead, live)
				}
				if dead < 1.4 {
					t.Errorf("%s dark=%v: 不能点的边框对 %s 只有 %.2f:1,淡到看不见——用户不知道这儿有个按钮",
						colorHex(seed), dark, name, dead)
				}
			}
		}
	}
}

// 灰按钮必须真的按不动:不是"看起来灰",而是点击不触发动作。
//
// 这条量的是行为,不是颜色。以前 confirmUndo 会在 canUndo=false 时被置真
// (按钮亮着就能点),修完必须还是 false。
func TestDisabledButtonIgnoresClicks(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	saved := 0
	a.dialogs.save = func(mygo.SaveDialogOptions) (string, error) { saved++; return "", nil }
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	if err := tt.Click("撤销上次"); err != nil {
		t.Fatalf("点不到「撤销上次」: %v", err)
	}
	tt.Frame()
	if a.confirmUndo {
		t.Error("没有可撤销的记录时点「撤销上次」,却进入了确认态——这颗按钮是禁用的,不该响应")
	}

	// 清单是空的:导出清单同理。
	if err := tt.Click("导出清单…"); err != nil {
		t.Fatalf("点不到「导出清单…」: %v", err)
	}
	tt.Frame()
	if saved > 0 {
		t.Error("清单为空时点「导出清单…」,却真的弹出了保存对话框")
	}
}

// 有框按钮的禁用分支必须真的禁用,不能只换颜色。
//
// 只换颜色的写法看起来一样、行为却完全不同(点击照常触发)。现在三种按钮
// (淡底/描边/普通)的禁用分支都收在 frameButton 一处,所以这里量两件事:
// frameButton 的禁用分支确实调了 Disabled(true) 且关掉自动调暗;三个辅助
// 函数在 enabled=false 时确实把活儿交给了它,而不是自己另调一套颜色。
func TestButtonHelpersReallyDisable(t *testing.T) {
	src, err := os.ReadFile("control_look.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "func frameButton(")
	if start < 0 {
		t.Fatal("找不到 frameButton")
	}
	seg := body[start:]
	if end := strings.Index(seg[1:], "\nfunc "); end > 0 {
		seg = seg[:end+1]
	}
	if !strings.Contains(seg, "Disabled(true)") {
		t.Error("frameButton 的禁用分支没有调用 Disabled(true)——只换颜色的话点击照样会触发")
	}
	if !strings.Contains(seg, "DimDisabled(false)") {
		t.Error("frameButton 的禁用分支没有关掉自动调暗——文字会被乘到读不清")
	}

	// 按不动的样子只能有一副面孔,所以每个按钮辅助函数都必须落到 frameButton:
	// tonalButton 启用时是淡底(没有框),禁用分支得自己拐过去;outlinedButton
	// 整颗就是 frameButton,连分支都不该有——它要是又自己调一遍颜色,过一阵
	// 就会有一处忘了改。
	src, err = os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	body = string(src)
	want := map[string]string{
		"tonalButton":    "frameButton(c, label, false)",
		"outlinedButton": "return frameButton(c, label, enabled)",
	}
	for fn, needle := range want {
		start := strings.Index(body, "func "+fn+"(")
		if start < 0 {
			t.Fatalf("找不到 %s", fn)
		}
		seg := body[start:]
		if end := strings.Index(seg[1:], "\nfunc "); end > 0 {
			seg = seg[:end+1]
		}
		if !strings.Contains(seg, needle) {
			t.Errorf("%s 里没有 %q——它的外观又自己长了一套", fn, needle)
		}
	}
}

// 全软件不许再出现裸 ui.Button:它的边框走 Theme.Border,和禁用态同一种灰。
func TestNoBareButtonsRemain(t *testing.T) {
	for _, f := range []string{"main.go", "flows.go", "help_ui.go", "settings_ui.go", "rules_editor.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "ui.Button(c,") {
			t.Errorf("%s 里还有裸 ui.Button——它会拿到中性灰边框,和「按不动」分不开", f)
		}
	}
}

// 预设卡片与流程卡片不能读成按钮。
//
// 按钮的处方是"一条边框 + 一块填色";卡片以前抄了同一份,于是四张预设卡
// 就是四颗大按钮。现在卡片:底 = 抬高一层的那个中性色(不掺主题色),
// 静止时没有轮廓,悬停才亮起主题色描边——描边是"能点"的信号,而它只在
// 悬停时出现,所以静止的卡片读成瓦片而不是按钮。
func TestCardsDoNotLookLikeButtons(t *testing.T) {
	for _, f := range []string{"main.go", "flows.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		body := string(src)
		if strings.Contains(body, "cardTint") {
			t.Errorf("%s 的卡片还在掺主题色——那就是淡底按钮的处方,铺满整块会变成一颗大按钮", f)
		}
		if !strings.Contains(body, "cardBorder(t)") || !strings.Contains(body, "cardBorderHover(t)") {
			t.Errorf("%s 的卡片没有走 cardBorder/cardBorderHover——静止无轮廓、悬停亮主题色描边是它和按钮的分界", f)
		}
	}
}

// 卡片必须真的从它坐的那层底上浮起来,而且压在卡片上的次要文字要读得清。
//
// 两条路都试过:掺主题色(0.16 太响、0.10 亮色下瓦片感只剩 1.01、暗色下
// 和淡底按钮只差 1.006),最后回到"抬高一层的那个中性色"——亮色 p.card
// 对栏底 1.15、暗色 1.17,弹窗里用 cardBgFor 对 Surface 也有 1.09 以上。
// 这条测试钉的就是这个数:卡片和底同色(或几乎同色)时会立刻红。
func TestCardsFloatAboveTheirGround(t *testing.T) {
	for _, seed := range lookSeeds() {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			th, p := dt.light, dt.lightPane
			if dark {
				th, p = dt.dark, dt.darkPane
			}
			// 主界面:卡片坐在中栏上。
			if v := contrastRatio(p.card, p.mid); v < 1.10 {
				t.Errorf("%s dark=%v: 预设卡对中栏底只有 %.3f:1,读不出是一块瓦片——它会退回成几行普通文字",
					colorHex(seed), dark, v)
			}
			if v := contrastRatio(th.TextMuted, p.card); v < 4.5 {
				t.Errorf("%s dark=%v: 预设卡上的次要文字只有 %.2f:1,应 ≥4.5", colorHex(seed), dark, v)
			}
			// 弹窗:卡片坐在 Surface 上。
			dlg := dt.cardBgFor(dark)
			if v := contrastRatio(dlg, th.Surface); v < 1.05 {
				t.Errorf("%s dark=%v: 弹窗里的流程卡对 Surface 只有 %.3f:1,和底同色就看不出一行一行",
					colorHex(seed), dark, v)
			}
			if v := contrastRatio(th.TextMuted, dlg); v < 4.5 {
				t.Errorf("%s dark=%v: 流程卡上的次要文字只有 %.2f:1,应 ≥4.5", colorHex(seed), dark, v)
			}
			// 卡片和淡底按钮真正的分界不是底色深浅(暗色里两者明度
			// 本来就只差 1.04–1.07),而是"有没有轮廓":淡底按钮常驻
			// 一条主题色边框,卡片静止时没有。这条必须守住——一旦
			// cardBorder 返回了颜色,四张预设卡立刻变回四颗按钮。
			if c := cardBorder(th); c.A != 0 {
				t.Errorf("%s dark=%v: 卡片静止时带了 %s 的边框——有轮廓就是按钮的样子",
					colorHex(seed), dark, colorHex(c))
			}
			// 悬停必须看得出来:描边亮起(底色不变,这是有意的)。
			if v := contrastRatio(cardBorderHover(th), p.card); v < 3.0 {
				t.Errorf("%s dark=%v: 悬停描边对卡片底只有 %.2f:1,应 ≥3——鼠标停在卡片上没有反应",
					colorHex(seed), dark, v)
			}
		}
	}
}

// 用户给的那条规则,直接量像素:框带主题色 = 能点,框是中性灰 = 不能点。
//
// 前面几条测的都是"函数返回了什么颜色",而这条测的是"屏幕上真的画出了
// 什么"——中间还隔着 mygo 的绘制、圆角裁剪、禁用调暗。用户看到的是像素,
// 所以规则必须在这里守一遍:一颗带框的按钮,能点时画面上必须出现主题色
// 的边框,不能点时画面上必须出现中性灰的边框,而且两者不能同时出现。
//
// 比的是"整像素是否精确等于 liveBorder/inertBorder 本身",不是"彩度多少"。
// 不缩放时 1 DIP 的边框正好落在一整列像素上,颜色是精确的;一旦改成量彩度,
// 栏底自己那点彩(亮色 0.0100、暗色 0.0203)就会把灰框顶成"彩框",抗锯齿的
// 半像素也会掺进来——试过三遍,守不住。
//
// 淡底按钮(添加文件…/导出清单…/导入清单…)不在"框"的管辖内:它启用时根本
// 没有框,靠一块掺了主色的底表示能点。对它只守一条——不能点时必须有灰框,
// 也就是"灰框 = 按不动"在任何按钮上都成立。
func TestBordersSayClickableOrNot(t *testing.T) {
	type probe struct {
		label   string
		enabled bool
		framed  bool // 带框按钮:启用时靠边框表示能点
	}
	cases := []struct {
		name  string
		files bool
		steps bool
		want  []probe
	}{
		{
			// 空清单:带框按钮里一个都不能点。
			name: "空清单", files: false,
			want: []probe{
				{"添加文件…", true, false},
				{"导出清单…", false, false},
				{"导入清单…", true, false},
				{"添加文件夹…", true, true},
				{"清空列表", false, true},
				{"撤销上次", false, true},
				{"执行更名", false, true},
				{"多选", false, true},
			},
		},
		{
			// 有文件、也排了步骤:「执行更名」终于能点。它是实心主色块,
			// 没有边框——那是第三种"能点"的画法,不归"框"管,只守一条:
			// 不许出现中性灰边框。
			name: "有文件有步骤", files: true, steps: true,
			want: []probe{
				{"添加文件…", true, false},
				{"导出清单…", true, false},
				{"导入清单…", true, false},
				{"清空列表", true, true},
				{"撤销上次", false, true},
				{"执行更名", true, false},
				{"多选", true, true},
			},
		},
		{
			// 有文件但一步没排:「清空列表」和三个淡底按钮能点,执行更名仍不能
			// ——没有步骤就没有任何名字会变,hasChanges() 是 false。
			// 这里没有「添加文件夹…」:它只在空态的拖放区里,清单有文件后就没了。
			name: "有文件无步骤", files: true,
			want: []probe{
				{"添加文件…", true, false},
				{"导出清单…", true, false},
				{"导入清单…", true, false},
				{"清空列表", true, true},
				{"撤销上次", false, true},
				{"执行更名", false, true},
				{"多选", false, true},
			},
		},
	}
	for _, tc := range cases {
		for _, dark := range []bool{false, true} {
			a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
			a.settings.Journal = filepath.Join(t.TempDir(), "j.json")
			a.settings.Theme.Mode = "浅色"
			if dark {
				a.settings.Theme.Mode = "深色"
			}
			if tc.files {
				dir := t.TempDir()
				writeTestFile(t, filepath.Join(dir, "01 - 序章.mp3"), "x")
				a.addPaths([]string{filepath.Join(dir, "01 - 序章.mp3")})
			}
			if tc.steps {
				a.steps = []guiRule{step("seq")}
			}
			if tc.files {
				a.preview()
			}
			tt := ui.NewTester(a.view, 1280, 800)
			for i := 0; i < 3; i++ {
				tt.Frame()
			}
			img := tt.Image()
			th := a.currentTheme().light
			if dark {
				th = a.currentTheme().dark
			}
			live, inert := liveBorder(th), inertBorder(th)
			for _, w := range tc.want {
				b, ok := tt.Find(w.label)
				if !ok {
					t.Errorf("%s dark=%v: 找不到「%s」", tc.name, dark, w.label)
					continue
				}
				hasLive := rectHasColor(img, b, live)
				hasInert := rectHasColor(img, b, inert)
				if w.enabled {
					if hasInert {
						t.Errorf("%s dark=%v: 「%s」现在能点,但画面上有中性灰边框(%s)——看着像按不动",
							tc.name, dark, w.label, colorHex(inert))
					}
					if w.framed && !hasLive {
						t.Errorf("%s dark=%v: 「%s」现在能点,但没有主题色边框(%s)——看着像按不动",
							tc.name, dark, w.label, colorHex(live))
					}
					continue
				}
				if !hasInert {
					t.Errorf("%s dark=%v: 「%s」现在点不了,但画面上没有中性灰边框(%s)——看着像能点",
						tc.name, dark, w.label, colorHex(inert))
				}
				if hasLive {
					t.Errorf("%s dark=%v: 「%s」现在点不了,却带着主题色边框(%s)——看着像能点",
						tc.name, dark, w.label, colorHex(live))
				}
			}
		}
	}
}

// rectHasColor 判断这颗按钮自己的范围里,有没有一个像素正好是这个颜色。
//
// 外扩量按按钮的内边距(Padding(8, 12))定:文字左缘到外框是 12,上下是 8,
// 各留 2 DIP 给抗锯齿。不能多扩——「执行更名」左边紧挨「撤销上次」,扩到
// 16 DIP 就会把邻居的灰框算成自己的。
func rectHasColor(img *image.RGBA, b ui.Rect, want ui.Color) bool {
	x0, x1 := int(b.X)-14, int(b.X+b.W)+14
	y0, y1 := int(b.Y)-10, int(b.Y+b.H)+10
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			r, g, bl, _ := img.At(x, y).RGBA()
			if uint8(r>>8) == want.R && uint8(g>>8) == want.G && uint8(bl>>8) == want.B {
				return true
			}
		}
	}
	return false
}
