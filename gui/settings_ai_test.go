package main

import (
	"strings"
	"testing"

	"ncreamer/core"
)

// A user-saved default prompt must flow into a freshly created "AI 按内容命名"
// step, so they don't retype it every time. Clearing it restores the built-in.
func TestAIStepUsesSavedPrompt(t *testing.T) {
	defer core.SetDefaultAIPrompt("")

	const custom = "根据文件内容生成一个文件名。只输出文件名,不要解释,不要引号,不要路径,不超过20个字。"
	core.SetDefaultAIPrompt(custom)

	r := step("ai")
	if !strings.Contains(r.tpl, custom) {
		t.Fatalf("AI 步骤未采用保存的提示词,实际 tpl=%q", r.tpl)
	}
	if !strings.HasSuffix(r.tpl, "<.EXT>") {
		t.Fatalf("AI 步骤模板应保留扩展名变量,实际 %q", r.tpl)
	}

	// Clearing restores the built-in default.
	core.SetDefaultAIPrompt("")
	r2 := step("ai")
	if strings.Contains(r2.tpl, custom) {
		t.Fatalf("清空后不应再含自定义提示词,实际 %q", r2.tpl)
	}
	if !strings.Contains(r2.tpl, core.DefaultAIPrompt()) {
		t.Fatalf("清空后应用内置默认 %q,实际 %q", core.DefaultAIPrompt(), r2.tpl)
	}
}

// The settings dialog commit funcs must parse the concurrency/prompt buffers
// the same way the timeout buffer works: bad/empty => default (0).
func TestCommitAIConcurrencyAndPrompt(t *testing.T) {
	a := &app{settings: defaultSettings()}

	a.aiConcurrencyText = "1"
	a.commitAIConcurrency()
	if a.settings.AI.Concurrency != 1 {
		t.Fatalf("并发应解析为 1,实际 %d", a.settings.AI.Concurrency)
	}
	a.aiConcurrencyText = "  "
	a.commitAIConcurrency()
	if a.settings.AI.Concurrency != 0 {
		t.Fatalf("空输入应回落默认(0),实际 %d", a.settings.AI.Concurrency)
	}
	a.aiConcurrencyText = "abc"
	a.commitAIConcurrency()
	if a.settings.AI.Concurrency != 0 {
		t.Fatalf("非法输入应回落默认(0),实际 %d", a.settings.AI.Concurrency)
	}

	a.aiPromptText = "  我的提示词  "
	a.commitAIPrompt()
	if a.settings.AI.Prompt != "我的提示词" {
		t.Fatalf("提示词应去首尾空白,实际 %q", a.settings.AI.Prompt)
	}
}

// ApplySettings must push the concurrency + prompt into the core, so a saved
// value actually takes effect at runtime.
func TestApplySettingsPushesConcurrencyAndPrompt(t *testing.T) {
	defer core.SetAIConcurrency(0)
	defer core.SetDefaultAIPrompt("")

	a := &app{settings: defaultSettings()}
	a.settings.AI.Concurrency = 2
	a.settings.AI.Prompt = "极简命名"
	a.ApplySettings()

	if got := core.AIConcurrency(); got != 2 {
		t.Fatalf("SetAIConcurrency 未生效,实际 %d", got)
	}
	if got := core.DefaultAIPrompt(); got != "极简命名" {
		t.Fatalf("SetDefaultAIPrompt 未生效,实际 %q", got)
	}
}
