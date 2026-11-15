package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// 流程(flow)= 一串排好的步骤,能存下来、能再套上去。
//
// 为什么要有这个:新手引导原来只在"任务步骤为空"时出现——四张意图卡
// 摆在空的中栏里。可用户排到一半想换个做法时,那四张卡已经不在屏幕上了,
// 左栏三十二条规则又答不上"我想做的是歌手-歌名"这种问题。于是流程把
// 意图卡从"空态的一部分"变成"随时能打开的入口":同一份预设,既能当
// 第一次的起点,也能当第五次的中途换道。
//
// 存的是参数而不是 spec 字符串:spec 是给 core 引擎的,读回来只能得到
// 一条规则,丢掉"这一步叫什么、属于哪个分组、界面怎么展开"。按 opID +
// 参数字典存,重建时先 stepFrom(opID) 拿到带默认值的完整步骤,再逐个
// 覆写用户改过的字段——这样以后给某个 opDef 加参数,老流程仍然能打开。
type flow struct {
	Name string `json:"name"`
	// Steps 按执行顺序;每项是 opID + 该步骤被改过的参数。
	Steps []flowStep `json:"steps"`
	// Builtin 标记内置预设(不可删、不可改名)。内置的不写进设置文件,
	// 每次从 intents 现算,这样升级后预设跟着更新,不会留一份旧快照。
	Builtin bool `json:"-"`
}

type flowStep struct {
	ID   string            `json:"id"`
	Args map[string]string `json:"args,omitempty"`
}

// flowFromRules 把当前步骤列表抓成一条流程。参数只存"和默认值不同"的,
// 让存下来的东西尽量小、也尽量能读。
func flowFromRules(name string, rules []guiRule) flow {
	f := flow{Name: name}
	for i := range rules {
		r := &rules[i]
		fs := flowStep{ID: r.opID, Args: map[string]string{}}
		if d, ok := opByID(r.opID); ok {
			def := d.make()
			putIfDiff(fs.Args, "oldStr", r.oldStr, def.oldStr)
			putIfDiff(fs.Args, "newStr", r.newStr, def.newStr)
			putIfDiff(fs.Args, "tpl", r.tpl, def.tpl)
			putIfDiff(fs.Args, "caseMode", r.caseMode, def.caseMode)
			putIfDiff(fs.Args, "text", r.text, def.text)
			putIfDiff(fs.Args, "newExt", r.newExt, def.newExt)
			putIfDiff(fs.Args, "numBase", r.numBase, def.numBase)
			putIfDiff(fs.Args, "numCase", r.numCase, def.numCase)
			putIfDiff(fs.Args, "numGroup", r.numGroup, def.numGroup)
			putIfDiff(fs.Args, "numTpl", r.numTpl, def.numTpl)
			putIfDiff(fs.Args, "nfoFile", r.nfoFile, def.nfoFile)
			putIfDiff(fs.Args, "insMode", r.insMode, def.insMode)
			putIfDiff(fs.Args, "pattern", r.pattern, def.pattern)
			putIfDiff(fs.Args, "scopeSel", r.scopeSel, def.scopeSel)
			putIfDiff(fs.Args, "ccSel", r.ccSel, def.ccSel)
			putIfNum(fs.Args, "pos", r.pos, def.pos)
			putIfNum(fs.Args, "count", r.count, def.count)
			putIfNum(fs.Args, "maxLen", r.maxLen, def.maxLen)
			putIfNum(fs.Args, "seqStart", r.seqStart, def.seqStart)
			putIfNum(fs.Args, "seqWidth", r.seqWidth, def.seqWidth)
			putIfNum(fs.Args, "seqStep", r.seqStep, def.seqStep)
			if r.regex != def.regex {
				fs.Args["regex"] = fmt.Sprint(r.regex)
			}
			if r.invert != def.invert {
				fs.Args["invert"] = fmt.Sprint(r.invert)
			}
			if r.oldTouched != def.oldTouched {
				fs.Args["oldTouched"] = fmt.Sprint(r.oldTouched)
			}
		}
		if len(fs.Args) == 0 {
			fs.Args = nil
		}
		f.Steps = append(f.Steps, fs)
	}
	return f
}

func putIfDiff(m map[string]string, key, got, def string) {
	if got != def {
		m[key] = got
	}
}

func putIfNum(m map[string]string, key string, got, def float64) {
	if got != def {
		m[key] = fmt.Sprint(got)
	}
}

// rules 把流程还原成步骤列表。找不到的 opID 直接跳过而不是报错:流程是
// 用户存下来的,程序升级后某个条目改名不该让整条流程作废。
func (f flow) rules() []guiRule {
	out := make([]guiRule, 0, len(f.Steps))
	for _, fs := range f.Steps {
		r, ok := stepFrom(fs.ID)
		if !ok {
			continue
		}
		for k, v := range fs.Args {
			r.setArg(k, v)
		}
		out = append(out, r)
	}
	return out
}

// setArg 按名字写一个参数。用字典而不是给每个字段写一遍 switch:新增
// 参数时只要在 flowFromRules 里加一行,这里不用动。
func (r *guiRule) setArg(k, v string) {
	num := func() float64 {
		var f float64
		fmt.Sscan(v, &f)
		return f
	}
	boolean := func() bool { return v == "true" }
	switch k {
	case "oldStr":
		r.oldStr = v
	case "newStr":
		r.newStr = v
	case "tpl":
		r.tpl = v
	case "caseMode":
		r.caseMode = v
	case "text":
		r.text = v
	case "newExt":
		r.newExt = v
	case "numBase":
		r.numBase = v
	case "numCase":
		r.numCase = v
	case "numGroup":
		r.numGroup = v
	case "numTpl":
		r.numTpl = v
	case "nfoFile":
		r.nfoFile = v
	case "insMode":
		r.insMode = v
	case "pattern":
		r.pattern = v
	case "scopeSel":
		r.scopeSel = v
	case "ccSel":
		r.ccSel = v
	case "pos":
		r.pos = num()
	case "count":
		r.count = num()
	case "maxLen":
		r.maxLen = num()
	case "seqStart":
		r.seqStart = num()
	case "seqWidth":
		r.seqWidth = num()
	case "seqStep":
		r.seqStep = num()
	case "regex":
		r.regex = boolean()
	case "invert":
		r.invert = boolean()
	case "oldTouched":
		r.oldTouched = boolean()
	}
}

// builtinFlows 把内置意图转成流程。它们不进设置文件,每次现算。
func builtinFlows() []flow {
	out := make([]flow, 0, len(intents))
	for _, in := range intents {
		f := flowFromRules(in.label, in.build())
		f.Builtin = true
		out = append(out, f)
	}
	return out
}

// allFlows 是"内置预设 + 用户存的",用户的在前(自己存的更常用)。
func (a *app) allFlows() []flow {
	out := make([]flow, 0, len(a.settings.Flows)+len(intents))
	out = append(out, a.settings.Flows...)
	out = append(out, builtinFlows()...)
	return out
}

// saveFlow 把当前步骤存成一条命名流程。同名的直接覆盖——用户点了
// "保存",期望的是"这个位置就是我现在这样",不是攒出十个同名条目。
func (a *app) saveFlow(name string) {
	name = strings.TrimSpace(name)
	if name == "" || len(a.steps) == 0 {
		return
	}
	f := flowFromRules(name, a.steps)
	for i := range a.settings.Flows {
		if a.settings.Flows[i].Name == name {
			a.settings.Flows[i] = f
			return
		}
	}
	a.settings.Flows = append(a.settings.Flows, f)
}

// deleteFlow 删一条用户流程。内置的不在这里(它们不在 settings 里)。
func (a *app) deleteFlow(name string) {
	out := a.settings.Flows[:0]
	for _, f := range a.settings.Flows {
		if f.Name != name {
			out = append(out, f)
		}
	}
	a.settings.Flows = out
}

// applyFlow 把流程的步骤接到现有列表后面(替换还是追加由 replace 决定)。
func (a *app) applyFlow(f flow, replace bool) {
	steps := f.rules()
	if len(steps) == 0 {
		return
	}
	for i := range steps {
		a.stepUID++
		steps[i].opUID = a.stepUID
	}
	if replace {
		a.steps = steps
		a.stepSel = 0
	} else {
		a.stepSel = len(a.steps)
		a.steps = append(a.steps, steps...)
	}
	a.flowOpen = false
	a.update()
}

// flowDialog 是流程入口:随时能打开,列出用户存的与内置的预设。
//
// 它取代了原来只在空态出现的四张卡。卡片仍然在(空态时它们是最好的
// 起点),但"想换个做法"这件事不再依赖"列表恰好是空的"这个前提。
func (a *app) flowDialog(c *ui.Context) {
	t := c.Theme()
	dt := a.currentTheme()
	cardBg := dt.cardBgFor(a.isDark())

	ui.Modal(c, &a.flowOpen, func() {
		w, h := c.Size()
		dw := float32(620)
		if w-48 < dw {
			dw = w - 48
		}
		dh := h - 48

		dialogPanel(c, dw, dh).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, "流程").Bold().FontSize(18)
				ui.Spacer(c)
				if ui.Button(c, "关闭").Radius(6).Clicked() {
					a.flowOpen = false
				}
			})
			ui.Text(c, "一串排好的步骤。套上去之后每一步都还能改——它是起点，不是模式。").
				FontSize(12).TextColor(t.TextMuted)
			ui.Scroll(c).Grow(1).TrackScroll(&a.flowScroll).Children(func() {
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					// 当前步骤能存成流程:这才是"用户可保存"的部分。
					if len(a.steps) > 0 {
						ui.Column(c).FillWidth().Gap(6).Padding(10, 12).Radius(10).
							Background(t.Accent.Alpha(0.08)).Children(func() {
							ui.Text(c, "把现在的步骤存成流程").FontSize(12).Bold().TextColor(t.Accent)
							ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.TextInput(c, &a.flowName).Placeholder("给这条流程起个名字").
									Grow(1).Radius(6)
								save := ui.Button(c, "保存").Radius(6)
								if strings.TrimSpace(a.flowName) == "" {
									save = save.Disabled(true)
								}
								if save.Clicked() {
									a.saveFlow(a.flowName)
									a.flowName = ""
									if err := SaveSettings(a.settings); err != nil {
										a.status = err.Error()
									} else {
										a.status = "流程已保存"
									}
								}
							})
						})
					}
					rows := a.allFlows()
					if len(rows) == 0 {
						ui.Text(c, "还没有流程。排好步骤之后，上面那个框可以把它存下来。").
							FontSize(12).TextColor(t.TextMuted)
						return
					}
					for _, f := range rows {
						row := ui.Column(c).FillWidth().Gap(4).Padding(8, 12).Radius(10).
							Background(cardBg).Border(1, t.Border)
						if row.Hovered() {
							row = row.Background(t.SurfaceHover).Border(1, t.Accent)
						}
						// 删除按钮必须留在行内右侧,不能挂到行外:挂在行外会
						// 变成一整条独立横幅,读起来像"这张卡片下面还有一张
						// 卡片",而它其实只是这一行的次要动作。
						del := false
						row.Children(func() {
							ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Text(c, f.Name).FontSize(14).TextColor(t.Text).Grow(1).
									SingleLine().Ellipsis("…")
								if f.Builtin {
									ui.Text(c, "预设").FontSize(12).TextColor(t.TextMuted).Shrink(0)
								} else if ui.Button(c, "删除").Radius(6).Shrink(0).Clicked() {
									// 内置的删了下次还会回来,所以它们没有这个按钮。
									del = true
								}
							})
							ui.Text(c, flowSummary(f)).FontSize(12).TextColor(t.TextMuted).
								SingleLine().Ellipsis("…")
						})
						if del {
							a.deleteFlow(f.Name)
							if err := SaveSettings(a.settings); err != nil {
								a.status = err.Error()
							}
							return
						}
						if row.Clicked() {
							a.applyFlow(f, true)
						}
					}
				})
			})
			ui.Text(c, "点一条就套用，会替换当前步骤；想保留现在这版，先存成流程。").
				FontSize(12).TextColor(t.TextMuted)
		})
	})
}

// flowSummary 是流程的一行摘要:步骤名按顺序连起来,太长就截断。
func flowSummary(f flow) string {
	names := make([]string, 0, len(f.Steps))
	for _, s := range f.Steps {
		if d, ok := opByID(s.ID); ok {
			names = append(names, d.label)
		}
	}
	if len(names) == 0 {
		return "（空流程）"
	}
	return strings.Join(names, " → ")
}
