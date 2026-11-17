package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// 含 AI 步骤时,改了步骤不会自动重算(重算就要联网),a.ops 于是停在旧
// 步骤算出来的那份计划上。这时候"执行更名"必须点不了:按下去执行的是
// 改之前的计划,行里显示的新名字也不是当前步骤的结果。
//
// 这条测试守的是"闸门真的连着按钮":只测 planStale() 返回 true 是不够的
// ——函数可以是对的,而按钮仍然按 hasChanges() 亮着。
func planStaleApp(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "新名"}}},
		})
	}))
	t.Cleanup(srv.Close)
	core.SetAIConfig(srv.URL, "test-key", "mock-model")
	t.Cleanup(func() { core.SetAIConfig("", "", "") })

	dir := t.TempDir()
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "j.json")
	a.steps = []guiRule{step("tpl")}
	a.steps[0].tpl = "<AI:起名>-<SEQ>"
	writeTestFile(t, filepath.Join(dir, "a.mp3"), "x")
	a.addPaths([]string{filepath.Join(dir, "a.mp3")})
	// addPaths 看到 AI 步骤不会自动预览(见那里的注释),显式算一次。
	a.preview()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	return a, tt
}

func TestStaleAIPlanDisablesApply(t *testing.T) {
	a, tt := planStaleApp(t)
	if a.planStale() {
		t.Fatal("刚算完计划就被判成过期")
	}
	if !a.hasChanges() {
		t.Fatalf("假接口给的新名字应当产生变更,实测 ops=%d", len(a.ops))
	}
	if ai, ok := tt.Access("执行更名"); !ok || ai.Disabled {
		t.Fatalf("计划新鲜时「执行更名」应当能点,access=%+v ok=%v", ai, ok)
	}

	// 改一步:含 AI 不重算,计划立刻过期。
	a.steps[0].tpl = "<AI:换个说法><.EXT>"
	tt.Frame()
	tt.Frame()

	if !a.planStale() {
		t.Fatal("步骤改了却没被判成过期")
	}
	if ai, ok := tt.Access("执行更名"); !ok || !ai.Disabled {
		t.Fatalf("计划过期时「执行更名」必须点不了,access=%+v ok=%v", ai, ok)
	}
	// 灰按钮上的 Tooltip 在 mygo 里不弹(禁用元素不收悬停),所以外面必须有
	// 一句话说清"下一步做什么"。
	if !tt.HasText("步骤改了，计划还是旧的") {
		t.Errorf("计划过期时应在按钮旁说明,实测文本: %v", tt.Texts())
	}

	// Ctrl+Enter 是同一条路的另一个入口,不能绕过闸门。
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	if a.confirmApply {
		t.Error("计划过期时 Ctrl+Enter 仍然挂上了确认态")
	}

	// 点过预览(这里直接调 preview,等价)之后闸门放开。
	a.preview()
	tt.Frame()
	tt.Frame()
	if a.planStale() {
		t.Fatal("重算之后仍然被判成过期")
	}
	if ai, ok := tt.Access("执行更名"); !ok || ai.Disabled {
		t.Fatalf("重算之后「执行更名」应当能点,access=%+v ok=%v", ai, ok)
	}
	if tt.HasText("步骤改了，计划还是旧的") {
		t.Error("重算之后不该再提示计划过期")
	}
}

// 文件清单变了也算过期:含 AI 时 addPaths 同样不重算,计划只覆盖老的那几个,
// 而列表里已经多出一行没有新名字的文件。只比步骤指纹的话,这种情况会被
// 当成"计划没问题",按下去新加的那个文件不会被改。
func TestStaleWhenFilesAddedUnderAIStep(t *testing.T) {
	a, tt := planStaleApp(t)
	if a.planStale() {
		t.Fatal("刚算完计划就被判成过期")
	}
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "b.mp3"), "x")
	a.addPaths([]string{filepath.Join(dir, "b.mp3")})
	tt.Frame()
	tt.Frame()

	if !a.planStale() {
		t.Fatal("加了文件却没被判成过期")
	}
	if ai, ok := tt.Access("执行更名"); !ok || !ai.Disabled {
		t.Fatalf("计划过期时「执行更名」必须点不了,access=%+v ok=%v", ai, ok)
	}
	// 提示要对着真正变了的东西说:文件变了却说"步骤改了",用户会去翻步骤。
	if !tt.HasText("文件变了，计划还是旧的") {
		t.Errorf("加文件导致的过期应当说文件变了,实测文本: %v", tt.Texts())
	}
	if tt.HasText("步骤改了，计划还是旧的") {
		t.Error("只是加了文件,不该说步骤改了")
	}
}

// 没有 AI 步骤时永远不过期:那些改动会立刻触发 preview(),计划和步骤天然
// 同步。这里若也判过期,每改一步都会闪一句"计划还是旧的",而计划其实
// 已经是新的。
func TestNoAIStepNeverStale(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "j.json")
	cs := step("case.lower")
	cs.caseMode = "小写"
	a.steps = []guiRule{cs}
	writeTestFile(t, filepath.Join(dir, "Hello.TXT"), "x")
	a.addPaths([]string{filepath.Join(dir, "Hello.TXT")})
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	// 换成大写:仍会改名,所以"能点"不是因为没变更。
	a.steps[0].caseMode = "大写"
	tt.Frame()
	tt.Frame()
	if a.planStale() {
		t.Fatal("没有 AI 步骤却判成计划过期")
	}
	if tt.HasText("步骤改了，计划还是旧的") {
		t.Error("没有 AI 步骤时不该出现计划过期提示")
	}
	if ai, ok := tt.Access("执行更名"); !ok || ai.Disabled {
		t.Fatalf("计划同步时「执行更名」应当能点,access=%+v ok=%v", ai, ok)
	}
}

// 导入清单之后不该被判成"计划过期"。
//
// 导入的计划不是按步骤算出来的,而是表格里写好的——它就是当前计划。
// 若被判过期,含 AI 步骤的人导入完就点不了执行,而那正是他最需要执行
// 的时刻(表格都改好了)。
//
// 这条还守着一个顺序坑:planFiles 必须记"导入之后"的文件数。写在重建
// a.files 之前的话,记的是导入前的数量,一进来就被判成过期。
func TestImportedPlanIsNotStale(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mp3")
	writeTestFile(t, src, "x")
	csvPath := filepath.Join(dir, "list.csv")
	var buf bytes.Buffer
	if err := core.ExportCSV(&buf, []string{src}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(csvPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "j.json")
	a.steps = []guiRule{step("tpl")}
	a.steps[0].tpl = "<AI:起名>-<SEQ>"
	plan, errs := a.importFrom(csvPath)
	if len(errs) > 0 {
		t.Fatalf("导入报错: %v", errs)
	}
	if len(plan) != 1 || len(a.files) != 1 {
		t.Fatalf("导入后应有 1 行 1 个文件,实测 plan=%d files=%d", len(plan), len(a.files))
	}
	if a.planStale() {
		t.Fatalf("刚导入的计划被判成过期(planSpec=%q specKey=%q planFiles=%d files=%d)",
			a.planSpec, a.specKey(), a.planFiles, len(a.files))
	}
}
