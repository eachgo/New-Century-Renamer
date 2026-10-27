package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 回归测试:AI 变量的结果必须真的进到最终文件名里。
// 曾经的 bug 是缓存 key 大小写/前缀/路径三处不一致,预填了却查不到,
// 模板里留下原样的 <AI:…> 占位符——看起来"AI 起了名",其实没起。
func TestAIVarExpandsIntoName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": "模型给的名字"}},
			},
		})
	}))
	defer srv.Close()

	t.Setenv("NCREAMER_AI_BASE", srv.URL+"/v1")
	t.Setenv("NCREAMER_AI_KEY", "k")
	t.Setenv("NCREAMER_AI_MODEL", "m")
	t.Setenv("NCREAMER_AI_TIMEOUT", "10")

	dir := t.TempDir()
	name := "sample.txt"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("一些内容，供模型参考。"), 0o644); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(dir, name)

	rules := []Rule{templateRule("<AI:根据内容起个名字><.EXT>")}
	plan, errs := RunPipeline(rules, []string{full})
	if len(errs) != 0 {
		t.Fatalf("意外错误: %v", errs)
	}
	if len(plan) != 1 {
		t.Fatalf("期望 1 条计划,实际 %d", len(plan))
	}
	got := plan[0].NewName
	if strings.Contains(got, "<AI:") {
		t.Fatalf("AI 占位符没被替换,仍是 %q", got)
	}
	if got != "模型给的名字.txt" {
		t.Fatalf("期望 模型给的名字.txt,实际 %q", got)
	}
}
