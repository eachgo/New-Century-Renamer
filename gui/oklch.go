package main

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// OKLCH math for deriving themes from a seed color, matugen-style. mygo's
// ui.Oklch builds colors FROM oklch; this adds the way back (sRGB → oklch)
// so any seed color can drive the palette.

// colorToOklch returns (L, C, H in degrees) of an sRGB color.
func colorToOklch(c ui.Color) (l, ch, h float64) {
	r := lin(float64(c.R) / 255)
	g := lin(float64(c.G) / 255)
	b := lin(float64(c.B) / 255)
	l_ := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	m_ := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	s_ := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	L := 0.2104542553*l_ + 0.7936177850*m_ - 0.0040720468*s_
	A := 1.9779984951*l_ - 2.4285922050*m_ + 0.4505937099*s_
	B := 0.0259040371*l_ + 0.7827717662*m_ - 0.8086757660*s_
	h = math.Atan2(B, A) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return L, math.Hypot(A, B), h
}

func lin(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// relativeLuminance is the WCAG luminance of a color, for contrast checks.
func relativeLuminance(c ui.Color) float64 {
	f := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

// contrastRatio is WCAG contrast between two colors.
func contrastRatio(a, b ui.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// oklchColor builds an sRGB ui.Color from OKLCH directly, bypassing
// ui.Oklch: its gamut mapping dead-loops on certain hues (found the hard
// way — the per-extension hue ring walks the whole circle).
func oklchColor(l, c, hDeg float64) ui.Color {
	h := hDeg * math.Pi / 180
	a := c * math.Cos(h)
	b := c * math.Sin(h)
	l_ := math.Pow(l+0.3963377774*a+0.2158037573*b, 3)
	m_ := math.Pow(l-0.1055613458*a-0.0638541728*b, 3)
	s_ := math.Pow(l-0.0894841775*a-1.2914855480*b, 3)
	lin := func(v float64) uint8 {
		if v <= 0 {
			return 0
		}
		if v >= 1 {
			return 255
		}
		srgb := 1.055*math.Pow(v, 1/2.4) - 0.055
		if srgb < 0 {
			srgb = 0
		}
		return uint8(math.Round(srgb * 255))
	}
	r := 4.0767416621*l_ - 3.3077115913*m_ + 0.2309699292*s_
	g := -1.2684380046*l_ + 2.6097574011*m_ - 0.3413193965*s_
	bl := -0.0041960863*l_ - 0.7034186147*m_ + 1.7076147010*s_
	return ui.Color{R: lin(r), G: lin(g), B: lin(bl), A: 255}
}
