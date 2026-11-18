package main

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// mustRead 读一个源文件;读不到就让测试失败,而不是静默返回空串。
func mustRead(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("读不到 %s: %v", name, err)
	}
	return string(b)
}

// seeds 是一批有代表性的主色:品牌绿、橙、黄、青、紫、红、蓝、亮绿、
// 纯黑、纯白。纯黑白最容易把"只在某个色相下成立"的取值打回原形。
var paletteSeeds = []ui.Color{
	ui.RGB(0x2F, 0x7A, 0x52), ui.RGB(0xF2, 0x70, 0x3C), ui.RGB(0xF5, 0xC5, 0x18),
	ui.RGB(0x00, 0xBC, 0xD4), ui.RGB(0x8A, 0x2B, 0xE2), ui.RGB(0xE2, 0x2B, 0x5C),
	ui.RGB(0x31, 0x40, 0xA8), ui.RGB(0x7F, 0xBF, 0x3F), ui.RGB(0x00, 0x00, 0x00),
	ui.RGB(0xFF, 0xFF, 0xFF),
}

// 深色下三栏是同色(实测 mid/right = 1.000),必须靠栏间那条 1px 线分界。
// 亮色不需要——亮色 base 与三栏本来就有 1.05 的差,再加线反而碎。
func TestDarkPanesNeedADivider(t *testing.T) {
	for _, seed := range paletteSeeds {
		dt := themeFor(seed)
		p := dt.darkPane
		if c := contrastRatio(p.mid, p.right); c > 1.02 {
			t.Fatalf("seed %v: 深色中栏与右栏应当接近同色(靠分隔线分界),实测 %.3f", seed, c)
		}
		if c := contrastRatio(p.outline, p.right); c < 1.5 {
			t.Errorf("seed %v: 深色栏间分隔线对栏底只有 %.2f:1,应 ≥1.5", seed, c)
		}
	}
}

// 选中底在深色下必须与左栏底分得开。原来是 0.30–0.42 压 0.250,
// 实测 1.22–1.95:选中行和没选中几乎一样。
func TestSelectionStandsOutInBothAppearances(t *testing.T) {
	for _, seed := range paletteSeeds {
		dt := themeFor(seed)
		if c := contrastRatio(dt.selBg, dt.lightPane.left); c < 4.5 {
			t.Errorf("seed %v: 亮色选中底对左栏底只有 %.2f:1,应 ≥4.5", seed, c)
		}
		if c := contrastRatio(dt.selBgDark, dt.darkPane.left); c < 3.0 {
			t.Errorf("seed %v: 深色选中底对左栏底只有 %.2f:1,应 ≥3", seed, c)
		}
		// 选中底上的字也要读得出:白字是 selFg。
		if c := contrastRatio(dt.selBgDark, dt.selFg); c < 4.5 {
			t.Errorf("seed %v: 深色选中底上的白字只有 %.2f:1,应 ≥4.5", seed, c)
		}
	}
}

// 目录图标不能只靠色相区分:11 个色相在灰度下的亮度原来全挤在
// 0.1614–0.1861(差 0.025),色盲用户与灰度打印都分不开。现在两档
// 明度交错,相邻两组的图标亮度必须拉开。
func TestCatalogGroupIconsDifferInLightness(t *testing.T) {
	for _, seed := range paletteSeeds {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			var prev ui.Color
			prevSet := false
			lo, hi := 1.0, 0.0
			for _, g := range stepGroups {
				c := dt.groupColorFor(g, dark)
				l := relativeLuminance(c)
				if l < lo {
					lo = l
				}
				if l > hi {
					hi = l
				}
				if prevSet {
					if r := contrastRatio(prev, c); r < 1.45 {
						t.Errorf("seed %v dark=%v: 相邻两组图标只差 %.2f:1,灰度下分不开", seed, dark, r)
					}
				}
				prev, prevSet = c, true
			}
			// 整组图标至少要跨过两档明度,不能全挤在一条线上。原来各组
			// 只跨 0.025;现在两档交替,实测跨 0.12 左右,相邻差 1.6:1。
			if hi-lo < 0.10 {
				t.Errorf("seed %v dark=%v: 图标亮度只跨 %.3f,灰度下仍是同一档", seed, dark, hi-lo)
			}
		}
	}
}

// 禁用按钮不能靠 mygo 的"整块乘 0.5"来表示:那会把文字一起调暗,
// 实测亮色 3.49:1、暗色 2.38:1,用户读不清按钮上写的是什么。
func TestDisabledButtonsStayReadable(t *testing.T) {
	for _, seed := range paletteSeeds {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			th := dt.light
			if dark {
				th = dt.dark
			}
			fg, bg := disabledColors(th)
			if c := contrastRatio(fg, bg); c < 4.5 {
				t.Errorf("seed %v dark=%v: 禁用按钮文字只有 %.2f:1,应 ≥4.5", seed, dark, c)
			}
			// 被禁用和不被禁用也要看得出差别:字色必须比正常字色淡。
			if c := contrastRatio(fg, th.Text); c > 1.0 && fg == th.Text {
				t.Errorf("seed %v dark=%v: 禁用字色和正常字色一样,看不出被禁用", seed, dark)
			}
		}
	}
}

// 悬停底色必须看得出来。以前是 Text.Alpha(0.08),对左栏底实测 1.16:1,
// 和"没画"差不多——用户看不出鼠标停在哪一行。
func TestHoverIsVisible(t *testing.T) {
	for _, seed := range paletteSeeds {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			th := dt.light
			p := dt.lightPane
			if dark {
				th, p = dt.dark, dt.darkPane
			}
			// 悬停底色是半透明的,量的是"叠上去之后"的实际颜色。
			for _, ground := range []ui.Color{p.left, p.right, p.card} {
				got := over(th.SurfaceHover, ground)
				// 门槛定 1.20 而不是 1.15:旧值 0.08 实测 1.162,刚好
				// 从 1.15 底下溜过去——门槛必须定在"旧值过不去"的位置,
				// 否则这条测试守不住它要守的东西。
				if c := contrastRatio(got, ground); c < 1.20 {
					t.Errorf("seed %v dark=%v: 悬停底色对底只有 %.3f:1,应 ≥1.20", seed, dark, c)
				}
			}
		}
	}
}

// 标题与正文不能只靠字号差区分:18 对 14 只差 4px,字体渲染一糊就分不出。
// 标题必须同时加粗。
func TestTitlesAreBoldNotJustBigger(t *testing.T) {
	hits := tokenHits(t, "FontSize")
	if hits[18] == 0 {
		t.Skip("当前没有 18 号字,标题档位可能变了")
	}
	src := mustRead(t, "main.go") + mustRead(t, "help_ui.go") + mustRead(t, "settings_ui.go")
	for _, line := range strings.Split(src, "\n") {
		if !strings.Contains(line, "FontSize(18)") {
			continue
		}
		if !strings.Contains(line, "Bold()") {
			t.Errorf("18 号字必须同时加粗,这一行没有: %s", strings.TrimSpace(line))
		}
	}
}

// 数字列要等宽:日期、大小、序号在行与行之间竖着对齐才扫得动。
func TestNumericTextIsTabular(t *testing.T) {
	src := mustRead(t, "main.go")
	// 序号列的 tnum 在 "Width(seqW)" 那一行,不是在 strconv.Itoa 那一行,
	// 所以按"这一处文本的链式调用"找,而不是按字面量找。
	for _, key := range []string{"f.modTime.Format(", "humanSize(f.size)", "Width(seqW).TextAlign(ui.End)"} {
		idx := strings.Index(src, key)
		if idx < 0 {
			t.Fatalf("找不到 %s,这条测试该跟着代码改", key)
		}
		window := src[idx:]
		if len(window) > 400 {
			window = window[:400]
		}
		if !strings.Contains(window, "FontFeatures(\"tnum\")") {
			t.Errorf("%s 所在的文本没有 tabular-nums,行与行之间的数字对不齐", key)
		}
	}
}

// 空态插图与目录图标要用同一套描边:线宽与端点不一致时,同一屏里
// 两处"线条画"会像两个人画的。
func TestIllustrationMatchesIconStroke(t *testing.T) {
	illus := mustRead(t, "illustration.go")
	if !strings.Contains(illus, `stroke-width="1.6"`) {
		t.Error("插图线宽应统一成 1.6")
	}
	if strings.Contains(illus, `stroke-width="1.4"`) {
		t.Error("插图里还有 1.4 的线宽,和其余部分不一致")
	}
	if n := strings.Count(illus, `stroke-linecap="round"`); n != 3 {
		t.Errorf("插图三条路径都该用圆头端点,实际 %d 条", n)
	}
	if n := strings.Contains(illus, `stroke-linejoin="round"`); !n {
		t.Error("插图的折角该用圆角连接")
	}
}
