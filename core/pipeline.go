package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	counters := map[string]int{} // shared across files: seq group numbering

	// AI calls block on the network, often for seconds to minutes each. Doing
	// them one file at a time inside the main loop made a batch of N files take
	// N round-trips serially — the more files, the slower it felt. Prefetch all
	// (file, instruction) pairs concurrently first, then the loop just reads
	// the cache. The cache key must match ExpandVars' lookup: self + "\x00" +
	// instruction (no "AI:" prefix), or the placeholder would never resolve.
	aiCache, aiErrs := prefetchAI(rules, files)

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
			if aiErr := aiErrs[f]; aiErr != nil {
				// Mark this file as failed so the preview shows it as a problem
				// and apply skips it — never rename it to a literal "<AI:…>"
				// placeholder or an empty fragment like a bare ".md".
				op.Err = aiErr
				op.NewName = name
				op.ErrText = op.Err.Error()
				errs = append(errs, aiErr)
				plan = append(plan, op)
				continue
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

// aiConcurrency is how many model requests run at once during a batch. Local
// servers usually handle a few in parallel; too many can thrash a small model,
// so keep it modest and overridable.
const defaultAIConcurrency = 4

// prefetchAI runs every needed AI request for the whole batch concurrently and
// returns the cache (key = self + "\x00" + instruction) plus per-file errors.
// A file is keyed by its path in aiErrs so the main loop can mark it failed.
func prefetchAI(rules []Rule, files []string) (cache map[string]string, aiErrs map[string]error) {
	cache = map[string]string{}
	aiErrs = map[string]error{}

	type job struct {
		file, instr, key string
	}
	var jobs []job
	for _, f := range files {
		dir, name := filepath.Split(f)
		dir = filepath.Clean(dir)
		if _, needsAI := analyze(rules, name); !needsAI {
			continue
		}
		self := filepath.Join(dir, name)
		for _, instr := range aiInstructions(rules, name) {
			key := self + "\x00" + instr
			if _, dup := cache[key]; dup {
				continue
			}
			cache[key] = "" // reserve so a duplicate key isn't queued twice
			jobs = append(jobs, job{file: f, instr: instr, key: key})
		}
	}
	if len(jobs) == 0 {
		return cache, aiErrs
	}

	conc := AIConcurrency()
	if conc < 1 {
		conc = 1
	}
	if conc > len(jobs) {
		conc = len(jobs)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, conc)
	// Circuit breaker: once the server proves it's down (fatal error), stop
	// starting new requests. Without this a batch of N files waits on N doomed
	// round-trips to the same dead gateway — which is exactly the "server 502
	// but the app keeps waiting" symptom.
	var aborted int32
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			if atomic.LoadInt32(&aborted) != 0 {
				mu.Lock()
				aiErrs[j.file] = fmt.Errorf("%s: AI: 已跳过(服务器无响应,批量已中止)", j.file)
				delete(cache, j.key)
				mu.Unlock()
				return
			}
			v, err := aiGenerate(j.file, j.instr)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				aiErrs[j.file] = fmt.Errorf("%s: AI: %w", j.file, err)
				delete(cache, j.key) // don't leave a false empty hit
				if isFatalAIError(err) {
					atomic.StoreInt32(&aborted, 1)
				}
				return
			}
			cache[j.key] = v
		}(j)
	}
	wg.Wait()
	return cache, aiErrs
}

// AIConcurrency is the batch concurrency; the GUI override wins, then
// NCREAMER_AI_CONCURRENCY, then the default. Cloud APIs rate-limit hard (use 1);
// a local model often serializes anyway, so 1~2 may beat a wide fan-out.
func AIConcurrency() int {
	if aiConcurrencyOverride > 0 {
		return aiConcurrencyOverride
	}
	if s := os.Getenv("NCREAMER_AI_CONCURRENCY"); s != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			return n
		}
	}
	return defaultAIConcurrency
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
