package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// openHelp 每次从顶部开始:上次读到哪是上次的事,重新打开时先看到
// "五步就够"的标题,而不是接着上次滚到的变量表中间。
func (a *app) openHelp() {
	a.helpScroll.Y = 0
	a.helpOpen = true
}

// openSettings starts editing on a copy of the current settings. Every widget
// in the dialog writes to the draft, so「取消」/Esc just drop it — the table
// no longer has two behaviours where some fields revert and some don't.
// Theme mode and seed read through liveSettings() so changing them still
// previews live while the dialog is open.
func (a *app) openSettings() {
	a.settingsDraft = a.settings
	a.saveErr = ""
	a.pickOpen = false
	a.syncSeedUI()
	a.settingsOpen = true
}

// closeSettings ends the dialog. commit=true adopts the draft (only after a
// successful save); false throws it away, leaving a.settings untouched.
func (a *app) closeSettings(commit bool) {
	if commit {
		a.settings = a.settingsDraft
		a.ApplySettings()
	} else {
		a.settingsDraft = a.settings
	}
	a.syncSeedUI()
	a.pickOpen = false
	a.settingsOpen = false
}

// settingsDialog is the settings modal: AI endpoint, journal file, theme,
// and import/export of the whole settings as JSON — extensible from here.
//
// Layout follows the four principles (写给大家看的设计书): related fields
// grouped into tinted cards (亲密性), every row the same label-left form on
// one axis (对齐/重复), one primary action right-aligned (对比). Rows are
// compact so the dialog rarely needs to scroll; it still fits the window
// and scrolls what overflows.
func (a *app) settingsDialog(c *ui.Context) {
	t := c.Theme()
	dt := a.currentTheme()
	cardBg := dt.cardBgFor(a.isDark())
	labelW := float32(120)

	ui.Modal(c, &a.settingsOpen, func() {
		w, h := c.Size()
		dw := float32(520)
		if w-48 < dw {
			dw = w - 48
		}
		dh := h - 48

		// row is the repeated unit: fixed-width label, control fills.
		// StartAlign keeps the label at the top when the control is multi-line
		// (e.g. the prompt TextArea) instead of floating it to the middle.
		row := func(label string, ctl func()) {
			ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
				ui.Text(c, label).Width(labelW).Padding(0, 6)
				ctl()
			})
		}
		card := func(title string, fields func()) {
			ui.Column(c).Fill().Gap(8).Padding(12, 14).Radius(10).
				Background(cardBg).Children(func() {
				ui.Text(c, title).FontSize(12).Bold().TextColor(t.TextMuted)
				fields()
			})
		}

		ui.Column(c).Width(dw).MaxHeight(dh).Gap(12).Padding(20).Radius(16).
			Background(t.Surface).Shadow(0, 12, 32, 0, ui.RGBA(0x10, 0x30, 0x1F, 0.10)).Children(func() {
			ui.Text(c, "设置").Bold().FontSize(18)

			// 表单在宽裕的窗口里贴着内容(不留被拉伸的空白),在矮窗口里滚动,
			// 按钮始终可见。这里必须用 Grow(1) 而不是写死 dh-190:硬编码在
			// 1280×720 下会把「主题」「设置文件」两张卡整个裁掉,而且裁得
			// 一点痕迹都没有——用户以为功能不存在。
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Gap(12).Padding(2, 4).Children(func() {
					card("AI 接口（OpenAI 兼容）", func() {
						row("接口地址", func() {
							ui.TextInput(c, &a.settingsDraft.AI.Base).Placeholder("https://api.deepseek.com/v1").Fill()
						})
						row("模型", func() {
							ui.TextInput(c, &a.settingsDraft.AI.Model).Placeholder("deepseek-chat").Fill()
						})
						row("API Key", func() {
							ui.TextInput(c, &a.settingsDraft.AI.Key).Placeholder("sk-…（留空用环境变量 NCREAMER_AI_KEY）").Fill()
						})
						row("等待秒数", func() {
							ui.Row(c).Gap(10).Children(func() {
								ui.TextInput(c, &a.aiTimeoutText).Width(90).Placeholder("300（默认）")
								ui.Text(c, "本地慢模型可调大").FontSize(12).TextColor(t.TextMuted).Grow(1)
							})
						})
						row("并发路数", func() {
							ui.Row(c).Gap(10).Children(func() {
								ui.TextInput(c, &a.aiConcurrencyText).Width(90).Placeholder("4（默认）")
								ui.Text(c, "云端 API 常限流，填 1；本地模型填 1~2 也可能更快").FontSize(12).TextColor(t.TextMuted).Grow(1)
							})
						})
						row("默认提示词", func() {
							ui.Column(c).Gap(4).Fill().Children(func() {
								ui.TextArea(c, &a.aiPromptText).Lines(2, 4).Fill().
									Placeholder("留空用内置默认：根据文件内容起一个简短的名字")
								ui.Text(c, "新建「AI 按内容命名」步骤时用它；只写要求，不必重复\"只输出文件名\"这类约束（程序会自动带上）。").
									FontSize(11).TextColor(t.TextMuted)
							})
						})
					})
					card("文件", func() {
						row("操作日志", func() {
							ui.TextInput(c, &a.settingsDraft.Journal).Placeholder(defaultJournalPath()).Fill()
						})
					})
					card("主题", func() {
						row("外观模式", func() {
							ui.Select(c, &a.settingsDraft.Theme.Mode, []string{"跟随系统", "浅色", "深色"}).Width(160)
						})
						row("主色来源", func() {
							ui.Row(c).Gap(8).Children(func() {
								ui.Switch(c, &a.seedFollow)
								ui.Text(c, "跟随系统强调色").TextColor(t.TextMuted)
							})
						})
						row("主色", func() {
							// 平时只显示当前色块 + 更改按钮(主色不是高频操作,
							// 展开的色彩面板会把整个对话框挤满)
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								sw := ui.Box(c).Size(22, 22).Radius(6).Background(a.seedPick)
								sw.Border(1, t.Border)
								ui.Text(c, colorHex(a.seedPick)).FontSize(12).TextColor(t.TextMuted)
								label := "更改…"
								if a.pickOpen {
									label = "收起"
								}
								if ui.Button(c, label).Radius(14).Disabled(a.seedFollow).Clicked() {
									a.pickOpen = !a.pickOpen
								}
							})
						})
						if a.pickOpen && !a.seedFollow {
							ui.Row(c).Children(func() {
								p := ui.ColorPicker(c, &a.seedPick)
								p.Disabled(a.seedFollow)
							})
						}
						row("配色预览", func() {
							// 由主色自动生成的三栏"海拔"色板:不是纯灰,都带一点
							// 主色倾向;亮/暗两套各有自己的三阶层次。
							pt := themeFor(a.seedPick).panes(a.isDark())
							swatch := func(name string, col ui.Color) {
								ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
									b := ui.Box(c).Size(16, 16).Radius(4).Background(col)
									b.Border(1, t.Border)
									ui.Text(c, name).FontSize(11).TextColor(t.TextMuted)
								})
							}
							ui.Row(c).Gap(12).Wrap().Children(func() {
								swatch("左栏", pt.left)
								swatch("中栏", pt.mid)
								swatch("右栏", pt.right)
								swatch("卡片", pt.card)
							})
						})
					})
					card("设置文件", func() {
						ui.Text(c, "设置保存在程序目录 "+settingsFile+"，可导出后在其他机器导入。").FontSize(12).TextColor(t.TextMuted)
						ui.Row(c).Gap(8).Children(func() {
							if ui.Button(c, "导出设置…").Radius(16).Clicked() {
								go a.exportSettings()
							}
							if ui.Button(c, "导入设置…").Radius(16).Clicked() {
								go a.importSettings()
							}
						})
					})
				})
			})

			// one primary action, right-aligned, apart from the rest (对比)。
			// 用 outline 而不是 Divider(它画的是 Theme.Border,那是输入框
			// 的边框色,比装饰线该有的深度深一档)。
			ui.Box(c).FillWidth().Height(1).Background(a.currentTheme().panes(a.isDark()).outline)
			if a.saveErr != "" {
				ui.Text(c, a.saveErr).TextColor(t.Danger)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Radius(16).Clicked() {
					a.closeSettings(false)
				}
				if ui.PrimaryButton(c, "保存").Radius(16).Clicked() {
					a.commitAITimeout()
					a.commitAIConcurrency()
					a.commitAIPrompt()
					if a.seedFollow {
						a.settingsDraft.Theme.Seed = ""
					} else {
						a.settingsDraft.Theme.Seed = colorHex(a.seedPick)
					}
					if err := SaveSettings(a.settingsDraft); err != nil {
						// keep the dialog open: the error must be where the
						// user is looking, not on the covered window behind
						a.saveErr = "保存失败：" + err.Error()
					} else {
						a.status = "设置已保存（" + settingsPath() + ")"
						a.saveErr = ""
						a.closeSettings(true)
					}
				}
			})
		})
	})
}

func (a *app) exportSettings() {
	a.busy = true
	a.update()
	out, err := a.saveDialog(mygo.SaveDialogOptions{
		Parent:      a.win,
		Title:       "导出设置",
		DefaultPath: "ncreamer-settings.json",
	})
	a.busy = false
	if err != nil || out == "" {
		a.update()
		return
	}
	if err := ExportSettingsTo(out, a.settings); err != nil {
		a.status = "导出设置失败：" + err.Error()
	} else {
		a.status = "设置已导出到 " + out
	}
	a.update()
}

func (a *app) importSettings() {
	a.busy = true
	a.update()
	paths, err := a.openDialog(mygo.OpenDialogOptions{
		Parent: a.win,
		Title:  "导入设置",
	})
	a.busy = false
	if err != nil || len(paths) == 0 {
		a.update()
		return
	}
	s, err := ImportSettingsFrom(paths[0])
	if err != nil {
		a.status = fmt.Sprintf("导入设置失败：%v", err)
		a.update()
		return
	}
	a.settings = s
	a.syncSeedUI()
	a.ApplySettings()
	if err := os.WriteFile(settingsPath(), mustJSON(a.settings), 0o644); err == nil {
		a.status = "设置已导入并保存"
	}
	a.update()
}

func mustJSON(v any) []byte {
	b, _ := json.MarshalIndent(v, "", "  ")
	return append(b, 10) // newline
}
