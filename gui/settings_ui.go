package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

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

		// row is the repeated unit: fixed-width label, control fills
		row := func(label string, ctl func()) {
			ui.Row(c).Gap(12).Children(func() {
				ui.Text(c, label).Width(labelW)
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

			// the form hugs its content on tall windows (no stretched
			// empty space) and scrolls on short ones; buttons stay visible
			ui.Scroll(c).MaxHeight(dh - 190).Children(func() {
				ui.Column(c).Gap(12).Padding(2, 4).Children(func() {
					card("AI 接口(OpenAI 兼容)", func() {
						row("接口地址", func() {
							ui.TextInput(c, &a.settings.AI.Base).Placeholder("https://api.deepseek.com/v1").Fill()
						})
						row("模型", func() {
							ui.TextInput(c, &a.settings.AI.Model).Placeholder("deepseek-chat").Fill()
						})
						row("API Key", func() {
							ui.TextInput(c, &a.settings.AI.Key).Placeholder("sk-…(留空用环境变量 REAMER_AI_KEY)").Fill()
						})
					})
					card("文件", func() {
						row("操作日志", func() {
							ui.TextInput(c, &a.settings.Journal).Placeholder(".reamer-journal.json").Fill()
						})
					})
					card("主题", func() {
						row("外观模式", func() {
							ui.Select(c, &a.settings.Theme.Mode, []string{"跟随系统", "浅色", "深色"}).Width(160)
						})
						row("主色来源", func() {
							ui.Row(c).Gap(8).Children(func() {
								ui.Switch(c, &a.seedFollow)
								ui.Text(c, "跟随系统强调色").TextColor(t.TextMuted)
							})
						})
						row("主色", func() {
							// disabled in place rather than hidden: the
							// layout never jumps, and the control explains
							// itself (gray = off)
							p := ui.ColorPicker(c, &a.seedPick)
							p.Disabled(a.seedFollow)
						})
					})
					card("设置文件", func() {
						ui.Text(c, "设置保存在程序目录 "+settingsFile+",可导出后在其他机器导入。").FontSize(12).TextColor(t.TextMuted)
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

			// one primary action, right-aligned, apart from the rest (对比)
			ui.Divider(c)
			if a.saveErr != "" {
				ui.Text(c, a.saveErr).TextColor(t.Danger)
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Spacer(c)
				if ui.Button(c, "取消").Radius(16).Clicked() {
					a.settingsOpen = false
				}
				if ui.PrimaryButton(c, "保存").Radius(16).Clicked() {
					if a.seedFollow {
						a.settings.Theme.Seed = ""
					} else {
						a.settings.Theme.Seed = colorHex(a.seedPick)
					}
					if err := SaveSettings(a.settings); err != nil {
						// keep the dialog open: the error must be where the
						// user is looking, not on the covered window behind
						a.saveErr = "保存失败: " + err.Error()
					} else {
						a.status = "设置已保存(" + settingsPath() + ")"
						a.saveErr = ""
						a.ApplySettings()
						a.settingsOpen = false
					}
				}
			})
		})
	})
}

func (a *app) exportSettings() {
	a.busy = true
	a.update()
	out, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      a.win,
		Title:       "导出设置",
		DefaultPath: "reamer-settings.json",
	})
	a.busy = false
	if err != nil || out == "" {
		a.update()
		return
	}
	if err := ExportSettingsTo(out, a.settings); err != nil {
		a.status = "导出设置失败: " + err.Error()
	} else {
		a.status = "设置已导出到 " + out
	}
	a.update()
}

func (a *app) importSettings() {
	a.busy = true
	a.update()
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
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
		a.status = fmt.Sprintf("导入设置失败: %v", err)
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
