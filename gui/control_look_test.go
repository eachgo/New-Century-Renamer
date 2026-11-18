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

// 按不动的按钮必须真的禁用,不能只换颜色。
//
// 只换颜色的写法看起来一样、行为却完全不同(点击照常触发)。现在两种按钮
// (无框的 softButton、带框的 frameButton)的禁用分支各有一处,所以这里量
// 三件事:两个分支都调了 Disabled(true) 且关掉自动调暗;四个辅助函数确实
// 把活儿交给它们,而不是自己另调一套颜色。
func TestButtonHelpersReallyDisable(t *testing.T) {
	src, err := os.ReadFile("control_look.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, fn := range []string{"softButton", "frameButton"} {
		start := strings.Index(body, "func "+fn+"(")
		if start < 0 {
			t.Fatalf("找不到 %s", fn)
		}
		seg := body[start:]
		if end := strings.Index(seg[1:], "\nfunc "); end > 0 {
			seg = seg[:end+1]
		}
		if !strings.Contains(seg, "Disabled(true)") {
			t.Errorf("%s 的禁用分支没有调用 Disabled(true)——只换颜色的话点击照样会触发", fn)
		}
		if !strings.Contains(seg, "DimDisabled(false)") {
			t.Errorf("%s 的禁用分支没有关掉自动调暗——文字会被乘到读不清", fn)
		}
	}

	// 按不动的样子只能有一副面孔:无框按钮落到 softButton,带框的落到
	// frameButton,都不许自己再拼一套颜色。
	src, err = os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	body = string(src)
	want := map[string]string{
		"tonalButton":    "softButton(c, label, enabled)",
		"plainButton":    "softButton(c, label, enabled)",
		"outlinedButton": "softButton(c, label, enabled)",
		"dangerButton":   "frameButton(c, label, enabled)",
		"primaryButton":  "softButton(c, label, false)",
	}
	for fn, needle := range want {
		start := strings.Index(body, "func "+fn)
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
// 现在的规则是"能点 = 有一块带主题色的底,按不动 = 有一块灰底"。两者都
// 有底(禁用态也要看得出是个按钮),分界在彩度:能点那颗的底是主色铺在地面
// 上(彩度 0.02 上下),按不动那颗的底是中性和主色混出来的灰(彩度 0.007)。
//
// 三种能点的画法都落在这条上:淡底(softFill)、实心主色(执行更名)、带框
// (删除/清空列表/添加文件夹)。带框的额外守一条——框必须是主题色那支
// (liveBorder),不许是中性灰(inertBorder):灰框在任何按钮上都只表示"按不动"。
//
// 底色取样有两个坑:
//   - 不能用 tt.Find 的矩形边界算——那是**文字**矩形,不是按钮矩形,边框
//     在它外面 12 DIP 处;
//   - 不能用固定偏移猜栏底,得取那一整行的众数:底栏和工具栏的底色不同,
//     「撤销上次」和「添加文件…」不在同一行。
func TestBordersSayClickableOrNot(t *testing.T) {
	type probe struct {
		label   string
		enabled bool
		framed  bool // 带框按钮:只有这三个动作(删除/清除/添加文件夹)保留框
	}
	cases := []struct {
		name  string
		files bool
		steps bool
		want  []probe
	}{
		{
			// 空清单:除了"添加"类,一个都不能点。
			name: "空清单", files: false,
			want: []probe{
				{"添加文件…", true, false},
				{"导出清单…", false, false},
				{"导入清单…", true, false},
				{"添加文件夹…", true, true},
				{"清空列表", false, true},
				{"撤销上次", false, false},
				{"执行更名", false, false},
				{"多选", false, false},
			},
		},
		{
			// 有文件、也排了步骤:「执行更名」终于能点。它是实心主色块,
			// 没有边框——靠整块底说话。
			name: "有文件有步骤", files: true, steps: true,
			want: []probe{
				{"添加文件…", true, false},
				{"导出清单…", true, false},
				{"导入清单…", true, false},
				{"清空列表", true, true},
				{"撤销上次", false, false},
				{"执行更名", true, false},
				{"多选", true, false},
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
				{"撤销上次", false, false},
				{"执行更名", false, false},
				{"多选", false, false},
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
			deadFill := inertFill(th)
			for _, w := range tc.want {
				b, ok := tt.Find(w.label)
				if !ok {
					t.Errorf("%s dark=%v: 找不到「%s」", tc.name, dark, w.label)
					continue
				}
				cy := int(b.Y + b.H/2)
				ground, _ := rowModeColor(img, cy)
				fill := pix(img, int(b.X)-7, cy)
				_, fillChroma, _ := colorToOklch(fill)
				hasLive := rectHasColor(img, b, live)
				hasInert := rectHasColor(img, b, inert)

				if w.enabled {
					if hasInert {
						t.Errorf("%s dark=%v: 「%s」现在能点,但画面上有中性灰边框(%s)——看着像按不动",
							tc.name, dark, w.label, colorHex(inert))
					}
					if fill == ground && !hasLive {
						t.Errorf("%s dark=%v: 「%s」现在能点,但和栏底(%s)一个样,也没有主题色边框——看不出能点",
							tc.name, dark, w.label, colorHex(ground))
					}
					if fill == deadFill {
						t.Errorf("%s dark=%v: 「%s」现在能点,但底用的是禁用那颗的灰(%s)——和按不动的一样",
							tc.name, dark, w.label, colorHex(deadFill))
					}
					if w.framed && !hasLive {
						t.Errorf("%s dark=%v: 「%s」是该带框的动作,但没有主题色边框(%s)",
							tc.name, dark, w.label, colorHex(live))
					}
					continue
				}
				if hasLive {
					t.Errorf("%s dark=%v: 「%s」现在点不了,却带着主题色边框(%s)——看着像能点",
						tc.name, dark, w.label, colorHex(live))
				}
				// 按不动的必须有底(否则看不出是个按钮),但那块底必须是灰的:
				// 要么正好是 inertFill,要么彩度低到读成中性。
				if fill == ground {
					t.Errorf("%s dark=%v: 「%s」现在点不了,但和栏底(%s)一个样——看不出这里本来是个按钮",
						tc.name, dark, w.label, colorHex(ground))
				}
				if fill != deadFill && fillChroma > 0.012 {
					t.Errorf("%s dark=%v: 「%s」现在点不了,但底还有 %.4f 的彩度(%s)——看着像活的",
						tc.name, dark, w.label, fillChroma, colorHex(fill))
				}
			}
		}
	}
}

// inertFill 是"按不动"那块底,单独守三条:它必须是灰的、必须是块瓦片、
// 而且必须和"能点"那颗分得开。前两条在像素测试里也有,但那条要跑真渲染;
// 这条是纯函数,十套种子 × 明暗全覆盖,调色的时候先红在这里。
func TestInertFillIsAGreyTile(t *testing.T) {
	seeds := []ui.Color{
		ui.Hex("#2F7A52"), ui.Hex("#F2703C"), ui.Hex("#F5C518"), ui.Hex("#2563EB"),
		ui.Hex("#8B5CF6"), ui.Hex("#E11D48"), ui.Hex("#14B8A6"), ui.Hex("#EC4899"),
		ui.Hex("#000000"), ui.Hex("#FFFFFF"),
	}
	for _, seed := range seeds {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			th := dt.light
			if dark {
				th = dt.dark
			}
			p := dt.panes(dark)
			dead := inertFill(th)
			if _, c, _ := colorToOklch(dead); c > 0.012 {
				t.Errorf("%s(dark=%v): 禁用底还有 %.4f 的彩度,读起来像活的", colorHex(seed), dark, c)
			}
			for _, g := range []ui.Color{p.base, p.left, p.mid, p.right, p.card} {
				if r := contrastRatio(dead, g); r < 1.10 {
					t.Errorf("%s(dark=%v): 禁用底对地面 %s 只有 %.2f:1,看不出是块瓦片",
						colorHex(seed), dark, colorHex(g), r)
				}
				if r := contrastRatio(th.TextMuted, dead); r < 4.5 {
					t.Errorf("%s(dark=%v): 禁用底上的次要字只有 %.2f:1,应 ≥4.5", colorHex(seed), dark, r)
				}
				// 死活的分界是彩,不是明度:两块瓦片一样厚(明度只差
				// 1.1–1.4),一颗带彩一颗不带彩。所以这里量彩度比,不量
				// 对比度——后者在亮色下永远过不了 1.15,因为灰瓦片本来就
				// 和淡底一样重。
				live := over(softFill(th), g)
				_, lc, _ := colorToOklch(live)
				_, dc, _ := colorToOklch(dead)
				if lc > 0.004 && dc/lc > 0.55 {
					t.Errorf("%s(dark=%v): 禁用的底彩度 %.4f 接近能点的 %.4f,分不出死活",
						colorHex(seed), dark, dc, lc)
				}
			}
		}
	}
}

// rowModeColor 取整行出现次数最多的颜色,当作这一行的栏底。
//
// 为什么不直接算:栏底在不同行不一样(工具栏一种、底栏另一种、空态拖放区
// 又一种),写死任何一个值都会在别的行上错。众数天然跟着行走。
func rowModeColor(img *image.RGBA, y int) (ui.Color, int) {
	counts := map[ui.Color]int{}
	for x := 0; x < img.Bounds().Dx(); x++ {
		counts[pix(img, x, y)]++
	}
	var best ui.Color
	bestN := 0
	for c, n := range counts {
		if n > bestN {
			best, bestN = c, n
		}
	}
	return best, bestN
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

// pix 取一个像素的颜色。测试里到处要"这一个点是什么色",包一层免得
// 每次都写 RGBA() 的移位。
func pix(img *image.RGBA, x, y int) ui.Color {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return ui.Color{}
	}
	r, g, b, _ := img.At(x, y).RGBA()
	return ui.Color{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
}

// 能点的按钮,鼠标移上去必须变色;按不动的必须一动不动。
//
// 这是用户定的规则里"能点"的**唯一**动态证据:既然默认按钮没有边框,
// 静止时靠"有没有一块底"区分,那悬停就得靠"底变深"来确认"这颗是活的"。
// 如果悬停没反应,用户只能靠点一下试试——那就退回原点。
func TestHoverIsWhatSaysClickable(t *testing.T) {
	for _, dark := range []bool{false, true} {
		a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
		a.settings.Journal = filepath.Join(t.TempDir(), "j.json")
		a.settings.Theme.Mode = "浅色"
		if dark {
			a.settings.Theme.Mode = "深色"
		}
		tt := ui.NewTester(a.view, 1280, 800)
		for i := 0; i < 3; i++ {
			tt.Frame()
		}
		// 空清单时「添加文件…」能点、「导出清单…」不能,正好一对。
		for _, c := range []struct {
			label   string
			enabled bool
		}{
			{"添加文件…", true},
			{"导出清单…", false},
		} {
			b, ok := tt.Find(c.label)
			if !ok {
				t.Fatalf("dark=%v: 找不到「%s」", dark, c.label)
			}
			cx, cy := b.X+b.W/2, b.Y+b.H/2
			before := pix(tt.Image(), int(b.X)-7, int(cy))
			tt.Move(cx, cy)
			tt.Frame()
			after := pix(tt.Image(), int(b.X)-7, int(cy))
			if c.enabled {
				if before == after {
					t.Errorf("dark=%v: 「%s」能点,但鼠标移上去底没有变化(%s)——用户没法确认它是活的",
						dark, c.label, colorHex(before))
				}
				continue
			}
			if before != after {
				t.Errorf("dark=%v: 「%s」按不动,鼠标移上去却变了色(%s → %s)——看着像能点",
					dark, c.label, colorHex(before), colorHex(after))
			}
			// 鼠标移开,免得影响下一颗。
			tt.Move(5, 5)
			tt.Frame()
		}
	}
}
