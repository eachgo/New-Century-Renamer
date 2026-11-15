package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// aiBudgetApp 造一个"有 AI 步骤、真的会改名字"的 app,并起一个假接口。
// 返回的 hits 是假接口收到的请求数,用来证明提示里的数字和真实请求一致。
func aiBudgetApp(t *testing.T, files ...string) (*app, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "新名"}}},
		})
	}))
	t.Cleanup(srv.Close)
	core.SetAIConfig(srv.URL, "test-key", "mock-model")
	t.Cleanup(func() { core.SetAIConfig("", "", "") })

	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "j.json")
	a.steps = []guiRule{step("tpl")}
	a.steps[0].tpl = "<AI:起名>-<SEQ>"
	dir := t.TempDir()
	for _, n := range files {
		a.files = append(a.files, fileInfo{path: filepath.Join(dir, n), size: 10})
	}
	a.preview()
	return a, &hits
}

// 执行按钮旁边要把这次会发多少次请求说出来。
func TestApplyButtonSaysHowManyAICalls(t *testing.T) {
	a, _ := aiBudgetApp(t, "a.mp3", "b.mp3", "c.mp3")
	if n := a.aiCallCount(); n != 3 {
		t.Fatalf("三个文件应估 3 次请求,实际 %d", n)
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("将调用 AI 3 次") {
		t.Fatalf("执行前应显示 AI 请求数,实测文本: %v", tt.Texts())
	}
}

// 没有 AI 步骤时不显示这条——凭空出现"0 次"只会让人以为坏了。
func TestNoAIBudgetHintWithoutAIStep(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "j.json")
	a.steps = []guiRule{step("case.lower")}
	dir := t.TempDir()
	a.files = append(a.files, fileInfo{path: filepath.Join(dir, "ABC.mp3"), size: 10})
	a.preview()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	if tt.HasText("将调用 AI") {
		t.Fatal("没有 AI 步骤时不该出现请求数提示")
	}
}

// 提示里的数字必须和真实请求数一致,否则用户按它估算等待时间是错的。
func TestAIBudgetHintMatchesRealRequestCount(t *testing.T) {
	a, hits := aiBudgetApp(t, "a.mp3", "b.mp3")
	guessed := a.aiCallCount()
	if guessed != 2 {
		t.Fatalf("应估 2 次,实际 %d", guessed)
	}
	if got := int(atomic.LoadInt32(hits)); got != guessed {
		t.Fatalf("提示说 %d 次,假接口实际收到 %d 次", guessed, got)
	}
}
