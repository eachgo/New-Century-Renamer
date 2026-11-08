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
	lt.SurfaceHover = tone(0.955, 0.16)
	lt.SurfacePressed = tone(0.930, 0.18)
	lt.Border = tone(0.920, 0.16)
	lt.Text = tone(0.24, 0.30)
	lt.TextMuted = tone(0.62, 0.20)
	accAt(lt, accL, cc, H)
	lt.AccentText = ui.RGB(255, 255, 255)
	lt.Selection = tone(0.925, 0.35)
	lt.Focus = oklchColor(accL, cc, H)
	lt.Inverse = tone(0.28, 0.85)
	lt.InverseText = ui.RGB(255, 255, 255)
	lt.Scrollbar = tone(0.870, 0.20)
	lt.Radius = 10
	lt.Spacing = 5

	dt := ui.DarkTheme()
	dt.Background = tone(0.165, 0.28)
	dt.Surface = tone(0.215, 0.24)
	dt.SurfaceHover = tone(0.265, 0.24)
	dt.SurfacePressed = tone(0.305, 0.24)
	dt.Border = tone(0.325, 0.22)
	dt.Text = tone(0.945, 0.10)
	dt.TextMuted = tone(0.700, 0.12)
	accAt(dt, clamp(L, 0.72, 0.82), cc, H)
	dt.AccentText = tone(0.16, 0.5)
	dt.Selection = tone(0.300, 0.55)
	dt.Focus = oklchColor(clamp(L, 0.72, 0.82), cc, H)
	dt.Inverse = tone(0.93, 0.10)
	dt.InverseText = tone(0.24, 0.30)
	dt.Scrollbar = tone(0.400, 0.20)
	dt.Radius = 10
	dt.Spacing = 5

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
	}
	dtPane := paneTones{
		base:    oklchColor(0.105, cc*0.20, H),
		left:    oklchColor(0.105, cc*0.20, H),
		mid:     oklchColor(0.165, cc*0.22, H),
		right:   oklchColor(0.135, cc*0.20, H),
		card:    oklchColor(0.205, cc*0.18, H),
		outline: oklchColor(0.300, cc*0.16, H),
		shadow:  ui.RGBA(0, 0, 0, 0.35), // 暗色下几乎不可见,只给一点体积
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
