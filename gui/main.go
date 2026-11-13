package main

import (
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// fileInfo is one file in the list, with the facts the table shows.
type fileInfo struct {
	path     string
	size     int64
	modTime  time.Time
	typ      string // by extension
	realType string // sniffed from content; differs → warning chip
	// group keys the file to a tree node when it was dragged in as part of a
	// folder; the tree header shows the folder's name. Empty = a loose file
	// added on its own (shown without a group header).
	group string
}

// app is the state the window shows: three panes — catalog, task steps,
// file preview.
type app struct {
	win       *mygo.Window
	files     []fileInfo
	ops       []core.RenameOp // preview or apply result, aligned with files
	status    string
	busy      bool
	busyAI    bool      // busy because an AI (network) call is in flight — show spinner
	busyStart time.Time // when the current busy op began (for the elapsed timer)
	applied   bool      // true after 执行/回滚, until steps or files change
	lastSpec  string

	settings          Settings
	settingsOpen      bool
	settingsDraft     Settings            // 设置对话框里正在编辑的副本;取消即丢弃
	extColors         map[string]ui.Color // per-extension chip colors (main-thread)
	seedFollow        bool                // theme seed follows the desktop accent
	seedPick          ui.Color            // theme seed when not following
	confirmClear      bool                // 清空列表 needs a second click
	confirmAt         time.Time           // 二次确认的截止时间:挂太久自动收回
	confirmDelStep    bool                // 删除步骤 needs a second click
	confirmChosen     bool                // 删除所选 needs a second click
	confirmUndo       bool                // 回滚上次 needs a second click
	saveErr           string              // settings dialog save error, shown in-dialog
	aiTimeoutText     string              // 设置里"等待秒数"输入框的文本缓冲
	aiConcurrencyText string              // 设置里"并发路数"输入框的文本缓冲
	aiPromptText      string              // 设置里"默认提示词"多行输入框的缓冲

	steps    []guiRule // ordered task steps
	stepSel  int       // selected step, -1 = none
	stepList ui.ListState
	palSel   string            // last catalog op added, highlighted in the list
	stepUID  int               // 自增步骤身份
	chosen   ui.Selection[int] // multi-selected step uids
	pickMode bool              // 显示行内多选圆点(触摸入口)
	pickOpen bool              // 设置里的主色选择器是否展开
	dialogs  dialogHooks       // 可注入的对话框(测试用; nil=真实对话框)
	palHint  string            // hovered/selected catalog entry's explanation

	splitL, splitM float32 // pane divider positions
	table          ui.ListState
	fileSel        int // 文件列表里选中的行(rows 下标,不是 a.files 下标),-1 = 没选
	// pendingRemove 是"下一帧要移除的文件下标 + 1",0 表示没有。
	// 不能在构建这一帧的途中直接删 a.files:列表正按旧长度逐行构建,
	// 删掉一个会让后面每一行的下标越界(实测直接 panic)。
	pendingRemove int
	// fileFocus 记着"上一帧结束时文件列表是否拿着焦点",见 handleShortcuts
	fileFocus bool
}

func (a *app) view(c *ui.Context) {
	dt := a.currentTheme()
	if a.isDark() {
		c.SetTheme(dt.dark)
	} else {
		c.SetTheme(dt.light)
	}
	// 逃生键:模式类状态必须能被一键解除,用户不会被困在里面
	if a.pickOpen && c.Shortcut(0, ui.KeyEscape) {
		a.pickOpen = false
	} else if a.pickMode && c.Shortcut(0, ui.KeyEscape) {
		a.pickMode = false
		a.chosen.Clear()
	} else if a.settingsOpen && c.Shortcut(0, ui.KeyEscape) {
		a.closeSettings(false)
	}
	a.handleShortcuts(c)
	// 上一帧排下的移除,等这一帧开头再动 a.files
	if a.pendingRemove > 0 {
		i := a.pendingRemove - 1
		a.pendingRemove = 0
		a.removeFile(i)
	}
	// live preview on any change (AI steps need the explicit button)
	if spec := strings.Join(a.stepSpecs(), "\x00"); spec != a.lastSpec {
		if !strings.Contains(spec, "<AI:") {
			a.preview()
		}
		a.lastSpec = spec
	}
	if a.splitL == 0 {
		a.splitL, a.splitM = 220, 330
	}
	// 顶部自绘标题栏:与窗口背景一体,整条可拖动;右侧留窗口按钮的位置。
	// 窗口用 TitleBarHidden 保留按钮/阴影/圆角,系统标题栏则不再出现
	bar := c.TitleBar()
	titleH := float32(38)
	if bar.Height > titleH {
		titleH = bar.Height
	}
	t := c.Theme()
	pt := a.currentTheme().panes(a.isDark())
	// 基底:整个窗口先铺 base 色;顶栏、底栏、左栏、四周留白都在这个底色上。
	ui.Column(c).Fill().Background(pt.base).Children(func() {
		ui.Row(c).Height(titleH).Padding(0, float32(bar.Right), 0, float32(bar.Left)).DragWindow().
			AlignItems(ui.Center).Children(func() {
			brand := ui.Text(c, "New Century Renamer").Bold().FontSize(13).Padding(0, 8)
			if bar.Left == 0 {
				brand.PaddingStart(float32(bar.Left) + 8)
			}
			ui.Spacer(c)
			// 右上角是用户扫视时第一个落脚点,放"这批有多少文件、几个会改名"
			// 比放一个和窗口标题重复的名字有用。
			ui.Text(c, a.batchSummary()).FontSize(11).TextColor(t.TextMuted).PaddingEnd(float32(bar.Right) + 8)
		})
		// 只 Grow 不 Fill:Fill 会让 Split 想占满整列(含标题栏那几十像素),
		// 于是内容画到标题栏底下。Grow(1) 只吃标题栏之外的剩余空间。
		// Split 外面留一点内边距,让中/右两张圆角卡片浮在 base 底色上、
		// 四周露出基底(和左栏同色),读起来就是"卡片浮在底板上"。
		ui.Split(c, &a.splitL,
			func() { a.catalogView(c) },
			func() {
				ui.Split(c, &a.splitM,
					func() { a.stepsView(c) },
					func() { a.filesView(c) }).Grow(1).FillWidth()
			}).Grow(1).FillWidth().Padding(8, 8, 8, 8)
		// ③ 应用级底部状态栏:这里是"对整个软件"的操作与反馈,与右栏
		// "对文件列表"的操作分开。左边是警示/状态文字(多少将更名、报错),
		// 右边是主操作"执行更名"和"回滚上次"——执行是主按钮实心,回滚镂空。
		a.statusBar(c)
	})
	a.settingsDialog(c)
	// 焦点要等这一帧构建完才有值:列表在构建时才把行标成可聚焦,
	// 所以记在帧尾,给下一帧的 handleShortcuts 用。
	a.fileFocus = a.table.FocusWithin(c)
}

// statusBar 是整个应用底部的固定状态栏(所有窗格之下,横贯全宽)。
// 左侧状态/警示:多少将更名、几个有问题、正在等模型的秒数;右侧:主操作。
func (a *app) statusBar(c *ui.Context) {
	t := c.Theme()
	// 顶部一条极细的分隔线,和上方内容区分开
	ui.Box(c).FillWidth().Height(1).Background(t.Border)
	ui.Row(c).FillWidth().Height(46).Padding(0, 12).Gap(10).AlignItems(ui.Center).
		Background(a.currentTheme().panes(a.isDark()).left).Children(func() {
		// ---- 左:状态 / 警示文字 ----
		if a.busy {
			ui.Spinner(c)
			label := "处理中…"
			if a.busyAI {
				label = "正在联网命名,模型思考中…"
			}
			if !a.busyStart.IsZero() {
				if sec := int(time.Since(a.busyStart).Seconds()); sec >= 1 {
					label = fmt.Sprintf("%s 已等待 %d 秒", label, sec)
				}
			}
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
		} else if a.status != "" {
			ui.Text(c, a.status).FontSize(12).TextColor(a.statusColor(t)).SingleLine().Ellipsis("…")
		} else {
			// 空态只陈述事实,不指挥用户点某个按钮——此刻"执行更名"是禁用的,
			// 让文案指向它等于把人引到一堵墙上。
			ui.Text(c, "拖入文件开始").FontSize(12).TextColor(t.TextMuted)
		}
		ui.Spacer(c)
		// ---- 右:主操作(执行=主按钮,回滚=次要镂空) ----
		// "回滚上次"必须说清是"哪一次、几个文件"。只写三个字,用户不
		// 知道自己即将撤销的是刚才那批,还是上周某个目录里的一批。
		sum, canUndo := a.journalInfo()
		if a.confirmUndo && a.confirmStale() {
			a.dropConfirm()
		}
		if a.confirmUndo {
			label := fmt.Sprintf("确认回滚 %d 个?", len(sum.Names))
			confirm := ui.ButtonBase(c).Radius(16).Padding(6, 14).Background(t.Danger).TextColor(ui.RGB(255, 255, 255))
			if confirm.Hovered() {
				confirm = confirm.Background(t.Danger.Alpha(0.85))
			}
			confirm.Children(func() { ui.Text(c, label).SingleLine() })
			if confirm.Clicked() {
				a.dropConfirm()
				go a.undo()
			}
		} else {
			undoLabel := "回滚上次"
			if canUndo {
				undoLabel = fmt.Sprintf("回滚上次(%d 个)", len(sum.Names))
			}
			undoBtn := outlinedButton(c, undoLabel, !a.busy && canUndo).Tooltip(a.undoTip(sum))
			if undoBtn.Clicked() {
				// 回滚会动磁盘,和"清空列表"一样属于"再点一下才真做"
				a.armConfirm("undo")
			}
		}
		applyEnabled := !a.busy && a.hasChanges()
		if applyEnabled {
			if ui.PrimaryButton(c, "执行更名").Radius(16).Tooltip(shortcutTip("Ctrl+Enter")).Clicked() {
				go a.apply()
			}
		} else {
			ui.PrimaryButton(c, "执行更名").Radius(16).Disabled(true)
		}
	})
}

// ---- left pane: grouped catalog of named operations ----

func (a *app) catalogView(c *ui.Context) {
	t := c.Theme()
	var groups []string
	for _, d := range catalog {
		if len(groups) == 0 || groups[len(groups)-1] != d.group {
			groups = append(groups, d.group)
		}
	}
	// a hand-built list rather than ui.Sidebar: its internal scroll
	// container never received wheel events here, hiding the lower groups
	// 左边栏:与顶栏/底栏同属"基底"层,不画底色(继承窗口 base),只做内容。
	ui.Column(c).Fill().Padding(4, 4).Children(func() {
		ui.Text(c, "功能 · 点击加入步骤").Bold().FontSize(13).Padding(8, 10)
		ui.Scroll(c).Fill().Padding(4, 6).Children(func() {
			for _, g := range groups {
				ui.Text(c, g).FontSize(12).TextColor(t.TextMuted).Padding(8, 8)
				for _, d := range catalog {
					if d.group != g {
						continue
					}
					selBg, selFg := a.selColors()
					item := ui.ButtonBase(c).Fill().Radius(8)
					switch {
					case a.palSel == d.id:
						item.Background(selBg)
					case item.Hovered():
						item.Background(t.SurfaceHover)
					}
					// 对齐即含义:处理步骤左对齐;流程控制与工具动作居中。
					// 不用读文字,扫一眼对齐方式就知道条目性质
					align := ui.Start
					if d.flow || d.act != "" {
						align = ui.Center
					}
					item.Children(func() {
						// 图标走色彩管线:按分组类别色(与步骤行类别胶囊同源),
						// 选中时白——图标即类别指示,扫色即知
						if ic := opIcon(d.icon); ic != nil {
							ie := ui.Icon(c, ic).FontSize(14)
							if a.palSel == d.id {
								ie.TextColor(selFg)
							} else {
								ie.TextColor(a.currentTheme().groupColor(d.group))
							}
						}
						if a.palSel == d.id {
							ui.Text(c, d.label).SingleLine().Fill().TextAlign(align).Padding(5, 10).TextColor(selFg)
						} else {
							ui.Text(c, d.label).SingleLine().Fill().TextAlign(align).Padding(5, 10)
						}
					})
					if item.Hovered() || a.palSel == d.id {
						// 悬停即说明:用户可以先弄明白再决定点不点。
						// 说明只显示功能本身的一句话;“点击加入”的提示固定放在框底,
						// 不拼进每条的说明里——否则长说明会撑成三四行溢出小格子。
						a.palHint = d.desc
					}
					if item.Clicked() {
						a.palHint = d.desc // 点过之后说明留在框里,不用再悬停一次
						switch d.act {
						case "import-csv":
							go a.importCSV()
						case "settings":
							a.openSettings()
						default:
							r := d.make()
							r.opID, r.opLabel, r.opDesc, r.opFlow, r.opGroup = d.id, d.label, d.desc, d.flow, d.group
							a.stepUID++
							r.opUID = a.stepUID
							a.steps = append(a.steps, r)
							a.stepSel = len(a.steps) - 1
							a.palSel = d.id
						}
					}
				}
			}
		})
		// 底部说明区:固定高度,内容变化不移动目录条目(否则点击会被位移打断)。
		// 加 Clip():说明文字再长也只在自己这一格里换行/省略,绝不溢出到框外。
		// 默认文案不写"把鼠标停在…"——键盘和触摸用户做不到这件事,
		// 那样等于给他们一个永远空着的框加一句指令。这里直接说清因果。
		ui.Column(c).Fill().Height(64).Padding(8, 10).Gap(4).Background(t.Background).Clip().Children(func() {
			if a.palHint != "" {
				ui.Text(c, a.palHint).FontSize(12).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "点一个功能,它就加入中栏;步骤从上到下依次执行").FontSize(12).TextColor(t.TextMuted)
			}
		})
	})
}

// stepRangeBars returns, per step, the two fixed range-bar slots:
// [0] = left slot for the 筛选 range (seed complement), [1] = right slot
// for the 选取 range — filename and extension take two neighboring hues so
// a switch from 选取文件名 to 选取扩展名 shows as a color change instead
// of one continuous bar. A zero Color means the slot is off (transparent).
// Slots never move: at most two ranges can be open, each owning its place.
func (a *app) stepRangeBars(dt *derivedTheme) [][2]ui.Color {
	out := make([][2]ui.Color, len(a.steps))
	filterOn := false
	scopeCol := ui.Color{} // zero = off
	for i := range a.steps {
		s := &a.steps[i]
		key, _ := kindByLabel[s.kind]
		switch key {
		case "filter":
			filterOn = true
		case "unfilter":
			filterOn = false
		case "scope":
			switch s.scopeSel {
			case "整体":
				scopeCol = ui.Color{}
			case "扩展名":
				scopeCol = dt.rangeScopeExt
			default: // 文件名
				scopeCol = dt.rangeScopeName
			}
		}
		if filterOn {
			out[i][0] = dt.rangeFilter
		}
		out[i][1] = scopeCol
	}
	return out
}

// ---- middle pane: ordered task steps + selected step's detail ----

func (a *app) stepsView(c *ui.Context) {
	t := c.Theme()
	if a.stepSel >= len(a.steps) {
		a.stepSel = len(a.steps) - 1
	}
	// 中间栏:核心工作区,浮得最高的一张白色圆角卡片(暗色下也比两侧亮一阶),
	// 压在基底之上;给它圆角 + 一层柔投影,像一张单独浮起的纸。
	pt := a.currentTheme().panes(a.isDark())
	ui.Column(c).Fill().Gap(8).Padding(10, 10).Radius(14).Background(pt.mid).Clip().
		Shadow(0, 3, 14, 0, pt.shadow).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("任务步骤(%d)", len(a.steps))).Bold().FontSize(13)
			ui.Spacer(c)
			// 触摸入口:点开后才显示行内多选圆点(Ctrl+点击亦可,随时可用)
			label := "多选"
			if a.pickMode {
				label = "完成"
			}
			if ui.Button(c, label).Radius(14).Clicked() {
				a.pickMode = !a.pickMode
				if !a.pickMode {
					a.chosen.Clear()
				}
			}
		})
		// 选中行自绘浅色底(Selection),不用 List 的深 Accent 绘制:
		// 选中时行内的彩色胶囊必须保持可读
		if len(a.steps) == 0 {
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "从左侧“功能”里点一个,它就成为这一步").TextColor(t.TextMuted)
			})
		} else {
			dt := a.currentTheme()
			bars := a.stepRangeBars(dt)
			// 始终可选中、可编辑;多选与拖动也不需要模式开关
			a.stepList.Key = func(i int) any { return a.steps[i].opUID }
			a.stepList.Selected = &a.stepSel
			a.stepList.Selection = &a.chosen
			a.stepList.Reorder = func(rows []int, to int) {
				a.moveSteps(rows, to)
			}
			ui.List(c, &a.stepList, len(a.steps), func(i int) {
				s := a.steps[i]
				cat := dt.groupColor(s.opGroup)
				// 行结构:编号 · 操作名 · 参数值胶囊 · 类别胶囊 · 范围色条。
				// 扫颜色即可读流程,不用逐个读编号
				// 当前步骤与多选集合都高亮
				hot := i == a.stepSel || a.chosen.Has(s.opUID)
				row := ui.Column(c).Fill().Padding(6, 6).MinHeight(38).Radius(6)
				if hot {
					row.Background(t.Selection)
				}
				// 范围色条自绘:两槽固定——右槽=选取范围,左槽=筛选范围;
				// 未激活透明占位,槽位永不移动
				row.DrawOver(func(p *ui.Painter, r ui.Rect) {
					for k := 0; k < 2; k++ {
						col := bars[i][1-k] // [1] 右槽,[0] 左槽
						if col.A == 0 {
							continue
						}
						x := r.X + r.W - 4 - float32(k)*6
						p.Fill(ui.Rect{X: x, Y: r.Y, W: 4, H: r.H}, col, 0)
					}
				})
				row.Children(func() {
					// 高亮行:mygo 会把选中行的文字染成强调色(浅),显式
					// 声明深色,不靠继承
					numCol, labCol := t.TextMuted, t.Text
					if hot {
						numCol, labCol = t.Text, t.Text
					}
					ui.Row(c).Gap(6).Children(func() {
						ui.Text(c, fmt.Sprintf("%d.", i+1)).FontSize(12).TextColor(numCol).Width(22).TextAlign(ui.End)
						// 全部步骤统一左对齐(设计书:不要在同一列表里混用对齐);
						// 流程控制靠类别胶囊 + 范围色条区分,不再靠居中
						if s.opFlow {
							ui.Row(c).Fill().Gap(6).Children(func() {
								// Shrink(0):操作名不参与压缩,要压缩就让后面的参数
								// 胶囊去省略。以前没有它,"追加序号"会被挤成"追…"、
								// "AI 按内容命名"挤成"AI…"——名字比参数重要。
								ui.Text(c, s.opLabel).FontSize(12).Bold().TextColor(labCol).SingleLine().Shrink(0)
								if v := s.summary(); v != "" {
									// SingleLine 才能让 Ellipsis 生效,否则长参数会折成两行撑高行
									ui.Text(c, v).FontSize(11).Padding(2, 8).Radius(8).MaxWidth(150).SingleLine().Ellipsis("…").
										Background(cat.Alpha(0.16)).TextColor(t.Text)
								}
							})
						} else {
							ui.Row(c).Fill().Gap(6).Children(func() {
								ui.Text(c, s.opLabel).FontSize(12).Bold().TextColor(labCol).SingleLine().Shrink(0)
								if v := s.summary(); v != "" && v != s.opLabel {
									ui.Text(c, v).FontSize(11).Padding(2, 8).Radius(8).MaxWidth(150).SingleLine().Ellipsis("…").
										Background(cat.Alpha(0.16)).TextColor(t.Text)
								}
							})
						}
						// 胶囊区:固定宽度单独管理,行内不再被挤
						catBg, catFg := chipTones(cat, a.isDark())
						ui.Row(c).Width(84).Gap(4).Children(func() {
							ui.Spacer(c)
							// 连续同类别:胶囊退化为同色圆点,换组才显示文字
							if i > 0 && a.steps[i-1].opGroup == s.opGroup {
								ui.Box(c).Size(9, 9).Background(catFg)
							} else {
								ui.Text(c, s.opGroup).FontSize(11).Padding(2, 8).Radius(8).
									Background(catBg).TextColor(catFg)
							}
						})
						// 多选圆点:默认不画(太重)。按需出现:开了多选模式、
						// 本行被悬停、或已在本行的选择里;悬停与已选时占位,
						// 保证布局不跳
						picked := a.chosen.Has(s.opUID)
						if a.pickMode || picked || row.Hovered() {
							dotColor := t.Border
							if picked {
								dotColor = t.Accent
							}
							pick := ui.ButtonBase(c).Size(16, 16).Radius(8).Border(1, dotColor)
							if picked {
								pick.Background(t.Accent)
							}
							pick.Children(func() {
								if picked {
									ui.Text(c, "✓").FontSize(10).TextColor(t.AccentText)
								}
							})
							if pick.Clicked() {
								if picked {
									a.chosen.Remove(s.opUID)
								} else {
									a.chosen.Add(s.opUID)
								}
							}
						}
						// 色条保留区:两条竖线永远画在这里,不压胶囊
						ui.Box(c).Width(12)
					})
				})
			}).Gap(0).Grow(1) // 行相接,范围竖线连续;行距由行内 Padding 提供
		}
		// 多选(Ctrl+点击)时的批量操作
		if a.chosen.Len() > 0 {
			n := a.chosen.Len()
			ui.Divider(c)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprintf("已选 %d 步", n)).Bold()
				// 批量删除同样是"丢东西",和单步删除、清空列表共用一套语言
				if a.confirmChosen && a.confirmStale() {
					a.dropConfirm()
				}
				if a.confirmChosen {
					confirm := ui.ButtonBase(c).Radius(16).Padding(6, 14).Background(t.Danger).TextColor(ui.RGB(255, 255, 255))
					confirm.Children(func() { ui.Text(c, "确认删除所选?").SingleLine() })
					if confirm.Clicked() {
						a.deleteChosen()
						a.dropConfirm()
					}
				} else if dangerButton(c, "删除所选", true).Clicked() {
					a.armConfirm("chosen")
				}
				if ui.Button(c, "全不选").Radius(16).Clicked() {
					a.chosen.Clear()
				}
			})
		}
		// detail of the selected step — 选中即可编辑,无需解锁
		if a.stepSel >= 0 && a.stepSel < len(a.steps) {
			r := &a.steps[a.stepSel]
			ui.Divider(c)
			ui.Text(c, r.opLabel).Bold()
			if r.opDesc != "" {
				ui.Text(c, r.opDesc).FontSize(12).TextColor(t.TextMuted)
			}
			r.paramsEditor(c, t)
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "上移").Disabled(a.stepSel == 0).Tooltip(shortcutTip("Alt+↑")).Clicked() {
					a.swapSteps(a.stepSel-1, a.stepSel)
				}
				if ui.Button(c, "下移").Disabled(a.stepSel >= len(a.steps)-1).Tooltip(shortcutTip("Alt+↓")).Clicked() {
					a.swapSteps(a.stepSel, a.stepSel+1)
				}
				// 删除步骤和"清空列表"是同一类动作(丢东西),所以用同一套语言:
				// 错误色 + 二次确认 + 确认态三秒自动收回。以前它和"复制"同款,
				// 一个像不可逆、一个像随手,用户分不出轻重。
				if a.confirmDelStep && a.confirmStale() {
					a.dropConfirm()
				}
				if a.confirmDelStep {
					confirm := ui.ButtonBase(c).Radius(16).Padding(6, 14).Background(t.Danger).TextColor(ui.RGB(255, 255, 255))
					confirm.Children(func() { ui.Text(c, "确认删除?").SingleLine() })
					if confirm.Clicked() {
						a.deleteStep(a.stepSel)
						a.dropConfirm()
					}
				} else if dangerButton(c, "删除", true).Tooltip(shortcutTip("Delete")).Clicked() {
					a.armConfirm("step")
				}
				if ui.Button(c, "复制").Tooltip(shortcutTip("Ctrl+D")).Clicked() {
					a.duplicateStep(a.stepSel)
				}
			})
		}
	})
}

// moveSteps is the List's Reorder callback: the user dragged rows (the
// multi-selection, or a single row) to before index to.
func (a *app) moveSteps(rows []int, to int) {
	sort.Ints(rows)
	moved := make([]guiRule, 0, len(rows))
	kept := make([]guiRule, 0, len(a.steps))
	shift := 0
	isDragged := map[int]bool{}
	for _, r := range rows {
		isDragged[r] = true
	}
	for i, s := range a.steps {
		if isDragged[i] {
			moved = append(moved, s)
			if i < to {
				shift++
			}
			continue
		}
		kept = append(kept, s)
	}
	at := to - shift
	if at < 0 {
		at = 0
	}
	if at > len(kept) {
		at = len(kept)
	}
	out := append(kept[:at:at], moved...)
	out = append(out, kept[at:]...)
	a.steps = out
}

// deleteChosen removes every multi-selected step.
func (a *app) deleteChosen() {
	drop := map[int]bool{}
	for uid := range a.chosen.All() {
		drop[uid] = true
	}
	kept := a.steps[:0]
	for _, s := range a.steps {
		if drop[s.opUID] {
			continue
		}
		kept = append(kept, s)
	}
	n := len(a.steps) - len(kept)
	a.steps = kept
	a.chosen.Clear()
	a.status = fmt.Sprintf("已删除 %d 个步骤", n)
}

// swapSteps 交换第 i 步与第 j 步。
// 选中必须跟着"那一步"走,而不是跟着"那个位置":把第 2 步上移之后
// 还停在第 2 行,用户再点一次"上移"动的就是原来第 1 步了。
func (a *app) swapSteps(i, j int) {
	if i < 0 || j < 0 || i >= len(a.steps) || j >= len(a.steps) {
		return
	}
	a.steps[i], a.steps[j] = a.steps[j], a.steps[i]
	switch a.stepSel {
	case i:
		a.stepSel = j
	case j:
		a.stepSel = i
	}
}

// deleteStep 删掉第 i 步。鼠标的「确认删除?」和键盘的第二次 Delete
// 走同一条路——两条路径各写一遍,迟早会长歪。
func (a *app) deleteStep(i int) {
	if i < 0 || i >= len(a.steps) {
		return
	}
	a.steps = append(a.steps[:i], a.steps[i+1:]...)
	if a.stepSel >= len(a.steps) {
		a.stepSel = len(a.steps) - 1
	}
}

// duplicateStep 复制第 i 步到列表末尾并选中它。
// 鼠标的「复制」和 Ctrl+D 走同一条路——两条路径各写一遍,迟早会长歪。
func (a *app) duplicateStep(i int) {
	if i < 0 || i >= len(a.steps) {
		return
	}
	cp := a.steps[i]
	a.stepUID++
	cp.opUID = a.stepUID
	a.steps = append(a.steps, cp)
	a.stepSel = len(a.steps) - 1
}

// removeFile 把第 i 个文件从清单里拿掉。这是"清空列表"的单文件版本:
// 拖进 200 个文件、只有 3 个不想改时,以前唯一的办法是整表清空重来。
//
// a.files 与 a.ops 是按下标对齐的两个切片(ops[i] 永远是 files[i] 的预览
// 结果),所以两边必须一起删,否则预览会整体错位一行——那比不删更糟。
func (a *app) removeFile(i int) {
	if i < 0 || i >= len(a.files) {
		return
	}
	a.files = append(a.files[:i], a.files[i+1:]...)
	if i < len(a.ops) {
		a.ops = append(a.ops[:i], a.ops[i+1:]...)
	}
	// 选中位置跟着收缩,别指到不存在的行上
	if n := len(buildRows(a.files)); a.fileSel >= n {
		a.fileSel = n - 1
	}
	a.applied = false
	if len(a.files) == 0 {
		a.ops = nil
		a.status = ""
		return
	}
	// 删完重算:新名字可能依赖序号、分组计数这类"上下文",
	// 只删一行也可能让后面每一行的预览都变。
	a.preview()
}

// removeChosenFiles 移除当前选中的文件(Delete 键与右键菜单都走这里)。
func (a *app) removeChosenFiles() {
	if a.fileSel < 0 {
		return
	}
	a.removeFile(a.rowFile(a.fileSel))
}

// rowFile 把列表行下标换成 a.files 下标。列表里除了文件卡还有分组头,
// 两者混在同一个 rows 序列里,所以行号不能直接当文件号用。
func (a *app) rowFile(ri int) int {
	rows := buildRows(a.files)
	if ri < 0 || ri >= len(rows) || rows[ri].header {
		return -1
	}
	return rows[ri].file
}

// newNameFor 返回第 i 个文件的预览新名字;没有预览结果时返回空串
// (右键菜单用它判断"复制新名字"要不要可点)。
func newNameFor(a *app, i int) string {
	if i < 0 || i >= len(a.ops) {
		return ""
	}
	return a.ops[i].NewName
}

// revealInFileManager 在系统文件管理器里定位这个文件。
// mygo 没有跨平台的"显示文件"接口(只有 OpenURL),所以各平台各叫各的。
func revealInFileManager(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// explorer 的 /select 后面直接跟路径,中间不能有空格
		cmd = exec.Command("explorer", "/select,"+path)
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	}
	_ = cmd.Start() // 打不开就算了:这不是改名流程的一环
}

// ---- right pane: file table with drop zone ----

func (a *app) filesView(c *ui.Context) {
	t := c.Theme()
	// 右边栏:结果空间,浮在基底之上的一张浅卡片(比中栏低一阶),也是圆角。
	pt := a.currentTheme().panes(a.isDark())
	ui.Column(c).Fill().Gap(8).Padding(10, 10).Radius(14).Background(pt.right).Clip().
		Shadow(0, 2, 10, 0, pt.shadow).Children(func() {
		// ① 工具栏(吸顶,固定在右栏顶部):这里的动作都是"对文件列表本身"的操作——
		// 添加、导出、清空。用次要的 Tonal / 危险样式,不跟右下角的主操作抢。
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "文件").Bold().FontSize(13)
			addBtn := tonalButton(c, "添加文件…", true).Tooltip(shortcutTip("Ctrl+O"))
			if addBtn.Clicked() {
				go a.addFilesDialog()
			}
			expBtn := tonalButton(c, "导出CSV", len(a.files) > 0)
			if expBtn.Clicked() {
				go a.exportCSV()
			}
			if a.confirmClear && a.confirmStale() {
				a.dropConfirm() // 挂太久:当用户改主意,收回确认态
			}
			if a.confirmClear {
				// 二次确认态:换成红色实心,明确告诉用户"再点就真删了"
				confirm := ui.ButtonBase(c).Radius(16).Padding(6, 14).Background(t.Danger).TextColor(ui.RGB(255, 255, 255))
				if confirm.Hovered() {
					confirm = confirm.Background(t.Danger.Alpha(0.85))
				}
				confirm.Children(func() { ui.Text(c, "确认清空?").SingleLine() })
				if confirm.Clicked() {
					a.files, a.ops, a.applied = nil, nil, false
					a.dropConfirm()
				}
			} else {
				clrBtn := dangerButton(c, "清空列表", len(a.files) > 0)
				if clrBtn.Clicked() {
					a.armConfirm("clear") // 防错:两次点击才清空
				}
			}
		})
		// ② AI 预览:也是"文件列表"上的操作(对清单里的文件联网起名),留在顶部
		if a.hasAIStep() {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				previewLabel := "预览(AI 联网命名)"
				if a.busyAI {
					previewLabel = "联网命名中…"
				}
				if ui.PrimaryButton(c, previewLabel).Radius(16).Disabled(a.busy || len(a.files) == 0).Clicked() {
					go a.previewAsync()
				}
			})
		}
		if len(a.files) == 0 {
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Text(c, "批量更名:左边挑处理步骤,这里放文件,改名会实时预览").FontSize(13).TextColor(t.Text)
				ui.Text(c, "把文件或文件夹拖进窗口,或点“添加文件…”").FontSize(12).TextColor(t.TextMuted)
			})
		} else {
			dt := a.currentTheme()
			// 分组:文件夹拖入的文件按文件夹分组,用 List 内置的吸顶分组头
			// (ListState.Header):滚动时当前分组的文件夹名固定在顶部,滚到下一个
			// 分组时新的文件夹名把它顶上去——文件再多也知道"现在是哪个文件夹"。
			//
			// 关键:分组头必须**独立成行**,不能兼任该组第一个文件那一行。
			// mygo 的吸顶不是裁剪内容,而是把整行元素的 y 直接设成列表顶部
			// (ui/list.go pinHeader);头和卡同行时,钉住这一行 = 钉住第一张卡,
			// 它会永远浮在顶部压住后面滚上来的卡。所以行数 = 文件数 + 分组数。
			rows := buildRows(a.files)
			a.table.Key = func(ri int) any {
				if ri < 0 || ri >= len(rows) {
					return ri
				}
				if rows[ri].header {
					return "h:" + rows[ri].group
				}
				return "f:" + a.files[rows[ri].file].path
			}
			a.table.Header = func(ri int) bool {
				return ri >= 0 && ri < len(rows) && rows[ri].header
			}
			// 选中一行文件:以前文件列表完全不可点,想删掉一个文件只能
			// 清空整表重来。Selected 让点击/方向键选中,Delete 与右键菜单
			// 都作用在选中的那一行上。
			a.table.Selected = &a.fileSel
			// 只有一个分组时,标题栏右侧已经在说「N 个文件」,组头再说一遍
			// 就是同一句话并排出现两次;多组时才需要每组各说各的。
			nGroups := 0
			for _, r := range rows {
				if r.header {
					nGroups++
				}
			}
			// 列头:结果列离名字有几百像素,没有表头的话右边那列只是
			// "一个小圆点加两个字",用户得自己猜那是结果。表头只在有文件时
			// 出现——空列表时它只是一行噪音。
			ui.Row(c).FillWidth().Gap(8).PaddingStart(10).Children(func() {
				ui.Text(c, "原名 → 新名").FontSize(11).TextColor(t.TextMuted)
				ui.Spacer(c)
				ui.Text(c, "结果").FontSize(11).TextColor(t.TextMuted).Width(64).TextAlign(ui.End)
			})
			ui.List(c, &a.table, len(rows), func(ri int) {
				r := rows[ri]
				// 分组头独立一行:吸顶的文件夹名 + 组内文件数
				if r.header {
					// 吸顶头:实底(和右栏同色)盖住从下面滚过的文件卡,
					// 读起来像"钉住"的标签条。
					ui.Row(c).Fill().Gap(8).Padding(6, 6).
						Background(dt.panes(a.isDark()).right).Children(func() {
						ui.Text(c, r.group).FontSize(13).Bold().TextColor(t.Text)
						if nGroups > 1 {
							ui.Text(c, fmt.Sprintf("%d 个文件", r.count)).FontSize(11).TextColor(t.TextMuted)
						}
						ui.Spacer(c)
					})
					return
				}
				i := r.file
				f := a.files[i]
				// 属于分组时缩进一点,视觉上从属于上面的文件夹头
				var indent float32 = 0
				if f.group != "" {
					indent = 14
				}
				row := ui.Row(c).Fill().Gap(8).PaddingStart(indent)
				// 右键菜单:mygo 的 ContextMenu 会在菜单打开时跑一次 build,
				// 再在"某一项被选中"的那一帧跑第二次,用 Chosen() 判断。
				// 所以每个 Item 都要先建、再问 Chosen——顺序不能反。
				row.ContextMenu(func(m *ui.Menu) {
					del := m.Item("从列表移除").Shortcut(0, ui.KeyDelete)
					if del.Chosen() {
						a.pendingRemove = i + 1
					}
					m.Separator()
					reveal := m.Item("在文件管理器中显示")
					if reveal.Chosen() {
						revealInFileManager(f.path)
					}
					copyNew := m.Item("复制新名字").Disabled(newNameFor(a, i) == "")
					if copyNew.Chosen() {
						c.WriteClipboard(newNameFor(a, i))
					}
					m.Separator()
					copyOld := m.Item("复制原文件名")
					if copyOld.Chosen() {
						c.WriteClipboard(filepath.Base(f.path))
					}
				})
				row.Children(func() {
					card := ui.Column(c).Fill().Gap(4).Padding(6, 10).Radius(8).
						Background(dt.panes(a.isDark()).card)
					card.Children(func() {
						oldName := filepath.Base(f.path)
						newName := oldName
						if i < len(a.ops) && a.ops[i].NewName != "" {
							newName = a.ops[i].NewName
						}
						changed := newName != oldName
						oStem, oExt := filepathSplitName(oldName)
						nStem, nExt := filepathSplitName(newName)
						// 类型色系:原名组按原扩展名,预览组按新扩展名——
						// 改扩展名时颜色随类别变化,本身就是信息
						oCol := a.fileColor(dt, oExt)
						nCol := a.fileColor(dt, nExt)
						// 名字组 = [胶囊文件名] . [胶囊扩展名],点是两胶囊之间
						// 的独立文本;原名中性胶囊组,预览类型色胶囊组
						nameChips := func(stem, ext string, stemBg, extBg, extFg ui.Color) {
							// SingleLine 才能让 Ellipsis 生效:没有它,超长文件名会折成
							// 四行,把 54px 的卡片撑到 95px,整列节奏全乱
							ui.Text(c, stem).FontSize(13).Padding(2, 10).Radius(8).
								Background(stemBg).MaxWidth(170).SingleLine().Ellipsis("…")
							if ext != "" {
								ui.Text(c, ".").FontSize(13).TextColor(t.TextMuted)
								ui.Text(c, strings.TrimPrefix(ext, ".")).FontSize(13).Padding(2, 10).Radius(8).
									Background(extBg).TextColor(extFg)
							}
						}
						ui.Row(c).Gap(4).Children(func() {
							// 原文件名:胶囊文件名 . 胶囊扩展名(原类型色,淡)
							nameChips(oStem, oExt, oCol.Alpha(0.10), oCol.Alpha(0.30), t.Text)
							if changed {
								ui.Text(c, "→").TextColor(t.TextMuted)
								// 预览:胶囊文件名 . 胶囊扩展名(新类型色,实)。
								// 以前这里用饱和色底 + 白字,实测只有 3.1–3.6,
								// 和上面那排类别胶囊是同一个毛病;改成同色相浅底深字,
								// 比原名胶囊重一档即可——要的是"能读出来",不是"很显眼"。
								nExtBg, nExtFg := chipTonesEmph(nCol, a.isDark())
								nameChips(nStem, nExt, nCol.Alpha(0.18), nExtBg, nExtFg)
							}
							ui.Spacer(c)
							// 悬停出 ×:200 个文件里只有 3 个不想改时,不必
							// 为这 3 个清空整表。画在卡片名字行右端而不是
							// 卡片外面——飘在卡外的红点看着像错误标记。
							// 宽度占位始终留着,否则一悬停整行就抖一下。
							ui.Row(c).Width(18).AlignItems(ui.Center).Children(func() {
								if !row.Hovered() {
									return
								}
								x := ui.ButtonBase(c).Size(18, 18).Radius(9).Padding(0).
									Background(t.Danger.Alpha(0.12)).Tooltip("从列表移除 (Delete)")
								x.Children(func() {
									ui.Text(c, "×").FontSize(12).TextColor(t.Danger)
								})
								if x.Clicked() {
									a.pendingRemove = i + 1
								}
							})
						})
						// 下行:原始数据胶囊。
						// 一个文件行里说同一件事两次是噪音:扩展名胶囊已经写着
						// 「mp3」,再挂一个「MP3 音频」只是把同样的信息换个说法;
						// 文件夹名已经在上面的分组头里,每行再挂一遍路径胶囊
						// 等于把「Awich - THE UNION」重复 4 次。所以:
						// 路径只在没有分组头时出现,类型只在扩展名缺失时出现,
						// 内容嗅探只在真不符时出现(相符是默认,不用报)。
						ui.Row(c).Gap(6).Children(func() {
							if f.group == "" {
								path := ui.Text(c, pathTail3(filepath.Dir(f.path))).FontSize(11).Padding(1, 6).Radius(6).
									Background(t.SurfaceHover).TextColor(t.TextMuted).MaxWidth(170).SingleLine().Ellipsis("…")
								path.Tooltip(filepath.Dir(f.path))
							}
							// 真实类型:与扩展名相符时不必说;只有真不符才标红。
							// EPUB/DOCX 的容器本来就是 ZIP,m4a/mov 本来就是 MP4,
							// 按字符串判等会把整屏正常文件标红,假警报淹掉真警报。
							extNoDot := strings.ToLower(strings.TrimPrefix(oExt, "."))
							if f.realType != "" && !realMatchesExt(extNoDot, f.realType) {
								ui.Text(c, "内容为 "+f.realType+" 容器").FontSize(11).Padding(1, 6).Radius(6).
									Background(t.Danger.Alpha(0.15)).TextColor(t.Danger).
									Tooltip("扩展名是 ." + extNoDot + ",但文件内容像 " + f.realType)
							} else if oExt == "" && f.typ != "" && f.typ != "无扩展名" {
								ui.Text(c, f.typ).FontSize(11).Padding(1, 6).Radius(6).
									Background(t.SurfaceHover).TextColor(t.TextMuted)
							}
							ui.Text(c, f.modTime.Format("2006-01-02")).FontSize(11).Padding(1, 6).Radius(6).
								Background(t.SurfaceHover).TextColor(t.TextMuted)
							ui.Text(c, humanSize(f.size)).FontSize(11).Padding(1, 6).Radius(6).
								Background(t.SurfaceHover).TextColor(t.TextMuted)
							ui.Spacer(c)
						})
						// 失败原因直接写在卡片里:只写"失败"等于不说
						if i < len(a.ops) && a.ops[i].Err != nil {
							ui.Text(c, a.ops[i].Err.Error()).FontSize(11).TextColor(t.Danger).SingleLine().Ellipsis("…")
						}
					})
					// 右缘结果:成功/失败/将更名/不变
					if i < len(a.ops) {
						// 结果是"状态",不是"类别":用状态点 + 纯文字,
						// 不用胶囊形状,免得和类别胶囊混成一种语言
						res, col := a.resultChip(a.ops[i])
						ui.Row(c).Width(64).Gap(5).AlignItems(ui.Center).Children(func() {
							ui.Spacer(c)
							ui.Box(c).Size(7, 7).Radius(4).Background(col)
							txt := ui.Text(c, res).FontSize(12).TextColor(col)
							if a.ops[i].Err != nil {
								txt.Tooltip(a.ops[i].Err.Error())
							}
						})
					}
				})
			}).Gap(10).Grow(1)
		}
	})
}

// resultChip gives the outcome text and its color.
func (a *app) resultChip(op core.RenameOp) (string, ui.Color) {
	t := a.currentTheme().light
	if a.isDark() {
		t = a.currentTheme().dark
	}
	switch {
	case op.Err != nil:
		return "失败", t.Danger
	case op.Applied:
		return "成功", t.Success
	case a.applied:
		return "跳过", t.TextMuted
	case op.OldName == op.NewName:
		return "不变", t.TextMuted
	}
	return "将更名", t.Accent
}

// tonalButton 是"次要动作"的浅填充(Tonal)按钮:用主色的一层低透明底,
// 文字用主色,视觉上比主按钮弱、比纯文字强。用于文件列表上的操作
// (添加文件/导出CSV),和真正的主操作(执行更名)拉开主次。
func tonalButton(c *ui.Context, label string, enabled bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Radius(16).Padding(6, 14).TextColor(t.Accent).
		Background(t.Accent.Alpha(0.12))
	if !enabled {
		b = b.TextColor(t.TextMuted).Background(t.SurfaceHover)
	} else if b.Hovered() {
		b = b.Background(t.Accent.Alpha(0.20))
	}
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}

// outlinedButton 是"中性次要动作"的镂空按钮:只描边、不填底,永远不抢眼。
// 用于回滚这类"撤销",它不是主路径,但又要能看见。
func outlinedButton(c *ui.Context, label string, enabled bool) ui.Element {
	t := c.Theme()
	fg := t.Text
	bg := ui.Color{}
	if !enabled {
		fg = t.TextMuted
	}
	b := ui.ButtonBase(c).Radius(16).Padding(6, 14).TextColor(fg).Background(bg).Border(1, t.Border)
	if enabled && b.Hovered() {
		b = b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}

// dangerButton 是"危险动作"按钮:清空列表这种不可逆操作,用错误色描边 +
// 错误色文字,平时不填底(避免太扎眼),悬停时给一层淡淡的错误色底。
func dangerButton(c *ui.Context, label string, enabled bool) ui.Element {
	t := c.Theme()
	fg := t.Danger
	bg := ui.Color{}
	if !enabled {
		fg = t.TextMuted
	}
	b := ui.ButtonBase(c).Radius(16).Padding(6, 14).TextColor(fg).Background(bg).Border(1, t.Danger.Alpha(0.5))
	if !enabled {
		b = b.Border(1, t.Border)
	} else if b.Hovered() {
		b = b.Background(t.Danger.Alpha(0.12))
	}
	b.Children(func() { ui.Text(c, label).SingleLine() })
	return b
}

// shortcutTip 把快捷键写成界面上看得见的样子。
// 快捷键不写在界面上,用户找不到就等于不存在;而 Mac 的修饰键是 ⌘ 不是
// Ctrl,写死 "Ctrl+O" 会让 Mac 用户去按一个不存在的组合。
func shortcutTip(combo string) string {
	if runtime.GOOS == "darwin" {
		return strings.ReplaceAll(strings.ReplaceAll(combo, "Ctrl+", "⌘"), "Alt+", "⌥")
	}
	return combo
}

// filepathSplitName splits a bare file name into stem and ".ext".
func filepathSplitName(name string) (stem, ext string) {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i:]
}

// resultText is the 结果 column: pending state in preview, outcome after apply.
func (a *app) resultText(op core.RenameOp) string {
	switch {
	case op.Err != nil:
		return "失败: " + op.Err.Error()
	case op.Applied:
		return "成功"
	case a.applied:
		return "跳过"
	case op.OldName == op.NewName:
		return "不变"
	default:
		return "将更名"
	}
}

// update re-renders the window after a change made off the view pass.
func (a *app) update() {
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) addPaths(paths []string) {
	seen := map[string]bool{}
	for i := range a.files {
		seen[a.files[i].path] = true
	}
	// Each entry remembers which folder group it belongs to. A dropped
	// folder contributes all its (loose) files under that folder's name; a
	// dropped file is a group of its own (empty → no header).
	type entry struct{ path, group string }
	var expanded []entry
	for _, p := range paths {
		p = filepath.Clean(p)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			// a dropped folder adds the files directly inside it (one level)
			ents, err := os.ReadDir(p)
			if err != nil {
				continue
			}
			group := filepath.Base(p)
			for _, e := range ents {
				if !e.IsDir() {
					expanded = append(expanded, entry{filepath.Join(p, e.Name()), group})
				}
			}
			continue
		}
		expanded = append(expanded, entry{p, ""})
	}
	for _, en := range expanded {
		p := en.path
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			continue
		}
		if !seen[p] {
			seen[p] = true
			fi := statFile(p)
			fi.group = en.group
			a.files = append(a.files, fi)
		}
	}
	a.assignExtColors(a.currentTheme())
	a.status = fmt.Sprintf("已添加,共 %d 个文件", len(a.files))
	a.confirmClear = false
	a.applied = false
	// AI 步骤不能在这里自动预览:preview() 是同步的,一旦含 <AI:…> 就会在
	// 主线程里联网阻塞几十秒,加入文件的瞬间 UI 卡死。AI 只走显式"预览"按钮。
	// 这与 view() 里的实时预览判断保持一致;否则清空列表后再加文件会立刻
	// 重新触发一轮查询并卡死。
	if !a.hasAIStep() {
		a.preview()
	}
	a.update() // addPaths runs off the view pass (dialogs, drops)
}

// dialogs are injectable so tests drive the whole flow without a window:
// nil means the real mygo dialogs.
type dialogHooks struct {
	open func(mygo.OpenDialogOptions) ([]string, error)
	save func(mygo.SaveDialogOptions) (string, error)
}

func (a *app) openDialog(opts mygo.OpenDialogOptions) ([]string, error) {
	if a.dialogs.open != nil {
		return a.dialogs.open(opts)
	}
	return mygo.Dialog.Open(opts)
}

func (a *app) saveDialog(opts mygo.SaveDialogOptions) (string, error) {
	if a.dialogs.save != nil {
		return a.dialogs.save(opts)
	}
	return mygo.Dialog.Save(opts)
}

func (a *app) addFilesDialog() {
	a.busy = true
	a.update()
	paths, err := a.openDialog(mygo.OpenDialogOptions{
		Parent:   a.win,
		Multiple: true,
	})
	a.busy = false
	if err != nil || len(paths) == 0 {
		a.status = ""
		a.update()
		return
	}
	a.addPaths(paths)
}

// exportCSV writes the sha256/dir/name/new_name list for external editing.
func (a *app) exportCSV() {
	a.busy = true
	a.update()
	out, err := a.saveDialog(mygo.SaveDialogOptions{
		Parent:      a.win,
		Title:       "导出文件列表 CSV",
		DefaultPath: "ncreamer-list.csv",
	})
	a.busy = false
	if err != nil || out == "" {
		a.status = ""
		a.update()
		return
	}
	a.exportTo(out)
}

// exportTo writes the list CSV to path (the dialog's part stays above).
func (a *app) exportTo(path string) error {
	f, err := os.Create(path)
	if err != nil {
		a.status = "导出失败: " + err.Error()
		a.update()
		return err
	}
	defer f.Close()
	paths := make([]string, len(a.files))
	for i := range a.files {
		paths[i] = a.files[i].path
	}
	if err := core.ExportCSV(f, paths); err != nil {
		a.status = "导出失败: " + err.Error()
		a.update()
		return err
	}
	a.status = fmt.Sprintf("已导出 %d 行到 %s;编辑 new_name 列后点“导入CSV”", len(a.files), path)
	a.update()
	return nil
}

// importCSV reads an edited list back: files are located by sha256, and
// the table becomes the imported rename plan.
func (a *app) importCSV() {
	a.busy = true
	a.update()
	paths, err := a.openDialog(mygo.OpenDialogOptions{
		Parent: a.win,
		Title:  "导入文件列表 CSV",
	})
	a.busy = false
	if err != nil || len(paths) == 0 {
		a.status = ""
		a.update()
		return
	}
	a.importFrom(paths[0])
}

// importFrom loads a list CSV as the current batch (dialog's part above).
func (a *app) importFrom(path string) (plan []core.RenameOp, errs []error) {
	plan, errs = core.ImportCSV(path)
	a.assignExtColors(a.currentTheme())
	a.ops = plan
	a.files = a.files[:0]
	// Imported rows lose their original folder grouping: group by directory
	// so a multi-directory list still reads as a tree.
	for _, op := range plan {
		fi := statFile(filepath.Join(op.Dir, op.OldName))
		fi.group = filepath.Base(op.Dir)
		a.files = append(a.files, fi)
	}
	a.applied = false
	a.status = fmt.Sprintf("已导入 %d 行,%d 个将更名", len(plan), countChanges(plan))
	if len(errs) > 0 {
		a.status += fmt.Sprintf(",%d 个有问题", len(errs))
	}
	a.update()
	return plan, errs
}

func statFile(p string) fileInfo {
	fi := fileInfo{path: p}
	if st, err := os.Stat(p); err == nil {
		fi.size, fi.modTime = st.Size(), st.ModTime()
	}
	fi.typ = detectType(p)
	fi.realType = sniffMagic(p)
	return fi
}

// listRow is one row of the file list. A group header is its own row, so a
// group of n files takes n+1 rows; this is what keeps the sticky header from
// covering the first file of its group.
type listRow struct {
	header bool
	file   int    // index into a.files, valid when !header
	group  string // group name, valid when header
	count  int    // files in this group, valid when header
}

// buildRows expands a.files into header rows plus file rows.
func buildRows(files []fileInfo) []listRow {
	rows := make([]listRow, 0, len(files)+4)
	for i := range files {
		g := files[i].group
		if g != "" && (i == 0 || files[i-1].group != g) {
			n := 0
			for j := range files {
				if files[j].group == g {
					n++
				}
			}
			rows = append(rows, listRow{header: true, group: g, count: n})
		}
		rows = append(rows, listRow{file: i})
	}
	return rows
}

// statFileKeepGroup 与 statFile 相同,但保留原分组。
// 执行/回滚后重建 fileInfo 时如果丢掉 group,列表会进入"有的文件有组名、
// 有的没有"的混合状态,而分组头判定是"组名非空且与上一行不同"——
// 于是空组名与组名的交界处凭空多出一个组头,每个组头还会吞掉一行。
func statFileKeepGroup(p, group string) fileInfo {
	fi := statFile(p)
	fi.group = group
	return fi
}

// stepSpecs renders the task steps to CLI rule specs (with any filters).
func (a *app) stepSpecs() []string {
	var out []string
	for i := range a.steps {
		if s := a.steps[i].fullSpec(); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// previewAsync runs the pipeline off the UI thread, because AI steps block on
// the network for as long as the model takes (often tens of seconds). Doing
// this synchronously in the click handler froze the window with no feedback;
// now the UI shows a spinner while we wait. apply/undo follow the same shape.
func (a *app) previewAsync() {
	a.busy = true
	a.busyAI = true
	a.busyStart = time.Now()
	a.status = "正在联网命名,模型思考中…"
	a.update()
	// Tick the UI once a second so the wait shows live elapsed seconds instead of
	// a frozen spinner. The user can tell it's working, not hung.
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				a.update()
			}
		}
	}()
	defer func() {
		close(stop)
		a.busy = false
		a.busyAI = false
		a.update()
	}()
	a.preview()
}

func (a *app) preview() {
	rules, err := core.ParseRules(a.stepSpecs())
	if err != nil {
		a.ops = nil
		a.status = err.Error()
		return
	}
	paths := make([]string, len(a.files))
	for i := range a.files {
		paths[i] = a.files[i].path
	}
	ops, _ := core.RunPipeline(rules, paths)
	a.ops = ops
	a.status = fmt.Sprintf("%d 个将更名", countChanges(ops))
	if n := countErrs(ops); n > 0 {
		a.status += fmt.Sprintf(",%d 个有问题", n)
		// 把第一个原因也说出来。只说"有问题"时用户要自己一行行找红字,
		// 而模板写错这类问题是所有文件同一个原因,说一次就够了。
		if msg := firstErrText(ops); msg != "" {
			a.status += ":" + msg
		}
	}
	if a.hasAIStep() && strings.Contains(strings.Join(a.stepSpecs(), ""), "<AI:") {
		a.status += " ·含 AI 步骤,点“预览”联网获取命名"
	}
	// no update() here: preview usually runs inside the view pass (live
	// preview), and updating mid-pass rebuilds the tree and loses scroll
	// positions; async callers update on their own
}

// handleShortcuts 是窗口级快捷键:不管焦点落在哪都响应,除非焦点所在的
// 输入框先把它吃掉了(比如在参数框里按 Ctrl+Z 是撤销输入,不是回滚文件)。
//
// 为什么必须每帧都问一遍:mygo 的快捷键注册表每帧交换一次
// (runtime.go 的 `rt.regs, rt.nextRegs = rt.nextRegs, rt.regs`),
// 只在某一帧问过的键,下一帧就不认了。所以这里在 view() 开头无条件调用。
//
// 用 ui.Cmd 而不是 ui.Ctrl:macOS 上 Cmd 就是 Super,其它平台是 Ctrl,
// 硬写 Ctrl 会让 Mac 用户按不出来。
func (a *app) handleShortcuts(c *ui.Context) {
	// 对话框/面板开着时不接管:它们有自己的 Esc 与默认按钮,
	// 底下这层要是还响应 Ctrl+O,就成了"看不见的窗口在后台动作"。
	if a.settingsOpen || a.pickOpen || a.pickMode {
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) && !a.busy {
		go a.addFilesDialog()
	}
	if c.Shortcut(ui.Cmd, ui.KeyEnter) && !a.busy && a.hasChanges() {
		go a.apply()
	}
	if c.Shortcut(ui.Cmd, ui.KeyZ) && !a.busy && a.journalExists() {
		// 与点按钮同一套语言:先挂确认态,再按一次才真回滚
		if a.confirmUndo && !a.confirmStale() {
			a.dropConfirm()
			go a.undo()
		} else {
			a.armConfirm("undo")
		}
	}
	// 文件列表拿着焦点时,Delete 删的是文件而不是步骤——同一个键在两处
	// 各管各的,靠焦点区分,不靠用户记"现在删的是什么"。
	// fileFocus 是上一帧结束时记下的"文件列表拿着焦点"。
	// 不能用 a.table.FocusWithin(c):焦点在这一帧的构建过程中才被设上,
	// 而 handleShortcuts 跑在构建之前,当场问永远是 false。
	if a.fileFocus {
		if c.Shortcut(0, ui.KeyDelete) {
			a.removeChosenFiles()
		}
		return
	}
	if a.stepSel < 0 || a.stepSel >= len(a.steps) {
		return
	}
	if c.Shortcut(0, ui.KeyDelete) {
		// 与鼠标点「删除」走同一条路:先挂确认态,三秒不点自动收回。
		// 已经挂着确认态时再按一次 Delete 就是"确认",不必去够鼠标。
		if a.confirmDelStep && !a.confirmStale() {
			a.deleteStep(a.stepSel)
			a.dropConfirm()
		} else {
			a.armConfirm("step")
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyD) {
		a.duplicateStep(a.stepSel)
	}
	if c.Shortcut(ui.Alt, ui.KeyUp) {
		a.swapSteps(a.stepSel-1, a.stepSel)
	}
	if c.Shortcut(ui.Alt, ui.KeyDown) {
		a.swapSteps(a.stepSel, a.stepSel+1)
	}
}

func (a *app) hasChanges() bool { return countChanges(a.ops) > 0 }

// batchSummary 只回答一个问题:这批一共有多少文件。
// 计数以前说三遍(右栏工具栏「N 个文件」+ 每个分组头「N 个文件」+
// 底栏「N 个将更名」),而"几个会改名"在底栏又说了一遍。现在分工是:
// 标题栏右侧=总数,分组头=每组数,底栏=将更名/有问题——各说一次。
func (a *app) batchSummary() string {
	if len(a.files) == 0 {
		return ""
	}
	return fmt.Sprintf("%d 个文件", len(a.files))
}

// confirmStale reports whether a pending second-click confirmation has
// outlived its window. 防错提示挂着不走,等于把"再点一下就删"的按钮
// 永远留在屏幕上——三秒内没点就当用户改主意了。
func (a *app) confirmStale() bool {
	return !a.confirmAt.IsZero() && time.Since(a.confirmAt) > 3*time.Second
}

// armConfirm 记下确认窗口的起点,并收回另一个待确认的动作——
// 同时挂着两个"再点一下就删"是没道理的状态。
func (a *app) armConfirm(which string) {
	a.confirmClear = which == "clear"
	a.confirmDelStep = which == "step"
	a.confirmChosen = which == "chosen"
	a.confirmUndo = which == "undo"
	a.confirmAt = time.Now()
	// 界面只在有事件时重画。用户点完第一下就撒手,没有事件,
	// confirmStale 永远没机会跑,确认态就一直挂着。所以自己排一次重画。
	go func(at time.Time) {
		time.Sleep(3*time.Second + 100*time.Millisecond)
		if a.confirmAt.Equal(at) {
			a.update()
		}
	}(a.confirmAt)
}

// dropConfirm 收回所有二次确认态。
func (a *app) dropConfirm() {
	a.confirmClear = false
	a.confirmDelStep = false
	a.confirmChosen = false
	a.confirmUndo = false
	a.confirmAt = time.Time{}
}

// firstErrText returns the first real error message, skipping sidecar files
// (cover.jpg next to an mp3 is not the user's problem).
func firstErrText(ops []core.RenameOp) string {
	for _, op := range ops {
		if op.Err != nil && !core.IsSidecarFile(op.OldName) {
			return op.Err.Error()
		}
	}
	return ""
}

// hasAIStep reports whether any step needs the network (<AI:...>).
func (a *app) hasAIStep() bool {
	for _, s := range a.stepSpecs() {
		if strings.Contains(s, "<AI:") {
			return true
		}
	}
	return false
}

// journalExists: 回滚只在真有日志可回滚时可用.
// 只看文件在不在不够——上一次执行如果全失败,日志里是空列表,
// 按钮却亮着,点下去一个文件也不动还报"已回滚"。
func (a *app) journalExists() bool {
	_, ok := a.journalInfo()
	return ok
}

// journalInfo 读出日志里还能回滚的那一批:时间、几个文件、都是谁。
// 界面上"回滚上次"要能说出这些,而不是三个字的哑按钮。
func (a *app) journalInfo() (core.JournalSummary, bool) {
	if a.settings.Journal == "" {
		return core.JournalSummary{}, false
	}
	return core.ReadJournalSummary(a.settings.Journal)
}

// undoTip 是"回滚上次"悬停时的说明:什么时候改的、前三个文件名。
// 点之前就该知道要点掉什么。
func (a *app) undoTip(sum core.JournalSummary) string {
	tip := shortcutTip("Ctrl+Z")
	if len(sum.Names) == 0 {
		return tip + " · 没有可回滚的记录"
	}
	if !sum.Time.IsZero() {
		tip = sum.Time.Format("01-02 15:04") + " 的更名 · " + tip
	}
	n := len(sum.Names)
	shown := sum.Names
	if len(shown) > 3 {
		shown = shown[:3]
	}
	tip += "\n" + strings.Join(shown, "\n")
	if n > len(shown) {
		tip += fmt.Sprintf("\n…等 %d 个文件", n)
	}
	return tip
}

// statusColor colors feedback: failures stand out, success reads calm.
func (a *app) statusColor(t *ui.Theme) ui.Color {
	switch {
	case a.busy:
		return t.TextMuted
	case strings.Contains(a.status, "失败") || strings.Contains(a.status, "有问题"):
		return t.Danger
	case strings.Contains(a.status, "已回滚") || strings.Contains(a.status, "成功"):
		return t.Success
	}
	return t.TextMuted
}

func countChanges(ops []core.RenameOp) (n int) {
	for _, op := range ops {
		// 已经改过的不再算"将更名":否则执行完主按钮还亮着,
		// 再点一次全部失败,用户会以为程序坏了。
		if op.Err == nil && op.OldName != op.NewName && !op.Applied {
			n++
		}
	}
	return
}

func countErrs(ops []core.RenameOp) (n int) {
	for _, op := range ops {
		if op.Err != nil && !core.IsSidecarFile(op.OldName) {
			n++
		}
	}
	return
}

func (a *app) apply() {
	a.busy = true
	a.busyStart = time.Now()
	a.update()
	err := core.ApplyPlan(a.ops, a.settings.Journal)
	a.busy = false
	a.busyStart = time.Time{}
	a.applied = true
	if err != nil {
		a.status = "执行出错: " + err.Error()
	} else {
		ok, failed := 0, 0
		for _, op := range a.ops {
			switch {
			case op.Applied:
				ok++
			case op.Err != nil:
				failed++
			}
		}
		// 只报告这一次真的发生了什么:"失败"不该把"名字没变化、无需处理"
		// 的文件也算进去,那会让一次干净的批量看起来像出了很多错。
		if ok == 0 {
			a.status = fmt.Sprintf("没有文件被更名,失败 %d", failed)
			if failed == 0 {
				a.status = "没有文件需要更名"
			}
		} else {
			a.status = fmt.Sprintf("成功 %d,失败 %d;可用“回滚上次”撤销", ok, failed)
		}
		for i := range a.files {
			if i < len(a.ops) && a.ops[i].Applied {
				a.files[i] = statFileKeepGroup(filepath.Join(a.ops[i].Dir, a.ops[i].NewName), a.files[i].group)
			}
		}
		a.assignExtColors(a.currentTheme())
	}
	a.update()
}

func (a *app) undo() {
	a.busy = true
	a.busyStart = time.Now()
	a.update()
	err := core.UndoJournal(a.settings.Journal)
	a.busy = false
	a.busyStart = time.Time{}
	if err != nil {
		// 尽力而为的回滚可能只退了一部分,状态里必须说清退了多少,
		// 而不是一句"回滚失败"让用户以为文件没动过。
		var ue *core.UndoError
		if errors.As(err, &ue) && ue.Done > 0 {
			a.status = "回滚部分完成: " + ue.Error()
			for i := range a.files {
				if i < len(a.ops) && a.ops[i].Applied {
					a.files[i] = statFileKeepGroup(filepath.Join(a.ops[i].Dir, a.ops[i].OldName), a.files[i].group)
					a.ops[i].Applied = false
				}
			}
			a.assignExtColors(a.currentTheme())
			a.update()
			return
		}
		a.status = "回滚失败: " + err.Error()
	} else {
		a.status = "已回滚"
		for i := range a.files {
			if i < len(a.ops) && a.ops[i].Applied {
				a.files[i] = statFileKeepGroup(filepath.Join(a.ops[i].Dir, a.ops[i].OldName), a.files[i].group)
				// 已回滚的项不再算"将更名",否则主按钮会在回滚后仍然亮着
				a.ops[i].Applied = false
			}
		}
		a.assignExtColors(a.currentTheme())
		a.applied = true
	}
	a.update()
}

// detectType gives the 识别类型 column: the format by extension, plus a
// note when we can read metadata out of it.
func detectType(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	labels := map[string]string{
		"epub": "EPUB 电子书", "pdf": "PDF 文档",
		"mp4": "MP4 视频", "m4v": "MP4 视频", "mov": "MP4 视频", "m4a": "MP4 音频", "m4b": "MP4 有声书",
		"mkv": "MKV 视频", "webm": "WebM 视频", "mka": "MKV 音频", "avi": "AVI 视频",
		"mp3": "MP3 音频", "flac": "FLAC 音频", "ogg": "OGG 音频", "wma": "WMA 音频", "wav": "WAV 音频",
		"jpg": "JPEG 图片", "jpeg": "JPEG 图片", "png": "PNG 图片", "gif": "GIF 图片", "webp": "WebP 图片", "bmp": "BMP 图片",
		"txt": "文本", "md": "Markdown", "srt": "字幕", "ass": "字幕",
		"zip": "压缩包", "rar": "压缩包", "7z": "压缩包", "tar": "压缩包",
	}
	label, ok := labels[ext]
	if !ok {
		if ext == "" {
			return "无扩展名"
		}
		label = strings.ToUpper(ext)
	}
	if m := core.ExtractMeta(path); m != nil {
		if _, has := m["title"]; has {
			label += " ·含元数据"
		}
	}
	return label
}

func humanSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	f := float64(n)
	for _, u := range units {
		f /= 1024
		if f < 1024 {
			return fmt.Sprintf("%.1f %s", math.Round(f*10)/10, u)
		}
	}
	return fmt.Sprintf("%.1f PB", f/1024)
}

func main() {
	// 单实例:两个 ncreamer 同时跑会互相覆盖 journal、各自执行更名
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	a := &app{stepSel: -1, fileSel: -1, settings: LoadSettings()}
	a.syncSeedUI()
	a.ApplySettings()
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:          "New Century Renamer",
			Width:          1280,
			Height:         720,
			MinWidth:       900,
			MinHeight:      480,
			StateKey:       "main",
			TitleBarStyle:  mygo.TitleBarHidden,
			TitleBarHeight: 38,
			Content:        ui.View(a.view),
		})
		// the whole window accepts file drops, not just one element
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			a.addPaths(e.Paths)
		})
		// follow the desktop's light/dark switches live
		mygo.Theme.OnUpdated(func() { a.update() })
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// syncSeedUI mirrors the settings' seed into the dialog widgets.
func (a *app) syncSeedUI() {
	s := a.liveSettings()
	a.seedFollow = s.Theme.Seed == ""
	if !a.seedFollow {
		a.seedPick = ui.Hex(s.Theme.Seed)
	} else if c, ok := systemAccent(); ok {
		a.seedPick = c
	} else {
		a.seedPick = ui.Hex(fallbackSeed)
	}
	// 等待秒数:0 表示用默认(300),输入框留空即显示默认提示
	if s.AI.Timeout > 0 {
		a.aiTimeoutText = strconv.Itoa(s.AI.Timeout)
	} else {
		a.aiTimeoutText = ""
	}
	// 并发路数:0 表示用默认(4 路),留空显示默认
	if s.AI.Concurrency > 0 {
		a.aiConcurrencyText = strconv.Itoa(s.AI.Concurrency)
	} else {
		a.aiConcurrencyText = ""
	}
	// 默认提示词:原样载入,留空表示内置默认
	a.aiPromptText = s.AI.Prompt
}

// commitAITimeout parses the "等待秒数" text buffer back into the settings.
// 空串 / 非数字 / <=0 一律视为"用默认",即清成 0。
func (a *app) commitAITimeout() {
	s := a.liveSettings()
	buf := strings.TrimSpace(a.aiTimeoutText)
	if n, err := strconv.Atoi(buf); err == nil && n > 0 {
		s.AI.Timeout = n
	} else {
		s.AI.Timeout = 0
	}
}

// commitAIConcurrency parses the "并发路数" buffer. 空串 / 非数字 / <=0 视为默认。
// 云端 API 限流时用户会填 1(单路);本地模型想多路也能填更大值。
func (a *app) commitAIConcurrency() {
	dst := a.liveSettings()
	buf := strings.TrimSpace(a.aiConcurrencyText)
	if n, err := strconv.Atoi(buf); err == nil && n > 0 {
		dst.AI.Concurrency = n
	} else {
		dst.AI.Concurrency = 0
	}
}

// commitAIPrompt saves the "默认提示词" buffer, trimmed. 空串即恢复内置默认。
func (a *app) commitAIPrompt() {
	a.liveSettings().AI.Prompt = strings.TrimSpace(a.aiPromptText)
}
