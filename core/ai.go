package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

// aiTimeout is how long to wait for one model reply. Local 7B~9B models can
// take minutes when the snippet is a long PDF, so the default is deliberately
// generous. Override with NCREAMER_AI_TIMEOUT (seconds) or SetAITimeout.
const defaultAITimeout = 300 * time.Second

// aiTimeoutOverride is set by the GUI settings dialog (seconds); 0 means unset.
var aiTimeoutOverride int

// SetAITimeout lets the GUI override the wait time without env vars.
// Zero or negative restores the default/env behavior.
func SetAITimeout(seconds int) {
	if seconds > 0 {
		aiTimeoutOverride = seconds
	} else {
		aiTimeoutOverride = 0
	}
}

func aiTimeout() time.Duration {
	if aiTimeoutOverride > 0 {
		return time.Duration(aiTimeoutOverride) * time.Second
	}
	if s := os.Getenv("NCREAMER_AI_TIMEOUT"); s != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultAITimeout
}

// aiConcurrencyOverride is set by the GUI settings dialog; 0 means unset.
var aiConcurrencyOverride int

// SetAIConcurrency lets the GUI pick how many model requests run in parallel
// without env vars. 0 restores env/default behavior. Cloud APIs rate-limit
// aggressively, so callers often want 1; a local model may be fastest at 1~2
// because it serializes generation anyway.
func SetAIConcurrency(n int) {
	if n > 0 {
		aiConcurrencyOverride = n
	} else {
		aiConcurrencyOverride = 0
	}
}

// defaultAIPrompt is the built-in instruction used when the user has not saved
// their own. The GUI settings dialog can persist a replacement.
const defaultAIPrompt = "根据文件内容起一个简短的名字"

// aiPromptOverride is the user-saved default prompt (GUI settings); empty
// means use the built-in default.
var aiPromptOverride string

// SetDefaultAIPrompt saves the user's preferred prompt for new "AI 按内容命名"
// steps. Empty restores the built-in default.
func SetDefaultAIPrompt(s string) {
	aiPromptOverride = strings.TrimSpace(s)
}

// DefaultAIPrompt returns the instruction a fresh "AI 按内容命名" step should
// carry: the user's saved prompt, or the built-in default.
func DefaultAIPrompt() string {
	if p := strings.TrimSpace(aiPromptOverride); p != "" {
		return p
	}
	return defaultAIPrompt
}

// aiSystemPrompt builds the system message. The core contract (only output the
// name fragment, no quotes/explanation/extension/newlines) is always kept; when
// the user has saved a custom instruction it is appended as their preference so
// the model still gets the hard constraints it must not break.
func aiSystemPrompt() string {
	base := "你是文件更名助手。根据用户给出的文件名、类型、内容片段和指示,只输出一个适合用作文件名的简短文本片段(不要引号、不要解释、不要文件扩展名、不要换行,保留关键信息,可用中文)。"
	if p := strings.TrimSpace(aiPromptOverride); p != "" {
		base += "用户另有要求,须优先遵守: " + p
	}
	return base
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
			{"role": "system", "content": aiSystemPrompt()},
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
	client := &http.Client{Timeout: aiTimeout()}
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
	s = sanitizeFragment(s)
	if s == "" {
		// An empty answer must be an error, not an empty fragment: silently
		// returning "" would let the template collapse to just its extension
		// (e.g. "<AI:…><.EXT>" -> ".md"), renaming the file to a bare ".md".
		return "", fmt.Errorf("AI 返回空内容")
	}
	return s, nil
}

// contentSnippet reads a text-ish prefix of the file for the model.
// Beyond plain text we read what the file has that is legible without OCR:
// PDF text layer, EPUB content documents, archive listings, image EXIF, and
// container metadata (title/artist/album). Reading the content instead of
// guessing from the name is the whole point of handing this to a small
// local model.
func contentSnippet(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	var texty = map[string]bool{
		"txt": true, "md": true, "log": true, "csv": true, "json": true,
		"srt": true, "ass": true, "vtt": true, "html": true, "htm": true, "xml": true,
	}
	var parts []string

	if texty[ext] {
		if s := readPrefix(path, 6000); s != "" {
			parts = append(parts, s)
		}
	} else {
		switch ext {
		case "pdf":
			parts = append(parts, pdfTextLayer(path))
		case "epub":
			parts = append(parts, epubText(path))
		case "zip", "cbz", "jar", "apk", "docx", "xlsx", "pptx":
			parts = append(parts, zipListing(path))
		case "tar", "gz", "tgz", "bz2", "xz", "7z", "rar":
			parts = append(parts, "(压缩包, 无法解出目录: 仅凭文件名和大小推断)")
		case "jpg", "jpeg", "tif", "tiff", "webp", "heic", "png":
			parts = append(parts, exifSummary(path))
		}
	}

	// Container metadata is useful on its own and cheap to add.
	if m := ExtractMeta(path); len(m) > 0 {
		var kv []string
		for _, k := range []string{"title", "artist", "album", "author", "creator", "date", "publisher"} {
			if v, ok := m[k]; ok && v != "" {
				kv = append(kv, k+": "+v)
			}
		}
		if len(kv) > 0 {
			parts = append(parts, "元数据: "+strings.Join(kv, "; "))
		}
	}

	joined := strings.TrimSpace(strings.Join(parts, "\n"))
	if joined == "" {
		// Nothing readable: say so, so the model leans on the name and type.
		return fmt.Sprintf("(无可用文本, 文件大小 %s)", humanSize(path))
	}
	return truncate(joined, 6000)
}

func readPrefix(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b := make([]byte, n)
	m, _ := io.ReadFull(f, b)
	if m <= 0 {
		return ""
	}
	return string(b[:m])
}

func humanSize(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return "未知"
	}
	n := float64(st.Size())
	units := []string{"B", "KB", "MB", "GB"}
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", n, units[i])
	}
	return fmt.Sprintf("%.1f %s", n, units[i])
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
