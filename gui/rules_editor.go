package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// guiRule is one step in the task list. Palette entries create one with
// defaults; the detail form edits it; spec() renders it to a CLI spec for
// core.ParseRules — the same engine as the CLI.
type guiRule struct {
	opID, opLabel, opDesc string
	opUID                 int    // step identity for stable multi-selection
	opFlow                bool   // 流程控制步骤:在任务列表里以居中区分
	opGroup               string // catalog group, for the category chip
	kind                  string // label of the rule type, for spec()
	// replace
	oldStr, newStr string
	regex          bool
	// template
	tpl string
	// case (stores the label)
	caseMode string
	// insert / delete
	pos, count float64
	text       string
	// truncate
	maxLen float64
	// ext
	newExt string
	// seq
	seqStart, seqWidth, seqStep float64
	numBase, numCase, numGroup  string // 数字/字母, 大写/小写, 不分组/按扩展名/按文件夹
	numTpl                      string
	// nfo: specific NFO file path (optional)
	nfoFile string
	// insert mode: front | end | pos
	insMode string
	// filter step
	pattern string
	invert  bool
	// scope step: 文件名 | 扩展名 | 整体
	scopeSel string
	// cc step
	ccSel string // 简体→繁体 | 繁体→简体
}

// opDef is one entry of the left-hand catalog. Entries with make build a
// task step; entries with act run a special action instead (import-csv,
// settings) and never join the step list.
type opDef struct {
	id, group, label string
	desc             string // one line of what it does, shown on hover
	icon             string // flowbite icon op id
	make             func() guiRule
	act              string // non-empty: runs an action instead of a step
	flow             bool   // 流程控制(筛选/范围): not a direct rename
}

var ruleKinds = []struct{ key, label string }{
	{"pinyin", "汉字转拼音"},
	{"sanitize", "清理非法字符"},
	{"replace", "替换"},
	{"template", "模板重建"},
	{"case", "大小写"},
	{"insert", "插入文本"},
	{"delete", "删除字符"},
	{"truncate", "截断长度"},
	{"ext", "改扩展名"},
	{"seq", "追加序号"},
	{"nfo", "按 NFO 编号命名"},
	{"cc", "繁简转换"},
	{"romaji", "假名转罗马音"},
	{"filter", "筛选"},
	{"unfilter", "清除筛选"},
	{"scope", "选取范围"},
}

var caseLabels = map[string]string{"大写": "upper", "小写": "lower", "标题": "title"}

// kindByLabel/labelByKey translate between display labels and rule keys.
var kindByLabel = map[string]string{}
var labelByKey = map[string]string{}

func init() {
	for _, k := range ruleKinds {
		kindByLabel[k.label] = k.key
		labelByKey[k.key] = k.label
	}
}

func baseRule(kind string) guiRule {
	return guiRule{kind: labelByKey[kind], caseMode: "小写", pos: 1, count: 1, maxLen: 40,
		seqStart: 1, seqWidth: 2, seqStep: 1, numBase: "数字", numCase: "大写", numGroup: "不分组",
		insMode: "pos", scopeSel: "整体", ccSel: "简体→繁体"}
}

// catalog is the grouped palette of the left pane. Each entry is a full,
// named operation ("大小转换 · 全部小写"), not a type plus sub-dropdowns.
var catalog = []opDef{
	{id: "filter", icon: "filter", group: "筛选/范围", flow: true, label: "筛选文件", desc: "之后步骤只作用于匹配通配符的文件（如 *.mp3），其余原样通过", make: func() guiRule {
		return baseRule("filter")
	}},
	{id: "filter.out", icon: "filter.out", group: "筛选/范围", flow: true, label: "排除文件", desc: "排除匹配的文件，其余文件继续被后续步骤处理", make: func() guiRule {
		r := baseRule("filter")
		r.invert = true
		return r
	}},
	{id: "unfilter", icon: "unfilter", group: "筛选/范围", flow: true, label: "清除筛选", desc: "取消筛选，恢复全部文件参与后续步骤", make: func() guiRule {
		return baseRule("unfilter")
	}},
	{id: "scope.name", icon: "scope.name", group: "筛选/范围", flow: true, label: "选取文件名", desc: "之后步骤只改文件名主体，不动扩展名", make: func() guiRule {
		r := baseRule("scope")
		r.scopeSel = "文件名"
		return r
	}},
	{id: "scope.ext", icon: "scope.ext", group: "筛选/范围", flow: true, label: "选取扩展名", desc: "之后步骤只改扩展名，不动文件名主体", make: func() guiRule {
		r := baseRule("scope")
		r.scopeSel = "扩展名"
		return r
	}},
	{id: "scope.all", icon: "scope.all", group: "筛选/范围", flow: true, label: "恢复整体", desc: "之后步骤作用于完整文件名（默认）", make: func() guiRule {
		r := baseRule("scope")
		r.scopeSel = "整体"
		return r
	}},
	{id: "pinyin", icon: "pinyin", group: "转换", label: "文件名转拼音", desc: "汉字转拼音，全角字母数字自动转半角", make: func() guiRule {
		return baseRule("pinyin")
	}},
	{id: "sanitize", icon: "sanitize", group: "转换", label: "非法字符转全角", desc: "Windows 保留字符（如 : / ? *）转为全角，信息不丢失", make: func() guiRule {
		return baseRule("sanitize")
	}},
	{id: "cc.s2t", icon: "cc.s2t", group: "转换", label: "简体转繁体", desc: "用 OpenCC 词典把文件名转为繁体", make: func() guiRule {
		r := baseRule("cc")
		r.ccSel = "简体→繁体"
		return r
	}},
	{id: "cc.t2s", icon: "cc.t2s", group: "转换", label: "繁体转简体", desc: "用 OpenCC 词典把文件名转为简体", make: func() guiRule {
		r := baseRule("cc")
		r.ccSel = "繁体→简体"
		return r
	}},
	{id: "romaji", icon: "romaji", group: "转换", label: "假名转罗马音", desc: "平假名/片假名转拉丁字母（汉字保留）", make: func() guiRule {
		return baseRule("romaji")
	}},
	{id: "case.upper", icon: "case.upper", group: "大小写", label: "全部转大写", desc: "文件名所有字母转大写", make: func() guiRule {
		r := baseRule("case")
		r.caseMode = "大写"
		return r
	}},
	{id: "case.lower", icon: "case.lower", group: "大小写", label: "全部转小写", desc: "文件名所有字母转小写", make: func() guiRule {
		r := baseRule("case")
		r.caseMode = "小写"
		return r
	}},
	{id: "case.title", icon: "case.title", group: "大小写", label: "单词首字母大写", desc: "每个单词首字母大写（标题式）", make: func() guiRule {
		r := baseRule("case")
		r.caseMode = "标题"
		return r
	}},
	{id: "replace", icon: "replace", group: "替换", label: "查找替换", desc: "把查找的文字替换为指定内容", make: func() guiRule {
		return baseRule("replace")
	}},
	{id: "replace.re", icon: "replace.re", group: "替换", label: "正则替换", desc: "用正则表达式查找并替换", make: func() guiRule {
		r := baseRule("replace")
		r.regex = true
		return r
	}},
	{id: "del.re", icon: "del.re", group: "替换", label: "正则删除", desc: "删除正则表达式匹配到的部分", make: func() guiRule {
		r := baseRule("replace")
		r.regex = true
		return r
	}},
	{id: "ins.front", icon: "ins.front", group: "结构", label: "文件名前添加", desc: "在文件名主体开头插入文字", make: func() guiRule {
		r := baseRule("insert")
		r.insMode = "front"
		return r
	}},
	{id: "ins.end", icon: "ins.end", group: "结构", label: "文件名后添加", desc: "在文件名主体末尾（扩展名之前）插入文字", make: func() guiRule {
		r := baseRule("insert")
		r.insMode = "end"
		return r
	}},
	{id: "insert", icon: "insert", group: "结构", label: "指定位置插入", desc: "从第 N 个字符处插入文字", make: func() guiRule {
		return baseRule("insert")
	}},
	{id: "delete", icon: "delete", group: "结构", label: "删除字符", desc: "从第 N 个字符起删除指定数量的字符", make: func() guiRule {
		return baseRule("delete")
	}},
	{id: "truncate", icon: "truncate", group: "结构", label: "截断长度", desc: "文件名主体最多保留 N 个字符", make: func() guiRule {
		return baseRule("truncate")
	}},
	{id: "ext", icon: "ext", group: "扩展名", label: "修改扩展名", desc: "替换扩展名；填空则删除扩展名", make: func() guiRule {
		return baseRule("ext")
	}},
	{id: "ext.del", icon: "ext.del", group: "扩展名", label: "删除扩展名", desc: "去掉文件的扩展名", make: func() guiRule {
		r := baseRule("ext")
		r.newExt = ""
		return r
	}},
	{id: "seq", icon: "seq", group: "编号", label: "追加序号", desc: "文件名末尾追加编号；可分组重新计数、补零、用字母", make: func() guiRule {
		return baseRule("seq")
	}},
	{id: "seq.prefix", icon: "seq.prefix", group: "编号", label: "序号前缀", desc: "编号放文件名前面，如 01-原名", make: func() guiRule {
		r := baseRule("seq")
		r.numTpl = "<N>-<NAME><.EXT>"
		return r
	}},
	{id: "meta.title", icon: "meta.title", group: "元数据", label: "用标题命名", desc: "用元数据标题（EPUB/MP4/PDF）作为文件名", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<META:title><.EXT>"
		return r
	}},
	{id: "meta.author", icon: "meta.author", group: "元数据", label: "作者 - 标题", desc: "用元数据的作者和标题命名，如 作者 - 书名", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<META:author> - <META:title><.EXT>"
		return r
	}},
	{id: "meta.nfo", icon: "meta.nfo", group: "元数据", label: "按 NFO 编号命名", desc: "读同文件夹的 .nfo，按文件编号匹配该集的标题来命名（适合剧集/压制包）", make: func() guiRule {
		r := baseRule("nfo")
		r.tpl = ""
		return r
	}},
	{id: "date.suffix", icon: "date.suffix", group: "日期", label: "文件名加日期", desc: "文件名后附加文件修改日期", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<NAME>_<DATE><.EXT>"
		return r
	}},
	{id: "ai", icon: "ai", group: "智能", label: "AI 按内容命名", desc: "调用 AI 接口读文件内容，按内容起名（需在设置里配置）", make: func() guiRule {
		r := baseRule("template")
		// 用用户在设置里保存的默认提示词;没有就用内置默认。
		r.tpl = "<AI:" + core.DefaultAIPrompt() + "><.EXT>"
		return r
	}},
	{id: "tpl", icon: "tpl", group: "高级", label: "自定义模板", desc: "用变量模板重建文件名，可用 <NAME> <DATE> <META:…> 等", make: func() guiRule {
		return baseRule("template")
	}},
}

// stepFrom builds a task step from a catalog entry, label and all. The
// catalog click handler and the intent cards share it, so a step created
// either way looks and behaves the same in the list.
func stepFrom(id string) (guiRule, bool) {
	d, ok := opByID(id)
	if !ok {
		return guiRule{}, false
	}
	r := d.make()
	r.opID, r.opLabel, r.opDesc, r.opFlow, r.opGroup = d.id, d.label, d.desc, d.flow, d.group
	return r, true
}

// intent is one of the "我想做这件事" cards shown while the pipeline is
// still empty. The catalog answers "which rule do I need"; these answer
// "what am I trying to do", which is the question someone opening this app
// for the first time actually has. Each expands to a ready-made pipeline
// that stays fully editable — a starting point, not a mode.
type intent struct {
	label, desc string
	build       func() []guiRule
}

var intents = []intent{
	{
		label: "用文件里的标题改名",
		desc:  "读 EPUB、MP3、PDF 等文件自带的标题",
		build: func() []guiRule { return stepsFromIDs("meta.title") },
	},
	{
		label: "统一改成 序号 + 原名",
		desc:  "01-原名，编号可补零、可分组重数",
		build: func() []guiRule { return stepsFromIDs("seq.prefix") },
	},
	{
		label: "改成 作者 - 标题",
		desc:  "音乐读歌手，书读作者，接标题",
		build: func() []guiRule { return stepsFromIDs("meta.author") },
	},
	{
		label: "去掉括号和特殊字符",
		desc:  "删掉（…）【…】这类尾巴，非法字符转全角",
		build: func() []guiRule {
			steps := stepsFromIDs("del.re")
			if len(steps) == 1 {
				steps[0].oldStr = `[（(\[【《][^）)\]】》]*[）)\]】》]`
			}
			return append(steps, stepsFromIDs("sanitize")...)
		},
	},
}

func stepsFromIDs(ids ...string) []guiRule {
	out := make([]guiRule, 0, len(ids))
	for _, id := range ids {
		if r, ok := stepFrom(id); ok {
			out = append(out, r)
		}
	}
	return out
}

func opByID(id string) (opDef, bool) {
	for _, d := range catalog {
		if d.id == id {
			return d, true
		}
	}
	return opDef{}, false
}

// escArg escapes commas and backslashes in free-text rule arguments, so
// regex like \d{1,3} survives the spec string's comma-separated form
// (core's splitArgs un-escapes).
func escArg(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, ",", "\\,")
}

// spec renders the rule as a CLI spec string. Positions in the UI are
// 1-based ("从第 2 个字符"), the CLI takes 0-based.
func (r *guiRule) spec() string {
	n := func(f float64) int { return int(f) }
	key, known := kindByLabel[r.kind]
	if !known {
		return ""
	}
	switch key {
	case "pinyin":
		return "pinyin"
	case "sanitize":
		return "sanitize"
	case "replace":
		s := "replace=" + escArg(r.oldStr) + "," + escArg(r.newStr)
		if r.regex {
			s += ",regex"
		}
		return s
	case "template":
		return "template=" + r.tpl
	case "case":
		return "case=" + caseLabels[r.caseMode]
	case "insert":
		switch r.insMode {
		case "front":
			return "insert=0," + escArg(r.text)
		case "end":
			return "insert=end," + escArg(r.text)
		default:
			return fmt.Sprintf("insert=%d,%s", n(r.pos)-1, escArg(r.text))
		}
	case "delete":
		return fmt.Sprintf("delete=%d,%d", n(r.pos)-1, n(r.count))
	case "truncate":
		return fmt.Sprintf("truncate=%d", n(r.maxLen))
	case "ext":
		return "ext=" + escArg(r.newExt)
	case "cc":
		if r.ccSel == "繁体→简体" {
			return "cc=t2s"
		}
		return "cc=s2t"
	case "romaji":
		return "romaji"
	case "filter":
		spec := "filter=" + r.pattern
		if r.invert {
			spec += ",invert"
		}
		return spec
	case "unfilter":
		return "unfilter"
	case "scope":
		return "scope=" + map[string]string{"文件名": "name", "扩展名": "ext", "整体": "all"}[r.scopeSel]
	case "seq":
		base := "digit"
		if r.numBase == "字母" {
			base = "letter"
		}
		cs := "upper"
		if r.numCase == "小写" {
			cs = "lower"
		}
		group := map[string]string{"不分组": "none", "按扩展名": "ext", "按文件夹": "dir"}[r.numGroup]
		spec := fmt.Sprintf("seq=start:%d,width:%d,step:%d,base:%s,case:%s,group:%s",
			n(r.seqStart), n(r.seqWidth), n(r.seqStep), base, cs, group)
		if r.numTpl != "" {
			spec += ",tpl:" + r.numTpl
		}
		return spec
	case "nfo":
		// Build the arg as [template][,file=PATH]. Keep the template's own
		// commas escaped so they survive the comma-separated spec form.
		var arg string
		if t := strings.TrimSpace(r.tpl); t != "" {
			arg = escArg(r.tpl)
		}
		if f := strings.TrimSpace(r.nfoFile); f != "" {
			if arg != "" {
				arg += ","
			}
			arg += "file=" + escArg(f)
		}
		return "nfo=" + arg
	}
	return ""
}

// fullSpec is the step's CLI spec.
func (r *guiRule) fullSpec() string { return r.spec() }

// summary is the one-line description of the step in the task list.
func (r *guiRule) summary() string {
	key, _ := kindByLabel[r.kind]
	// 摘要只放操作名里没有的增量信息:名称已经说清的一律返回空,
	// 不再出现 "清除筛选 — 清除筛选" 这种同义重复
	switch key {
	case "pinyin", "sanitize", "unfilter", "romaji":
		return ""
	case "cc":
		return r.ccSel
	case "filter":
		if r.pattern == "" {
			return ""
		}
		if r.invert {
			return "排除 " + r.pattern
		}
		return "仅 " + r.pattern
	case "scope":
		return "作用于" + r.scopeSel
	case "replace":
		if r.regex {
			return fmt.Sprintf("正则 %q → %q", r.oldStr, r.newStr)
		}
		return fmt.Sprintf("%q → %q", r.oldStr, r.newStr)
	case "template":
		return r.tpl
	case "case":
		return r.caseMode
	case "insert":
		switch r.insMode {
		case "front":
			return fmt.Sprintf("前缀 %q", r.text)
		case "end":
			return fmt.Sprintf("后缀 %q", r.text)
		default:
			return fmt.Sprintf("第 %d 位插入 %q", int(r.pos), r.text)
		}
	case "delete":
		return fmt.Sprintf("第 %d 位起删 %d 个", int(r.pos), int(r.count))
	case "truncate":
		return fmt.Sprintf("保留 %d 字符", int(r.maxLen))
	case "ext":
		if r.newExt == "" {
			return "删除扩展名"
		}
		return "." + r.newExt
	case "seq":
		unit := "数字"
		if r.numBase == "字母" {
			unit = r.numCase + "字母"
		}
		desc := fmt.Sprintf("%s，从 %d 起，步长 %d，补 %d 位，%s", unit, int(r.seqStart), int(r.seqStep), int(r.seqWidth), r.numGroup)
		if r.numTpl != "" {
			desc += "，模板 " + r.numTpl
		}
		return desc
	case "nfo":
		base := "按 .nfo 标题命名"
		if t := strings.TrimSpace(r.tpl); t != "" {
			base = t
		}
		if f := strings.TrimSpace(r.nfoFile); f != "" {
			base += "（指定 NFO）"
		}
		return base
	}
	return ""
}

// templateVars is the list offered by the 插入变量 dropdown. Ordered by how
// often each one is actually wanted, not alphabetically: <NAME> and <EXT> in
// almost every template, <AI:…> last because it needs an endpoint configured.
var templateVars = []struct{ name, insert, desc string }{
	{"<NAME>", "<NAME>", "原文件名（不含扩展名）"},
	{"<EXT>", "<EXT>", "原扩展名"},
	{"<.EXT>", "<.EXT>", "带点的扩展名"},
	{"<SEQ>", "<SEQ>", "序号（原始数字，不补零）"},
	{"<DATE>", "<DATE>", "修改日期 YYYY-MM-DD"},
	{"<META:title>", "<META:title>", "标题"},
	{"<META:artist>", "<META:artist>", "歌手"},
	{"<META:author>", "<META:author>", "作者"},
	{"<META:album>", "<META:album>", "专辑"},
	{"<META:date>", "<META:date>", "元数据里的日期"},
	// 插入的是可用的占位指示,不是省略号:用户要在这里写自己的话
	{"<AI:…>", "<AI:按内容起名>", "让模型按内容起名"},
}

// insertVar appends a variable to the template at the caret's end. There is
// no caret API for "insert here", and appending is what people actually do
// (build "<META:artist> - <META:title>" left to right); selecting and typing
// over is still available for the rest.
func insertVar(dst *string, v string) {
	*dst += v
}

// varPicker is the 插入变量 dropdown next to a template input. A menu
// button rather than a modal: picking a variable is a two-second act and
// should not take the user away from the form they are filling in.
func varPicker(c *ui.Context, dst *string) {
	ui.MenuButton(c, "插入变量", func(m *ui.Menu) {
		for _, v := range templateVars {
			if m.Item(v.name + "  " + v.desc).Chosen() {
				insertVar(dst, v.insert)
			}
		}
	})
}

// numDecimals is how many decimal places a step of this size needs: 1 → 0,
// 0.05 → 2. It decides how a number is shown, never how it is stored.
func numDecimals(step float64) int {
	if s := strconv.FormatFloat(step, 'f', -1, 64); strings.Contains(s, ".") {
		return len(s) - strings.IndexByte(s, '.') - 1
	}
	return 0
}

// numField is one number the user types or steps, as a text field with a
// stepper beside it.
//
// It replaces ui.NumberInput, whose look we could not live with: that
// widget puts a text field and two −/+ buttons in one row, each with its
// own border and radius, and its parts are not the same height (the field
// is 33 DIP tall from its padding, the buttons 28). At 175% zoom the
// field's frame ends up visibly taller than the buttons next to it and
// runs into the select above, and its digits sit left, not centered.
// Here the field and the stepper are one height, the field's text is
// centered, and the width is ours to set.
// width is the whole control, stepper included: the stepper is a fixed
// 20 DIP (Theme.Space(5)) plus the 6 DIP gap, and the field takes the rest.
// Callers size the pair once instead of guessing the two parts separately.
func numField(c *ui.Context, value *float64, lo, hi, step, width float32) {
	const h = 32
	const stepperW, gap = float32(20), float32(6)
	fieldW := width - stepperW - gap
	if fieldW < 28 {
		fieldW = 28
	}
	format := func(v float64) string { return strconv.FormatFloat(v, 'f', numDecimals(float64(step)), 64) }
	row := ui.Row(c).Gap(gap).AlignItems(ui.Center)
	text := ui.Local(row, "text", func() string { return format(*value) })
	row.Children(func() {
		in := ui.TextInput(c, text).Width(fieldW).Height(h).TextAlign(ui.Center)
		if in.Changed() {
			if v, err := strconv.ParseFloat(strings.TrimSpace(*text), 64); err == nil {
				*value = clamp(v, float64(lo), float64(hi))
			}
		}
		if in.Shortcut(0, ui.KeyUp) {
			*value = clamp(*value+float64(step), float64(lo), float64(hi))
			*text = format(*value)
		}
		if in.Shortcut(0, ui.KeyDown) {
			*value = clamp(*value-float64(step), float64(lo), float64(hi))
			*text = format(*value)
		}
		if !in.Focused() {
			*text = format(*value)
		}
		ui.Stepper(c, value, float64(lo), float64(hi), float64(step)).Height(h)
	})
}

// paramsEditor builds the detail form of one step below the task list.
//
// Every parameter is one row: a fixed-width label and the controls it names.
// The rows are spaced, not packed — before, they were direct children of the
// scroll box, so one row's control border sat exactly on the next one's and
// the form read as a single striped block instead of a list of fields.
//
// w is the width the form really has. The divider between the panes can be
// dragged almost shut, and at that point a 96 DIP label plus a 160 DIP select
// no longer fit side by side: the control used to run off the pane's edge.
// Below the breakpoint the label moves above its controls, which then take
// the full width, so nothing is cut off at any pane width.
func (r *guiRule) paramsEditor(c *ui.Context, t *ui.Theme, w float32) {
	const labelW, labelGap = float32(96), float32(8)
	// Side by side needs the label, the gap, and room for a usable control.
	stacked := w < labelW+labelGap+150
	avail := w - labelW - labelGap
	if stacked {
		avail = w
	}
	// fit caps a control at the space the row really has. A select asked
	// for 180 DIP in a 96 DIP pane used to keep 180 and run off the card;
	// now it takes what is there and the pane never clips a control.
	fit := func(req float32) float32 {
		if req > avail {
			return max(avail, 40)
		}
		return req
	}
	pline := func(label string, ctl func()) {
		if stacked {
			// A column stretches its children, so the controls below the
			// label get the whole width and the wide ones (a FillWidth
			// input) are as usable here as in the side-by-side layout.
			// Wrap lets a row of several controls (起始 / 步长) use two
			// lines instead of overflowing.
			ui.Column(c).Gap(4).Children(func() {
				ui.Text(c, label).FontSize(13).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(ctl)
			})
			return
		}
		ui.Row(c).Gap(labelGap).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(13).TextColor(t.TextMuted).Width(labelW)
			ctl()
		})
	}
	ui.Column(c).FillWidth().Gap(10).Children(func() { r.paramsFields(c, t, pline, fit) })
}

// paramsFields is the per-kind body of the form: one pline call per
// parameter. Split out so paramsEditor stays about layout.
func (r *guiRule) paramsFields(c *ui.Context, t *ui.Theme, pline func(label string, ctl func()), fit func(float32) float32) {
	key, _ := kindByLabel[r.kind]
	switch key {
	case "pinyin", "sanitize", "unfilter", "romaji":
		pline("参数", func() { ui.Text(c, "此步骤无参数").TextColor(t.TextMuted) })
	case "filter":
		pline("匹配通配符", func() { ui.TextInput(c, &r.pattern).Placeholder("如 *.mp3").FillWidth() })
		pline("排除匹配项", func() { ui.Switch(c, &r.invert) })
	case "scope":
		pline("选取范围", func() { ui.Select(c, &r.scopeSel, []string{"文件名", "扩展名", "整体"}).Width(fit(160)) })
	case "replace":
		pline("查找", func() {
			ui.Column(c).Gap(4).Children(func() {
				ui.TextInput(c, &r.oldStr).Placeholder("查找").FillWidth()
				// 空查找串不是"没填",它是一条会执行成功的规则:替换会把
				// 替换文本插到每个字符之间,正则在每个位置匹配一次。预览
				// 会显示每个文件名被切碎,执行出来就是一堆谁也不认识的
				// 名字。core 里已经拒绝,这里就地说明,省得用户对着一条
				// 报错的步骤猜是哪一格没填。
				if r.oldStr == "" {
					ui.Text(c, "还没填查找内容——空字符串会匹配每个字符之间，这一步现在不会执行。").
						FontSize(12).TextColor(t.Danger).SingleLine()
				}
			})
		})
		pline("替换为", func() { ui.TextInput(c, &r.newStr).Placeholder("替换为").FillWidth() })
		pline("正则", func() { ui.Switch(c, &r.regex) })
	case "template":
		pline("模板", func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &r.tpl).Placeholder("<META:author> - <META:title><.EXT>").FillWidth()
				varPicker(c, &r.tpl)
			})
		})
	case "case":
		pline("转换方式", func() { ui.Select(c, &r.caseMode, []string{"大写", "小写", "标题"}).Width(fit(160)) })
	case "insert":
		pline("插入位置", func() {
			ui.Select(c, &r.insMode, []string{"文件名前", "文件名后", "指定位置"}).Width(fit(160))
		})
		if r.insMode == "指定位置" {
			pline("第几个字符", func() { numField(c, &r.pos, 1, 9999, 1, fit(90)) })
		}
		pline("插入文本", func() { ui.TextInput(c, &r.text).Placeholder("插入文本").FillWidth() })
	case "delete":
		pline("从第几个字符", func() { numField(c, &r.pos, 1, 9999, 1, fit(90)) })
		pline("删除数量", func() { numField(c, &r.count, 1, 9999, 1, fit(90)) })
	case "truncate":
		pline("保留长度", func() { numField(c, &r.maxLen, 1, 9999, 1, fit(90)) })
	case "ext":
		pline("新扩展名", func() { ui.TextInput(c, &r.newExt).Placeholder("空=删除扩展名").FillWidth() })
	case "seq":
		pline("编号类型", func() { ui.Select(c, &r.numBase, []string{"数字", "字母"}).Width(fit(120)) })
		if r.numBase == "字母" {
			pline("大小写", func() { ui.Select(c, &r.numCase, []string{"大写", "小写"}).Width(fit(120)) })
		} else {
			pline("不足位补齐", func() { numField(c, &r.seqWidth, 1, 9, 1, fit(90)) })
		}
		pline("分组方式", func() {
			ui.Select(c, &r.numGroup, []string{"不分组", "按扩展名", "按文件夹"}).Width(fit(180))
		})
		pline("起始 / 步长", func() {
			numField(c, &r.seqStart, -9999, 999999, 1, fit(70))
			ui.Text(c, "/").TextColor(t.TextMuted)
			numField(c, &r.seqStep, -999, 999, 1, fit(70))
		})
		pline("编号模板", func() { ui.TextInput(c, &r.numTpl).Placeholder("如 第<N>话_<NAME><.EXT>，空=追加").FillWidth() })
	case "nfo":
		pline("命名模板", func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &r.tpl).Placeholder("空=<剧名 - S01E01 - 标题>").FillWidth()
				varPicker(c, &r.tpl)
			})
		})
		pline("NFO 文件", func() {
			ui.TextInput(c, &r.nfoFile).Placeholder("留空=同文件夹的 .nfo；或填某个 .nfo 的完整路径").FillWidth()
		})
	}
}
