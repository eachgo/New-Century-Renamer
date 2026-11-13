package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 文件卡片里的小胶囊(路径、日期、大小)以前用 Theme.SurfaceHover 当底色,
// 而 SurfaceHover 和右栏底的明度几乎一样——实测对比度 1.00,胶囊贴在卡片
// 上只剩圆角能认出来,看着像没画底。给它们一个专属的 chip 色。
func TestInfoPillHasVisibleBackground(t *testing.T) {
	for _, seed := range []ui.Color{
		ui.RGB(0x2F, 0x7A, 0x52),
		ui.RGB(0xF2, 0x70, 0x3C),
		ui.RGB(0x31, 0x40, 0xA8),
		ui.RGB(0x8A, 0x2B, 0xE2),
	} {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			p := dt.panes(dark)
			// 1.2 是"看得出有一块底"的下限。色相不同会略差,取 1.2 留余量。
			if c := contrastRatio(p.chip, p.card); c < 1.2 {
				t.Errorf("seed %v dark=%v: 胶囊底与卡片底对比度只有 %.2f,看不出来", seed, dark, c)
			}
			// 同时必须比卡片底更深,不能比卡片还亮——那样胶囊会像凸起。
			if lum(p.chip) >= lum(p.card) && !dark {
				t.Errorf("seed %v: 亮色下胶囊底应当比卡片底深", seed)
			}
			if lum(p.chip) <= lum(p.card) && dark {
				t.Errorf("seed %v: 暗色下胶囊底应当比卡片底浅", seed)
			}
		}
	}
}

func lum(c ui.Color) float64 {
	return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
}
