package main

import (
	"github.com/egoist/mygo/ui"
)

// 边框的两种语气:能点的带主题色,不能点的变中性灰。
//
// 原来全软件只有一种边框色 Theme.Border = tone(0.555, 0.16)——彩度只有
// 0.004–0.018,肉眼就是一块灰。于是"能点"和"不能点"的框长得一模一样,
// 用户只能靠试:点「回滚上次」没反应、点「导出清单」没反应,而它们看起来
// 和「添加文件」一样亮。更要命的是这些灰按钮原来根本没被禁用——
// Clicked() 照样返回 true,只是动作跑起来无事发生,看起来就是"点了没反应"。
//
// 现在的规则只有一条,全软件共用:
//
//   - 能点:边框是主题色的次生色(同色相、彩度降到主色的六成、中等明度),
//     对任何一种底都 ≥3:1,WCAG 1.4.11 说的"控件边界"。
//   - 不能点:边框是**零彩度**的中性灰,比能点的更淡(1.6–2.6:1),而且
//     真的禁用(flagDisabled)——点击不触发、不进 Tab 序列。
//
// 为什么用"有没有彩"当信号:这块软件里所有底色、卡片、胶囊、图标、正文都
// 带一点主题色相,只有"这里按不动"的框是纯灰。彩色 = 活着,灰色 = 死了,
// 这条线在灰度截图里也成立(因为两者明度也差着)。

// isDarkTheme 从主题本身判断明暗,不依赖 app 的设置——这样颜色函数只要
// 拿到 *ui.Theme 就能用,不必知道当前是哪套外观。
func isDarkTheme(t *ui.Theme) bool {
	return relativeLuminance(t.Background) < 0.35
}

// liveBorder 是"这里能点"的边框:主题色的次生色。
//
// 明度按外观给两档(亮色压深、暗色提亮),彩度取主色的六成——主色本身
// 被夹在 CC 上限 0.115,六成后是 0.012–0.069,足够看出"这是绿的/橙的",
// 又不到饱和色的程度(饱和描边会把每个按钮变成重色块,和"次要动作"的
// 定位相反)。下限 0.012:纯灰的种子(黑白)也要留一丝彩,否则"能点"和
// "不能点"又回到同一种灰。
func liveBorder(t *ui.Theme) ui.Color {
	_, c, h := colorToOklch(t.Accent)
	return accentBorder(c, h, isDarkTheme(t))
}

// accentBorder 是 liveBorder 的实质:明度按外观两档,彩度取主色六成。
// 单独拿出来是因为画主题的那几十行(见 material.go 的 buildTheme)在
// 设 Theme.Border 的时候还没设 Theme.Accent——那时只能把主色的 OKLCH
// 三个分量显式传进来。两处若各写一遍,调一半就会分叉。
func accentBorder(c, h float64, dark bool) ui.Color {
	l := 0.545
	if dark {
		l = 0.660
	}
	return oklchColor(l, clamp(c*0.60, 0.012, 0.070), h)
}

// inertBorder 是"这里按不动"的边框:零彩度的中性灰。
//
// 明度取"看得见但立不起来"的那一档:亮色 0.700(对卡片 2.4:1、对左栏
// 2.0:1),暗色 0.420(对卡片 2.1:1、对中栏 1.7:1)。都低于 3:1——低于
// 3:1 正是有意的:WCAG 1.4.11 只管"能操作的控件",一个禁用控件不需要
// 边界达标,而"边界不达标"本身就在说"这不是能操作的东西"。
//
// 但不能淡到看不见:用户需要知道"这儿有个按钮、只是现在不能按",否则
// 那块地方就是一片空白,连"少了什么"都看不出来。
func inertBorder(t *ui.Theme) ui.Color {
	if isDarkTheme(t) {
		return oklchColor(0.420, 0, 0)
	}
	return oklchColor(0.700, 0, 0)
}

// 卡片不掺主题色,这一点是量出来的,不是审美。
//
// 一开始给预设卡掺 Accent.Alpha(0.16),整块卡立刻绿得比淡底按钮还响——
// 那正是 tonalButton(Accent.Alpha(0.12)) 的处方,同样的浓度铺满一整块,
// 卡片就成了一颗大按钮。降到 0.10 也不行:亮色的卡片底本来就是 p.card
// (L 0.995,近白),掺色只会把它往下拉,瓦片感从 1.152 掉到 1.010;
// 暗色反过来,掺色把它拉向淡底按钮的绿,和 tonalButton 只差 1.006。
//
// 所以卡片底就是"抬高一层的那个中性色"本身——栏里的 p.card、弹窗里的
// Surface。它和栏底本来就有 1.15/1.17 的差,够读成一块瓦片,而且明度上
// 离实心按钮和淡底按钮都很远。卡片"能点"这件事由悬停时的主题色描边
// 承担,和按钮同一条规则(框带主题色 = 能点),静止时它没有轮廓,所以
// 四张预设卡不会读成四颗按钮。

// cardBorder 是卡片的常驻边框:透明。
//
// 透明而不是不画,是因为 mygo 的边框算在盒子尺寸里——平时没有、悬停才
// 加一条,整块内容会横竖各跳 2 DIP(拖放区踩过这个坑)。所以平时留一条
// 看不见的,悬停时只换颜色。
func cardBorder(t *ui.Theme) ui.Color { return ui.Color{} }

// cardBorderHover 是悬停时卡片亮起来的轮廓。
//
// 用"能点"的主题色(和按钮同一条规则),因为卡片本来就点得动;而它只在
// 悬停时出现,静止时没有轮廓,所以四张预设卡不会读成四颗按钮。
func cardBorderHover(t *ui.Theme) ui.Color { return liveBorder(t) }

// neutralStroke 是"这不是控件"的描边:纯灰、比控件边框淡。
//
// 它和 inertBorder 是同一个颜色,用途却更广——装饰性的描边(颜色小方块、
// 进度条槽、流程图方框、状态点)也用它。这些地方如果借用控件边框,整屏
// 就会出现一片"看起来能点"的框;而它们本来就只是"把这块区域圈出来"。
func neutralStroke(t *ui.Theme) ui.Color { return inertBorder(t) }

// inertShell 把一颗不能点的按钮包进一层"还能被指针碰到"的壳里。
//
// 为什么必须包:mygo v0.9.0 里禁用的元素既不收点击,也不收悬停
// (ui/tooltip.go 的 tipped 要求 flagDisabled == 0),所以挂在它身上的
// Tooltip 永远不会出现。而"为什么这颗按钮点不了"恰恰是最该解释的一句。
// 壳本身没被禁用,提示挂在壳上就能照常弹出;壳里那颗照样是真禁用。
//
// 壳用 Box(纵向容器)+ Shrink(0):尺寸由里面那颗按钮撑开,放进工具栏
// 或行里不改变任何布局。
func inertShell(c *ui.Context, build func()) ui.Element {
	shell := ui.Box(c).Shrink(0)
	shell.Children(build)
	return shell
}

// frameButton 是全软件唯一的"有框按钮":Surface 底 + 主题色边框。
//
// 取代 ui.Button:它的边框走 Theme.Border(中性灰),和禁用态分不开。
// 几何与 ui.Button 一致(Padding 6/14、Radius 6),所以换掉之后布局不变。
func frameButton(c *ui.Context, label string, enabled bool) ui.Element {
	t := c.Theme()
	if !enabled {
		return inertShell(c, func() {
			b := ui.ButtonBase(c).Radius(6).Padding(8, 12).
				TextColor(t.TextMuted).Background(ui.Color{}).
				Border(1, inertBorder(t)).DimDisabled(false).Disabled(true)
			b.Children(func() { ui.Text(c, label).SingleLine() })
		})
	}
	b := ui.ButtonBase(c).Radius(6).Padding(8, 12).
		TextColor(t.Text).Background(t.Surface).Border(1, liveBorder(t))
	if b.Hovered() {
		b = b.Background(t.SurfaceHover).Border(1, t.Accent)
	}
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}
