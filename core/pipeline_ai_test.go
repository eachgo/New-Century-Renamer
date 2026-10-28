package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAI serves an OpenAI-compatible reply with the given content and reports
// how many requests it handled plus the peak number in flight.
func fakeAI(t *testing.T, content func(name string) string) (*httptest.Server, *int32, *int32) {
	t.Helper()
	var hits, inflight, peak int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		n := atomic.AddInt32(&inflight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		defer atomic.AddInt32(&inflight, -1)
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		// extract the filename from the user message ("文件名: xxx\n")
		name := ""
		if len(req.Messages) > 1 {
			for _, line := range strings.Split(req.Messages[1].Content, "\n") {
				if strings.HasPrefix(line, "文件名: ") {
					name = strings.TrimPrefix(line, "文件名: ")
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": content(name)}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, &peak
}

func setupAI(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("NCREAMER_AI_BASE", srv.URL+"/v1")
	t.Setenv("NCREAMER_AI_KEY", "k")
	t.Setenv("NCREAMER_AI_MODEL", "m")
	t.Setenv("NCREAMER_AI_TIMEOUT", "20")
}

// Bug 2: an empty model reply must NOT become a bare ".md" (the original file
// name entirely replaced by an empty fragment). The file must be flagged as a
// problem and left untouched.
func TestAIEmptyReplyMarksFileFailed(t *testing.T) {
	srv, _, _ := fakeAI(t, func(string) string { return "" })
	setupAI(t, srv)

	dir := t.TempDir()
	name := "magpie-session-pi.tar"
	os.WriteFile(filepath.Join(dir, name), []byte("content"), 0o644)
	full := filepath.Join(dir, name)

	rules := []Rule{templateRule("<AI:起个名字><.EXT>")}
	plan, errs := RunPipeline(rules, []string{full})

	if len(errs) == 0 {
		t.Fatalf("AI 返回空时应报错,实际没有任何错误")
	}
	if len(plan) != 1 {
		t.Fatalf("期望 1 条计划,实际 %d", len(plan))
	}
	if plan[0].Err == nil {
		t.Fatalf("该文件应被标为有问题(Err!=nil),实际 NewName=%q", plan[0].NewName)
	}
	if plan[0].NewName != name {
		t.Fatalf("失败时新名字应保留原名 %q,实际 %q", name, plan[0].NewName)
	}
	if strings.HasPrefix(plan[0].NewName, ".") || strings.Contains(plan[0].NewName, "<AI:") {
		t.Fatalf("绝不能产出空名或残留占位符,实际 %q", plan[0].NewName)
	}
}

// Bug 1: AI requests for a batch must run concurrently, not one-by-one. We
// serve N files each taking ~120ms; serial would be N*120ms, concurrency should
// finish much faster and peak in-flight should exceed 1.
func TestAIBatchRunsConcurrently(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Messages) > 1 {
			for _, line := range strings.Split(req.Messages[1].Content, "\n") {
				if strings.HasPrefix(line, "文件名: ") {
					mu.Lock()
					seen[strings.TrimPrefix(line, "文件名: ")] = true
					mu.Unlock()
				}
			}
		}
		time.Sleep(120 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "名字"}}},
		})
	}))
	defer srv.Close()
	setupAI(t, srv)

	dir := t.TempDir()
	const n = 8
	var files []string
	for i := 0; i < n; i++ {
		f := filepath.Join(dir, "f"+string(rune('a'+i))+".txt")
		os.WriteFile(f, []byte("x"), 0o644)
		files = append(files, f)
	}

	rules := []Rule{templateRule("<AI:命名><.EXT>")}
	// Pin concurrency so the test measures fan-out regardless of ambient env or
	// a previously-set override (a user may legitimately run single-lane).
	SetAIConcurrency(4)
	defer SetAIConcurrency(0)
	start := time.Now()
	plan, errs := RunPipeline(rules, files)
	elapsed := time.Since(start)
	if len(errs) != 0 {
		t.Fatalf("意外错误: %v", errs)
	}
	if len(plan) != n {
		t.Fatalf("期望 %d 条,实际 %d", n, len(plan))
	}
	// serial would be 8*120ms = ~960ms; with concurrency 4 it should be well
	// under half of that. Use a generous ceiling to avoid CI flakiness.
	if elapsed > 600*time.Millisecond {
		t.Fatalf("批处理太慢(%v),AI 调用似乎没有并发", elapsed)
	}
	if len(seen) != n {
		t.Fatalf("应请求全部 %d 个文件,实际只请求了 %d 个", n, len(seen))
	}
}

// Concurrency env override is honored and clamped sanely.
func TestAIConcurrencyOverride(t *testing.T) {
	t.Setenv("NCREAMER_AI_CONCURRENCY", "7")
	if got := AIConcurrency(); got != 7 {
		t.Fatalf("期望 7,实际 %d", got)
	}
	t.Setenv("NCREAMER_AI_CONCURRENCY", "0")
	if got := AIConcurrency(); got != defaultAIConcurrency {
		t.Fatalf("0 应回落默认 %d,实际 %d", defaultAIConcurrency, got)
	}
	t.Setenv("NCREAMER_AI_CONCURRENCY", "abc")
	if got := AIConcurrency(); got != defaultAIConcurrency {
		t.Fatalf("非法值应回落默认,实际 %d", got)
	}
}

// The GUI override must win over the environment (settings dialog > env).
func TestAIConcurrencyOverrideWinsOverEnv(t *testing.T) {
	t.Setenv("NCREAMER_AI_CONCURRENCY", "7")
	SetAIConcurrency(1)
	defer SetAIConcurrency(0)
	if got := AIConcurrency(); got != 1 {
		t.Fatalf("GUI override 应优先于环境变量,期望 1,实际 %d", got)
	}
	SetAIConcurrency(0)
	if got := AIConcurrency(); got != 7 {
		t.Fatalf("清除 override 后应回落到环境变量 7,实际 %d", got)
	}
}

// Saved default prompt: used by new steps, and folded into the system message
// as the user's preference while keeping the hard output constraints.
func TestDefaultAIPromptOverride(t *testing.T) {
	defer SetDefaultAIPrompt("")
	if got := DefaultAIPrompt(); got != defaultAIPrompt {
		t.Fatalf("无 override 时应为内置默认 %q,实际 %q", defaultAIPrompt, got)
	}
	const custom = "只输出不解释,不超过20个字"
	SetDefaultAIPrompt(custom)
	if got := DefaultAIPrompt(); got != custom {
		t.Fatalf("应返回保存的提示词,实际 %q", got)
	}
	sys := aiSystemPrompt()
	if !strings.Contains(sys, custom) {
		t.Fatalf("系统提示应包含用户要求,实际 %q", sys)
	}
	if !strings.Contains(sys, "不要引号") {
		t.Fatalf("系统提示必须保留硬约束(不要引号等),实际 %q", sys)
	}
}

// The reverse of the concurrency test: when the server proves it's down (502),
// the batch must stop hammering it. Without a circuit breaker, N files = N
// doomed round-trips to the same dead gateway — the "server 502 but the app
// keeps waiting" symptom. Here 20 files must produce only a handful of hits.
func TestAIFatalErrorTripsCircuitBreaker(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":{"message":"Bad gateway"}}`))
	}))
	defer srv.Close()
	setupAI(t, srv)

	dir := t.TempDir()
	const n = 20
	var files []string
	for i := 0; i < n; i++ {
		f := filepath.Join(dir, "f"+string(rune('a'+i))+".txt")
		os.WriteFile(f, []byte("x"), 0o644)
		files = append(files, f)
	}

	SetAIConcurrency(1) // one lane, so the breaker can skip the rest
	defer SetAIConcurrency(0)

	rules := []Rule{templateRule("<AI:命名><.EXT>")}
	plan, errs := RunPipeline(rules, files)

	if len(plan) != n {
		t.Fatalf("期望 %d 条计划,实际 %d", n, len(plan))
	}
	for _, op := range plan {
		if op.Err == nil {
			t.Fatalf("文件 %s 应标为失败", op.OldName)
		}
		if op.NewName != op.OldName {
			t.Fatalf("失败文件应保留原名 %q,实际 %q", op.OldName, op.NewName)
		}
	}
	if len(errs) == 0 {
		t.Fatal("应有错误返回")
	}
	got := atomic.LoadInt32(&hits)
	if got > 3 {
		t.Fatalf("502 应触发熔断,最多再试几次;实际向死服务器请求了 %d 次(共 %d 文件)", got, n)
	}
	t.Logf("circuit breaker: %d requests for %d files", got, n)
}

// A non-fatal error (one bad/empty answer) must NOT trip the breaker — the
// other files should still be tried.
func TestAIEmptyAnswerDoesNotTripBreaker(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		name := ""
		if len(req.Messages) > 1 {
			for _, line := range strings.Split(req.Messages[1].Content, "\n") {
				if strings.HasPrefix(line, "文件名: ") {
					name = strings.TrimPrefix(line, "文件名: ")
				}
			}
		}
		content := "好名字_" + strings.TrimSuffix(name, ".txt")
		if strings.HasPrefix(name, "bad") {
			content = "" // empty reply for one file only
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}}},
		})
	}))
	defer srv.Close()
	setupAI(t, srv)

	dir := t.TempDir()
	var files []string
	for _, nm := range []string{"bad1.txt", "good1.txt", "good2.txt", "good3.txt"} {
		f := filepath.Join(dir, nm)
		os.WriteFile(f, []byte("x"), 0o644)
		files = append(files, f)
	}
	SetAIConcurrency(1)
	defer SetAIConcurrency(0)

	rules := []Rule{templateRule("<AI:命名><.EXT>")}
	plan, _ := RunPipeline(rules, files)
	if got := atomic.LoadInt32(&hits); got != int32(len(files)) {
		t.Fatalf("空回复不应触发熔断,应请求全部 %d 个文件,实际 %d", len(files), got)
	}
	failed, ok := 0, 0
	for _, op := range plan {
		if op.Err != nil {
			failed++
		} else if op.NewName != op.OldName {
			ok++
		}
	}
	if failed != 1 || ok != 3 {
		t.Fatalf("期望 1 失败 3 成功,实际 failed=%d ok=%d", failed, ok)
	}
}

// isFatalAIError classification sanity.
func TestIsFatalAIError(t *testing.T) {
	cases := []struct {
		msg   string
		fatal bool
	}{
		{"AI 接口 502: bad gateway", true},
		{"AI 接口 503: unavailable", true},
		{"AI 接口 504: timeout", true},
		{"AI 接口 429: rate limited", true},
		{"AI 接口超时(服务器无响应): context deadline exceeded", true},
		{"AI 返回空内容", false},
		{"AI 接口 400: bad request", false},
	}
	for _, c := range cases {
		if got := isFatalAIError(fmt.Errorf("%s", c.msg)); got != c.fatal {
			t.Errorf("%q: 期望 fatal=%v 实际 %v", c.msg, c.fatal, got)
		}
	}
}
