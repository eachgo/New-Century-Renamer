package main

import (
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

// Reproduces the reported case: the gateway returns 502 (after a short hang).
// The preview must (a) surface the error on the file, and — critically — (b)
// clear busyAI so the UI stops saying "联网命名中…". If busyAI sticks, the
// window looks hung forever even though the request already failed.
func TestPreviewAsync502ClearsBusy(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(300 * time.Millisecond) // gateway "thinking" then 502
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":{"message":"Bad gateway"}}`))
	}))
	defer srv.Close()

	core.SetAIConfig(srv.URL, "test-key", "mock-model")
	defer core.SetAIConfig("", "", "")

	dir := t.TempDir()
	p := filepath.Join(dir, "hello.txt")
	os.WriteFile(p, []byte("hello world"), 0o644)

	a := &app{stepSel: -1, settings: defaultSettings()}
	a.files = []fileInfo{statFile(p)}
	r := step("ai")
	r.tpl = "<AI:视内容起个名字><.EXT>"
	a.steps = []guiRule{r}

	tt := ui.NewTester(a.view, 1280, 720)
	if err := tt.Click("预览(AI 联网命名)"); err != nil {
		t.Fatalf("点击预览: %v", err)
	}
	if !a.busyAI {
		t.Fatal("点击后应进入 busyAI 状态")
	}

	// Wait for the failing call to settle.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !a.busyAI && atomic.LoadInt32(&hits) >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("模型未被调用")
	}
	if a.busyAI {
		t.Fatal("502 失败后 busyAI 未复位(界面会一直显示'联网命名中…')")
	}
	if a.busy {
		t.Fatal("502 失败后 busy 未复位")
	}

	tt.Frame()
	texts := strings.Join(tt.Texts(), "\n")
	if strings.Contains(texts, "联网命名中") {
		t.Fatalf("失败后界面仍显示'联网命名中',文字: %q", texts)
	}
	// The error should be visible somewhere (status or per-file).
	if !strings.Contains(texts, "502") && !strings.Contains(texts, "失败") && !strings.Contains(texts, "有问题") {
		t.Logf("提示:界面上没看到明确的 502/失败字样,文字=%q", texts)
	}
}
