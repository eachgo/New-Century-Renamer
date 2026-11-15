package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// helpDialog 讲清楚"这个程序是怎么运转的"。
//
// 它以前是五条编号的步骤,写的是操作顺序。但用户真正要建立的是"这是个
// 流水线"的心智模型,而操作顺序只是它的一个投影——五条读完,还是有人
// 问"顺序重要吗""改坏了怎么办"。现在改成两条泳道的图:一条从"增加
// 文件"起手,一条从"预设/添加步骤"起手,各自走到"执行更名",再经由
// "撤回"回到"调整步骤"——那个回环就是"它是可以反复走的"这句话本身。
//
// 五步文字已经删掉:图和它下面那几句已经把同一件事说完了,再列一遍
// 编号步骤,读者会在两套说法之间对不上号(而且那五条里的"先放文件"
// 与"先排步骤"本来就在自相矛盾地并列)。
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
				ui.Text(c, "它是这么一条线").Bold().FontSize(18)
				ui.Spacer(c)
				if ui.Button(c, "知道了").Radius(6).Clicked() {
					a.helpOpen = false
				}
			})
			ui.Scroll(c).Grow(1).TrackScroll(&a.helpScroll).Children(func() {
				ui.Column(c).FillWidth().Gap(10).Children(func() {
					// 两条完整的路,都从起手走到"撤回"再回到"调整步骤"。
					// 只画到"执行更名"就停,等于把最需要被知道的那一步
					// (改坏了能退回来)留在了图外面。
					ui.Column(c).FillWidth().Gap(6).Padding(10, 12).Radius(10).
						Background(t.Accent.Alpha(0.08)).Children(func() {
						ui.Text(c, "两条起手，最后是同一条路").FontSize(12).Bold().TextColor(t.Accent)
						flowDiagram(c, [][]string{
							{"增加文件", "调整步骤", "预览", "执行更名", "撤回", "调整步骤"},
							{"预设/添加步骤", "调整步骤", "增加文件", "预览", "执行更名", "撤回", "调整步骤"},
						}, "流程图：两条起手方式。第一条先增加文件再调整步骤；"+
							"第二条先点预设或从空白添加步骤，再增加文件。"+
							"两条都在预览处看到结果，点执行更名才真的改，改完可以撤回，然后接着调整步骤再来一轮。")
						ui.Text(c, "预览不动磁盘：右栏“原名 → 新名”随时重算，只有“执行更名”会真的改。"+
							"改完不满意，底栏“回滚上次”整批退回去，回来接着调。").
							FontSize(12).TextColor(t.TextMuted)
					})

					// 存档是另一件事,不该混进主流程:它不产生新名字,只
					// 让你把"这套排法"带走。
					ui.Text(c, "这套排法可以带走").FontSize(12).Bold().TextColor(t.TextMuted).Padding(4, 2)
					ui.Column(c).FillWidth().Gap(6).Padding(10, 12).Radius(10).
						Background(cardBg).Children(func() {
						flowDiagram(c, [][]string{
							{"排好步骤", "存成流程", "下次套用"},
							{"导出设置", "换台机器", "导入设置"},
						}, "流程图：存档。排好步骤后存成流程，下次从“自定义流程”里直接套用；"+
							"导出设置成文件，换台机器后导入设置就回来了。")
						ui.Text(c, "左栏“高级 → 自定义流程”里能套用预设，也能把当前步骤存成自己的流程。"+
							"整套设置（含主色、AI 接口、已存流程）可以“导出设置…”存成一个文件，"+
							"在另一台机器上“导入设置…”。").
							FontSize(12).TextColor(t.TextMuted)
					})

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
					ui.Text(c, "Delete 删除：焦点在文件列表时删文件，在步骤列表时删步骤（删步骤可以撤回）").
						FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, fmt.Sprintf("%s 复制步骤 · %s 把步骤上移/下移（改的是执行顺序，底栏会说移到第几位）",
						shortcutTip("Ctrl+D"), shortcutTip("Alt+↑↓"))).
						FontSize(12).TextColor(t.TextMuted)
				})
			})
		})
	})
}
