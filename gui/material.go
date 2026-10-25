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
// The derivation keeps reamer's "Mono" temperament: calm ground, restrained
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
		return ui.Oklch(float32(l), float32(cc*cMul), float32(H))
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
	lt.Focus = ui.Oklch(float32(accL), float32(cc), float32(H))
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
	dt.Focus = ui.Oklch(float32(clamp(L, 0.72, 0.82)), float32(cc), float32(H))
	dt.Inverse = tone(0.93, 0.10)
	dt.InverseText = tone(0.24, 0.30)
	dt.Scrollbar = tone(0.400, 0.20)
	dt.Radius = 10
	dt.Spacing = 5

	groupColors := make(map[string]ui.Color, len(stepGroups))
	for i, g := range stepGroups {
		hue := math.Mod(H+float64(i)*360/float64(len(stepGroups)), 360)
		groupColors[g] = ui.Oklch(0.64, 0.125, float32(hue))
	}
	return &derivedTheme{
		light:  lt,
		dark:   dt,
		selBg:  ui.Oklch(float32(clamp(L, 0.30, 0.42)), float32(cc*0.9), float32(H)),
		selFg:  ui.RGB(255, 255, 255),
		cardBg: ui.Oklch(0.962, float32(cc*0.12), float32(H)),
		// 补色给筛选范围,相近色给选取范围(文件名/扩展名两个相邻色相)
		rangeFilter:    ui.Oklch(0.62, 0.13, float32(math.Mod(H+180, 360))),
		rangeScopeName: ui.Oklch(0.62, 0.13, float32(math.Mod(H+30, 360))),
		rangeScopeExt:  ui.Oklch(0.62, 0.13, float32(math.Mod(H+60, 360))),
		groupColors:    groupColors,
	}
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
		return ui.Oklch(0.245, 0.012, float32(H))
	}
	return d.cardBg
}

func accAt(t *ui.Theme, l, c, h float64) {
	t.Accent = ui.Oklch(float32(l), float32(c), float32(h))
	t.AccentHover = ui.Oklch(float32(l-0.05), float32(c), float32(h))
	t.AccentPressed = ui.Oklch(float32(l-0.10), float32(c), float32(h))
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
