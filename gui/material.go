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
	//
	// 0.60 是"对五种底都 ≥3:1"的上界(实测算出来的:base/left/mid/right/
	// card/chip 里最难的是 card,0.605 才刚过线,取 0.60 留一点余量)。
	// 这是 WCAG 1.4.11 对"非文本界面元素"的要求——输入框、下拉、按钮的
	// 轮廓属于"理解内容所必需的图形部分",淡到 2.0 就等于让人靠猜。
	// 分隔线是装饰,不受这条约束,所以它走 outline 而不是这里。
	lt.Border = tone(0.555, 0.16)
	lt.Text = tone(0.24, 0.30)
	// 次要文字承载路径、日期、大小、分组计数——是信息不是装饰,必须读得出来。
	// L 0.62 时在左栏底色上只有 2.94:1(远低于 WCAG 4.5);0.50 让四种底色
	// (左/中/右栏/卡片)全部 ≥4.8,同时仍明显浅于正文(L 0.24),层级还在。
	lt.TextMuted = tone(0.455, 0.20)
	accAt(lt, accL, cc, H)
	// 压在实心主色上的字:白字还是黑字,看哪个读得出来。
	//
	// 主色的明度被 accL 夹在 0.55–0.68,白字在浅色相(黄、青、亮绿)上
	// 只有 2.76–2.89:1——「执行更名」四个字糊成一片。全色域扫一遍
	// (H 每 5°、C 六档、L 四档)可以证明:按"白字与深墨取对比度高者"
	// 这条规则,最差的一档是 4.56:1,全部达标;而只用白字最差 3.35。
	lt.AccentText = inkOn(lt.Accent)
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
	// 0.325 时控件边界只有 1.46:1,等于没有边界。暗色下"更显眼"是往上抬
	// 而不是往下压——底是 L 0.20–0.30,边框越亮对比越高。实测要 0.625 才能
	// 让最难的 card(L 0.300)过 3:1;0.56 在卡片上只有 2.91。
	dt.Border = tone(0.665, 0.22)
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

	// 目录图标(11 个组的色相标记)。0.64 时对左栏底色只有 2.52–2.57:1,
	// 低于 WCAG 1.4.11 对"图形与相邻颜色"的 3:1——图标是"这一组是哪类
	// 处理"的唯一非文字线索,读不出来就只剩文字标签,色相编码白给。
	// 实测算到 0.59 是上界(11 组里最差那组刚好过线),取 0.58 留余量;
	// 色度也从 0.125 降到 0.10,亮色下高彩度会显脏。
	groupColors := make(map[string]ui.Color, len(stepGroups))
	for i, g := range stepGroups {
		hue := math.Mod(H+float64(i)*360/float64(len(stepGroups)), 360)
		// L 0.58 时对左栏底只有 2.6:1(高彩度会把感知亮度往下拽,比中性色
		// 暗一档),低于 WCAG 1.4.11 对"理解内容所必需的图形"要求的 3:1。
		//
		// 0.557 是亮暗两侧唯一都过线的值,窗口只有 0.002 宽:
		// 亮色下左栏底 0.923,再亮就掉到 3.0 以下(上界 0.558);
		// 暗色下左栏底 0.265,再暗也掉到 3.0 以下(下界 0.556)。
		// 改这个数会同时踩到一侧,动之前先跑 TestCatalogGroupIconsAreReadable。
		groupColors[g] = oklchColor(0.557, 0.10, hue)
	}

	// 三栏海拔配色。OKLCH 的 L 即感知明度,直接当 M3 的 tone/100 用。
	// 亮色:base 是"基底"(顶栏/底栏/左栏/四周留白都用它,最暖的一层);
	// 中栏(steps)浮得最高、最白,tone≈0.99;右栏(files)次之,tone≈0.955。
	// 暗色:base 最暗≈0.10、steps≈0.165、files≈0.135,再给文件卡片浮到≈0.205。
	// chroma 都取极小的一点点主题色,避免纯灰发死。
	ltPane := paneTones{
		// 海拔只有两级:窗口底 → 三栏 → 卡片。三栏是并排的兄弟,不是
		// 叠起来的层,它们之间靠分隔缝区分——早先给左/中/右各一个不同的
		// 明度(0.930/0.990/0.955),结果相邻级差只有 1.02–1.10,看着像
		// 三个没调匀的灰,而不是三个并列的面板;而且中栏比右栏亮、右栏
		// 比左栏亮,读者会以为它们有前后关系。
		//
		// 现在:三栏同高 0.923,窗口底压到 0.852,卡片 0.999。
		// base→pane = 1.25:1、pane→card = 1.25:1,两级都够看得出。
		// 窗口底不能再亮了:再亮一级就只剩 1.1,压再暗 TextMuted 就
		// 过不了 4.5:1(实测 pane=0.923 时 TextMuted 是 4.75)。
		base:    oklchColor(0.852, cc*0.14, H),
		left:    oklchColor(0.923, cc*0.10, H),
		mid:     oklchColor(0.923, cc*0.10, H),
		right:   oklchColor(0.923, cc*0.10, H),
		card:    oklchColor(0.999, cc*0.02, H),
		outline: oklchColor(0.800, cc*0.16, H),
		shadow:  ui.RGBA(0, 0, 0, 0.14), // 卡片浮在 1.25 级差的栏上,影子要跟得上
		chip:    oklchColor(0.870, cc*0.14, H),
	}
	// 暗色 base 原来 L=0.105,实测渲出来 #000400——几乎纯黑,违反
	// PUI p.77「Avoid pure black」,而且三栏之间只有 1.03–1.12 的层次,
	// 根本读不出"谁浮在谁上面"。整体抬到 0.20/0.23/0.255/0.30,
	// 层次仍是 base < right < mid < card,只是不再贴着黑。
	dtPane := paneTones{
		// 暗色同一套两级:base 0.170 → pane 0.265 → card 0.330,
		// 实测 1.24 / 1.25(暗色下同一组数字略偏,已按实测微调)。
		base:    oklchColor(0.170, cc*0.22, H),
		left:    oklchColor(0.265, cc*0.20, H),
		mid:     oklchColor(0.265, cc*0.20, H),
		right:   oklchColor(0.265, cc*0.20, H),
		card:    oklchColor(0.330, cc*0.18, H),
		outline: oklchColor(0.430, cc*0.16, H),
		shadow:  ui.RGBA(0, 0, 0, 0.45), // 暗色下影子本来就不明显,加深一档才有体积
		chip:    oklchColor(0.390, cc*0.20, H),
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
// chipTones 给类别胶囊一对"底 + 字"。底只要"看得出是一块底"就够——
// 胶囊里写着类别名,色块不是唯一线索,所以不受 WCAG 1.4.11 对图形边界
// 的 3:1 约束(硬拉到 3:1 会把浅标签变成 L 0.655 的实心重色块,和它
// "轻量标注"的定位相反)。真正必须达标的是字对底 4.5:1,实测 6.2–7.4。
//
// 暗色底从 0.300 抬到 0.325:中栏底是 0.255,原来只差 1.11–1.14,胶囊
// 在中栏上几乎看不出边界。
func chipTones(col ui.Color, dark bool) (bg, fg ui.Color) {
	_, _, h := colorToOklch(col)
	if dark {
		return oklchColor(0.420, 0.060, h), oklchColor(0.880, 0.100, h)
	}
	// 亮色底从 0.930 压到 0.880:三栏统一到 0.923 之后,0.930 与中栏底
	// 只差 1.00–1.04,胶囊彻底看不出边界(和三栏分级是同一件事的两面)。
	return oklchColor(0.866, 0.050, h), oklchColor(0.395, 0.120, h)
}

// chipTonesEmph 是 chipTones 的强调版:同一个色相,底色再深一档。
// 用于"这就是结果"的胶囊(改名后的扩展名),让它比周围的原名胶囊重一点,
// 但不靠饱和色块去抢注意力。
func chipTonesEmph(col ui.Color, dark bool) (bg, fg ui.Color) {
	_, _, h := colorToOklch(col)
	if dark {
		return oklchColor(0.455, 0.075, h), oklchColor(0.945, 0.090, h)
	}
	return oklchColor(0.860, 0.075, h), oklchColor(0.345, 0.130, h)
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

// inkOn 返回压在实心底色 ground 上读得最清楚的字色:纯白或深墨,
// 取对比度高的那个。深墨的明度取 0.10——比正文的 0.24 还深,
// 因为它要压的是一块饱和色,不是近白的底。
func inkOn(ground ui.Color) ui.Color {
	dark := oklchColor(0.10, 0.02, 0)
	if contrastRatio(ground, dark) > contrastRatio(ground, ui.RGB(255, 255, 255)) {
		return dark
	}
	return ui.RGB(255, 255, 255)
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
