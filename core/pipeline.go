package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RenameOp is the planned rename for one file plus any pipeline error.
type RenameOp struct {
	Dir     string `json:"dir"`
	OldName string `json:"old"`
	NewName string `json:"new"`
	Err     error  `json:"-"`
	ErrText string `json:"error,omitempty"`
	Applied bool   `json:"applied,omitempty"`
}

// RunPipeline applies the rule chain to every file, gathering errors
// per-file instead of aborting the batch (xTools' "check before apply").
func RunPipeline(rules []Rule, files []string) (plan []RenameOp, errs []error) {
	metaCache := map[string]map[string]string{}
	aiCache := map[string]string{}
	counters := map[string]int{} // shared across files: seq group numbering
	for i, f := range files {
		dir, name := filepath.Split(f)
		dir = filepath.Clean(dir)
		op := RenameOp{Dir: dir, OldName: name}
		cur := name
		ctx := &Ctx{
			Seq:      i + 1,
			Active:   true,
			Counters: counters,
			Vars:     func(key string) (string, bool) { v, ok := aiCache[key]; return v, ok },
		}
		needsMeta, needsAI := analyze(rules, name)
		if needsMeta {
			if m, ok := metaCache[f]; ok {
				ctx.Meta = m
			} else {
				m := ExtractMeta(f)
				metaCache[f] = m
				ctx.Meta = m
			}
		}
		if needsAI {
			// 预填 AI 结果缓存:key 必须与 ExpandVars 查询时用的表达式一致
			// (filepath.Join(dir, name) + "\x00" + 指令正文),否则查不到、
			// 模板里会残留原样的 <AI:…> 占位符。
			self := filepath.Join(dir, name)
			for _, instr := range aiInstructions(rules, name) {
				key := self + "\x00" + instr
				if _, done := aiCache[key]; done {
					continue
				}
				v, err := aiGenerate(f, instr)
				if err != nil {
					errs = append(errs, fmt.Errorf("%s: AI: %w", f, err))
					v = "" // leave placeholder; error reported
				}
				aiCache[key] = v
			}
		}
		for ri, rule := range rules {
			next, err := applyRule(ctx, dir, cur, rule)
			if err != nil {
				op.Err = fmt.Errorf("规则 %d (%T): %w", ri+1, rule, err)
				break
			}
			cur = next
		}
		if op.Err == nil && cur == "" {
			op.Err = fmt.Errorf("结果文件名为空")
		}
		if op.Err == nil && cur == name {
			cur = name
		}
		op.NewName = cur
		if op.Err != nil {
			op.ErrText = op.Err.Error()
			errs = append(errs, fmt.Errorf("%s: %w", f, op.Err))
		}
		plan = append(plan, op)
	}
	preflight(plan)
	return plan, errs
}

// applyRule runs one rule for one file, honoring the pipeline state: state
// steps (filter/scope) always run; other rules pass inactive files through
// and, with a scope set, act on the stem or the extension alone.
func applyRule(ctx *Ctx, dir, name string, rule Rule) (string, error) {
	if _, isState := rule.(stateRuleMarker); isState {
		return rule.Apply(ctx, dir, name)
	}
	if !ctx.Active {
		return name, nil
	}
	switch ctx.Scope {
	case "name":
		stem, ext := splitExt(name)
		s, err := rule.Apply(ctx, dir, stem)
		return s + ext, err
	case "ext":
		stem, ext := splitExt(name)
		e, err := rule.Apply(ctx, dir, strings.TrimPrefix(ext, "."))
		if err != nil {
			return name, err
		}
		if e == "" {
			return stem, nil
		}
		return stem + "." + e, nil
	}
	return rule.Apply(ctx, dir, name)
}

// preflight marks ops that would fail at apply time, so the user sees it
// in the preview instead of after a half-done batch: several files landing
// on the same new name, or an existing file already occupying the target.
func preflight(plan []RenameOp) {
	type target struct{ count, first int }
	targets := map[[2]string]target{}
	for i := range plan {
		op := &plan[i]
		if op.Err != nil || op.OldName == op.NewName {
			continue
		}
		k := [2]string{op.Dir, op.NewName}
		t := targets[k]
		if t.count == 0 {
			t.first = i
		}
		t.count++
		targets[k] = t
	}
	for i := range plan {
		op := &plan[i]
		if op.Err != nil || op.OldName == op.NewName {
			continue
		}
		k := [2]string{op.Dir, op.NewName}
		if targets[k].count > 1 {
			op.Err = fmt.Errorf("新名字与其他 %d 个文件相同,批量执行会互相覆盖", targets[k].count-1)
			op.ErrText = op.Err.Error()
			continue
		}
		// "大小写/全角半角"这类改名在 Windows、macOS 上目标是同一个文件:
		// 只要目标路径指向源文件本身就不算冲突
		if samePathAsSource(op.Dir, op.OldName, op.NewName) {
			continue
		}
		if _, err := os.Lstat(filepath.Join(op.Dir, op.NewName)); err == nil {
			op.Err = fmt.Errorf("目标已存在同名文件")
			op.ErrText = op.Err.Error()
		}
	}
}

func analyze(rules []Rule, _ string) (needsMeta, needsAI bool) {
	for _, r := range rules {
		if t, ok := r.(templateRule); ok {
			s := string(t)
			if containsVarPrefix(s, "META:") {
				needsMeta = true
			}
			if containsVarPrefix(s, "AI:") {
				needsAI = true
			}
		}
		if rr, ok := r.(*replaceRule); ok {
			if containsVarPrefix(rr.new, "META:") {
				needsMeta = true
			}
			if containsVarPrefix(rr.new, "AI:") {
				needsAI = true
			}
		}
	}
	return
}

func containsVarPrefix(s, prefix string) bool {
	return len(s) >= len("<"+prefix+">")-2 &&
		(indexAny(s, "<"+prefix) >= 0)
}

func indexAny(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// aiInstructions extracts the distinct <AI:...> instructions used by rules.
func aiInstructions(rules []Rule, _ string) []string {
	seen := map[string]bool{}
	var out []string
	collect := func(s string) {
		rest := s
		for {
			i := indexAny(rest, "<AI:")
			if i < 0 {
				return
			}
			rest = rest[i+4:]
			j := indexByte(rest, '>')
			if j < 0 {
				return
			}
			instr := rest[:j]
			if !seen[instr] {
				seen[instr] = true
				out = append(out, instr)
			}
			rest = rest[j:]
		}
	}
	for _, r := range rules {
		if t, ok := r.(templateRule); ok {
			collect(string(t))
		}
		if rr, ok := r.(*replaceRule); ok {
			collect(rr.new)
		}
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// ApplyPlan performs the renames and writes an undo journal.
func ApplyPlan(plan []RenameOp, journal string) error {
	var ops []RenameOp
	for i := range plan {
		op := &plan[i]
		if op.Err != nil || op.OldName == op.NewName {
			continue
		}
		old := filepath.Join(op.Dir, op.OldName)
		new := filepath.Join(op.Dir, op.NewName)
		self := samePathAsSource(op.Dir, op.OldName, op.NewName)
		if !self {
			if _, err := os.Lstat(new); err == nil {
				op.Err = fmt.Errorf("目标已存在: %s", new)
				op.ErrText = op.Err.Error()
				continue
			}
		}
		if err := rename(old, new, self); err != nil {
			op.Err = err
			op.ErrText = err.Error()
			continue
		}
		op.Applied = true
		ops = append(ops, *op)
		fmt.Printf("* %s => %s\n", old, op.NewName)
	}
	return WriteJournal(journal, ops)
}

// samePathAsSource reports whether dir/newName is the very file at
// dir/oldName, which happens when a rename only changes letter case or
// other characters a case-insensitive volume folds together.
func samePathAsSource(dir, oldName, newName string) bool {
	if oldName == newName {
		return true
	}
	a, err1 := os.Stat(filepath.Join(dir, oldName))
	b, err2 := os.Stat(filepath.Join(dir, newName))
	if err1 != nil || err2 != nil {
		return false
	}
	return os.SameFile(a, b)
}

// rename moves old to new; when they are the same file spelled differently
// (case-only rename on a case-insensitive volume) it goes through a
// temporary name, as the two names collide for the OS otherwise.
func rename(old, new string, self bool) error {
	if !self {
		return os.Rename(old, new)
	}
	tmp := new + ".ncreamer-tmp"
	if err := os.Rename(old, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, new); err != nil {
		os.Rename(tmp, old) // 尽量还原
		return err
	}
	return nil
}
