package main

import (
	"fmt"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The theme is derived from one seed color (matugen-style): the user picks
// it in settings, or it follows the desktop's accent color. See
// material.go for the palette derivation.
const fallbackSeed = "#2F7A52" // Mono deep green

// currentTheme resolves seed + mode into the frame's theme.
func (a *app) currentTheme() *derivedTheme { return themeFor(a.currentSeed()) }

func (a *app) isDark() bool {
	switch a.settings.Theme.Mode {
	case "浅色":
		return false
	case "深色":
		return true
	}
	return systemIsDark()
}

func (a *app) currentSeed() ui.Color {
	if a.settings.Theme.Seed != "" {
		return ui.Hex(a.settings.Theme.Seed)
	}
	if c, ok := systemAccent(); ok {
		return c
	}
	return ui.Hex(fallbackSeed)
}

// selColors gives the catalog/steps selection colors for the current theme.
func (a *app) selColors() (bg, fg ui.Color) {
	dt := a.currentTheme()
	return dt.selBg, dt.selFg
}

// inTests short-circuits mygo calls that block until the app runs, which
// would hang unit tests; TestMain sets it.
var inTests bool

// systemAccent reads the desktop's accent color; ok=false where the
// desktop has none (some Linux setups) or in tests.
func systemAccent() (c ui.Color, ok bool) {
	if inTests {
		return ui.Color{}, false
	}
	defer func() { recover() }()
	p := mygo.Theme.Preferences()
	if p.Accent.A == 0 {
		return ui.Color{}, false
	}
	return ui.RGB(p.Accent.R, p.Accent.G, p.Accent.B), true
}

func systemIsDark() (dark bool) {
	if inTests {
		return false
	}
	defer func() { recover() }()
	return mygo.Theme.IsDark()
}

func hexc(r, g, b uint8) ui.Color { return ui.Color{R: r, G: g, B: b, A: 255} }

// colorHex renders a ui.Color as #RRGGBB.
func colorHex(c ui.Color) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }
