package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// <AI:指示> variables: send the filename plus a content snippet to an
// OpenAI-compatible chat endpoint and get back a filename-safe fragment.
// Config via env: NCREAMER_AI_BASE / NCREAMER_AI_KEY / NCREAMER_AI_MODEL.
// Without a key, AI variables fail soft (placeholder stays, error shown).

// settings can override the environment (SetAIConfig), so the GUI's
// settings dialog works without requiring env vars.
var aiOverrideBase, aiOverrideKey, aiOverrideModel string

// SetAIConfig overrides the NCREAMER_AI_* environment variables. Empty
// fields fall back to the environment.
func SetAIConfig(base, key, model string) {
	aiOverrideBase, aiOverrideKey, aiOverrideModel = base, key, model
}

func aiConfig() (base, key, model string, ok bool) {
	base, key, model = aiOverrideBase, aiOverrideKey, aiOverrideModel
	if base == "" {
		base = os.Getenv("NCREAMER_AI_BASE")
	}
	if key == "" {
		key = os.Getenv("NCREAMER_AI_KEY")
	}
	if model == "" {
		model = os.Getenv("NCREAMER_AI_MODEL")
	}
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return base, key, model, key != ""
}

func aiGenerate(path, instruction string) (string, error) {
	base, key, model, ok := aiConfig()
	if !ok {
		return "", fmt.Errorf("未设置 NCREAMER_AI_KEY")
	}
	snippet := contentSnippet(path)
	user := fmt.Sprintf("文件名: %s\n文件类型: %s\n%s\n文件内容片段:\n%s",
		filepath.Base(path), strings.TrimPrefix(filepath.Ext(path), "."), instruction, snippet)
	body, _ := json.Marshal(map[string]any{
		"model":       model,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": "你是文件更名助手。根据用户给出的文件名、类型、内容片段和指示,只输出一个适合用作文件名的简短文本片段(不要引号、不要解释、不要文件扩展名、不要换行,保留关键信息,可用中文)。"},
			{"role": "user", "content": user},
		},
	})
	url := strings.TrimRight(base, "/") + "/chat/completions"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("AI 接口 %d: %s", resp.StatusCode, truncate(string(rb), 200))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("AI 返回空")
	}
	s := strings.TrimSpace(out.Choices[0].Message.Content)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\"", "")
	return sanitizeFragment(s), nil
}

// contentSnippet reads a text-ish prefix of the file for the model.
// For unknown binaries we send only the name and type.
func contentSnippet(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	var texty = map[string]bool{
		"txt": true, "md": true, "log": true, "csv": true, "json": true,
		"srt": true, "ass": true, "vtt": true, "html": true, "htm": true, "xml": true,
	}
	if !texty[ext] {
		return "(二进制文件, 无文本片段)"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(b) > 6000 {
		b = b[:6000]
	}
	return string(b)
}

func sanitizeFragment(s string) string {
	var b strings.Builder
	for _, r := range s {
		if fw, ok := reserved[r]; ok {
			b.WriteRune(fw)
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	return truncate(out, 80)
}

func truncate(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}
