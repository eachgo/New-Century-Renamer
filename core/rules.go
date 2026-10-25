package core

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Rule is one step of the rename pipeline. Rules are applied in order to
// each file's current name, like xTools' multi-step renaming.
type Rule interface {
	// Apply transforms the name. ctx provides sequence number, variables
	// and per-file metadata. dir is the file's directory (unchanged).
	Apply(ctx *Ctx, dir, name string) (string, error)
}

type Ctx struct {
	Seq      int // 1-based index within the batch
	Meta     map[string]string
	Vars     func(name string) (string, bool) // extra variable source (AI cache)
	Final    func(dir, name string) string    // hook used by template var lookup
	Counters map[string]int                   // per-rule group counters (seq/num)
	Active   bool                             // set by filter steps; false = step over this file
	Scope    string                           // "", "name" or "ext": what later rules act on
}

// stateRule marks rules that change pipeline state (filter / scope) instead
// of transforming the name. They run even for inactive files.
type stateRuleMarker interface{ isStateRule() }

// ParseRules turns "name=args" strings into Rule values. A spec may be
// prefixed with "if:PATTERN>>" to restrict the rule to files whose current
// name matches the wildcard pattern (case-insensitive), e.g.
// "if:*.mp3>>pinyin"; other files pass through unchanged.
func ParseRules(specs []string) ([]Rule, error) {
	var rules []Rule
	for i, spec := range specs {
		filter := ""
		if rest, ok := strings.CutPrefix(spec, "if:"); ok {
			pat, inner, ok := strings.Cut(rest, ">>")
			if !ok {
				return nil, fmt.Errorf("规则 %d: if: 前缀缺少 >> 分隔符", i+1)
			}
			filter, spec = pat, inner
		}
		name, argstr, _ := strings.Cut(spec, "=")
		args := splitArgs(argstr)
		var r Rule
		var err error
		switch name {
		case "template":
			if len(args) < 1 {
				return nil, fmt.Errorf("规则 %d: template 需要模板参数", i+1)
			}
			r = templateRule(spec[strings.Index(spec, "=")+1:])
		case "replace":
			r, err = newReplaceRule(args)
		case "pinyin":
			r = pinyinRule{}
		case "sanitize":
			r = sanitizeRule{}
		case "case":
			r, err = newCaseRule(args)
		case "insert":
			r, err = newInsertRule(args)
		case "delete":
			r, err = newDeleteRule(args)
		case "truncate":
			r, err = newTruncateRule(args)
		case "ext":
			r = extRule(val(args, 0))
		case "seq", "num":
			r, err = newSeqRule(argstr)
		case "filter":
			pat, flags, _ := strings.Cut(argstr, ",")
			r = filterStep{pattern: pat, invert: strings.Contains(flags, "invert")}
		case "unfilter":
			r = unfilterStep{}
		case "cc":
			if argstr != "s2t" && argstr != "t2s" {
				return nil, fmt.Errorf("规则 %d: cc 需要 s2t 或 t2s", i+1)
			}
			r = ccRule(argstr)
		case "romaji":
			r = romajiRule{}
		case "scope":
			r = scopeStep(argstr)
		default:
			return nil, fmt.Errorf("规则 %d: 未知规则 %q", i+1, name)
		}
		if err != nil {
			return nil, fmt.Errorf("规则 %d (%s): %w", i+1, name, err)
		}
		if filter != "" {
			r = filteredRule{pattern: filter, inner: r}
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// ---- filter wrapper ----

type filteredRule struct {
	pattern string
	inner   Rule
}

func (f filteredRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	if !matchFilter(f.pattern, name) {
		return name, nil // this step does not apply to the file
	}
	return f.inner.Apply(ctx, dir, name)
}

// matchFilter matches a wildcard pattern (*.mp3, track*) against the whole
// name, case-insensitively.
func matchFilter(pattern, name string) bool {
	ok, err := filepath.Match(strings.ToLower(pattern), strings.ToLower(name))
	return err == nil && ok
}

// ---- filter / scope steps: pipeline state, not name transforms ----

// filterStep narrows the batch: later steps apply only to files whose
// current name matches the wildcard pattern (or does not, with invert).
type filterStep struct {
	pattern string
	invert  bool
}

func (filterStep) isStateRule() {}
func (f filterStep) Apply(ctx *Ctx, dir, name string) (string, error) {
	ctx.Active = matchFilter(f.pattern, name) != f.invert
	return name, nil
}

// unfilterStep restores the whole batch.
type unfilterStep struct{}

func (unfilterStep) isStateRule() {}
func (unfilterStep) Apply(ctx *Ctx, dir, name string) (string, error) {
	ctx.Active = true
	return name, nil
}

// scopeStep picks what later rules act on: the stem (name), the extension
// without its dot (ext), or the whole name again.
type scopeStep string

func (scopeStep) isStateRule() {}
func (s scopeStep) Apply(ctx *Ctx, dir, name string) (string, error) {
	ctx.Scope = string(s)
	return name, nil
}

// splitArgs splits a rule's argument string on commas, honoring \, and
// \ escapes so regex arguments (like {1,3} or [a-z,]) survive the trip
// from the GUI's spec strings.
func splitArgs(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && (s[i+1] == ',' || s[i+1] == '\\'):
			cur.WriteByte(s[i+1])
			i++
		case s[i] == ',':
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(s[i])
		}
	}
	out = append(out, cur.String())
	return out
}

func val(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

// ---- template ----

type templateRule string

func (t templateRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	tpl := string(t)
	// 模板里用到的 META 变量若全部为空(文件没有元数据),跳过该文件:
	// 否则 "作者 - 标题" 会退化成 " - ",把所有无元数据的文件改名成同一个
	if refs := metaRefs(tpl); len(refs) > 0 {
		anyValue := false
		for _, k := range refs {
			if ctx.Meta != nil && ctx.Meta[k] != "" {
				anyValue = true
				break
			}
		}
		if !anyValue {
			return name, nil
		}
	}
	out := ExpandVars(tpl, ctx, dir, name)
	if strings.TrimSpace(out) == "" || strings.TrimSpace(splitExtStem(out)) == "" {
		return name, nil
	}
	return out, nil
}

// metaRefs lists the <META:...> keys a template refers to.
func metaRefs(tpl string) []string {
	var out []string
	rest := tpl
	for {
		i := strings.Index(rest, "<META:")
		if i < 0 {
			return out
		}
		rest = rest[i+len("<META:"):]
		j := strings.IndexByte(rest, '>')
		if j < 0 {
			return out
		}
		out = append(out, rest[:j])
		rest = rest[j:]
	}
}

func splitExtStem(name string) string {
	stem, _ := splitExt(name)
	return stem
}

// ---- replace ----

type replaceRule struct {
	old, new string
	re       *regexp.Regexp
}

func newReplaceRule(args []string) (Rule, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("需要 OLD,NEW 参数")
	}
	r := &replaceRule{old: args[0], new: args[1]}
	if val(args, 2) == "regex" {
		re, err := regexp.Compile(args[0])
		if err != nil {
			return nil, err
		}
		r.re = re
	}
	return r, nil
}

func (r *replaceRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	// the replacement itself supports variables (e.g. inserting <SEQ>)
	repl := ExpandVars(r.new, ctx, dir, name)
	if r.re != nil {
		return r.re.ReplaceAllString(name, repl), nil
	}
	return strings.ReplaceAll(name, r.old, repl), nil
}

// ---- case ----

type caseRule string // upper|lower|title

func newCaseRule(args []string) (Rule, error) {
	switch val(args, 0) {
	case "upper", "lower", "title":
		return caseRule(val(args, 0)), nil
	}
	return nil, fmt.Errorf("case 需要 upper|lower|title")
}

func (c caseRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	switch c {
	case "upper":
		return strings.ToUpper(name), nil
	case "lower":
		return strings.ToLower(name), nil
	default:
		words := strings.Fields(name)
		for i, w := range words {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
		return strings.Join(words, " "), nil
	}
}

// ---- insert / delete (positions may be negative = from end; insert also
// takes "end" = before the extension, and 0 = front) ----

type insertRule struct {
	pos  int
	end  bool // insert before the extension instead of a position
	text string
}

func newInsertRule(args []string) (Rule, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("需要 POS,TEXT 参数")
	}
	if args[0] == "end" {
		return insertRule{end: true, text: args[1]}, nil
	}
	pos, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, err
	}
	return insertRule{pos: pos, text: args[1]}, nil
}

func (r insertRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	if r.end {
		stem, ext := splitExt(name)
		return stem + r.text + ext, nil
	}
	i := normPos(r.pos, len([]rune(name)))
	rs := []rune(name)
	return string(rs[:i]) + r.text + string(rs[i:]), nil
}

type deleteRule struct {
	pos, n int
}

func newDeleteRule(args []string) (Rule, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("需要 POS,LEN 参数")
	}
	pos, err1 := strconv.Atoi(args[0])
	n, err2 := strconv.Atoi(args[1])
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("POS,LEN 需为整数")
	}
	return deleteRule{pos, n}, nil
}

func (r deleteRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	rs := []rune(name)
	i := normPos(r.pos, len(rs))
	j := i + r.n
	if j > len(rs) {
		j = len(rs)
	}
	if i < 0 || i > len(rs) {
		return name, nil
	}
	return string(rs[:i]) + string(rs[j:]), nil
}

func normPos(pos, length int) int {
	if pos < 0 {
		pos += length
	}
	if pos < 0 {
		pos = 0
	}
	if pos > length {
		pos = length
	}
	return pos
}

// ---- truncate ----

type truncateRule int

func newTruncateRule(args []string) (Rule, error) {
	n, err := strconv.Atoi(val(args, 0))
	if err != nil || n <= 0 {
		return nil, fmt.Errorf("需要正整数 MAXLEN")
	}
	return truncateRule(n), nil
}

func (t truncateRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	stem, ext := splitExt(name)
	rs := []rune(stem)
	if len(rs) <= int(t) {
		return name, nil
	}
	return string(rs[:t]) + ext, nil
}

// ---- ext ----

type extRule string

func (r extRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	// under scope=ext the input is the bare extension: set it directly
	if ctx.Scope == "ext" {
		return string(r), nil
	}
	stem, _ := splitExt(name)
	if r == "" {
		return stem, nil
	}
	return stem + "." + string(r), nil
}

func splitExt(name string) (stem, ext string) {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 || i == len(name)-1 {
		return name, ""
	}
	e := name[i:]
	// a "dot" followed by spaces / containing spaces is not an extension
	if strings.ContainsAny(e, " \t") {
		return name, ""
	}
	return name[:i], e
}

// ---- seq / num: numbering with grouping, padding, letters, template ----
//
// Legacy positional spec: seq=START,WIDTH,STEP (appends zero-padded number).
// Key/value spec: seq=start:1,width:2,step:1,base:digit|letter,case:upper|lower,
//                 group:none|ext|dir,tpl:第<N>话_<NAME><.EXT>
// tpl (last field, may contain anything except ',') rebuilds the name with
// <N> the number text; without tpl the number is appended to the stem.

type seqRule struct {
	start, width, step int
	letters            bool
	upper              bool
	group              string // none | ext | dir
	tpl                string
}

func newSeqRule(argstr string) (Rule, error) {
	r := seqRule{start: 1, width: 2, step: 1, upper: true, group: "none"}
	if argstr == "" {
		return r, nil
	}
	if !strings.Contains(argstr, ":") {
		// legacy positional START,WIDTH,STEP
		args := strings.Split(argstr, ",")
		var err error
		if v := val(args, 0); v != "" {
			if r.start, err = strconv.Atoi(v); err != nil {
				return nil, err
			}
		}
		if v := val(args, 1); v != "" {
			if r.width, err = strconv.Atoi(v); err != nil {
				return nil, err
			}
		}
		if v := val(args, 2); v != "" {
			if r.step, err = strconv.Atoi(v); err != nil {
				return nil, err
			}
		}
		return r, nil
	}
	// key:value pairs; tpl takes the remainder of the string
	rest := argstr
	for rest != "" {
		var kv string
		if i := strings.Index(rest, ","); i >= 0 {
			kv, rest = rest[:i], rest[i+1:]
		} else {
			kv, rest = rest, ""
		}
		k, v, _ := strings.Cut(kv, ":")
		var err error
		switch k {
		case "start":
			if r.start, err = strconv.Atoi(v); err != nil {
				return nil, err
			}
		case "width":
			if r.width, err = strconv.Atoi(v); err != nil {
				return nil, err
			}
		case "step":
			if r.step, err = strconv.Atoi(v); err != nil {
				return nil, err
			}
		case "base":
			r.letters = v == "letter"
		case "case":
			r.upper = v != "lower"
		case "group":
			r.group = v
		case "tpl":
			r.tpl = v
			if idx := strings.Index(rest, ","); idx >= 0 {
				r.tpl += rest[:idx]
				rest = rest[idx+1:]
			} else {
				r.tpl += rest
				rest = ""
			}
		default:
			return nil, fmt.Errorf("未知参数 %q", k)
		}
	}
	return r, nil
}

func (r seqRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	// group key: numbering restarts within each group
	g := ""
	switch r.group {
	case "ext":
		_, ext := splitExt(name)
		g = ext
	case "dir":
		g = dir
	}
	key := fmt.Sprintf("seq|%s", g)
	if ctx.Counters == nil {
		ctx.Counters = map[string]int{}
	}
	seen := ctx.Counters[key]
	ctx.Counters[key] = seen + 1
	n := r.start + seen*r.step

	var s string
	if r.letters {
		s = letterNumber(n, r.upper)
	} else {
		s = fmt.Sprintf("%0*d", r.width, n)
	}
	stem, ext := splitExt(name)
	if r.tpl != "" {
		out := strings.ReplaceAll(r.tpl, "<N>", s)
		out = strings.ReplaceAll(out, "<NAME>", stem)
		out = strings.ReplaceAll(out, "<.EXT>", ext)
		return out, nil
	}
	return stem + s + ext, nil
}

// letterNumber renders 1 as A, 2 as B, ..., 26 as Z, 27 as AA.
func letterNumber(n int, upper bool) string {
	if n < 1 {
		n = 1
	}
	var b []byte
	for n > 0 {
		n--
		b = append([]byte{byte('a' + n%26)}, b...)
		n /= 26
	}
	s := string(b)
	if upper {
		return strings.ToUpper(s)
	}
	return s
}

// ---- pinyin ----

type pinyinRule struct{}

func (pinyinRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	stem, ext := splitExt(name)
	return ToPinyin(stem) + ext, nil
}

// ---- character-set conversion (OpenCC) and kana romanization ----

type ccRule string

func (r ccRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	stem, ext := splitExt(name)
	var out string
	var err error
	if r == "s2t" {
		out, err = ToTraditional(stem)
	} else {
		out, err = ToSimplified(stem)
	}
	if err != nil {
		return name, err
	}
	return out + ext, nil
}

type romajiRule struct{}

func (romajiRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	stem, ext := splitExt(name)
	return ToRomaji(stem) + ext, nil
}

// ---- sanitize: xTools' trick - keep information by mapping reserved
// characters to their full-width lookalikes instead of dropping them. ----

var reserved = map[rune]rune{
	'/': '／', '\\': '＼', ':': '：', '*': '＊',
	'?': '？', '"': '＂', '<': '＜', '>': '＞', '|': '｜',
}

type sanitizeRule struct{}

func (sanitizeRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	s := strings.Map(func(r rune) rune {
		if r < 0x20 {
			return -1
		}
		if fw, ok := reserved[r]; ok {
			return fw
		}
		return r
	}, name)
	s = strings.TrimRight(s, " .")
	// also trim leading spaces per stem segment
	stem, ext := splitExt(s)
	stem = strings.TrimSpace(stem)
	return stem + ext, nil
}
