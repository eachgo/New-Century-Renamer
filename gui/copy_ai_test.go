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

	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// 底栏成功文案不能带"失败 0"。
//
// 用户扫底栏时只认"失败"两个字——见到就要去找是哪几个文件出了问题。
// 一次干净的成功里没有失败,就不该提这两个字;以前一律写"成功 N，失败 0"，
// 读起来像"出了 0 个错"。
func TestCleanApplyNeverSaysFailedZero(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	for _, n := range []string{"A.TXT", "B.TXT"} {
		writeTestFile(t, filepath.Join(dir, n), "x")
	}
	a.addPaths([]string{filepath.Join(dir, "A.TXT"), filepath.Join(dir, "B.TXT")})
	a.steps = []guiRule{step("case.lower")}
	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	a.apply()
	if strings.Contains(a.status, "失败") {
		t.Fatalf("一次干净的成功不该提「失败」,实测: %q", a.status)
	}
	if !strings.Contains(a.status, "成功 2") {
		t.Fatalf("应当说清成功几个,实测: %q", a.status)
	}
}

// 真有失败时必须报数——把"失败 0"去掉不能顺手把失败也吞了。
func TestApplyWithFailureStillCounts(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "j.json")
	// 手搓计划:一个能成、一个目标已被占住。不用规则造,是因为"目标已存在"
	// 这种失败要看磁盘,而不是看规则算出来的名字。
	writeTestFile(t, filepath.Join(dir, "a.txt"), "x")
	writeTestFile(t, filepath.Join(dir, "b.txt"), "x")
	writeTestFile(t, filepath.Join(dir, "taken.txt"), "x")
	a.ops = []core.RenameOp{
		{Dir: dir, OldName: "a.txt", NewName: "A2.txt"},
		{Dir: dir, OldName: "b.txt", NewName: "taken.txt"},
	}
	a.files = []fileInfo{
		statFile(filepath.Join(dir, "a.txt")),
		statFile(filepath.Join(dir, "b.txt")),
	}
	a.apply()
	if !strings.Contains(a.status, "失败 1") {
		t.Fatalf("有失败时要报出来,实测: %q", a.status)
	}
	if !strings.Contains(a.status, "成功 1") {
		t.Fatalf("成功的那一个也要报,实测: %q", a.status)
	}
}

// addPathsNote 的三条提示各自只在真发生时才出现。
//
// 抽成纯函数就是为了能这样测:状态行是拼出来的字符串,而"哪些该说"
// 是这里的判断。子文件夹是设计选择(只加一层),必须说,否则用户会以为
// 坏了;读不到的目录和找不到的路径是真问题,更要报。
func TestAddPathsNoteSaysWhatWasSkipped(t *testing.T) {
	cases := []struct {
		sub, unreadable, missing int
		want                     []string
		notWant                  []string
	}{
		{0, 0, 0, nil, []string{"跳过", "读不到", "不存在"}},
		{3, 0, 0, []string{"跳过 3 个子文件夹"}, []string{"读不到", "不存在"}},
		{0, 2, 0, []string{"2 个文件夹读不到"}, []string{"跳过", "不存在"}},
		{0, 0, 5, []string{"5 个路径不存在"}, []string{"跳过", "读不到"}},
		{1, 1, 1, []string{"跳过 1 个", "1 个文件夹读不到", "1 个路径不存在"}, nil},
	}
	for _, c := range cases {
		got := addPathsNote(c.sub, c.unreadable, c.missing)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("addPathsNote(%d,%d,%d) = %q,缺少 %q", c.sub, c.unreadable, c.missing, got, w)
			}
		}
		for _, w := range c.notWant {
			if strings.Contains(got, w) {
				t.Errorf("addPathsNote(%d,%d,%d) = %q,不该出现 %q", c.sub, c.unreadable, c.missing, got, w)
			}
		}
	}
}

// 真的拖一个含子文件夹的目录进来:状态行要说"跳过了几个子文件夹"。
//
// 上面那条测的是拼字符串;这条测的是 addPaths 真的把数字传下去了——
// 传成 0 的话状态行干干净净,用户以为全都加进来了。
func TestAddFolderReportsSkippedSubdirs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "子文件夹")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(sub, "deep.txt"), "x")
	writeTestFile(t, filepath.Join(dir, "top.txt"), "x")

	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "j.json")
	a.addPaths([]string{dir})
	if len(a.files) != 1 {
		t.Fatalf("只加一层:应当只有 top.txt,实测 %d 个", len(a.files))
	}
	if !strings.Contains(a.status, "跳过 1 个子文件夹") {
		t.Fatalf("状态行没说跳过了子文件夹: %q", a.status)
	}
}

// AI 接口的默认值必须留空。
//
// core.aiConfig() 的优先级是 override > 环境变量 > 内置默认。GUI 的
// defaultSettings() 一旦写死 Base,override 就非空,NCREAMER_AI_BASE
// 在 GUI 里彻底失效——用户在 shell 里配好的地址永远用不上。
func TestAIDefaultsStayEmptySoEnvWins(t *testing.T) {
	s := defaultSettings()
	if s.AI.Base != "" {
		t.Errorf("默认接口地址必须留空(否则压掉 NCREAMER_AI_BASE),实测 %q", s.AI.Base)
	}
	if s.AI.Model != "" {
		t.Errorf("默认模型必须留空(否则压掉 NCREAMER_AI_MODEL),实测 %q", s.AI.Model)
	}
}

// 环境变量在 GUI 里真的能生效:默认设置 Apply 之后 override 全是空的,
// core 于是读到 NCREAMER_AI_BASE/KEY,请求真的发到那个地址。
//
// 以前 defaultSettings() 写死 OpenAI 的 Base,override 永远非空,shell 里
// 配好的地址在 GUI 里根本用不上——这条测试就是那次事故的现场:把假接口
// 的地址塞进环境变量,看请求有没有打到它。
func TestEnvAIBaseIsUsedByTheGUI(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "环境变量起的名字"}}},
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("NCREAMER_AI_BASE", srv.URL)
	t.Setenv("NCREAMER_AI_KEY", "env-key")
	t.Setenv("NCREAMER_AI_MODEL", "env-model")
	t.Cleanup(func() { core.SetAIConfig("", "", "") })

	dir := t.TempDir()
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "j.json")
	a.ApplySettings()
	if !core.AIConfigured() {
		t.Fatal("环境变量里有 key,却报没配置")
	}
	writeTestFile(t, filepath.Join(dir, "a.mp3"), "x")
	a.files = []fileInfo{statFile(filepath.Join(dir, "a.mp3"))}
	a.steps = []guiRule{step("tpl")}
	a.steps[0].tpl = "<AI:起名>-<SEQ>"
	a.preview()

	if atomic.LoadInt32(&hits) == 0 {
		t.Fatal("请求没有发到环境变量里的地址")
	}
	if len(a.ops) != 1 || !strings.Contains(a.ops[0].NewName, "环境变量起的名字") {
		t.Fatalf("没拿到假接口给的答案: %+v", a.ops)
	}
}

// 设置面板里的占位符必须和 core 的默认值同源。
//
// 以前占位符写死 DeepSeek,而默认值是 OpenAI——照提示填一半的人会拿到
// 一个 key 与地址不匹配的配置。现在两边都来自 core 的常量。
//
// 这两个字段没有 Label(标签是左边的独立文字),所以只能按 Placeholder
// 认,不能 AccessByRole(Name)。
func TestAIFieldPlaceholdersMatchCoreDefaults(t *testing.T) {
	_, tt := settingsApp(t)
	var phs []string
	for _, ai := range tt.AccessAll() {
		if ai.Role == ui.RoleTextField && ai.Placeholder != "" {
			phs = append(phs, ai.Placeholder)
		}
	}
	joined := strings.Join(phs, "|")
	if !strings.Contains(joined, core.DefaultAIBase+"（留空用这个）") {
		t.Errorf("接口地址占位符应与 core.DefaultAIBase 同源,实测: %q", phs)
	}
	if !strings.Contains(joined, core.DefaultAIModel+"（留空用这个）") {
		t.Errorf("模型占位符应与 core.DefaultAIModel 同源,实测: %q", phs)
	}
	// 内置默认地址之外不该再有第二个地址:写死 DeepSeek 的那版就是这里露馅的。
	if strings.Contains(joined, "deepseek") {
		t.Errorf("占位符里还留着 DeepSeek,与默认值不同源: %q", phs)
	}
}

// 长参数胶囊必须是单行:两行会把整行步骤撑高,列表节奏全乱。
//
// 守的是"注释与实现别脱节"——这里的注释早就写了"SingleLine 才能让
// Ellipsis 生效",而代码一度没有调它。
func TestParamChipStaysOnOneLine(t *testing.T) {
	src := mustRead(t, "main.go")
	// 两处(流程行与普通行)都必须是 MaxWidth + SingleLine + Shrink 三件套。
	if n := strings.Count(src, ".MaxWidth(150).SingleLine().Shrink(1).Ellipsis"); n < 2 {
		t.Fatalf("参数胶囊应当是单行可省略,实测只有 %d 处三件套", n)
	}
	if strings.Contains(src, ".MaxWidth(150).MaxLines(2)") {
		t.Error("参数胶囊还在用 MaxLines(2),长参数会折成两行")
	}
}
