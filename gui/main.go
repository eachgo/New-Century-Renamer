package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"reamer/core"
)

// fileInfo is one file in the list, with the facts the table shows.
type fileInfo struct {
	path    string
	size    int64
	modTime time.Time
	typ     string // 识别类型
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
	seedFollow   bool     // theme seed follows the desktop accent
	seedPick     ui.Color // theme seed when not following
	confirmClear bool     // 清空列表 needs a second click
	saveErr      string   // settings dialog save error, shown in-dialog

	steps    []guiRule // ordered task steps
	stepSel  int       // selected step, -1 = none
	stepList ui.ListState
	palSel   string // last catalog op added, highlighted in the list
	palHint  string // hovered/selected catalog entry's explanation

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
							r.opID, r.opLabel, r.opDesc = d.id, d.label, d.desc
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

// ---- middle pane: ordered task steps + selected step's detail ----

func (a *app) stepsView(c *ui.Context) {
	t := c.Theme()
	if a.stepSel >= len(a.steps) {
		a.stepSel = len(a.steps) - 1
	}
	ui.Column(c).Fill().Gap(8).Padding(8).Children(func() {
		ui.Text(c, fmt.Sprintf("② 任务步骤(%d)", len(a.steps))).Bold().FontSize(13)
		a.stepList.Selected = &a.stepSel
		if len(a.steps) == 0 {
			ui.Text(c, "从左侧选择功能加入步骤").TextColor(t.TextMuted)
		} else {
			ui.List(c, &a.stepList, len(a.steps), func(i int) {
				s := a.steps[i]
				mark := " "
				if i == a.stepSel {
					mark = "▸"
				}
				ui.Text(c, fmt.Sprintf("%s%d. %s\n    %s", mark, i+1, s.opLabel, s.summary())).FontSize(12)
			}).Grow(1)
		}
		// detail of the selected step
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
					a.steps = append(a.steps, cp)
					a.stepSel = len(a.steps) - 1
				}
			})
		}
	})
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
			cols := []ui.TableColumn{
				{Title: "文件名", Width: 230, Sortable: true},
				{Title: "预览", Width: 230},
				{Title: "扩展名", Width: 64},
				{Title: "日期", Width: 120, Sortable: true},
				{Title: "路径", Width: 200},
				{Title: "大小", Width: 76, Align: ui.End, Sortable: true},
				{Title: "识别类型", Width: 110},
				{Title: "结果", Width: 190},
			}
			ui.Table(c, &a.table, cols, len(a.files), func(row, col int) {
				f := a.files[row]
				switch col {
				case 0:
					ui.Text(c, filepath.Base(f.path)).SingleLine()
				case 1:
					if row < len(a.ops) {
						ui.Text(c, a.ops[row].NewName).SingleLine()
					}
				case 2:
					ui.Text(c, strings.TrimPrefix(filepath.Ext(f.path), "."))
				case 3:
					ui.Text(c, f.modTime.Format("2006-01-02 15:04"))
				case 4:
					ui.Text(c, filepath.Dir(f.path)).SingleLine()
				case 5:
					ui.Text(c, humanSize(f.size))
				case 6:
					ui.Text(c, f.typ).SingleLine()
				case 7:
					if row < len(a.ops) {
						ui.Text(c, a.resultText(a.ops[row])).SingleLine()
					}
				}
			}).Grow(1)
		}
	})
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
	f, err := os.Create(out)
	if err != nil {
		a.status = "导出失败: " + err.Error()
		a.update()
		return
	}
	defer f.Close()
	paths := make([]string, len(a.files))
	for i := range a.files {
		paths[i] = a.files[i].path
	}
	if err := core.ExportCSV(f, paths); err != nil {
		a.status = "导出失败: " + err.Error()
	} else {
		a.status = fmt.Sprintf("已导出 %d 行到 %s;编辑 new_name 列后点“导入CSV”", len(a.files), out)
	}
	a.update()
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
	plan, errs := core.ImportCSV(paths[0])
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
}

func statFile(p string) fileInfo {
	fi := fileInfo{path: p}
	if st, err := os.Stat(p); err == nil {
		fi.size, fi.modTime = st.Size(), st.ModTime()
	}
	fi.typ = detectType(p)
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
