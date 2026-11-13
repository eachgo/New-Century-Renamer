package main

import (
	"github.com/egoist/mygo/ui"
)

// helpDialog is the five-step cheat sheet. It exists because the app has 32
// catalog entries, 9 template variables, NFO numbering and a CSV round-trip
// — and, before this, zero words anywhere in the interface explaining any of
// it. "Figure it out by trying" is not an onboarding plan.
//
// Deliberately a list of five lines rather than screenshots: a screenshot
// goes stale the first time a button moves, and this dialog is the thing
// users open when they are already confused.
func (a *app) helpDialog(c *ui.Context) {
	t := c.Theme()
	dt := a.currentTheme()
	cardBg := dt.cardBgFor(a.isDark())

	ui.Modal(c, &a.helpOpen, func() {
		w, h := c.Size()
		dw := float32(560)
		if w-48 < dw {
			dw = w - 48
		}
		dh := h - 48

		ui.Column(c).Width(dw).MaxHeight(dh).Gap(12).Padding(16, 18).Radius(14).
			Background(t.Background).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, "五步就够").Bold().FontSize(15)
				ui.Spacer(c)
				if ui.Button(c, "知道了").Radius(14).Clicked() {
					a.helpOpen = false
				}
			})
			ui.Scroll(c).Grow(1).TrackScroll(&a.helpScroll).Children(func() {
				ui.Column(c).FillWidth().Gap(10).Children(func() {
					steps := []struct{ n, title, body string }{
						{"1", "把文件拖进来", "拖文件夹也行——会按文件夹分组，一个文件夹一组。也可以点右栏“添加文件…”。"},
						{"2", "说清想改成什么", "中栏空着的时候有四张卡片，点一张就把步骤配好；或者从左栏“功能”里自己挑。"},
						{"3", "调参数，看右栏", "改任何一步，右栏的“原名 → 新名”都会立刻重算。这里不会动磁盘。"},
						{"4", "确认没问题再执行", "点右下角“执行更名”。只有这一步会真的改文件名，改完可以整批撤销。"},
						{"5", "改坏了就回滚", "底栏“回滚上次（N 个）”会把上一批改回去；想更保险，先“导出CSV”留个底。"},
					}
					for _, s := range steps {
						ui.Column(c).FillWidth().Gap(4).Padding(10, 12).Radius(10).
							Background(cardBg).Children(func() {
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Text(c, s.n).FontSize(13).Bold().TextColor(t.Accent).Width(14)
								ui.Text(c, s.title).FontSize(14).TextColor(t.Text).SingleLine()
							})
							ui.Text(c, s.body).FontSize(13).TextColor(t.TextMuted)
						})
					}

					ui.Text(c, "模板里能用的变量").FontSize(13).Bold().TextColor(t.TextMuted).Padding(4, 2)
					ui.Column(c).FillWidth().Gap(6).Padding(10, 12).Radius(10).
						Background(cardBg).Children(func() {
						vars := []struct{ name, desc string }{
							{"<NAME>", "原文件名（不含扩展名）"},
							{"<EXT>", "原扩展名；<.EXT> 连点一起给"},
							{"<SEQ>", "当前序号（原始数字，不补零）"},
							{"<DATE>", "文件修改日期，YYYY-MM-DD"},
							{"<META:title>", "标题；音乐读 ID3，书读 EPUB，文档读 PDF"},
							{"<META:artist>", "歌手；书里是作者"},
							{"<META:album>", "专辑"},
							{"<NFO:episode>", "从同目录 .nfo 里取的集数"},
							{"<AI:…>", "让模型按内容起名，需在设置里配接口"},
						}
						for _, v := range vars {
							ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
								ui.Text(c, v.name).FontSize(13).Width(130).TextColor(t.Text)
								ui.Text(c, v.desc).FontSize(13).TextColor(t.TextMuted).FillWidth()
							})
						}
					})
					ui.Text(c, "快捷键：Ctrl+O 添加 · Ctrl+Enter 执行 · Ctrl+Z 回滚 · Delete 删除").
						FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, "Ctrl+D 复制步骤 · Alt+↑↓ 调整顺序 · Ctrl+逗号 设置 · F1 帮助").
						FontSize(12).TextColor(t.TextMuted)
				})
			})
		})
	})
}
