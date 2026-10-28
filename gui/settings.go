package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"ncreamer/core"
)

// Settings are everything worth keeping between runs. Designed extensible
// from day one: new fields just join the struct, and import/export keeps
// working (unknown fields in imported files are ignored by json.Unmarshal).
type Settings struct {
	AI struct {
		Base  string `json:"base,omitempty"`
		Key   string `json:"key,omitempty"`
		Model string `json:"model,omitempty"`
		// Timeout 是等待模型回复的秒数;0 表示用默认(300 秒)。
		// 本地慢模型读完长 PDF 常要一两分钟,可在此调大。
		Timeout int `json:"timeout,omitempty"`
		// Concurrency 是批量请求 AI 时同时发出的路数;0 表示用默认(4 路)。
		// 云端 API 常限流,填 1 稳妥;本地模型常是串行生成,填 1~2 也可能更快。
		Concurrency int `json:"concurrency,omitempty"`
		// Prompt 是用户保存的默认提示词,新建「AI 按内容命名」步骤时用它;
		// 空表示用内置默认。
		Prompt string `json:"prompt,omitempty"`
	} `json:"ai"`
	Journal string `json:"journal,omitempty"`
	Theme   struct {
		// 跟随系统 / 浅色 / 深色
		Mode string `json:"mode,omitempty"`
		// Seed is a hex color; empty means follow the desktop accent.
		Seed string `json:"seed,omitempty"`
	} `json:"theme"`
}

const settingsFile = "ncreamer-settings.json"

// settingsPath puts ncreamer-settings.json next to the executable.
func settingsPath() string {
	exe, err := os.Executable()
	if err != nil {
		return settingsFile
	}
	return filepath.Join(filepath.Dir(exe), settingsFile)
}

func defaultSettings() Settings {
	s := Settings{Journal: ".ncreamer-journal.json"}
	s.AI.Base = "https://api.openai.com/v1"
	s.Theme.Mode = "跟随系统"
	return s
}

func LoadSettings() Settings {
	s := defaultSettings()
	b, err := os.ReadFile(settingsPath())
	if err == nil {
		json.Unmarshal(b, &s) // keep defaults for absent fields
	}
	return s
}

func SaveSettings(s Settings) error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(settingsPath(), append(b, '\n'), 0o644)
}

// ApplySettings pushes the settings into the core (AI override) and returns
// the journal path to use.
func (a *app) ApplySettings() {
	a.assignExtColors(a.currentTheme())
	core.SetAIConfig(a.settings.AI.Base, a.settings.AI.Key, a.settings.AI.Model)
	core.SetAITimeout(a.settings.AI.Timeout)
	core.SetAIConcurrency(a.settings.AI.Concurrency)
	core.SetDefaultAIPrompt(a.settings.AI.Prompt)
}

// ExportSettingsTo writes the settings as JSON to an arbitrary path.
func ExportSettingsTo(path string, s Settings) error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// ImportSettingsFrom reads a settings JSON, replacing current settings.
func ImportSettingsFrom(path string) (Settings, error) {
	s := defaultSettings()
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	return s, err
}
