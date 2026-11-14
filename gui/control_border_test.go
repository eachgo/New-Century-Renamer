package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 控件边框必须读得出来。
//
// WCAG 1.4.11「非文本对比度」要求界面控件的视觉边界对相邻颜色 ≥3:1——
// 输入框、下拉、按钮的轮廓属于"理解内容所必需的图形部分",淡到 2:1 就等于
// 让人靠位置猜哪里可以点。装饰性的分隔线不受这条约束(它不承载功能),
// 所以两者分成两个色:控件走 Theme.Border,分隔线走 paneTones.outline。
//
// 这条断言量的是 Theme.Border 对它可能落上的每一种底。改回 0.72(2.0–2.4)
// 会立刻红。
func TestControlBorderIsReadableOnEveryGround(t *testing.T) {
	seeds := []ui.Color{
		ui.RGB(0x2F, 0x7A, 0x52), // 默认绿
		ui.RGB(0xF2, 0x70, 0x3C), // 橙
		ui.RGB(0xF5, 0xC5, 0x18), // 黄(浅色相里最难)
		ui.RGB(0x00, 0xBC, 0xD4), // 青
		ui.RGB(0x8A, 0x2B, 0xE2), // 紫
		ui.RGB(0xE2, 0x2B, 0x5C), // 品红
		ui.RGB(0x31, 0x40, 0xA8), // 蓝
		ui.RGB(0x7F, 0xBF, 0x3F), // 亮绿
		ui.RGB(0x00, 0x00, 0x00), // 纯黑
		ui.RGB(0xFF, 0xFF, 0xFF), // 纯白
	}
	for _, seed := range seeds {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			th := dt.light
			p := dt.lightPane
			if dark {
				th, p = dt.dark, dt.darkPane
			}
			grounds := []struct {
				name string
				c    ui.Color
			}{
				{"base", p.base}, {"left", p.left}, {"mid", p.mid},
				{"right", p.right}, {"card", p.card}, {"chip", p.chip},
			}
			for _, g := range grounds {
				if c := contrastRatio(th.Border, g.c); c < 3.0 {
					t.Errorf("seed %v dark=%v: 控件边框对 %s 只有 %.2f:1,应 ≥3:1",
						seed, dark, g.name, c)
				}
			}
		}
	}
}

// 分隔线是装饰,不受 3:1 约束——但它必须"看得见",否则一条 1px 的线
// 就是白画的。同时它不能比控件边框还显眼,否则装饰抢了功能的注意力。
func TestDividerIsVisibleButQuieterThanControls(t *testing.T) {
	for _, seed := range []ui.Color{ui.RGB(0x2F, 0x7A, 0x52), ui.RGB(0x00, 0x00, 0x00)} {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			th := dt.light
			p := dt.lightPane
			if dark {
				th, p = dt.dark, dt.darkPane
			}
			div := contrastRatio(p.outline, p.base)
			bord := contrastRatio(th.Border, p.base)
			if div < 1.05 {
				t.Errorf("seed %v dark=%v: 分隔线对 base 只有 %.3f:1,等于没画", seed, dark, div)
			}
			if div >= bord {
				t.Errorf("seed %v dark=%v: 分隔线(%.2f)不该比控件边框(%.2f)更显眼",
					seed, dark, div, bord)
			}
		}
	}
}

// 目录图标是"这一组是哪类处理"的唯一非文字线索,必须对左栏底色 ≥3:1。
// 11 组色相不同,取最差那组——平均值没有意义,用户看到的是最淡的那个。
func TestCatalogGroupIconsAreReadable(t *testing.T) {
	seeds := []ui.Color{
		ui.RGB(0x2F, 0x7A, 0x52), ui.RGB(0xF2, 0x70, 0x3C), ui.RGB(0xF5, 0xC5, 0x18),
		ui.RGB(0x00, 0xBC, 0xD4), ui.RGB(0x8A, 0x2B, 0xE2), ui.RGB(0xE2, 0x2B, 0x5C),
		ui.RGB(0x31, 0x40, 0xA8), ui.RGB(0x7F, 0xBF, 0x3F), ui.RGB(0x00, 0x00, 0x00), ui.RGB(0xFF, 0xFF, 0xFF),
	}
	for _, seed := range seeds {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			p := dt.lightPane
			if dark {
				p = dt.darkPane
			}
			// 图标坐在左栏那张卡片上,不是坐在窗口底上——左栏容器用的是
			// pt.right(0.923),不是 pt.base(0.852)。量错底会低估 0.3 左右。
			for _, g := range stepGroups {
				if c := contrastRatio(dt.groupColor(g), p.right); c < 3.0 {
					t.Errorf("seed %v dark=%v: 目录图标 %q 对左栏底只有 %.2f:1,应 ≥3:1",
						seed, dark, g, c)
				}
			}
		}
	}
}

// 类别胶囊(步骤行右端那个色块 + 文字)是"哪一类处理"的第二重编码。
// 文字对胶囊底、胶囊底对所在底,两段都要读得出来。
func TestCategoryChipIsReadable(t *testing.T) {
	for _, seed := range []ui.Color{ui.RGB(0x2F, 0x7A, 0x52), ui.RGB(0x00, 0x00, 0x00), ui.RGB(0xF5, 0xC5, 0x18)} {
		dt := themeFor(seed)
		for _, dark := range []bool{false, true} {
			p := dt.lightPane
			if dark {
				p = dt.darkPane
			}
			for _, g := range stepGroups {
				bg, fg := chipTones(dt.groupColor(g), dark)
				if c := contrastRatio(fg, bg); c < 4.5 {
					t.Errorf("seed %v dark=%v: 类别胶囊 %q 的字对胶囊底只有 %.2f:1,应 ≥4.5",
						seed, dark, g, c)
				}
				if c := contrastRatio(bg, p.mid); c < 1.15 {
					t.Errorf("seed %v dark=%v: 类别胶囊 %q 的底对中栏底只有 %.2f:1,看不出是一块底",
						seed, dark, g, c)
				}
			}
		}
	}
}

// TestElevationHasTwoSteps 守住"三栏同色、底与卡片各差一级"这个结构。
//
// 三栏是并排的兄弟,不是叠起来的层——它们之间靠分隔缝区分,不靠明度。
// 早先给左/中/右各一个不同明度(0.930/0.990/0.955),相邻级差只有
// 1.02–1.10:看着像三个没调匀的灰,而且中栏比右栏亮、右栏比左栏亮,
// 读者会以为它们有前后关系。
//
// 为什么不是三级各 1.25:亮色下从 card 0.999 倒推会得到
// mid=0.923 right=0.853 left=0.786,而 L 0.786 的左栏上 TextMuted 必须
// 压到 L≤0.410 才能 4.5:1——那个亮度与正文 0.24 只差 1.95:1,次要文字
// 就和正文分不开了。三级 1.25 与文字可读性不可兼得,所以只做两级。
func TestElevationHasTwoSteps(t *testing.T) {
	seeds := []ui.Color{
		ui.RGB(0x2F, 0x7A, 0x52), ui.RGB(0xF2, 0x70, 0x3C), ui.RGB(0xF5, 0xC5, 0x18),
		ui.RGB(0x00, 0xBC, 0xD4), ui.RGB(0x8A, 0x2B, 0xE2), ui.RGB(0xE2, 0x2B, 0x5C),
		ui.RGB(0x31, 0x40, 0xA8), ui.RGB(0x7F, 0xBF, 0x3F), ui.RGB(0x00, 0x00, 0x00),
		ui.RGB(0xFF, 0xFF, 0xFF),
	}
	for _, seed := range seeds {
		dt := themeFor(seed)
		hex := colorHex(seed)
		for _, dark := range []bool{false, true} {
			p := dt.lightPane
			name := "亮色"
			if dark {
				p = dt.darkPane
				name = "暗色"
			}
			if p.left != p.mid || p.mid != p.right {
				t.Errorf("%s %s: 三栏底色应当一致(左/中/右 = %v/%v/%v)——它们是并排的兄弟,不是有前后的层",
					hex, name, p.left, p.mid, p.right)
			}
			if got := contrastRatio(p.left, p.base); got < 1.22 {
				t.Errorf("%s %s: 窗口底→三栏只有 %.2f:1,应当 ≥1.22(两级海拔之一)", hex, name, got)
			}
			if got := contrastRatio(p.card, p.left); got < 1.22 {
				t.Errorf("%s %s: 三栏→卡片只有 %.2f:1,应当 ≥1.22(两级海拔之二)", hex, name, got)
			}
		}
	}
}
