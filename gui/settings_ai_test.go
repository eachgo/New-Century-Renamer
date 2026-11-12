package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

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

// 同一张表里"取消"必须是一个意思:所有字段都丢弃。
// 之前只有等待秒数/并发/提示词走缓冲,接口地址等直接绑 a.settings,
// 于是取消之后一半改了一半没改,而界面已经关掉了。
func TestSettingsCancelDiscardsEveryField(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.AI.Base = "https://old.example/v1"
	a.settings.AI.Model = "old-model"
	a.settings.Journal = "old.json"
	a.settings.Theme.Mode = "浅色"
	before := a.settings

	a.openSettings()
	// 用户开始乱改
	a.settingsDraft.AI.Base = "https://new.example/v1"
	a.settingsDraft.AI.Model = "new-model"
	a.settingsDraft.Journal = "new.json"
	a.settingsDraft.Theme.Mode = "深色"
	a.aiTimeoutText = "42"
	a.aiConcurrencyText = "7"
	a.aiPromptText = "新提示词"
	a.commitAITimeout()
	a.commitAIConcurrency()
	a.commitAIPrompt()
	a.seedFollow = false
	a.seedPick = ui.Hex("#FF0000")

	a.closeSettings(false)

	if a.settings != before {
		t.Fatalf("取消后设置不该有任何改动:\n before=%+v\n after=%+v", before, a.settings)
	}
	if a.settingsOpen {
		t.Error("取消后对话框应关闭")
	}
	// 缓冲也要回到已保存的值,否则下次打开会显示上一次的残留
	if a.aiTimeoutText != "" || a.aiConcurrencyText != "" || a.aiPromptText != "" {
		t.Errorf("取消后缓冲未复位: timeout=%q conc=%q prompt=%q",
			a.aiTimeoutText, a.aiConcurrencyText, a.aiPromptText)
	}
}

// 保存则必须全部生效——包括原先直绑 a.settings 的那几个字段。
func TestSettingsSaveAppliesEveryField(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.openSettings()
	a.settingsDraft.AI.Base = "https://new.example/v1"
	a.settingsDraft.AI.Model = "new-model"
	a.settingsDraft.Journal = "new.json"
	a.settingsDraft.Theme.Mode = "深色"
	a.aiTimeoutText = "42"
	a.commitAITimeout()
	a.settingsDraft.Theme.Seed = ""
	a.closeSettings(true)

	if a.settings.AI.Base != "https://new.example/v1" {
		t.Errorf("接口地址未生效: %q", a.settings.AI.Base)
	}
	if a.settings.AI.Model != "new-model" {
		t.Errorf("模型未生效: %q", a.settings.AI.Model)
	}
	if a.settings.Journal != "new.json" {
		t.Errorf("日志路径未生效: %q", a.settings.Journal)
	}
	if a.settings.Theme.Mode != "深色" {
		t.Errorf("外观模式未生效: %q", a.settings.Theme.Mode)
	}
	if a.settings.AI.Timeout != 42 {
		t.Errorf("等待秒数未生效: %d", a.settings.AI.Timeout)
	}
}
