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

// slowAIApp 造一个"有 AI 步骤、模型很慢"的 app。delay 决定每个请求等多久,
// 用来模拟本地大模型那种"一次几十秒"的等待。
func slowAIApp(t *testing.T, delay time.Duration, files ...string) (*app, *int32, *int32) {
	t.Helper()
	var hits, finished int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		select {
		case <-time.After(delay):
			atomic.AddInt32(&finished, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"content": "起好的名字"}}},
			})
		case <-r.Context().Done():
			// 客户端放手了:真实接口在这里也会立刻断开
		}
	}))
	t.Cleanup(srv.Close)
	core.SetAIConfig(srv.URL, "test-key", "mock-model")
	t.Cleanup(func() { core.SetAIConfig("", "", "") })

	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings(), stepScrollTo: -1, previewErrStep: -1}
	dir := t.TempDir()
	for _, n := range files {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		a.files = append(a.files, statFile(p))
	}
	r := step("ai")
	r.tpl = "<AI:起名><.EXT>"
	a.steps = []guiRule{r}
	return a, &hits, &finished
}

// 联网命名跑起来之后，三个编辑入口都必须关掉：这时候改步骤或往里塞文件，
// 改的是"已经在算的那份计划"的另一半，而算完的 a.ops 还是旧的。
//
// 左栏、中栏、右栏各取一个代表：目录条目、步骤行、添加文件按钮。
func TestBusyDisablesEditingEntrances(t *testing.T) {
	a, _, _ := slowAIApp(t, 2*time.Second, "a.mp3")
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	if err := tt.Click("预览（AI 联网命名）"); err != nil {
		t.Fatalf("点击预览: %v", err)
	}
	tt.Frame()
	if !a.busyAI {
		t.Fatal("点击后应进入忙碌态")
	}

	for _, name := range []string{"筛选文件", "添加文件…"} {
		info, ok := tt.Access(name)
		if !ok {
			t.Fatalf("界面上找不到 %q", name)
		}
		if !info.Disabled {
			t.Errorf("忙碌时 %q 仍然可以点:这时候改动会算在旧计划上", name)
		}
	}
}

// 「停止」是长任务唯一的出口：没有它，用户只能杀进程，而那会把已经算出来的
// 名字一起丢掉。它必须真的能按，而且要真的让在飞的请求立刻放手。
func TestStopCancelsInFlightAIRequests(t *testing.T) {
	a, hits, finished := slowAIApp(t, 1500*time.Millisecond, "a.mp3", "b.mp3", "c.mp3")
	_ = hits
	_ = finished
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()

	if err := tt.Click("预览（AI 联网命名）"); err != nil {
		t.Fatalf("点击预览: %v", err)
	}
	tt.Frame()

	stop, ok := tt.Access("停止")
	if !ok {
		t.Fatalf("联网命名时必须有「停止」,界面文字: %v", tt.Texts())
	}
	if stop.Disabled {
		t.Fatal("「停止」不该是灰的:它是这段时间唯一的出口")
	}

	start := time.Now()
	if err := tt.Click("停止"); err != nil {
		t.Fatalf("点击停止: %v", err)
	}
	// 三个请求各要 1.5 秒，取消之后应当在一秒内收尾。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !a.busyAI && !a.busy {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if a.busyAI || a.busy {
		t.Fatalf("停止后仍然停在忙碌态(等了 %v)", time.Since(start))
	}
	if n := atomic.LoadInt32(finished); n > 0 {
		t.Fatalf("停止后还有 %d 个请求跑完了,说明取消没有传到网络层", n)
	}
	if n := atomic.LoadInt32(hits); n == 0 {
		t.Fatal("停止之前应当已经发出过请求")
	}
}

// 停止不等于失败：底栏不能写成"3 个有问题"，那会把人赶去查一个一直好着的网络。
func TestStopDoesNotLookLikeAnError(t *testing.T) {
	a, _, _ := slowAIApp(t, 1500*time.Millisecond, "a.mp3", "b.mp3")
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if err := tt.Click("预览（AI 联网命名）"); err != nil {
		t.Fatalf("点击预览: %v", err)
	}
	tt.Frame()
	if err := tt.Click("停止"); err != nil {
		t.Fatalf("点击停止: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && a.busy {
		time.Sleep(20 * time.Millisecond)
	}
	tt.Frame()

	texts := strings.Join(tt.Texts(), "\n")
	if strings.Contains(texts, "个有问题") {
		t.Fatalf("停止被写成了失败: %q", texts)
	}
	// 行里的结果词同样不能写"失败":底栏说"已停止"、每行却标红,
	// 用户还是会以为这批文件出了问题。
	if strings.Contains(texts, "\n失败\n") || strings.HasSuffix(texts, "失败") {
		t.Fatalf("停止后每行仍标成失败: %q", texts)
	}
	if !strings.Contains(texts, "已停止") {
		t.Fatalf("停止后底栏该说已停止,实际: %q", texts)
	}
}

// 已经算出来的名字不能因为"停止"就作废：那一部分是真花时间换来的。
func TestStopKeepsNamesAlreadyComputed(t *testing.T) {
	a, _, _ := slowAIApp(t, 1500*time.Millisecond, "a.mp3")
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if err := tt.Click("预览（AI 联网命名）"); err != nil {
		t.Fatalf("点击预览: %v", err)
	}
	tt.Frame()
	if err := tt.Click("停止"); err != nil {
		t.Fatalf("点击停止: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && a.busy {
		time.Sleep(20 * time.Millisecond)
	}

	// 这一轮一个名字都没算出来（请求还在飞），但计划必须已经落地：
	// 停在"没有任何计划"上，用户点执行时会发现无事可做却不知道为什么。
	if a.planSpec == "" {
		t.Fatal("停止后应当留下一份(可能是空的)计划,否则界面停在上一轮")
	}
	tt.Frame()
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), "已停止") {
		t.Fatalf("停止后没有说明,界面: %v", tt.Texts())
	}
}
