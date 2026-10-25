package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"reamer/core"
)

func newTestFile(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func step(id string) guiRule {
	d, ok := opByID(id)
	if !ok {
		panic("unknown op " + id)
	}
	r := d.make()
	r.opID, r.opLabel = d.id, d.label
	return r
}

// The view runs without a window in tests, which click as a user would.
func TestPreviewFlowTextReplace(t *testing.T) {
	p := newTestFile(t, "旧名字.txt")
	r := step("replace")
	r.oldStr, r.newStr = "旧", "新"
	a := &app{files: []fileInfo{statFile(p)}, steps: []guiRule{r}}
	tt := ui.NewTester(a.view, 1280, 720)
	if err := tt.Click("预览"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("新名字.txt") {
		t.Errorf("预览表格未出现新名字, texts %q", tt.Texts())
	}
	if n := countChanges(a.ops); n != 1 {
		t.Errorf("countChanges = %d, want 1", n)
	}
}

// Structured steps preview live as soon as the view is built — no button.
func TestLivePreviewStructuredSteps(t *testing.T) {
	p := newTestFile(t, "旧名字.txt")
	d := step("delete")
	d.pos, d.count = 2, 15 // 从第 2 个字符起删 15 个
	a := &app{files: []fileInfo{statFile(p)}, steps: []guiRule{d}}
	ui.NewTester(a.view, 1280, 720)
	if len(a.ops) != 1 || a.ops[0].Err != nil {
		t.Fatalf("live preview did not run: %+v", a.ops)
	}
	// delete works on the whole name: 旧名字.txt from index 1 -> "旧"
	if a.ops[0].NewName != "旧" {
		t.Errorf("NewName = %q, want 旧", a.ops[0].NewName)
	}
}

// Clicking a catalog entry in the sidebar appends a step.
func TestCatalogAddsStep(t *testing.T) {
	a := &app{stepSel: -1}
	tt := ui.NewTester(a.view, 1280, 720)
	if err := tt.Click("文件名转拼音"); err != nil {
		t.Fatal(err)
	}
	if len(a.steps) != 1 || a.steps[0].opID != "pinyin" {
		t.Fatalf("sidebar click did not add step: %+v", a.steps)
	}
}

func TestStepSpecs(t *testing.T) {
	d := step("delete")
	d.pos, d.count = 2, 15
	if got := d.spec(); got != "delete=1,15" {
		t.Errorf("delete spec = %q", got)
	}
	c := step("case.lower")
	if got := c.spec(); got != "case=lower" {
		t.Errorf("case spec = %q", got)
	}
	s := step("seq")
	if got := s.spec(); got != "seq=start:1,width:2,step:1,base:digit,case:upper,group:none" {
		t.Errorf("seq spec = %q", got)
	}
	m := step("meta.author")
	if got := m.spec(); got != "template=<META:author> - <META:title><.EXT>" {
		t.Errorf("meta spec = %q", got)
	}
}

func TestCatalogCoversAllGroups(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range catalog {
		seen[d.group] = true
		if d.group == "高级" && d.act != "" {
			continue // action entries (import-csv, settings) build no step
		}
		r := d.make()
		if r.spec() == "" && d.id != "ext.del" {
			t.Errorf("op %s renders empty spec", d.id)
		}
	}
	for _, g := range []string{"转换", "大小写", "替换", "结构", "扩展名", "编号", "元数据", "日期", "智能", "高级"} {
		if !seen[g] {
			t.Errorf("missing group %s", g)
		}
	}
}

func TestParseRulesViaCore(t *testing.T) {
	if _, err := core.ParseRules([]string{"delete=1,15", "case=upper", "seq=1,2,1"}); err != nil {
		t.Fatal(err)
	}
}

func TestFilterStepSpec(t *testing.T) {
	r := step("filter")
	r.pattern, r.invert = "*.mp3", true
	if got := r.fullSpec(); got != "filter=*.mp3,invert" {
		t.Errorf("fullSpec = %q", got)
	}
	if _, err := core.ParseRules([]string{r.fullSpec()}); err != nil {
		t.Fatal(err)
	}
}

// The user's worked example, driven through GUI step specs.
func TestGuiPipelineExample(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"abc.JPGE", "ddd.mp3", "EEE.jpg"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("x"), 0o644)
		files = append(files, p)
	}
	steps := []guiRule{
		step("filter.out"), step("scope.ext"), step("ext"), step("case.lower"),
		step("unfilter"), step("scope.name"), step("case.lower"), step("case.title"),
	}
	steps[0].pattern = "*.mp3"
	steps[2].newExt = "jpg"
	a := &app{files: []fileInfo{statFile(files[0]), statFile(files[1]), statFile(files[2])}, steps: steps}
	ui.NewTester(a.view, 1280, 720)
	want := []string{"Abc.jpg", "Ddd.mp3", "Eee.jpg"}
	for i, w := range want {
		if a.ops[i].NewName != w {
			t.Errorf("file %d: got %q want %q", i, a.ops[i].NewName, w)
		}
	}
}

func TestSeqStepOptions(t *testing.T) {
	r := step("seq")
	r.numBase, r.numCase, r.numGroup, r.numTpl = "字母", "小写", "按扩展名", "第<N>话_<NAME><.EXT>"
	spec := r.fullSpec()
	if _, err := core.ParseRules([]string{spec}); err != nil {
		t.Fatalf("spec %q: %v", spec, err)
	}
	r2 := step("ins.end")
	r2.text = "_完"
	if got := r2.spec(); got != "insert=end,_完" {
		t.Errorf("ins.end spec = %q", got)
	}
}

// Settings persist, export to a chosen path, and import back — the
// extensibility contract for future settings fields.
func TestSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := defaultSettings()
	s.AI.Base = "https://api.deepseek.com/v1"
	s.AI.Model = "deepseek-chat"
	s.AI.Key = "sk-test"
	s.Journal = "custom-journal.json"
	p := filepath.Join(dir, "exported.json")
	if err := ExportSettingsTo(p, s); err != nil {
		t.Fatal(err)
	}
	got, err := ImportSettingsFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.AI.Base != s.AI.Base || got.AI.Key != s.AI.Key ||
		got.AI.Model != s.AI.Model || got.Journal != s.Journal {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

// The catalog's 高级 group carries the CSV-rename and settings actions.
func TestAdvancedActions(t *testing.T) {
	acts := map[string]bool{}
	for _, d := range catalog {
		if d.group == "高级" && d.act != "" {
			acts[d.act] = true
		}
	}
	if !acts["import-csv"] || !acts["settings"] {
		t.Fatalf("missing advanced actions: %v", acts)
	}
}

// Clicking 设置 opens the modal (not a step).
func TestSettingsEntryOpensDialog(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 1600) // tall window: 高级 group is at the bottom
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.settingsOpen {
		t.Fatal("settings dialog did not open")
	}
	if len(a.steps) != 0 {
		t.Fatalf("设置 wrongly added a step: %+v", a.steps)
	}
}

func TestMain(m *testing.M) {
	inTests = true
	os.Exit(m.Run())
}

// The derived theme keeps WCAG text contrast on both appearances, and
// actually follows the seed hue.
func TestDerivedThemeQuality(t *testing.T) {
	for _, hex := range []string{"#2F7A52", "#F2703C", "#3140A8", "#8A2BE2"} {
		dt := themeFor(ui.Hex(hex))
		for name, th := range map[string]*ui.Theme{"light": dt.light, "dark": dt.dark} {
			if r := contrastRatio(th.Text, th.Background); r < 4.5 {
				t.Errorf("%s seed %s: text contrast %.2f < 4.5", name, hex, r)
			}
			if r := contrastRatio(th.TextMuted, th.Background); r < 2.5 {
				t.Errorf("%s seed %s: muted contrast %.2f < 2.5", name, hex, r)
			}
		}
	}
	// different seeds give visibly different backgrounds
	green := themeFor(ui.Hex("#2F7A52")).light
	orange := themeFor(ui.Hex("#F2703C")).light
	_ = orange
	if green.Background == orange.Background {
		t.Error("background does not follow the seed")
	}
	// caching returns the same instance
	if themeFor(ui.Hex("#2F7A52")).light != green {
		t.Error("theme cache miss")
	}
}

// OKLCH round trip: parse a color, keep its hue.
func TestOklchHue(t *testing.T) {
	_, _, h := colorToOklch(ui.Hex("#F2703C")) // orange
	if h < 20 || h > 70 {
		t.Errorf("orange hue = %.1f, want ~40", h)
	}
	_, _, h2 := colorToOklch(ui.Hex("#2F7A52")) // green
	if h2 < 100 || h2 > 180 {
		t.Errorf("green hue = %.1f, want ~150", h2)
	}
}

// Theme settings persist through export/import like everything else.
func TestThemeSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := defaultSettings()
	s.Theme.Mode = "深色"
	s.Theme.Seed = "#F2703C"
	p := filepath.Join(dir, "s.json")
	if err := ExportSettingsTo(p, s); err != nil {
		t.Fatal(err)
	}
	got, err := ImportSettingsFrom(p)
	if err != nil || got.Theme.Mode != "深色" || got.Theme.Seed != "#F2703C" {
		t.Fatalf("theme round trip: %+v err=%v", got.Theme, err)
	}
}

// Krug round: clear needs a second click; rollback is disabled without a
// journal; the AI preview button only exists for AI steps.
func TestClearNeedsConfirm(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.files = []fileInfo{statFile(newTestFile(t, "a.txt"))}
	tt := ui.NewTester(a.view, 1280, 720)
	if err := tt.Click("清空列表"); err != nil {
		t.Fatal(err)
	}
	if len(a.files) != 1 {
		t.Fatal("第一次点击就清空了")
	}
	if !a.confirmClear {
		t.Fatal("未进入确认状态")
	}
	if err := tt.Click("确认清空?"); err != nil {
		t.Fatal(err)
	}
	if len(a.files) != 0 {
		t.Fatal("第二次点击未清空")
	}
}

func TestRollbackDisabledWithoutJournal(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	if a.settings.Journal != "" && !fileExistsFor(a.settings.Journal) {
		// default journal likely absent in cwd of tests; just check the helper
	}
	a.settings.Journal = filepath.Join(t.TempDir(), "nope.json")
	if a.journalExists() {
		t.Fatal("日志不存在时应不可回滚")
	}
}

func TestPreviewButtonOnlyForAI(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	if a.hasAIStep() {
		t.Fatal("无步骤时不应有 AI 步骤")
	}
	r := step("ai")
	a.steps = []guiRule{r}
	if !a.hasAIStep() {
		t.Fatal("AI 步骤未被识别")
	}
}

func fileExistsFor(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
