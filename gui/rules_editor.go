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
	// oldTouched 记"查找框被用户动过没有"。没动过时空值是正常的
	// (步骤刚建出来),只有动过又清空才是真的填错了。
	oldTouched bool
	regex      bool
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
	// opens 非空表示"点了打开一个选择器,不直接加步骤"。和 act 的区别:
	// act 是"点了就跑",opens 只是把另一处已经存在的选择摊开——用户
	// 还得在那边挑,挑完才决定加什么。目录里放一个选择器不算混入动作。
	opens string
	flow  bool // 流程控制(筛选/范围): not a direct rename
	// palLabel 只在左栏目录里显示,用来区分"往任务里插一步"和"清空搜索框"
	// 这类同名不同义的动作(如「插入清除筛选步骤」)。步骤行里仍用 label,
	// 免得每一步的编号后面拖一长串。
	palLabel string
	// alias 是别称:用户搜「改名」「更名」「重命名」「后缀」这类日常说法时,
	// 条目名里一个都对不上,于是"产品自己叫 Renamer,却搜不到改名"。
	// 别名只参与搜索匹配,不显示、不影响分组。
	alias []string
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

// catalogMatch 是目录搜索的匹配条件:名字、说明、组名,以及别名。
// 别名是给人用的日常说法("改名""后缀""括号"),名字里不一定出现。
// palName 是目录里显示/搜索用的名字:有 palLabel 用它,否则用 label。
func (d opDef) palName() string {
	if d.palLabel != "" {
		return d.palLabel
	}
	return d.label
}

func catalogMatch(d opDef, q string) bool {
	if strings.Contains(strings.ToLower(d.palName()), q) ||
		strings.Contains(strings.ToLower(d.label), q) ||
		strings.Contains(strings.ToLower(d.desc), q) ||
		strings.Contains(strings.ToLower(d.group), q) {
		return true
	}
	for _, a := range d.alias {
		if strings.Contains(strings.ToLower(a), q) {
			return true
		}
	}
	return false
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
	{id: "unfilter", icon: "unfilter", group: "筛选/范围", flow: true, label: "清除筛选", palLabel: "插入「清除筛选」步骤", desc: "取消筛选，恢复全部文件参与后续步骤", make: func() guiRule {
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
	{id: "replace", icon: "replace", group: "替换", label: "查找替换", alias: []string{"改名", "更名", "重命名", "替换文字", "改文字"}, desc: "把查找的文字替换为指定内容", make: func() guiRule {
		return baseRule("replace")
	}},
	{id: "replace.re", icon: "replace.re", group: "替换", label: "正则替换", alias: []string{"正则", "改名", "重命名", "括号", "方括号"}, desc: "用正则表达式查找并替换", make: func() guiRule {
		r := baseRule("replace")
		r.regex = true
		return r
	}},
	{id: "del.re", icon: "del.re", group: "替换", label: "正则删除", alias: []string{"删除括号", "去掉括号", "括号", "删除多余"}, desc: "删除正则表达式匹配到的部分", make: func() guiRule {
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
	{id: "ext", icon: "ext", group: "扩展名", label: "修改扩展名", alias: []string{"后缀", "改后缀", "扩展名"}, desc: "替换扩展名；填空则删除扩展名", make: func() guiRule {
		return baseRule("ext")
	}},
	{id: "ext.del", icon: "ext.del", group: "扩展名", label: "删除扩展名", desc: "去掉文件的扩展名", make: func() guiRule {
		r := baseRule("ext")
		r.newExt = ""
		return r
	}},
	{id: "seq", icon: "seq", group: "编号", label: "追加序号", alias: []string{"编号", "序号", "加序号", "自动编号"}, desc: "文件名末尾追加编号；可分组重新计数、补零、用字母", make: func() guiRule {
		return baseRule("seq")
	}},
	{id: "seq.prefix", icon: "seq.prefix", group: "编号", label: "序号前缀", alias: []string{"前缀", "编号", "序号"}, desc: "编号放文件名前面，如 01-原名", make: func() guiRule {
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
	{id: "tpl", icon: "tpl", group: "高级", label: "自定义模板", alias: []string{"改名", "更名", "重命名", "命名", "按规则改名"}, desc: "用变量模板重建文件名，可用 <NAME> <DATE> <META:…> 等", make: func() guiRule {
		return baseRule("template")
	}},
	// 自定义流程:排到一半想换做法时的入口。它不直接加步骤,而是把
	// "流程"弹窗摊开——预设和用户存过的都在那里。放在"高级"组,
	// 因为它不是一条规则,而是"换一整套规则"。
	{id: "flow.open", icon: "act.settings", group: "高级", label: "自定义流程",
		desc: "打开流程列表：套用预设，或把当前步骤存成自己的流程", opens: "flows"},
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
func (r *guiRule) fullSpec() string {
	// 一条还没填的「查找替换」不参与整轮预览,而不是让整轮预览失败。
	//
	// 空查找串在 core 里是硬错误(它会把替换文本插到每个字符之间),
	// 所以 spec 一旦生成就是 "replace=," → ParseRules 直接报错 →
	// previewErr 非空 → 右栏顶部一条红条、所有文件的新名列变成
	// "等步骤修好"。用户刚点了一下"查找替换",什么都还没输,屏幕上
	// 就已经"整轮算不出来"——这正是"开局就错"。
	//
	// 没碰过的空步骤是"还没写完的草稿",不是"写错的规则":它不产生
	// spec,预览里就少一条规则,其余照常算。用户填了之后(或填过又
	// 清空,oldTouched 为真)才进 spec——那时报错才是对的。
	if key, ok := kindByLabel[r.kind]; ok && key == "replace" && r.oldStr == "" && !r.oldTouched {
		return ""
	}
	return r.spec()
}

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
//
// PopoverMenuButton: the menu is drawn in the window with the theme's look.
// A system menu next to a form made of rounded cards reads as another
// program's.
func varPicker(c *ui.Context, dst *string) {
	ui.PopoverMenuButton(c, "插入变量", func(m *ui.Menu) {
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
// 20 DIP (Theme.Space(5)) and the field takes the rest. Callers size the
// pair once instead of guessing the two parts separately.
//
// The two live inside one bordered box with no gap between them, so the
// control reads as one field with a step area, not as two controls that
// happen to sit next to each other.
func numField(c *ui.Context, value *float64, lo, hi, step, width float32) {
	const h = 32
	const stepperW = float32(20)
	fieldW := width - stepperW
	if fieldW < 28 {
		fieldW = 28
	}
	format := func(v float64) string { return strconv.FormatFloat(v, 'f', numDecimals(float64(step)), 64) }
	// 一个容器装下输入框和步进器,而不是两个各自描边的框并排。
	//
	// 以前是「圆角框 + 6 DIP 缝 + 圆角框」:同一行里两条竖边紧挨着,中间
	// 还夹一条底色,看上去像两个控件拼在一起(它们本来就是一个)。现在
	// 容器画唯一的边框与圆角,里面的输入框和步进器都去掉自己的边框,
	// 于是只剩一条外轮廓。
	t := c.Theme()
	box := ui.Row(c).Gap(0).AlignItems(ui.Center).Width(width).Height(h).
		Radius(6).Border(1, t.Border).Background(t.Surface).Clip()
	text := ui.Local(box, "text", func() string { return format(*value) })
	box.Children(func() {
		in := ui.TextInput(c, text).Width(fieldW).Height(h).TextAlign(ui.Center).
			Border(0, ui.Color{}).Background(ui.Color{})
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
		ui.Stepper(c, value, float64(lo), float64(hi), float64(step)).
			Height(h).Border(0, ui.Color{})
	})
}

// intField is numField for a setting kept in a text buffer instead of an int.
//
// The two AI numbers (等待秒数, 并发路数) live in string buffers because the
// settings dialog has to survive half-typed input: "3" on the way to "300" is
// not a valid timeout, and parsing on every keystroke would either clamp it to
// 3 seconds or throw the edit away. So the buffer stays the source of truth
// (commit reads it, tests write it), and this control keeps a real int beside
// it: the stepper steps a clamped value, blur rewrites the buffer, and an
// out-of-range or non-numeric entry is reported in place instead of silently
// becoming the default when the dialog closes.
//
// An empty buffer means "use the built-in default", so the field shows def
// rather than 0 — what the user reads must be what actually takes effect.
// label is the accessible name: without it a screen reader announces an
// unnamed spin button, and a test cannot find the field to click it.
func intField(c *ui.Context, label string, out *string, lo, hi, def int, width float32) {
	t := c.Theme()
	const h = 32
	const stepperW = float32(20)
	fieldW := width - stepperW
	if fieldW < 28 {
		fieldW = 28
	}
	box := ui.Column(c).Gap(4)
	value := ui.Local(box, "int-value", func() float64 {
		if n, err := strconv.Atoi(strings.TrimSpace(*out)); err == nil {
			return clamp(float64(n), float64(lo), float64(hi))
		}
		return float64(def)
	})
	text := ui.Local(box, "int-text", func() string {
		if strings.TrimSpace(*out) == "" {
			return strconv.Itoa(def)
		}
		return *out
	})
	// touched 记住"这个框被编辑过"。没有它,打开设置面板的一瞬间
	// 空缓冲就会被写成 "300"(默认值),用户没动过手,设置里却多出一条
	// 显式配置——之后再想"留空用默认"就没法表达了。
	touched := ui.Local(box, "int-touched", func() bool { return false })
	box.Children(func() {
		// 与 numField 同样的单容器:一条外轮廓装下输入框和步进器,
		// 不是两个各自描边的框并排(那会在相接处留两条竖边)。
		ui.Row(c).Gap(0).AlignItems(ui.Center).Width(width).Height(h).
			Radius(6).Border(1, t.Border).Background(t.Surface).Clip().Children(func() {
			in := ui.TextInput(c, text).Width(fieldW).Height(h).TextAlign(ui.Center).
				Label(label).Border(0, ui.Color{}).Background(ui.Color{})
			if in.Focused() {
				*touched = true
			}
			if in.Changed() {
				// 边打边同步到设置缓冲:点「保存」时焦点未必先离开这个框,
				// 只在失焦时写会丢掉最后一次编辑(填了 42 直接点保存,存的
				// 还是空串 = 默认 300 秒)。
				*out = strings.TrimSpace(*text)
				if v, err := strconv.Atoi(strings.TrimSpace(*text)); err == nil {
					*value = clamp(float64(v), float64(lo), float64(hi))
				}
			}
			if in.Shortcut(0, ui.KeyUp) {
				*value = clamp(*value+1, float64(lo), float64(hi))
				*text = strconv.Itoa(int(*value))
				*out = *text
				*touched = true
			}
			if in.Shortcut(0, ui.KeyDown) {
				*value = clamp(*value-1, float64(lo), float64(hi))
				*text = strconv.Itoa(int(*value))
				*out = *text
				*touched = true
			}
			ui.Stepper(c, value, float64(lo), float64(hi), 1).
				Height(h).Border(0, ui.Color{})
			if !in.Focused() && *touched {
				*touched = false
				if strings.TrimSpace(*text) == "" {
					*value = float64(def)
					*text = ""
				} else {
					*text = strconv.Itoa(int(*value))
				}
				*out = *text
			}
		})
		// 就地校验:填了但不是数字、或超出范围,当场说清楚。
		// 以前这两种输入都会被 commit 静默当成 0(= 用默认),
		// 用户以为填的 5 生效了,实际跑的还是 300 秒。
		if raw := strings.TrimSpace(*text); raw != "" {
			if n, err := strconv.Atoi(raw); err != nil {
				ui.Text(c, "只能填整数").FontSize(12).TextColor(t.Danger)
			} else if n < lo || n > hi {
				ui.Text(c, fmt.Sprintf("范围 %d–%d", lo, hi)).FontSize(12).TextColor(t.Danger)
			}
		}
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
				ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
				ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(ctl)
			})
			return
		}
		ui.Row(c).Gap(labelGap).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).Width(labelW)
			ctl()
		})
	}
	ui.Column(c).FillWidth().Gap(10).Children(func() { r.paramsFields(c, t, pline, fit, stacked) })
}

// paramsFields is the per-kind body of the form: one pline call per
// parameter. Split out so paramsEditor stays about layout.
func (r *guiRule) paramsFields(c *ui.Context, t *ui.Theme, pline func(label string, ctl func()), fit func(float32) float32, stacked bool) {
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
			// touched 记"用户动过这一格没有"。
			//
			// 新建一条「查找替换」步骤时 oldStr 天生是空的,而空查找串
			// 是条会执行成功的坏规则(见下面的注释)。以前的做法是直接
			// 按 oldStr=="" 判红——于是用户刚点完"查找替换",还没来得
			// 及填,屏幕上就已经是一条红字,像是他做错了什么。
			//
			// 现在分两档:没碰过 → 中性灰提示(说清这一步会怎么走);
			// 碰过又清空 → 才是红的。同样的信息,但"还没开始"和
			// "填错了"不再长成一个样子。
			ui.Column(c).Gap(4).Children(func() {
				in := ui.TextInput(c, &r.oldStr).Placeholder("查找").FillWidth()
				// 输入框一有内容就说明碰过了;清空之后 touched 仍留着,
				// 所以"先填再删"能正确落进红色那一档。
				if r.oldStr != "" {
					r.oldTouched = true
				}
				// 粘贴等路径不一定经过按键,失焦时再确认一次。
				if !in.Focused() && r.oldStr != "" {
					r.oldTouched = true
				}
				if r.oldStr == "" {
					// 空查找串不是"没填",它是一条会执行成功的规则:替换会把
					// 替换文本插到每个字符之间,正则在每个位置匹配一次。预览
					// 会显示每个文件名被切碎,执行出来就是一堆谁也不认识的
					// 名字。core 里已经拒绝,这里就地说明,省得用户对着一条
					// 报错的步骤猜是哪一格没填。
					if r.oldTouched {
						ui.Text(c, "查找内容不能为空——空字符串会匹配每个字符之间，这一步现在不会执行。").
							FontSize(12).TextColor(t.Danger).SingleLine()
					} else {
						ui.Text(c, "填上要查找的内容，这一步才会生效。").
							FontSize(12).TextColor(t.TextMuted).SingleLine()
					}
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
		// 起始 / 步长:并排时是一行两组(中间一个 "/"),窄栏下拆成
		// 两行、各带自己的标签。
		//
		// 以前 stacked 时这一行被 Wrap 折成两行,但两个数字框共用一个
		// 「起始 / 步长」标签,折完之后第二个框前面没有任何说明——
		// 读者看到的是 [1] 换行 [1],只能靠记住左边那个是起始。
		// 拆开之后两行各自完整:"起始 1" / "步长 1"。
		if stacked {
			pline("起始", func() { numField(c, &r.seqStart, -9999, 999999, 1, fit(90)) })
			pline("步长", func() { numField(c, &r.seqStep, -999, 999, 1, fit(90)) })
		} else {
			pline("起始 / 步长", func() {
				numField(c, &r.seqStart, -9999, 999999, 1, fit(70))
				ui.Text(c, "/").TextColor(t.TextMuted)
				numField(c, &r.seqStep, -999, 999, 1, fit(70))
			})
		}
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
