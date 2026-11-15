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
			// 图标色按明暗各一套(见 material.go 的 groupColorsDark):
			// 同一个明度不可能同时压亮底和暗底,所以这里也要按 dark 取。
			for _, g := range stepGroups {
				if c := contrastRatio(dt.groupColorFor(g, dark), p.right); c < 3.0 {
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

// TestElevationReadsAsContentNotAsFrames 守住"卡片才是要读的那一层"。
//
// 窗口底与三栏之间不追求对比:它们是同一张桌面上的三块面板,近似色就够。
// 把这一层拉开到 1.25 会把整屏切成"深底 + 浅卡"两截,反而更吵——对比度
// 数值达标不等于好看。真正必须读得出来的是"栏里的内容":卡片对栏底。
//
// 所以断言分成两半:
//   - 三栏必须同色(它们是并排的兄弟,不该有前后关系)
//   - 卡片对三栏 ≥1.15:1(这一层是内容,必须浮起来)
//   - 窗口底与三栏之间不管——近似色是有意的
func TestElevationReadsAsContentNotAsFrames(t *testing.T) {
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
			p := dt.panes(dark)
			name := "亮色"
			if dark {
				name = "暗色"
			}
			if p.left != p.mid || p.mid != p.right {
				t.Errorf("%s %s: 三栏底色应当一致(左/中/右 = %v/%v/%v)——它们是并排的兄弟,不是有前后的层",
					hex, name, p.left, p.mid, p.right)
			}
			if got := contrastRatio(p.card, p.left); got < 1.15 {
				t.Errorf("%s %s: 卡片对栏底只有 %.2f:1,应当 ≥1.15——这一层是内容,必须浮得起来",
					hex, name, got)
			}
			// 底与栏之间不该被拉开:超过 1.15 说明又把桌面切成了两截。
			if got := contrastRatio(p.left, p.base); got > 1.15 {
				t.Errorf("%s %s: 窗口底与三栏差到 %.2f:1,它们是同一张桌面,近似色就够(拉开会把整屏切成两截)",
					hex, name, got)
			}
		}
	}
}
