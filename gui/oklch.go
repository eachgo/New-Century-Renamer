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
