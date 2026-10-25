package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"reamer/core"
)

// fileInfo is one file in the list, with the facts the table shows.
type fileInfo struct {
	path     string
	size     int64
	modTime  time.Time
	typ      string // by extension
	realType string // sniffed from content; differs → warning chip
}

// app is the state the window shows: three panes — catalog, task steps,
// file preview.
type app struct {
	win      *mygo.Window
	files    []fileInfo
	ops      []core.RenameOp // preview or apply result, aligned with files
	status   string
	busy     bool
	applied  bool // true after 执行/回滚, until steps or files change
	lastSpec string

	settings     Settings
	settingsOpen bool
	extColors    map[string]ui.Color // per-extension chip colors (main-thread)
	seedFollow   bool                // theme seed follows the desktop accent
	seedPick     ui.Color            // theme seed when not following
	confirmClear bool                // 清空列表 needs a second click
	saveErr      string              // settings dialog save error, shown in-dialog

	steps    []guiRule // ordered task steps
	stepSel  int       // selected step, -1 = none
	stepList ui.ListState
	palSel   string            // last catalog op added, highlighted in the list
	stepUID  int               // 自增步骤身份
	chosen   ui.Selection[int] // multi-selected step uids
	pickMode bool              // 显示行内多选圆点(触摸入口)
	palHint  string            // hovered/selected catalog entry's explanation

	splitL, splitM float32 // pane divider positions
	table          ui.ListState
}

func (a *app) view(c *ui.Context) {
	dt := a.currentTheme()
	if a.isDark() {
		c.SetTheme(dt.dark)
	} else {
		c.SetTheme(dt.light)
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
	ui.Split(c, &a.splitL,
		func() { a.catalogView(c) },
		func() {
			ui.Split(c, &a.splitM,
				func() { a.stepsView(c) },
				func() { a.filesView(c) })
		}).Fill()
	a.settingsDialog(c)
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
	ui.Column(c).Fill().Children(func() {
		ui.Text(c, "③ 功能 · 点击加入步骤").Bold().FontSize(13).Padding(8, 10)
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
						// 悬停即说明:用户可以先弄明白再决定点不点
						if d.act != "" {
							a.palHint = d.desc
						} else {
							a.palHint = d.desc + "\n点击加入任务步骤,在“② 任务步骤”里调整参数与顺序"
						}
					}
					if item.Clicked() {
						switch d.act {
						case "import-csv":
							go a.importCSV()
						case "settings":
							a.syncSeedUI()
							a.settingsOpen = true
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
		// 底部说明区:固定高度,内容变化不移动目录条目(否则点击会被位移打断)
		ui.Column(c).Fill().Height(64).Padding(8, 10).Gap(4).Background(t.Background).Children(func() {
			if a.palHint != "" {
				ui.Text(c, a.palHint).FontSize(12).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "悬停查看功能说明\n左对齐 = 改名步骤 · 居中 = 流程控制/工具").FontSize(12).TextColor(t.TextMuted)
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
	ui.Column(c).Fill().Gap(8).Padding(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("② 任务步骤(%d)", len(a.steps))).Bold().FontSize(13)
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
			ui.Text(c, "从左侧选择功能加入步骤").TextColor(t.TextMuted)
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
						ui.Text(c, fmt.Sprintf("%d.", i+1)).FontSize(12).TextColor(numCol).Width(22)
						// 文字区:处理步骤左对齐,流程控制居中——胶囊区
						// 固定宽度,居中位置稳定不被挤掉
						if s.opFlow {
							ui.Text(c, s.opLabel+" — "+s.summary()).FontSize(12).Fill().TextAlign(ui.Center).TextColor(labCol)
						} else {
							ui.Row(c).Fill().Gap(6).Children(func() {
								ui.Text(c, s.opLabel).FontSize(12).Bold().TextColor(labCol).SingleLine()
								if v := s.summary(); v != "" && v != s.opLabel {
									ui.Text(c, v).FontSize(11).Padding(2, 8).Radius(8).MaxWidth(150).Ellipsis("…").
										Background(cat.Alpha(0.16)).TextColor(t.Text)
								}
							})
						}
						// 胶囊区:固定宽度单独管理,行内不再被挤
						ui.Row(c).Width(84).Gap(4).Children(func() {
							ui.Spacer(c)
							// 连续同类别:胶囊退化为同色圆点,换组才显示文字
							if i > 0 && a.steps[i-1].opGroup == s.opGroup {
								ui.Box(c).Size(9, 9).Background(cat)
							} else {
								ui.Text(c, s.opGroup).FontSize(11).Padding(2, 8).Radius(8).
									Background(cat).TextColor(ui.RGB(255, 255, 255))
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
				if ui.Button(c, "删除所选").Radius(16).Clicked() {
					a.deleteChosen()
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
				if ui.Button(c, "上移").Disabled(a.stepSel == 0).Clicked() {
					a.swapSteps(a.stepSel-1, a.stepSel)
				}
				if ui.Button(c, "下移").Disabled(a.stepSel >= len(a.steps)-1).Clicked() {
					a.swapSteps(a.stepSel, a.stepSel+1)
				}
				if ui.Button(c, "删除").Clicked() {
					a.steps = append(a.steps[:a.stepSel], a.steps[a.stepSel+1:]...)
					a.stepSel--
				}
				if ui.Button(c, "复制").Clicked() {
					cp := *r
					a.stepUID++
					cp.opUID = a.stepUID
					a.steps = append(a.steps, cp)
					a.stepSel = len(a.steps) - 1
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

func (a *app) swapSteps(i, j int) {
	if i < 0 || j < 0 || i >= len(a.steps) || j >= len(a.steps) {
		return
	}
	a.steps[i], a.steps[j] = a.steps[j], a.steps[i]
	a.stepSel = j
}

// ---- right pane: file table with drop zone ----

func (a *app) filesView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Gap(8).Padding(10).Children(func() {
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "① 文件").Bold().FontSize(13)
			if ui.Button(c, "添加文件…").Radius(16).Clicked() {
				go a.addFilesDialog()
			}
			if ui.Button(c, "导出CSV").Radius(16).Disabled(len(a.files) == 0).Clicked() {
				go a.exportCSV()
			}
			if a.confirmClear {
				if ui.Button(c, "确认清空?").Radius(16).Clicked() {
					a.files, a.ops, a.applied = nil, nil, false
					a.confirmClear = false
				}
			} else {
				if ui.Button(c, "清空列表").Radius(16).Disabled(len(a.files) == 0).Clicked() {
					a.confirmClear = true // 防错:两次点击才清空
				}
			}
			ui.Text(c, fmt.Sprintf("%d 个文件", len(a.files))).TextColor(t.TextMuted)
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			// 预览只对 AI 步骤有意义(其余实时预览已自动),藏起来免得让人想"要不要点"
			if a.hasAIStep() {
				if ui.PrimaryButton(c, "预览(AI 联网命名)").Radius(16).Disabled(a.busy || len(a.files) == 0).Clicked() {
					a.preview()
				}
			}
			if ui.Button(c, "执行更名").Radius(16).Disabled(a.busy || !a.hasChanges()).Clicked() {
				go a.apply()
			}
			if ui.Button(c, "回滚上次").Radius(16).Disabled(a.busy || !a.journalExists()).Clicked() {
				go a.undo()
			}
			if a.busy {
				ui.Text(c, "处理中…").TextColor(t.TextMuted)
			} else if a.status != "" {
				ui.Text(c, a.status).TextColor(a.statusColor(t))
			}
		})
		if len(a.files) == 0 {
			ui.Column(c).Grow(1).Center().Children(func() {
				ui.Text(c, "把文件或文件夹拖到窗口任意位置,或点“添加文件…”").TextColor(t.TextMuted)
			})
		} else {
			dt := a.currentTheme()
			// 每个文件两行卡:上行 原名 → 新名(预览)对照,左原右新;
			// 下行 原始数据胶囊,路径悬停显示绝对地址
			ui.List(c, &a.table, len(a.files), func(i int) {
				f := a.files[i]
				ui.Row(c).Fill().Gap(8).Children(func() {
					card := ui.Column(c).Fill().Gap(4).Padding(6, 10).Radius(8).
						Background(dt.cardBgFor(a.isDark()))
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
							ui.Text(c, stem).FontSize(13).Padding(2, 10).Radius(8).
								Background(stemBg).MaxWidth(170).Ellipsis("…")
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
								// 预览:胶囊文件名 . 胶囊扩展名(新类型色,实)
								nameChips(nStem, nExt, nCol.Alpha(0.18), nCol, ui.RGB(255, 255, 255))
							}
							ui.Spacer(c)
						})
						// 下行:原始数据胶囊;路径悬停显示绝对地址
						ui.Row(c).Gap(6).Children(func() {
							path := ui.Text(c, pathTail3(filepath.Dir(f.path))).FontSize(11).Padding(1, 6).Radius(6).
								Background(t.SurfaceHover).TextColor(t.TextMuted).MaxWidth(170).Ellipsis("…")
							path.Tooltip(filepath.Dir(f.path))
							// 真实类型:始终显示;与扩展名不符时红色警示
							extNoDot := strings.ToLower(strings.TrimPrefix(oExt, "."))
							if f.realType != "" {
								if !strings.EqualFold(f.realType, extNoDot) {
									ui.Text(c, "实际 "+f.realType).FontSize(11).Padding(1, 6).Radius(6).
										Background(t.Danger.Alpha(0.15)).TextColor(t.Danger)
								} else {
									rc := a.fileColor(dt, f.realType)
									ui.Text(c, f.realType).FontSize(11).Padding(1, 6).Radius(6).
										Background(rc.Alpha(0.18)).TextColor(t.Text)
								}
							} else if f.typ != "" && f.typ != "无扩展名" {
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
							ui.Text(c, "⚠ "+a.ops[i].Err.Error()).FontSize(11).TextColor(t.Danger).SingleLine().Ellipsis("…")
						}
					})
					// 右缘结果:成功/失败/将更名/不变
					if i < len(a.ops) {
						res, col := a.resultChip(a.ops[i])
						chip := ui.Text(c, res).FontSize(12).Padding(2, 10).Radius(8).
							Background(col.Alpha(0.16)).TextColor(col).Width(56)
						if a.ops[i].Err != nil {
							chip.Tooltip(a.ops[i].Err.Error())
						}
					}
				})
			}).Gap(6).Grow(1)
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
	var expanded []string
	for _, p := range paths {
		p = filepath.Clean(p)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			// a dropped folder adds the files directly inside it (one level)
			ents, err := os.ReadDir(p)
			if err != nil {
				continue
			}
			for _, e := range ents {
				if !e.IsDir() {
					expanded = append(expanded, filepath.Join(p, e.Name()))
				}
			}
			continue
		}
		expanded = append(expanded, p)
	}
	for _, p := range expanded {
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			continue
		}
		if !seen[p] {
			seen[p] = true
			a.files = append(a.files, statFile(p))
		}
	}
	a.assignExtColors(a.currentTheme())
	a.status = fmt.Sprintf("已添加,共 %d 个文件", len(a.files))
	a.confirmClear = false
	a.applied = false
	a.preview()
	a.update() // addPaths runs off the view pass (dialogs, drops)
}

func (a *app) addFilesDialog() {
	a.busy = true
	a.update()
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
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
	out, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      a.win,
		Title:       "导出文件列表 CSV",
		DefaultPath: "reamer-list.csv",
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
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
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
	for _, op := range plan {
		a.files = append(a.files, statFile(filepath.Join(op.Dir, op.OldName)))
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
	ops, errs := core.RunPipeline(rules, paths)
	a.ops = ops
	a.status = fmt.Sprintf("%d 个将更名", countChanges(ops))
	if n := len(errs) + countErrs(ops); n > 0 {
		a.status += fmt.Sprintf(",%d 个有问题", n)
	}
	if a.hasAIStep() && strings.Contains(strings.Join(a.stepSpecs(), ""), "<AI:") {
		a.status += " ·含 AI 步骤,点“预览”联网获取命名"
	}
	// no update() here: preview usually runs inside the view pass (live
	// preview), and updating mid-pass rebuilds the tree and loses scroll
	// positions; async callers update on their own
}

func (a *app) hasChanges() bool { return countChanges(a.ops) > 0 }

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
func (a *app) journalExists() bool {
	if a.settings.Journal == "" {
		return false
	}
	_, err := os.Stat(a.settings.Journal)
	return err == nil
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
		if op.Err == nil && op.OldName != op.NewName {
			n++
		}
	}
	return
}

func countErrs(ops []core.RenameOp) (n int) {
	for _, op := range ops {
		if op.Err != nil {
			n++
		}
	}
	return
}

func (a *app) apply() {
	a.busy = true
	a.update()
	err := core.ApplyPlan(a.ops, a.settings.Journal)
	a.busy = false
	a.applied = true
	if err != nil {
		a.status = "执行出错: " + err.Error()
	} else {
		ok := 0
		for _, op := range a.ops {
			if op.Applied {
				ok++
			}
		}
		a.status = fmt.Sprintf("成功 %d,失败 %d;可用“回滚上次”撤销", ok, len(a.ops)-ok)
		for i := range a.files {
			if i < len(a.ops) && a.ops[i].Applied {
				a.files[i] = statFile(filepath.Join(a.ops[i].Dir, a.ops[i].NewName))
			}
		}
		a.assignExtColors(a.currentTheme())
	}
	a.update()
}

func (a *app) undo() {
	a.busy = true
	a.update()
	err := core.UndoJournal(a.settings.Journal)
	a.busy = false
	if err != nil {
		a.status = "回滚失败: " + err.Error()
	} else {
		a.status = "已回滚"
		for i := range a.files {
			if i < len(a.ops) && a.ops[i].Applied {
				a.files[i] = statFile(filepath.Join(a.ops[i].Dir, a.ops[i].OldName))
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
	a := &app{stepSel: -1, settings: LoadSettings()}
	a.syncSeedUI()
	a.ApplySettings()
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "reamer",
			Width:     1280,
			Height:    720,
			MinWidth:  900,
			MinHeight: 480,
			StateKey:  "main",
			Content:   ui.View(a.view),
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
	a.seedFollow = a.settings.Theme.Seed == ""
	if !a.seedFollow {
		a.seedPick = ui.Hex(a.settings.Theme.Seed)
	} else if c, ok := systemAccent(); ok {
		a.seedPick = c
	} else {
		a.seedPick = ui.Hex(fallbackSeed)
	}
}
