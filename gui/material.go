package main

import (
	"math"
	"sync"

	"github.com/egoist/mygo/ui"
)

// themeFromSeed derives a whole widget theme from one seed color — the
// matugen idea: the user (or the desktop's accent) picks a single color and
// every role follows, in OKLCH so tones land perceptually even.
//
// The derivation keeps ncreamer's "Mono" temperament: calm ground, restrained
// chroma, the seed as the only accent.
type derivedTheme struct {
	light, dark *ui.Theme
	selBg       ui.Color // catalog/steps selection: deep seed tone
	selFg       ui.Color
	cardBg      ui.Color // settings card ground: near-neutral seed tint
	// range bars: two fixed slots — right = 选取范围 (filename/extension,
	// two neighboring hues so a switch shows), left = 筛选范围 (the seed's
	// complement). A slot holds its place transparent when its range is off
	rangeFilter    ui.Color
	rangeScopeName ui.Color
	rangeScopeExt  ui.Color
	groupColors    map[string]ui.Color // fixed color per catalog group

	// 三栏"海拔"配色(Material 3 的 surface container 阶)。
	// 亮色:越核心越亮——左栏最暗(后退),中栏最亮(浮起),右栏居中。
	// 暗色:反过来——背景最暗,越核心越亮一点点,模拟光照高度。
	// paneLeft/paneMid/paneRight 是各栏容器的底色;cardOnPane 是右栏
	// 文件卡片在 paneRight 之上的浮起底色;outline 是极浅的分隔线;
	// shadow 是给中栏加的一层极弱投影(亮色略深、暗色几乎不可见)。
	lightPane paneTones
	darkPane  paneTones
}

// paneTones is one appearance's three-pane elevation palette.
type paneTones struct {
	base             ui.Color // 基底:顶栏/底栏/左栏/四周留白都是它
	left, mid, right ui.Color // 左/中/右三栏容器底色(mid、right 是浮在 base 上的卡片)
	card             ui.Color // 右栏文件卡片(在 right 之上浮一层)
	outline          ui.Color // 极浅分隔线(带一点主题色倾向,不是纯灰)
	shadow           ui.Color // 浮起卡片的投影色(含 alpha)
	// chip 是文件卡片里那些小胶囊(路径、日期、大小)的底色。以前它们用
	// Theme.SurfaceHover,而 SurfaceHover 的明度和右栏底几乎相同
	// (实测对比度 1.00)——胶囊贴在卡片上只剩圆角能认出来,像没画底。
	// 单独给一个比卡片深一档的色,让它确实是"一块底"。
	chip ui.Color
}

// stepGroups is the fixed group order; each gets its own hue offset.
var stepGroups = []string{"筛选/范围", "转换", "大小写", "替换", "结构", "扩展名", "编号", "元数据", "日期", "智能", "高级"}

var (
	themeMu    sync.Mutex
	themeCache = map[string]*derivedTheme{}
)

func themeFor(seed ui.Color) *derivedTheme {
	themeMu.Lock()
	defer themeMu.Unlock()
	key := seedKey(seed)
	if t, ok := themeCache[key]; ok {
		return t
	}
	t := buildTheme(seed)
	themeCache[key] = t
	return t
}

func seedKey(c ui.Color) string {
	return string([]byte{c.R, c.G, c.B, c.A})
}

func buildTheme(seed ui.Color) *derivedTheme {
	L, C, H := colorToOklch(seed)
	// clamp chroma: keep derived colors calm whatever the seed
	cc := clamp(C, 0.02, 0.115)
	tone := func(l, cMul float64) ui.Color {
		return oklchColor(l, cc*cMul, H)
	}

	// the accent itself sits at a mid tone with enough contrast for white
	// text in light mode
	accL := clamp(L, 0.55, 0.68)

	lt := ui.LightTheme()
	lt.Background = tone(0.972, 0.10) // warm/cool tint follows the seed
	lt.Surface = tone(0.995, 0.04)
	// 悬停/按下用"文字色的状态层"(M3 的 state layer),不用固定的色块。
	// 原来 SurfaceHover 是 tone(0.955):在右栏底(同样 L≈0.955)上实测
	// 1.00:1,等于没画;而三栏四层海拔明度各不相同,任何固定色调都会
	// 与其中某一层撞上。改成半透明叠色后,它在 base/mid/right/card 上
	// 都稳定在 1.17:1 左右,深浅两种外观也自动对(亮色压深、暗色提亮)。
	lt.SurfaceHover = lt.Text.Alpha(0.08)
	lt.SurfacePressed = lt.Text.Alpha(0.14)
	// 控件边界。0.920 时按钮描边/输入框边框对底色只有 1.03–1.24:1,
	// 等于"没有边界"——用户看不出哪里可以点。0.72 抬到 2.0–2.4,
	// 仍然很轻,但轮廓立得住;装饰用的分隔线另走 paneTones.outline。
	lt.Border = tone(0.720, 0.16)
	lt.Text = tone(0.24, 0.30)
	// 次要文字承载路径、日期、大小、分组计数——是信息不是装饰,必须读得出来。
	// L 0.62 时在左栏底色上只有 2.94:1(远低于 WCAG 4.5);0.50 让四种底色
	// (左/中/右栏/卡片)全部 ≥4.8,同时仍明显浅于正文(L 0.24),层级还在。
	lt.TextMuted = tone(0.50, 0.20)
	accAt(lt, accL, cc, H)
	lt.AccentText = ui.RGB(255, 255, 255)
	lt.Selection = tone(0.925, 0.35)
	lt.Focus = oklchColor(accL, cc, H)
	lt.Inverse = tone(0.28, 0.85)
	lt.InverseText = ui.RGB(255, 255, 255)
	// 滚动条只有 1.43:1,几乎看不见;0.72 让它读得出来又不抢眼
	lt.Scrollbar = tone(0.720, 0.20)
	// 圆角阶梯只有三档:行 6 / 卡片 10 / 面板 14(见 gui/main.go 的注释)。
	// Spacing 回到 4pt 基准——以前是 5,于是 Padding/Gap 的取值散成
	// 19 种和 7 种,每个数字都是"看着差不多"手工调的。
	lt.Radius = 10
	lt.Spacing = 4

	dt := ui.DarkTheme()
	dt.Background = tone(0.165, 0.28)
	dt.Surface = tone(0.215, 0.24)
	// 同亮色:状态层叠色,不做固定色块(理由见上面 lt.SurfaceHover)
	dt.SurfaceHover = dt.Text.Alpha(0.10)
	dt.SurfacePressed = dt.Text.Alpha(0.17)
	// 0.325 时控件边界只有 1.46:1,等于没有边界;0.56 抬到 3.9(对 base),
	// 卡片上也有 2.9,暗色下按钮轮廓才立得住
	dt.Border = tone(0.560, 0.22)
	dt.Text = tone(0.945, 0.10)
	dt.TextMuted = tone(0.700, 0.12)
	accAt(dt, clamp(L, 0.72, 0.82), cc, H)
	dt.AccentText = tone(0.16, 0.5)
	dt.Selection = tone(0.300, 0.55)
	dt.Focus = oklchColor(clamp(L, 0.72, 0.82), cc, H)
	dt.Inverse = tone(0.93, 0.10)
	dt.InverseText = tone(0.24, 0.30)
	dt.Scrollbar = tone(0.560, 0.20)
	dt.Radius = 10
	dt.Spacing = 4

	groupColors := make(map[string]ui.Color, len(stepGroups))
	for i, g := range stepGroups {
		hue := math.Mod(H+float64(i)*360/float64(len(stepGroups)), 360)
		groupColors[g] = oklchColor(0.64, 0.125, hue)
	}

	// 三栏海拔配色。OKLCH 的 L 即感知明度,直接当 M3 的 tone/100 用。
	// 亮色:base 是"基底"(顶栏/底栏/左栏/四周留白都用它,最暖的一层);
	// 中栏(steps)浮得最高、最白,tone≈0.99;右栏(files)次之,tone≈0.955。
	// 暗色:base 最暗≈0.10、steps≈0.165、files≈0.135,再给文件卡片浮到≈0.205。
	// chroma 都取极小的一点点主题色,避免纯灰发死。
	ltPane := paneTones{
		base:    oklchColor(0.930, cc*0.12, H),
		left:    oklchColor(0.930, cc*0.12, H),
		mid:     oklchColor(0.990, cc*0.03, H),
		right:   oklchColor(0.955, cc*0.09, H),
		card:    oklchColor(0.995, cc*0.03, H),
		outline: oklchColor(0.900, cc*0.16, H),
		shadow:  ui.RGBA(0, 0, 0, 0.08), // ≈8% 黑,让浮起的卡片有一点体积
		chip:    oklchColor(0.918, cc*0.14, H),
	}
	// 暗色 base 原来 L=0.105,实测渲出来 #000400——几乎纯黑,违反
	// PUI p.77「Avoid pure black」,而且三栏之间只有 1.03–1.12 的层次,
	// 根本读不出"谁浮在谁上面"。整体抬到 0.20/0.23/0.255/0.30,
	// 层次仍是 base < right < mid < card,只是不再贴着黑。
	dtPane := paneTones{
		base:    oklchColor(0.200, cc*0.20, H),
		left:    oklchColor(0.200, cc*0.20, H),
		mid:     oklchColor(0.255, cc*0.22, H),
		right:   oklchColor(0.230, cc*0.20, H),
		card:    oklchColor(0.300, cc*0.18, H),
		outline: oklchColor(0.400, cc*0.16, H),
		shadow:  ui.RGBA(0, 0, 0, 0.35), // 暗色下几乎不可见,只给一点体积
		chip:    oklchColor(0.360, cc*0.20, H),
	}

	return &derivedTheme{
		light:  lt,
		dark:   dt,
		selBg:  oklchColor(clamp(L, 0.30, 0.42), cc*0.9, H),
		selFg:  ui.RGB(255, 255, 255),
		cardBg: oklchColor(0.962, cc*0.12, H),
		// 补色给筛选范围,相近色给选取范围(文件名/扩展名两个相邻色相)
		rangeFilter:    oklchColor(0.62, 0.13, math.Mod(H+180, 360)),
		rangeScopeName: oklchColor(0.62, 0.13, math.Mod(H+30, 360)),
		rangeScopeExt:  oklchColor(0.62, 0.13, math.Mod(H+60, 360)),
		groupColors:    groupColors,
		lightPane:      ltPane,
		darkPane:       dtPane,
	}
}

// panes returns the three-pane elevation palette for the appearance.
func (d *derivedTheme) panes(dark bool) paneTones {
	if dark {
		return d.darkPane
	}
	return d.lightPane
}

// chipTones 把一个类别色转成"可读的胶囊底色/字色"这一对。
//
// 直接拿类别色当底色、白字压上去,实测 11 个分组只有 3.13–3.59、36 个
// 扩展名只有 3.10–3.60,全部低于 4.5。而且十几个饱和色块挤在同一行里,
// 谁都不突出——颜色本该用来区分,结果变成了噪声。
//
// 改成同一个色相的"浅底深字"(暗色反之):底色只留一点点色相,字色压深,
// 对比度 6.1 以上,类别仍然一眼可分,整行却安静下来。
func chipTones(col ui.Color, dark bool) (bg, fg ui.Color) {
	_, _, h := colorToOklch(col)
	if dark {
		return oklchColor(0.300, 0.060, h), oklchColor(0.880, 0.100, h)
	}
	return oklchColor(0.930, 0.050, h), oklchColor(0.420, 0.120, h)
}

// chipTonesEmph 是 chipTones 的强调版:同一个色相,底色再深一档。
// 用于"这就是结果"的胶囊(改名后的扩展名),让它比周围的原名胶囊重一点,
// 但不靠饱和色块去抢注意力。
func chipTonesEmph(col ui.Color, dark bool) (bg, fg ui.Color) {
	_, _, h := colorToOklch(col)
	if dark {
		return oklchColor(0.380, 0.075, h), oklchColor(0.930, 0.090, h)
	}
	return oklchColor(0.880, 0.075, h), oklchColor(0.360, 0.130, h)
}

// groupColor is the fixed color of a catalog group (falls back to the seed).
func (d *derivedTheme) groupColor(group string) ui.Color {
	if c, ok := d.groupColors[group]; ok {
		return c
	}
	return d.selBg
}

// cardBgFor is the settings-card ground for the current appearance: a
// near-neutral whisper of the seed, clean in light and dark.
func (d *derivedTheme) cardBgFor(dark bool) ui.Color {
	if dark {
		_, _, H := colorToOklch(d.selBg)
		return oklchColor(0.245, 0.012, H)
	}
	return d.cardBg
}

func accAt(t *ui.Theme, l, c, h float64) {
	t.Accent = oklchColor(l, c, h)
	t.AccentHover = oklchColor(l-0.05, c, h)
	t.AccentPressed = oklchColor(l-0.10, c, h)
	// 失焦不变灰:mygo 0.8 起窗口不活跃时把 Accent 换成 InactiveAccent,
	// 留空时按 AccentText 混出一层灰——切到别的窗口,整屏的主题色就褪掉,
	// 看起来像"这个程序被禁用了"。主色是用户选的,不该由焦点决定。
	t.InactiveAccent = t.Accent
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
