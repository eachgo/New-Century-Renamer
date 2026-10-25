package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// guiRule is one step in the task list. Palette entries create one with
// defaults; the detail form edits it; spec() renders it to a CLI spec for
// core.ParseRules — the same engine as the CLI.
type guiRule struct {
	opID, opLabel string
	kind          string // label of the rule type, for spec()
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
	{id: "filter", group: "筛选/范围", label: "筛选文件", make: func() guiRule {
		return baseRule("filter")
	}},
	{id: "filter.out", group: "筛选/范围", label: "排除文件", make: func() guiRule {
		r := baseRule("filter")
		r.invert = true
		return r
	}},
	{id: "unfilter", group: "筛选/范围", label: "清除筛选", make: func() guiRule {
		return baseRule("unfilter")
	}},
	{id: "scope.name", group: "筛选/范围", label: "选取文件名", make: func() guiRule {
		r := baseRule("scope")
		r.scopeSel = "文件名"
		return r
	}},
	{id: "scope.ext", group: "筛选/范围", label: "选取扩展名", make: func() guiRule {
		r := baseRule("scope")
		r.scopeSel = "扩展名"
		return r
	}},
	{id: "scope.all", group: "筛选/范围", label: "恢复整体", make: func() guiRule {
		r := baseRule("scope")
		r.scopeSel = "整体"
		return r
	}},
	{id: "pinyin", group: "转换", label: "文件名转拼音", make: func() guiRule {
		return baseRule("pinyin")
	}},
	{id: "sanitize", group: "转换", label: "非法字符转全角", make: func() guiRule {
		return baseRule("sanitize")
	}},
	{id: "cc.s2t", group: "转换", label: "简体转繁体", make: func() guiRule {
		r := baseRule("cc")
		r.ccSel = "简体→繁体"
		return r
	}},
	{id: "cc.t2s", group: "转换", label: "繁体转简体", make: func() guiRule {
		r := baseRule("cc")
		r.ccSel = "繁体→简体"
		return r
	}},
	{id: "romaji", group: "转换", label: "假名转罗马音", make: func() guiRule {
		return baseRule("romaji")
	}},
	{id: "case.upper", group: "大小写", label: "全部转大写", make: func() guiRule {
		r := baseRule("case")
		r.caseMode = "大写"
		return r
	}},
	{id: "case.lower", group: "大小写", label: "全部转小写", make: func() guiRule {
		r := baseRule("case")
		r.caseMode = "小写"
		return r
	}},
	{id: "case.title", group: "大小写", label: "单词首字母大写", make: func() guiRule {
		r := baseRule("case")
		r.caseMode = "标题"
		return r
	}},
	{id: "replace", group: "替换", label: "查找替换", make: func() guiRule {
		return baseRule("replace")
	}},
	{id: "replace.re", group: "替换", label: "正则替换", make: func() guiRule {
		r := baseRule("replace")
		r.regex = true
		return r
	}},
	{id: "del.re", group: "替换", label: "正则删除", make: func() guiRule {
		r := baseRule("replace")
		r.regex = true
		return r
	}},
	{id: "ins.front", group: "结构", label: "文件名前添加", make: func() guiRule {
		r := baseRule("insert")
		r.insMode = "front"
		return r
	}},
	{id: "ins.end", group: "结构", label: "文件名后添加", make: func() guiRule {
		r := baseRule("insert")
		r.insMode = "end"
		return r
	}},
	{id: "insert", group: "结构", label: "指定位置插入", make: func() guiRule {
		return baseRule("insert")
	}},
	{id: "delete", group: "结构", label: "删除字符", make: func() guiRule {
		return baseRule("delete")
	}},
	{id: "truncate", group: "结构", label: "截断长度", make: func() guiRule {
		return baseRule("truncate")
	}},
	{id: "ext", group: "扩展名", label: "修改扩展名", make: func() guiRule {
		return baseRule("ext")
	}},
	{id: "ext.del", group: "扩展名", label: "删除扩展名", make: func() guiRule {
		r := baseRule("ext")
		r.newExt = ""
		return r
	}},
	{id: "seq", group: "编号", label: "追加序号", make: func() guiRule {
		return baseRule("seq")
	}},
	{id: "seq.prefix", group: "编号", label: "序号前缀", make: func() guiRule {
		r := baseRule("seq")
		r.numTpl = "<N>-<NAME><.EXT>"
		return r
	}},
	{id: "meta.title", group: "元数据", label: "用标题命名", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<META:title><.EXT>"
		return r
	}},
	{id: "meta.author", group: "元数据", label: "作者 - 标题", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<META:author> - <META:title><.EXT>"
		return r
	}},
	{id: "date.suffix", group: "日期", label: "文件名加日期", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<NAME>_<DATE><.EXT>"
		return r
	}},
	{id: "ai", group: "智能", label: "AI 按内容命名", make: func() guiRule {
		r := baseRule("template")
		r.tpl = "<AI:根据文件内容起一个简短的名字><.EXT>"
		return r
	}},
	{id: "tpl", group: "高级", label: "自定义模板", make: func() guiRule {
		return baseRule("template")
	}},
	{id: "act.import-csv", group: "高级", label: "导入编辑好的CSV改名", act: "import-csv"},
	{id: "act.settings", group: "高级", label: "设置", act: "settings"},
}

func opByID(id string) (opDef, bool) {
	for _, d := range catalog {
		if d.id == id {
			return d, true
		}
	}
	return opDef{}, false
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
		s := "replace=" + r.oldStr + "," + r.newStr
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
			return "insert=0," + r.text
		case "end":
			return "insert=end," + r.text
		default:
			return fmt.Sprintf("insert=%d,%s", n(r.pos)-1, r.text)
		}
	case "delete":
		return fmt.Sprintf("delete=%d,%d", n(r.pos)-1, n(r.count))
	case "truncate":
		return fmt.Sprintf("truncate=%d", n(r.maxLen))
	case "ext":
		return "ext=" + r.newExt
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
func (r *guiRule) paramsEditor(c *ui.Context, t *ui.Theme) {
	lbl := func(s string) { ui.Text(c, s).FontSize(12).TextColor(t.TextMuted) }
	key, _ := kindByLabel[r.kind]
	// parameters stack vertically: the middle pane can be narrow, and a
	// single long row would overflow it
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			switch key {
			case "pinyin", "sanitize", "unfilter", "romaji":
				lbl("此步骤无参数")
			case "cc":
				ui.Select(c, &r.ccSel, []string{"简体→繁体", "繁体→简体"}).Width(130)
			case "filter":
				ui.TextInput(c, &r.pattern).Placeholder("通配符,如 *.mp3").Width(180)
				lbl("排除匹配项(取反)")
				ui.Switch(c, &r.invert)
			case "scope":
				ui.Select(c, &r.scopeSel, []string{"文件名", "扩展名", "整体"}).Width(110)
			case "replace":
				ui.TextInput(c, &r.oldStr).Placeholder("查找").Width(150)
				ui.TextInput(c, &r.newStr).Placeholder("替换为").Width(150)
				lbl("正则")
				ui.Switch(c, &r.regex)
			case "template":
				ui.TextInput(c, &r.tpl).Placeholder("<META:author> - <META:title><.EXT>").Fill()
			case "case":
				ui.Select(c, &r.caseMode, []string{"大写", "小写", "标题"}).Width(110)
			case "insert":
				ui.Select(c, &r.insMode, []string{"文件名前", "文件名后", "指定位置"}).Width(110)
				if r.insMode == "指定位置" {
					lbl("位置(第几个字符)")
					ui.NumberInput(c, &r.pos, 1, 9999, 1).Width(72)
				}
				ui.TextInput(c, &r.text).Placeholder("插入文本").Width(180)
			case "delete":
				lbl("从第几个字符")
				ui.NumberInput(c, &r.pos, 1, 9999, 1).Width(72)
				lbl("删除数量")
				ui.NumberInput(c, &r.count, 1, 9999, 1).Width(72)
			case "truncate":
				lbl("保留长度")
				ui.NumberInput(c, &r.maxLen, 1, 9999, 1).Width(72)
			case "ext":
				ui.TextInput(c, &r.newExt).Placeholder("新扩展名,空=删除扩展名").Width(200)
			case "seq":
				ui.Select(c, &r.numBase, []string{"数字", "字母"}).Width(90)
				if r.numBase == "字母" {
					ui.Select(c, &r.numCase, []string{"大写", "小写"}).Width(90)
				} else {
					lbl("不足位补齐到")
					ui.NumberInput(c, &r.seqWidth, 1, 9, 1).Width(56)
				}
				ui.Select(c, &r.numGroup, []string{"不分组", "按扩展名", "按文件夹"}).Width(120)
				lbl("起始")
				ui.NumberInput(c, &r.seqStart, -9999, 999999, 1).Width(72)
				lbl("步长")
				ui.NumberInput(c, &r.seqStep, -999, 999, 1).Width(64)
				ui.TextInput(c, &r.numTpl).Placeholder("编号模板,如 第<N>话_<NAME><.EXT>;空=追加到文件名末尾").Fill()
			}
		})
	})
}
