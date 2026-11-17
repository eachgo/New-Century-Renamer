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
				ui.Text(c, "帮助").Bold().FontSize(18)
				ui.Spacer(c)
				// 点 ? 多半是进来查对照(变量、快捷键),不是来学怎么用,
				// 所以流程图默认收起,由这颗按钮按需展开。
				flowLabel := "不会用？看使用流程示例图"
				if a.helpShowFlows {
					flowLabel = "收起流程示例图"
				}
				if frameButton(c, flowLabel, true).Radius(6).Clicked() {
					a.helpShowFlows = !a.helpShowFlows
				}
				if frameButton(c, "知道了", true).Radius(6).Clicked() {
					a.helpOpen = false
				}
			})
			ui.Scroll(c).Grow(1).TrackScroll(&a.helpScroll).Children(func() {
				ui.Column(c).FillWidth().Gap(10).Children(func() {
					if a.helpShowFlows {
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
								"改完不满意，底栏“撤销上次”整批退回去，回来接着调。").
								FontSize(12).TextColor(t.TextMuted)
						})

						// 导出清单是另一件事,不该混进主流程:它不产生新名字,只
						// 让你把"这一批文件是哪几个"带出程序去编辑。用户不知道
						// CSV 里存了什么,所以图要和那句 sha256 一起讲。
						ui.Text(c, "导出成一张表，改完名也找得回来").FontSize(12).Bold().TextColor(t.TextMuted).Padding(4, 2)
						ui.Column(c).FillWidth().Gap(6).Padding(10, 12).Radius(10).
							Background(cardBg).Children(func() {
							flowDiagram(c, [][]string{
								{"导出清单", "在表格里改新名", "导入清单"},
							}, "流程图：导出清单。右栏「导出清单…」把这一批文件存成一张表，"+
								"在表格里改新名字那一列，再「导入清单…」读回来。")
							ui.Text(c, "每行四列：文件指纹（sha256）、所在文件夹、现在的名字、想改成的新名字。"+
								"导入时按指纹找文件——文件就算已经被别的工具改过名，也能对回自己那一行；"+
								"内容被改过，那一行会标成「有问题」，不会硬改。").
								FontSize(12).TextColor(t.TextMuted)
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
					ui.Text(c, fmt.Sprintf("快捷键：%s 添加 · %s 执行 · %s 撤销 · %s 设置 · %s 帮助",
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
