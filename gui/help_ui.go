package main

import (
	"fmt"

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

		dialogPanel(c, dw, dh).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, "五步就够").Bold().FontSize(18)
				ui.Spacer(c)
				if ui.Button(c, "知道了").Radius(6).Clicked() {
					a.helpOpen = false
				}
			})
			ui.Scroll(c).Grow(1).TrackScroll(&a.helpScroll).Children(func() {
				ui.Column(c).FillWidth().Gap(10).Children(func() {
					// 五步是操作顺序,但用户要建立的是"这是个流水线"这个
					// 心智模型——它是空间性的,写成句子读者得在脑子里搭一遍。
					// 所以先画一张图:两条入口各自出发,到"预览"之前汇合,
					// 汇合点就是那句话的重点(前两步谁先谁后都行)。
					ui.Column(c).FillWidth().Gap(6).Padding(10, 12).Radius(10).
						Background(t.Accent.Alpha(0.08)).Children(func() {
						ui.Text(c, "它是这么一条线").FontSize(12).Bold().TextColor(t.Accent)
						flowDiagram(c)
						ui.Text(c, "两条入口谁先谁后都行，到“预览”就并成一条路。"+
							"预览不动磁盘，只有“执行更名”会真的改。").
							FontSize(12).TextColor(t.TextMuted)
					})
					steps := []struct{ n, title, body string }{
						{"1", "先放文件，或者先排步骤", "两条路都行，顺序不影响结果：右栏可以拖文件/文件夹进来，也可以点“添加文件…”；中栏空着的时候有四张卡片，点一张就把步骤配好，等文件进来立刻套上去。"},
						{"2", "说清想改成什么", "中栏的卡片是现成的四种常见改法；要别的效果，就从左栏“功能”里自己挑，一项一项往上叠。"},
						{"3", "调参数，看右栏", "改任何一步，右栏的“原名 → 新名”都会立刻重算。这里不会动磁盘。"},
						{"4", "确认没问题再执行", "点右下角“执行更名”。只有这一步会真的改文件名，改完可以整批撤销。"},
						{"5", "改坏了就回滚", "底栏“回滚上次（N 个）”会把上一批改回去；想更保险，先“导出CSV”留个底。"},
					}
					for _, s := range steps {
						ui.Column(c).FillWidth().Gap(4).Padding(10, 12).Radius(10).
							Background(cardBg).Children(func() {
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Text(c, s.n).FontSize(12).Bold().TextColor(t.Accent).Width(14)
								ui.Text(c, s.title).FontSize(14).TextColor(t.Text).SingleLine()
							})
							ui.Text(c, s.body).FontSize(12).TextColor(t.TextMuted)
						})
					}

					ui.Text(c, "模板里能用的变量").FontSize(12).Bold().TextColor(t.TextMuted).Padding(4, 2)
					ui.Column(c).FillWidth().Gap(6).Padding(10, 12).Radius(10).
						Background(cardBg).Children(func() {
						// 与 README 的「变量」表逐条对齐:两处不一致时,
						// 用户不知道该信哪一个。帮助页在程序里,是查得最快的
						// 那一份,所以它不能比 README 少。
						vars := []struct{ name, desc string }{
							{"<NAME>", "原文件名（不含扩展名）"},
							{"<EXT>", "原扩展名"},
							{"<.EXT>", "原扩展名，连点一起给"},
							{"<DIR>", "所在目录"},
							{"<CRC32>", "文件内容 CRC32"},
							{"<SEQ>", "当前序号（原始数字，不补零）"},
							{"<DATE>", "文件修改日期，YYYY-MM-DD"},
							{"<Pinyin:NAME>", "括号里那个变量的内容转拼音"},
							{"<META:title>", "标题；音乐读 ID3，书读 EPUB，文档读 PDF"},
							{"<META:artist>", "歌手；书里是作者"},
							{"<META:album>", "专辑"},
							{"<NFO:episode>", "从同目录 .nfo 里取的集数"},
							{"<NFO:title>", "同一集在 .nfo 里的标题"},
							{"<NFOName>", "组合好的「剧名 - S01E01 - 标题」"},
							{"<AI:…>", "让模型按内容起名，需在设置里配接口"},
						}
						for _, v := range vars {
							ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
								ui.Text(c, v.name).FontSize(12).Width(130).TextColor(t.Text)
								ui.Text(c, v.desc).FontSize(12).TextColor(t.TextMuted).FillWidth()
							})
						}
					})
					// 键盘这一节要把"焦点在哪"说清楚,因为同一个 Delete 在两处
					// 管不同的事。只列按键不列前提,等于让用户猜。
					//
					// 组合键一律走 shortcutTip:这里以前把 "Ctrl+O" 写死,
					// 而 macOS 上真正的修饰键是 ⌘——同一份说明在苹果机器上
					// 教的是一个按不出来的键。
					ui.Text(c, fmt.Sprintf("快捷键：%s 添加 · %s 执行 · %s 回滚 · %s 设置 · %s 帮助",
						shortcutTip("Ctrl+O"), shortcutTip("Ctrl+Enter"), shortcutTip("Ctrl+Z"),
						shortcutTip("Ctrl+,"), "F1")).
						FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, "Tab / Shift+Tab 换区域，方向键在区域内移动（左栏目录是一整块，进去后用上下键挑条目）").
						FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, "Delete 删除：焦点在文件列表时删文件，在步骤列表时删步骤（都要按两次确认）").
						FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, fmt.Sprintf("%s 复制步骤 · %s 把步骤上移/下移（改的是执行顺序，底栏会说移到第几位）",
						shortcutTip("Ctrl+D"), shortcutTip("Alt+↑↓"))).
						FontSize(12).TextColor(t.TextMuted)
				})
			})
		})
	})
}
