package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
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
	{id: "filter", icon: "filter", group: "筛选/范围", flow: true, label: "筛选文件", desc: "之后步骤只作用于匹配通配符的文件(如 *.mp3),其余原样通过", make: func() guiRule {
		return baseRule("filter")
	}},
	{id: "filter.out", icon: "filter.out", group: "筛选/范围", flow: true, label: "排除文件", desc: "排除匹配的文件,其余文件继续被后续步骤处理", make: func() guiRule {
		r := baseRule("filter")
		r.invert = true
		return r
	}},
	{id: "unfilter", icon: "unfilter", group: "筛选/范围", flow: true, label: "清除筛选", desc: "取消筛选,恢复全部文件参与后续步骤", make: func() guiRule {
		return baseRule("unfilter")
	}},
	{id: "scope.name", icon: "scope.name", group: "筛选/范围", flow: true, label: "选取文件名", desc: "之后步骤只改文件名主体,不动扩展名", make: func() guiRule {
		r := baseRule("scope")
		r.scopeSel = "文件名"
		return r
	}},
	{id: "scope.ext", icon: "scope.ext", group: "筛选/范围", flow: true, label: "选取扩展名", desc: "之后步骤只改扩展名,不动文件名主体", make: func() guiRule {
		r := baseRule("scope")
		r.scopeSel = "扩展名"
		return r
	}},
	{id: "scope.all", icon: "scope.all", group: "筛选/范围", flow: true, label: "恢复整体", desc: "之后步骤作用于完整文件名(默认)", make: func() guiRule {
		r := baseRule("scope")
		r.scopeSel = "整体"
		return r
	}},
	{id: "pinyin", icon: "pinyin", group: "转换", label: "文件名转拼音", desc: "汉字转拼音,全角字母数字自动转半角", make: func() guiRule {
		return baseRule("pinyin")
	}},
	{id: "sanitize", icon: "sanitize", group: "转换", label: "非法字符转全角", desc: "Windows 保留字符(如 : / ? *)转为全角,信息不丢失", make: func() guiRule {
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
	{id: "romaji", icon: "romaji", group: "转换", label: "假名转罗马音", desc: "平假名/片假名转拉丁字母(汉字保留)", make: func() guiRule {
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
	{id: "case.title", icon: "case.title", group: "大小写", label: "单词首字母大写", desc: "每个单词首字母大写(标题式)", make: func() guiRule {
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
	{id: "ins.end", icon: "ins.end", group: "结构", label: "文件名后添加", desc: "在文件名主体末尾(扩展名之前)插入文字", make: func() guiRule {
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
	{id: "ext", icon: "ext", group: "扩展名", label: "修改扩展名", desc: "替换扩展名;填空则删除扩展名", make: func() guiRule {
		return baseRule("ext")
	}},
	{id: "ext.del", icon: "ext.del", group: "扩展名", label: "删除扩展名", desc: "去掉文件的扩展名", make: func() guiRule {
		r := baseRule("ext")
		r.newExt = ""
		return r
	}},
	{id: "seq", icon: "seq", group: "编号", label: "追加序号", desc: "文件名末尾追加编号;可分组重新计数、补零、用字母", make: func() guiRule {
		return baseRule("seq")
	}},
	{id: "seq.prefix", icon: "seq.prefix", group: "编号", label: "序号前缀", desc: "编号放文件名前面,如 01-原名", make: func() guiRule {
		r := baseRule("seq")
		r.numTpl = "<N>-<NAME><.EXT>"
		return r
	}},
	{id: "meta.title", icon: "meta.title", group: "元数据", label: "用标题命名", desc: "用元数据标题(EPUB/MP4/PDF)作为文件名", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<META:title><.EXT>"
		return r
	}},
	{id: "meta.author", icon: "meta.author", group: "元数据", label: "作者 - 标题", desc: "用元数据的作者和标题命名,如 作者 - 书名", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<META:author> - <META:title><.EXT>"
		return r
	}},
	{id: "date.suffix", icon: "date.suffix", group: "日期", label: "文件名加日期", desc: "文件名后附加文件修改日期", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<NAME>_<DATE><.EXT>"
		return r
	}},
	{id: "ai", icon: "ai", group: "智能", label: "AI 按内容命名", desc: "调用 AI 接口读文件内容,按内容起名(需在设置里配置)", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<AI:根据文件内容起一个简短的名字><.EXT>"
		return r
	}},
	{id: "tpl", icon: "tpl", group: "高级", label: "自定义模板", desc: "用变量模板重建文件名,可用 <NAME> <DATE> <META:…> 等", make: func() guiRule {
		return baseRule("template")
	}},
	{id: "act.import-csv", icon: "act.import-csv", group: "高级", label: "导入编辑好的CSV改名", desc: "按导出的 CSV 列表(new_name 列)批量改名;点击立即执行,不加入步骤", act: "import-csv"},
	{id: "act.settings", icon: "act.settings", group: "高级", label: "设置", desc: "AI 接口、主题颜色、日志位置;点击打开设置窗口,不加入步骤", act: "settings"},
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
	}
	return ""
}

// fullSpec is the step's CLI spec.
func (r *guiRule) fullSpec() string { return r.spec() }

// summary is the one-line description of the step in the task list.
func (r *guiRule) summary() string {
	key, _ := kindByLabel[r.kind]
	switch key {
	case "pinyin", "sanitize", "unfilter", "romaji":
		return r.opLabel
	case "cc":
		return r.ccSel
	case "filter":
		if r.invert {
			return "排除匹配 " + r.pattern
		}
		return "只保留匹配 " + r.pattern
	case "scope":
		return "后续步骤作用于:" + r.scopeSel
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
		desc := fmt.Sprintf("%s,从 %d 起,步长 %d,补 %d 位,%s", unit, int(r.seqStart), int(r.seqStep), int(r.seqWidth), r.numGroup)
		if r.numTpl != "" {
			desc += ",模板 " + r.numTpl
		}
		return desc
	}
	return ""
}

// paramsEditor builds the detail form of one step below the task list.
// One parameter per line (fixed-width label + control): the middle pane
// can be narrow, so nothing wraps into mismatched pairs.
func (r *guiRule) paramsEditor(c *ui.Context, t *ui.Theme) {
	pline := func(label string, ctl func()) {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).Width(96)
			ctl()
		})
	}
	key, _ := kindByLabel[r.kind]
	switch key {
	case "pinyin", "sanitize", "unfilter", "romaji":
		pline("参数", func() { ui.Text(c, "此步骤无参数").TextColor(t.TextMuted) })
	case "filter":
		pline("匹配通配符", func() { ui.TextInput(c, &r.pattern).Placeholder("如 *.mp3").FillWidth() })
		pline("排除匹配项", func() { ui.Switch(c, &r.invert) })
	case "scope":
		pline("选取范围", func() { ui.Select(c, &r.scopeSel, []string{"文件名", "扩展名", "整体"}).Width(160) })
	case "replace":
		pline("查找", func() { ui.TextInput(c, &r.oldStr).Placeholder("查找").FillWidth() })
		pline("替换为", func() { ui.TextInput(c, &r.newStr).Placeholder("替换为").FillWidth() })
		pline("正则", func() { ui.Switch(c, &r.regex) })
	case "template":
		pline("模板", func() { ui.TextInput(c, &r.tpl).Placeholder("<META:author> - <META:title><.EXT>").FillWidth() })
	case "case":
		pline("转换方式", func() { ui.Select(c, &r.caseMode, []string{"大写", "小写", "标题"}).Width(160) })
	case "insert":
		pline("插入位置", func() { ui.Select(c, &r.insMode, []string{"文件名前", "文件名后", "指定位置"}).Width(160) })
		if r.insMode == "指定位置" {
			pline("第几个字符", func() { ui.NumberInput(c, &r.pos, 1, 9999, 1).Width(90) })
		}
		pline("插入文本", func() { ui.TextInput(c, &r.text).Placeholder("插入文本").FillWidth() })
	case "delete":
		pline("从第几个字符", func() { ui.NumberInput(c, &r.pos, 1, 9999, 1).Width(90) })
		pline("删除数量", func() { ui.NumberInput(c, &r.count, 1, 9999, 1).Width(90) })
	case "truncate":
		pline("保留长度", func() { ui.NumberInput(c, &r.maxLen, 1, 9999, 1).Width(90) })
	case "ext":
		pline("新扩展名", func() { ui.TextInput(c, &r.newExt).Placeholder("空=删除扩展名").FillWidth() })
	case "seq":
		pline("编号类型", func() { ui.Select(c, &r.numBase, []string{"数字", "字母"}).Width(120) })
		if r.numBase == "字母" {
			pline("大小写", func() { ui.Select(c, &r.numCase, []string{"大写", "小写"}).Width(120) })
		} else {
			pline("不足位补齐", func() { ui.NumberInput(c, &r.seqWidth, 1, 9, 1).Width(90) })
		}
		pline("分组方式", func() { ui.Select(c, &r.numGroup, []string{"不分组", "按扩展名", "按文件夹"}).Width(180) })
		pline("起始 / 步长", func() {
			ui.NumberInput(c, &r.seqStart, -9999, 999999, 1).Width(70)
			ui.Text(c, "/").TextColor(t.TextMuted)
			ui.NumberInput(c, &r.seqStep, -999, 999, 1).Width(70)
		})
		pline("编号模板", func() { ui.TextInput(c, &r.numTpl).Placeholder("如 第<N>话_<NAME><.EXT>,空=追加").FillWidth() })
	}
}
