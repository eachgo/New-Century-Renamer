package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// TestPreviewAsyncShowsSpinner verifies that clicking the AI preview button
// does NOT block the UI thread, flips busyAI so the spinner and a waiting
// message show while the model "thinks", and settles back to idle with the
// AI-produced name filled in. A slow local mock LLM stands in for the model.
func TestPreviewAsyncShowsSpinner(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(1200 * time.Millisecond) // model "thinking"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "慢慢来也能起好名字"}}},
		})
	}))
	defer srv.Close()

	core.SetAIConfig(srv.URL, "test-key", "mock-model")
	defer core.SetAIConfig("", "", "")

	dir := t.TempDir()
	p := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &app{stepSel: -1, settings: defaultSettings()}
	a.files = []fileInfo{statFile(p)}
	// One AI step: template=<AI:…><.EXT>
	r := step("ai")
	r.tpl = "<AI:视内容起个名字><.EXT>"
	a.steps = []guiRule{r}
	if !a.hasAIStep() {
		t.Fatal("AI 步骤未被识别")
	}

	tt := ui.NewTester(a.view, 1280, 720)

	// Click the preview button. It must return promptly (async), not block for
	// the 1.2s model call.
	start := time.Now()
	if err := tt.Click("预览(AI 联网命名)"); err != nil {
		t.Fatalf("点击预览: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("点击预览阻塞了 UI %v(应异步返回)", elapsed)
	}

	// While the model is thinking we should be busy and showing feedback.
	if !a.busyAI {
		t.Fatal("点击后应进入 busyAI 状态")
	}
	tt.Frame() // repaint to pick up the busy UI
	texts := strings.Join(tt.Texts(), "\n")
	if !strings.Contains(texts, "正在联网命名") {
		t.Errorf("忙碌时应显示等待文案,实际界面文字: %q", texts)
	}

	// Wait for the background call to finish (poll up to ~5s).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !a.busyAI && atomic.LoadInt32(&hits) >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if a.busyAI {
		t.Fatal("模型回答后 busyAI 未复位(界面会一直卡住'联网中')")
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("模型根本没有被调用")
	}

	// The AI name should now be in the ops, not a raw <AI:…> placeholder.
	got := ""
	for _, op := range a.ops {
		if strings.HasSuffix(op.NewName, ".txt") {
			got = op.NewName
		}
	}
	if strings.Contains(got, "<AI:") {
		t.Fatalf("预览结果仍含 AI 占位符: %q", got)
	}
	if !strings.Contains(got, "慢慢来") {
		t.Fatalf("预期 AI 起的名字含'慢慢来',实际 %q", got)
	}
}

// TestAddFilesWithAIStepDoesNotAutoQuery reproduces the third reported bug:
// after clicking the AI preview button (and even after clearing the list),
// adding any file immediately kicked off another round of AI queries and
// froze the UI. addPaths must not auto-preview when there is an AI step —
// the AI call only runs from the explicit button.
func TestAddFilesWithAIStepDoesNotAutoQuery(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(800 * time.Millisecond) // a slow call that would freeze the UI if awaited
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "名字"}}},
		})
	}))
	defer srv.Close()

	core.SetAIConfig(srv.URL, "test-key", "mock-model")
	defer core.SetAIConfig("", "", "")

	dir := t.TempDir()
	p1 := filepath.Join(dir, "one.txt")
	p2 := filepath.Join(dir, "two.txt")
	os.WriteFile(p1, []byte("a"), 0o644)
	os.WriteFile(p2, []byte("b"), 0o644)

	a := &app{stepSel: -1, settings: defaultSettings()}
	r := step("ai")
	r.tpl = "<AI:视内容起个名字><.EXT>"
	a.steps = []guiRule{r}

	// Adding a file with an AI step must return immediately and must NOT
	// trigger any model request or block on the network.
	start := time.Now()
	a.addPaths([]string{p1})
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Fatalf("添加文件时阻塞了 %v(含 AI 步骤不应自动预览)", elapsed)
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("添加文件不应自动发起 AI 查询,实际请求了 %d 次", n)
	}
	if a.busy || a.busyAI {
		t.Fatal("添加文件后不应处于忙碌状态")
	}

	// Simulate the reported sequence: clear the list, then add another file.
	// Still must not start a query.
	a.files = nil
	a.addPaths([]string{p2})
	time.Sleep(50 * time.Millisecond)
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("清空后再加文件仍不应自动查询,实际 %d 次", n)
	}
}
