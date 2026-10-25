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

var testStepUID int

func step(id string) guiRule {
	d, ok := opByID(id)
	if !ok {
		panic("unknown op " + id)
	}
	r := d.make()
	r.opID, r.opLabel = d.id, d.label
	r.opDesc, r.opGroup, r.opFlow = d.desc, d.group, d.flow
	testStepUID++
	r.opUID = testStepUID
	return r
}

// The view runs without a window in tests, which click as a user would.
func TestPreviewFlowTextReplace(t *testing.T) {
	p := newTestFile(t, "旧名字.txt")
	r := step("replace")
	r.oldStr, r.newStr = "旧", "新"
	a := &app{files: []fileInfo{statFile(p)}, steps: []guiRule{r}}
	ui.NewTester(a.view, 1280, 720) // live preview runs on the first pass
	if len(a.ops) != 1 || a.ops[0].NewName != "新名字.txt" {
		t.Fatalf("live preview: %+v", a.ops)
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

// Fixed two slots: [1]=选取 range (right), [0]=筛选 range (left); zero
// Color = off. 文件名 and 扩展名 take neighboring hues, so switching shows.
func TestStepRangeBars(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	// 1..3 processing
	for _, id := range []string{"case.lower", "pinyin", "truncate"} {
		a.steps = append(a.steps, step(id))
	}
	// 4: 选取文件名; 5..8 processing; 9: 排除文件; 10..11 processing;
	// 12: 选取扩展名(切换); 13: 清除筛选; 14: 恢复整体
	a.steps = append(a.steps, step("scope.name"))
	a.steps = append(a.steps, step("case.lower"), step("case.title"), step("pinyin"), step("truncate"))
	f := step("filter.out")
	f.pattern = "*.tmp"
	a.steps = append(a.steps, f)
	a.steps = append(a.steps, step("case.lower"), step("pinyin"))
	a.steps = append(a.steps, step("scope.ext"))
	a.steps = append(a.steps, step("unfilter"), step("scope.all"))

	dt := themeFor(ui.Hex("#2F7A52"))
	bars := a.stepRangeBars(dt)

	// before 4: both slots off
	if bars[2] != [2]ui.Color{} {
		t.Fatalf("step 3: %v", bars[2])
	}
	// 4..8: right slot = scopeName, left off
	for i := 3; i <= 7; i++ {
		if bars[i][0].A != 0 || bars[i][1] != dt.rangeScopeName {
			t.Fatalf("step %d: %v", i+1, bars[i])
		}
	}
	// 9..12: left = filter, right = scope (name until 11, ext from 12)
	for i := 8; i <= 10; i++ {
		if bars[i][0] != dt.rangeFilter || bars[i][1] != dt.rangeScopeName {
			t.Fatalf("step %d: %v", i+1, bars[i])
		}
	}
	if bars[11][0] != dt.rangeFilter || bars[11][1] != dt.rangeScopeExt {
		t.Fatalf("step 12 (选取扩展名): %v", bars[11])
	}
	// 13 清除筛选: left off, right still ext
	if bars[12][0].A != 0 || bars[12][1] != dt.rangeScopeExt {
		t.Fatalf("step 13: %v", bars[12])
	}
	// 14 恢复整体: both off
	if bars[13] != [2]ui.Color{} {
		t.Fatalf("step 14: %v", bars[13])
	}
}

// GUI-generated specs must survive commas inside regex arguments:
// escArg on the GUI side, splitArgs un-escaping in core.
func TestRegexCommaEndToEnd(t *testing.T) {
	r := step("replace.re")
	r.oldStr, r.newStr, r.regex = `\d{1,3}`, `N`, true
	spec := r.fullSpec()
	rules, err := core.ParseRules([]string{spec})
	if err != nil {
		t.Fatalf("spec %q: %v", spec, err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "ab12cd.txt")
	os.WriteFile(p, []byte("x"), 0o644)
	plan, errs := core.RunPipeline(rules, []string{p})
	for _, e := range errs {
		t.Errorf("pipeline: %v", e)
	}
	if plan[0].NewName != "abNcd.txt" {
		t.Fatalf("got %q want abNcd.txt", plan[0].NewName)
	}
}

// Per-extension colors: common ones are distinct through the pipeline;
// unknown ones get unique hues within the list, both readable on cards.
func TestExtColors(t *testing.T) {
	dt := themeFor(ui.Hex("#2F7A52"))
	a := &app{stepSel: -1, settings: defaultSettings(), files: []fileInfo{}}
	for _, n := range []string{"a.mp3", "b.flac", "c.jpg", "d.png", "e.pdf", "f.epub", "g.zip", "h.mp4", "i.xyz1", "j.zzz9"} {
		a.files = append(a.files, fileInfo{path: n})
	}
	a.assignExtColors(dt)
	// every listed extension has a distinct color
	seen := map[ui.Color]string{}
	for _, e := range []string{"mp3", "flac", "jpg", "png", "pdf", "epub", "zip", "mp4", "xyz1", "zzz9"} {
		c := a.fileColor(dt, e)
		if c == ui.RGB(128, 128, 128) {
			t.Errorf("%s fell back to gray", e)
		}
		if prev, dup := seen[c]; dup {
			t.Errorf("%s shares a color with %s", e, prev)
		}
		seen[c] = e
	}
	// readable against the card ground
	for c := range seen {
		if r := contrastRatio(c, dt.cardBgFor(false)); r < 1.15 {
			t.Errorf("chip vs card contrast too low: %.2f", r)
		}
	}
}

// 多选模式:批量删除与拖动重排
func TestStepsMultiMode(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("pinyin"), step("scope.name"), step("case.lower"), step("truncate")}
	for i := range a.steps {
		a.stepUID++
		a.steps[i].opUID = a.stepUID
	}
	a.listUnlocked = true
	// select steps 1 (pinyin) and 3 (truncate) by uid
	a.chosen.Add(a.steps[0].opUID)
	a.chosen.Add(a.steps[2].opUID)

	tt := ui.NewTester(a.view, 1280, 800)
	if !tt.HasText("已选 2 步") {
		t.Error("多选面板未显示")
	}
	tt.Frame() // 物化当前帧,避免点击落在首帧构建中途
	// batch delete
	if err := tt.Click("删除所选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.steps) != 2 || a.chosen.Len() != 0 {
		t.Fatalf("batch delete: steps=%d chosen=%d", len(a.steps), a.chosen.Len())
	}
	if a.steps[0].opLabel != "选取文件名" || a.steps[1].opLabel != "截断长度" {
		t.Fatalf("wrong survivors: %s, %s", a.steps[0].opLabel, a.steps[1].opLabel)
	}
}

// moveSteps is the Reorder callback: dragged rows land before `to`.
func TestMoveSteps(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("pinyin"), step("sanitize"), step("case.lower"), step("truncate")}
	// drag rows 0 and 1 to before index 4 (end)
	a.moveSteps([]int{0, 1}, 4)
	want := []string{"全部转小写", "截断长度", "文件名转拼音", "非法字符转全角"}
	for i, w := range want {
		if a.steps[i].opLabel != w {
			t.Fatalf("move end: %d got %s want %s", i, a.steps[i].opLabel, w)
		}
	}
	// drag row 3 (非法字符转全角) back to index 0
	a.moveSteps([]int{3}, 0)
	if a.steps[0].opLabel != "非法字符转全角" {
		t.Fatalf("move to front: %s", a.steps[0].opLabel)
	}
}
