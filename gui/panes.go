package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// splitPanes 把一栏分成左右两半，中间是一条**可拖拽的空白缝**，不画线。
//
// 为什么不用 ui.Split：mygo 的分隔条在 split.go 里把颜色写死成
// t.Border（悬停时 t.Accent），没有参数可传。而 t.Border 同时是输入框、
// 下拉框、普通按钮的边框色——为了不让输入框的边看不见，Border 必须
// 足够深；深了以后那两条 1 DIP 的分隔线就成了整屏最扎眼的线，比它们
// 分隔的内容还显眼。用户的原话是"出现的细线分割不对"。
//
// 这里改成留缝：三栏本来就是浮在 base 底色上的圆角卡片（见 view() 的
// 注释），中间露出 base 本身就完成了分隔，还能顺便呼吸。缝的宽度与
// 卡片四周的内边距同一个值，所以三栏的节奏是统一的。
//
// 内边距只在最外层加一次：嵌套调用（中/右栏那一层）传 pad=0，否则
// 第二层又缩进 8px，中/右栏的卡片顶边会比左栏低 8px——三栏就不再
// 站在同一条水平线上了。
func (a *app) splitPanes(c *ui.Context, size *float32, first, second func()) {
	a.splitPanesInset(c, size, 8, 0, first, second)
}

// splitPanesInset 里的 availW 是**这一层真正能用的宽度**。嵌套调用必须
// 由调用方算好传进来：c.Size() 返回的始终是整个窗口的尺寸，不是父级
// 那一栏的宽度。以前中/右栏那一层也拿窗口宽度做夹紧，于是把 splitM
// 一路拖到 330 时右栏只剩几十 DIP，卡片里的名字被压成一列竖排——
// 夹紧看上去生效了，其实上限比实际可用宽度大了两倍多。
// availW <= 0 表示"就用窗口宽度"（最外层）。
func (a *app) splitPanesInset(c *ui.Context, size *float32, pad, availW float32, first, second func()) {
	t := c.Theme()
	w, _ := c.Size()
	// 缝宽与内边距同值：拖拽热区只有 1 DIP 时几乎点不中，这里给足
	// 10 DIP，视觉上仍是"一条空缝"。
	gripW := float32(10)
	minPane := t.Space(32) // 再窄就放不下工具栏，宁可不让拖
	avail := w - 2*pad - gripW
	if availW > 0 {
		avail = availW - gripW
	}
	if maxSize := avail - minPane; maxSize > minPane {
		if *size < minPane {
			*size = minPane
		}
		if *size > maxSize {
			*size = maxSize
		}
	} else if *size < 0 {
		*size = 0
	}

	ui.Row(c).Grow(1).FillWidth().Padding(pad, pad, pad, pad).Children(func() {
		ui.Column(c).Width(*size).FillHeight().Clip().Children(first)

		// 缝本身是拖拽把手：整条竖带都能抓，光标提示可拖。
		//
		// 深色下三栏同色(实测 mid/right = 1.000),不加一条线整屏会糊成
		// 一块——用户分不清哪块内容属于哪一栏。亮色不用:亮色 base 与
		// 三栏本来就有 1.05 的差,再加线反而显得碎。
		grip := ui.Row(c).Width(gripW).FillHeight().Focusable().Cursor(ui.CursorResizeEW).
			AlignItems(ui.Center)
		if a.isDark() {
			// 缝有 10 DIP 宽(拖拽热区),但分界线只要 1 DIP:整条填色会变成
			// 一根 10 像素粗的亮条,比三栏本身还抢眼。
			grip.Children(func() {
				ui.Box(c).Width(1).FillHeight().Background(a.currentTheme().panes(true).outline)
			})
		}
		if dx, _, held := grip.Dragged(); held && dx != 0 {
			*size = clampPane(*size+dx, minPane, avail-minPane)
			a.splitDragging = true
		} else if !held && a.splitDragging {
			// 手一松就落盘。拖动途中每帧写文件既没必要(鼠标一动就是几十
			// 次写盘),也会让"拖到一半崩了"留下一个半截的宽度。
			a.splitDragging = false
			a.persistSplit()
		}
		// 键盘：缝拿到焦点后左右键推，和拖动等价（可访问性底线）。
		if grip.Focused() {
			if c.Shortcut(0, ui.KeyLeft) {
				*size = clampPane(*size-t.Space(2.5), minPane, avail-minPane)
				a.persistSplit()
			}
			if c.Shortcut(0, ui.KeyRight) {
				*size = clampPane(*size+t.Space(2.5), minPane, avail-minPane)
				a.persistSplit()
			}
		}

		ui.Column(c).Grow(1).FillHeight().Clip().Children(second)
	})
}

// persistSplit 把两条分隔线的位置写回设置文件。
//
// 用户把左栏拖宽是要看完整的规则名,把右栏拖宽是因为文件名长——都是
// "调过一次就不想再调"的东西。原来只在内存里,重启回到 220/330,于是
// 每天开机第一件事是把两根线再拖一遍。
//
// 拖动结束才写:拖动中每帧写盘既没必要(鼠标一动几十次),也会让
// "拖到一半崩了"留下半截宽度。
func (a *app) persistSplit() {
	a.settings.SplitL, a.settings.SplitM = a.splitL, a.splitM
	_ = SaveSettings(a.settings)
}

// persistSteps 把中栏排着的步骤写回设置文件。
//
// 步骤是用户一点点调出来的:模板、正则、编号起始值、保留长度……关一次
// 窗口就全没了,等于每次开机从头排一遍。和分栏宽度一样属于"调过一次就
// 不想再调"的东西,区别只是它得在退出时写——步骤每帧都可能被改(打字),
// 逐帧写盘会把磁盘打满。
//
// 存的是 flowFromRules 的结果:只留和默认值不同的参数,格式与用户自己存
// 的流程完全一致,读回来走同一条 rules() 还原路径。
func (a *app) persistSteps() {
	a.settings.Steps = flowFromRules("", a.steps).Steps
	_ = SaveSettings(a.settings)
}

// restoreSteps 把上次退出时的步骤装回来。
//
// 认不出的 opID(比如某个条目在新版本里改名了)由 rules() 跳过,不会让
// 整串步骤作废——剩下的那些仍然能用,用户补一步就行。
func (a *app) restoreSteps() {
	if len(a.settings.Steps) == 0 {
		return
	}
	a.steps = flow{Steps: a.settings.Steps}.rules()
	a.stepSel = -1
	// lastSpec 留空:第一帧照常跑一次实时预览,右栏一打开就有新名字,
	// 而不是先显示一片空白再等用户动一下。
}

// defaultOpenGroups 是第一次打开左栏时展开的组数。
//
// 全摊开时一屏只放得下前三四组,剩下的要靠滚——用户既不知道
// 下面还有多少,也不知道该从哪儿找起。展开两组、其余折起,一屏看到的是
// "有哪十来个类别",而不是"前三个类别里的全部条目"。
//
// 但也不能全折:全折之后左栏只剩一行行组名,第一次打开的人看不出这里面
// 装的是能点来用的规则。留两组开着,一眼就能看出条目长什么样。
const defaultOpenGroups = 2

// groupOpen 决定某个组此刻是否展开。默认状态由位置决定(前两组开),
// 用户点过的那几组以 a.folded 为准——两套状态分开存,免得"默认"把
// 用户的选择覆盖掉,也免得用户的选择被当成默认带到别的组上。
func (a *app) groupOpen(g string, groups []string) bool {
	if g == pinGroup {
		return true // 常用组是入口,折起来等于没有
	}
	if strings.TrimSpace(a.palQuery) != "" {
		return true // 搜索结果本来就少,折起来只会多一次点击
	}
	if v, ok := a.folded[g]; ok {
		return !v
	}
	for i, name := range groups {
		if name != g {
			continue
		}
		// 常用组不占默认展开的额度:它自己恒开,不该挤掉一个真分组
		slot := i
		if groups[0] == pinGroup {
			slot = i - 1
		}
		return slot < defaultOpenGroups
	}
	return false
}

// clampPane 把分隔位置夹在 [lo, hi] 内。hi 小于 lo 时（窗口被拖得
// 极窄）返回 lo，让第一栏占满，第二栏自然被挤出去而不是画成负数宽度。
func clampPane(v, lo, hi float32) float32 {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
