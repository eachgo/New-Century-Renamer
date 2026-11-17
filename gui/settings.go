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
	// Pinned 是用户钉在左栏顶端的目录条目 id,按钉的顺序排。
	//
	// 左栏按"分组 + 类别色"组织:分组是语义(筛选、转换、编号……),
	// 颜色是类别线索,两者都不能为"常用"让路——打散分组之后,用户
	// 得靠记住位置来找东西,比按重要性排更糟。所以不动分组,另开一
	// 个"常用"组放在最上面:用户自己决定哪几条进去,顺序就是他钉的
	// 顺序。
	Pinned []string `json:"pinned,omitempty"`
	// Folded 记住用户手动折叠了哪些目录分组。true = 折叠。
	// 以前折叠状态只在内存里,重启就全展开——用户每天重开一次软件,
	// 就得每天重新折一遍同样的几组。和 Pinned 一样属于"用户调过的
	// 界面偏好",该一起记住。
	Folded map[string]bool `json:"folded,omitempty"`
	// Flows 是用户自己存下来的流程(一串排好的步骤)。内置预设不进这里:
	// 它们每次从 intents 现算,升级后跟着更新,不会在设置文件里留一份
	// 旧快照。
	Flows []flow `json:"flows,omitempty"`
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

// settingsFilePath is settingsPath behind a variable: the import path has to
// back up the file it is about to replace, and a test that exercised that
// against the real location would write next to the test binary.
var settingsFilePath = settingsPath

// defaultJournalPath 把回滚日志放到用户配置目录。
// 以前是相对路径 .ncreamer-journal.json,跟着进程工作目录走:换个目录
// 启动,"回滚上次"就变成不可用(或者更糟——回滚到别处记录里的文件)。
func defaultJournalPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		// 拿不到配置目录就退到可执行文件旁边,至少是稳定的绝对路径
		return filepath.Join(filepath.Dir(settingsPath()), "ncreamer-journal.json")
	}
	return filepath.Join(dir, "ncreamer", "journal.json")
}

func defaultSettings() Settings {
	s := Settings{Journal: defaultJournalPath()}
	// 接口地址/模型默认留空,而不是写死 https://api.openai.com/v1。
	//
	// SetAIConfig 的 override 优先于 NCREAMER_AI_BASE/MODEL,所以默认值
	// 一写死,GUI 里这两个环境变量就彻底失效了——用户按 README 设好
	// Ollama 的地址启动 GUI,连的还是 OpenAI,而且设置面板里看不出是
	// 谁覆盖了谁。留空 = "没意见",由 core 按 环境变量 → 内置默认 的
	// 顺序决定;面板上的占位符说明内置默认是什么。
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
