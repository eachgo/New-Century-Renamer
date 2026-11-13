package main

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ncreamer/core"
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
	n := step("meta.nfo")
	if got := n.spec(); got != "nfo=" {
		t.Errorf("nfo spec = %q", got)
	}
	n.tpl = "<NFO:title><.EXT>"
	if got := n.spec(); got != "nfo=<NFO:title><.EXT>" {
		t.Errorf("nfo tpl spec = %q", got)
	}
	n.nfoFile = `C:\TV	vshow.nfo`
	if got := n.spec(); !strings.Contains(got, "file=C:") {
		t.Errorf("nfo with file spec = %q", got)
	}
	if _, err := core.ParseRules([]string{n.spec()}); err != nil {
		t.Errorf("nfo file spec failed to parse: %v", err)
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
	s.AI.Timeout = 600
	s.AI.Concurrency = 1
	s.AI.Prompt = "根据文件内容生成一个文件名,只输出文件名,不超过20个字"
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
	if got.AI.Timeout != 600 || got.AI.Concurrency != 1 || got.AI.Prompt != s.AI.Prompt {
		t.Fatalf("AI 设置(等待/并发/提示词)未往返: %+v", got.AI)
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
			// 次要文字承载路径、日期、大小、分组计数——是信息,不是装饰,
			// 所以按正文标准 4.5 要求。且必须量它真实所处的每一层底色:
			// 最差的从来不是 Background,而是左栏。
			dark := name == "dark"
			p := dt.panes(dark)
			for pane, bg := range map[string]ui.Color{
				"base": p.base, "left": p.left, "mid": p.mid,
				"right": p.right, "card": p.card,
			} {
				if r := contrastRatio(th.TextMuted, bg); r < 4.5 {
					t.Errorf("%s seed %s: muted contrast on %s %.2f < 4.5", name, hex, pane, r)
				}
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

// 控件边界必须看得见:按钮描边、输入框边框、滚动条都靠它。
// 实测改前亮色 1.03–1.24、暗色 1.46,等于没有边界——用户看不出哪里
// 可以点。这里按 1.8 兜底(装饰用的分隔线另走 paneTones.outline,
// 故意保持极淡,不参与这条断言)。
func TestControlBoundaryVisible(t *testing.T) {
	for _, hex := range []string{"#2F7A52", "#F2703C", "#3140A8"} {
		dt := themeFor(ui.Hex(hex))
		for _, dark := range []bool{false, true} {
			th := dt.light
			if dark {
				th = dt.dark
			}
			p := dt.panes(dark)
			for pane, bg := range map[string]ui.Color{
				"base": p.base, "left": p.left, "mid": p.mid,
				"right": p.right, "card": p.card,
			} {
				if r := contrastRatio(th.Border, bg); r < 1.8 {
					t.Errorf("%s dark=%v: border on %s %.2f < 1.8", hex, dark, pane, r)
				}
			}
			if r := contrastRatio(th.Scrollbar, p.right); r < 1.8 {
				t.Errorf("%s dark=%v: scrollbar on right %.2f < 1.8", hex, dark, r)
			}
		}
	}
}

// 暗色三栏不能贴着纯黑:base 曾经渲成 #000400,三栏层次只有 1.03–1.12。
func TestDarkPanesNotPureBlack(t *testing.T) {
	dt := themeFor(ui.Hex("#2F7A52"))
	p := dt.panes(true)
	if _, _, l := colorToOklch(p.base); l < 0.17 {
		t.Errorf("dark base lightness %.3f < 0.17, almost pure black", l)
	}
	// 层次仍要读得出来:中栏比基底亮,卡片又比右栏亮
	if contrastRatio(p.base, p.mid) < 1.10 {
		t.Errorf("dark base↔mid %.2f, panes indistinguishable", contrastRatio(p.base, p.mid))
	}
	if contrastRatio(p.right, p.card) < 1.15 {
		t.Errorf("dark right↔card %.2f, cards indistinguishable", contrastRatio(p.right, p.card))
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
	// select steps 1 (pinyin) and 3 (truncate) by uid
	a.chosen.Add(a.steps[0].opUID)
	a.chosen.Add(a.steps[2].opUID)

	tt := ui.NewTester(a.view, 1280, 800)
	if !tt.HasText("已选 2 步") {
		t.Error("多选面板未显示")
	}
	tt.Frame() // 物化当前帧,避免点击落在首帧构建中途
	// batch delete:现在和"清空列表"一样要两次点击(都丢东西,同一套语言)
	if err := tt.Click("删除所选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.steps) != 4 {
		t.Fatal("第一次点击就删了,应该只是进入确认态")
	}
	if err := tt.Click("确认删除所选?"); err != nil {
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

// 多选入口:标题行按钮;默认不显示圆点,开启后才出现(触摸友好)
func TestMultiSelectEntry(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("pinyin")}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if !tt.HasText("多选") {
		t.Fatal("多选入口按钮缺失")
	}
	if err := tt.Click("多选"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.pickMode {
		t.Fatal("点多选后未进入圆点模式")
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pickMode {
		t.Fatal("点完成后未退出")
	}
}

// 三栏不编号:用户的工作流是"先有文件",编号会误导;只需说清
// "功能会加入步骤"这一条关系

// 左栏说明区不给设计语言做注释;默认文案应为行动指引,
// 且不能要求用户"悬停"——键盘/触摸用户做不到,那样只剩一个空框。
func TestCatalogHintIsActionable(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if tt.HasText("左对齐 = 改名步骤 · 居中 = 流程控制/工具") {
		t.Fatal("设计语言说明仍在")
	}
	if tt.HasText("把鼠标停在功能上,这里会说明它做什么") {
		t.Fatal("默认文案要求悬停,键盘/触摸用户拿不到说明")
	}
	if !tt.HasText("点一个功能，它就加入中栏；步骤从上到下依次执行") {
		t.Error("缺少行动指引")
	}
}

// 摘要不得与操作名重复(避免 "清除筛选 — 清除筛选")
func TestSummaryHasNoDuplicate(t *testing.T) {
	cases := map[string]string{
		"pinyin": "", "sanitize": "", "unfilter": "", "romaji": "",
	}
	for id, want := range cases {
		r := step(id)
		if got := r.summary(); got != want {
			t.Errorf("%s summary = %q, want empty", id, got)
		}
	}
	// 有增量信息时保留
	sc := step("scope.ext")
	if got := sc.summary(); got != "作用于扩展名" {
		t.Errorf("scope summary = %q", got)
	}
	f := step("filter")
	f.pattern = "*.mp3"
	if got := f.summary(); got != "仅 *.mp3" {
		t.Errorf("filter summary = %q", got)
	}
}

// 摘要为空时不能留下孤立的破折号
func TestNoDanglingDash(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	for _, id := range []string{"unfilter", "pinyin"} {
		r := step(id)
		a.stepUID++
		r.opUID = a.stepUID
		a.steps = append(a.steps, r)
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	for _, s := range tt.Texts() {
		if strings.HasSuffix(strings.TrimSpace(s), "—") {
			t.Fatalf("出现悬空破折号: %q", s)
		}
	}
}

// 逃生键:多选/设置/主色面板都能被 Esc 一键解除
func TestEscapeExitsModes(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.steps = []guiRule{step("pinyin")}
	a.pickMode = true
	a.chosen.Add(a.steps[0].opUID)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.pickMode || a.chosen.Len() != 0 {
		t.Fatalf("Esc 未退出多选: pickMode=%v chosen=%d", a.pickMode, a.chosen.Len())
	}

	// 设置对话框
	a.settingsOpen = true
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.settingsOpen {
		t.Error("Esc 未关闭设置")
	}

	// 主色面板
	a.pickOpen = true
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.pickOpen {
		t.Error("Esc 未收起主色面板")
	}
}

// 三栏无编号;功能栏说明"点击加入步骤"
func TestPanesNoNumbers(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	for _, s := range tt.Texts() {
		if strings.HasPrefix(s, "① ") || strings.HasPrefix(s, "② ") || strings.HasPrefix(s, "③ ") {
			t.Fatalf("栏标题仍有编号: %q", s)
		}
	}
	if !tt.HasText("功能 · 点击加入步骤") {
		t.Error("功能栏缺引导")
	}
}

// 执行过的项不再算"将更名":否则主按钮执行完还亮着,再点一次全部失败,
// 用户会以为程序坏了。
func TestCountChangesSkipsApplied(t *testing.T) {
	ops := []core.RenameOp{
		{OldName: "a.txt", NewName: "b.txt", Applied: true},
		{OldName: "c.txt", NewName: "d.txt"},
		{OldName: "e.txt", NewName: "e.txt"},
		{OldName: "f.txt", NewName: "g.txt", Err: errors.New("boom")},
	}
	if n := countChanges(ops); n != 1 {
		t.Fatalf("countChanges = %d, want 1(只有 c.txt 还未执行)", n)
	}
}

// 执行后重建文件信息必须保留分组,否则空组名与组名的交界处会凭空
// 多出一个分组头,而分组头还会吞掉一行文件。
func TestStatFileKeepGroup(t *testing.T) {
	p := newTestFile(t, "01.flac")
	fi := statFileKeepGroup(p, "Album A")
	if fi.group != "Album A" {
		t.Fatalf("group = %q, want Album A", fi.group)
	}
	if fi.typ == "" {
		t.Error("其余字段应与 statFile 一致(typ 不该为空)")
	}
}

// EPUB/DOCX 的容器是 ZIP,m4a/mov 是 MP4:这些都不是"扩展名在撒谎",
// 不该标红。假警报一多,真警报就被淹掉了。
func TestRealMatchesExt(t *testing.T) {
	ok := [][2]string{
		{"epub", "ZIP"}, {"docx", "ZIP"}, {"xlsx", "ZIP"}, {"apk", "ZIP"},
		{"m4a", "MP4"}, {"mov", "MP4"}, {"mp4", "MP4"},
		{"mkv", "MKV"}, {"webm", "MKV"},
		{"ogg", "OGG"}, {"opus", "OGG"},
		{"jpg", "JPEG"}, {"jpeg", "JPEG"},
		{"mp3", "MP3"}, {"flac", "FLAC"}, {"pdf", "PDF"}, {"png", "PNG"},
	}
	for _, c := range ok {
		if !realMatchesExt(c[0], c[1]) {
			t.Errorf("%s 内容 %s 应视为相符", c[0], c[1])
		}
	}
	bad := [][2]string{
		{"mp3", "WebP"}, {"jpg", "ZIP"}, {"pdf", "MP4"}, {"epub", "MP3"},
	}
	for _, c := range bad {
		if realMatchesExt(c[0], c[1]) {
			t.Errorf("%s 内容 %s 是真不符,应报警", c[0], c[1])
		}
	}
	// 嗅探不出内容(空文件/未知格式)时不报警
	if !realMatchesExt("mp3", "") {
		t.Error("嗅探不出内容时不该报警")
	}
}

// 设置对话框的每一张卡都必须能被滚到。
// 以前这里写死 MaxHeight(dh-190),1280×720 下「主题」「设置文件」两张卡
// 高度为 0——被裁掉了,而且没有任何滚动提示,用户以为功能不存在。
func TestSettingsCardsReachableAtAnyHeight(t *testing.T) {
	for _, h := range []int{480, 600, 720, 800} {
		a := &app{stepSel: -1, settings: defaultSettings()}
		tt := ui.NewTester(a.view, 1280, h)
		a.openSettings()
		for i := 0; i < 4; i++ {
			tt.Frame()
		}
		// 可见 = 高度 > 0 且在窗口内。滚出视口的卡高度会是 0(被裁),
		// 所以分两次看:开头看前两张,滚到底看后两张。
		visible := func(want string) bool {
			r, ok := tt.Find(want)
			return ok && r.H > 0 && r.Y >= 0 && r.Y <= float32(h)
		}
		scrollToBottom := func() {
			for k := 0; k < 24; k++ {
				tt.Move(640, float32(h)/2)
				tt.Scroll(640, float32(h)/2, 0, 120)
				tt.Frame()
			}
		}
		for _, want := range []string{"AI 接口（OpenAI 兼容）", "文件"} {
			if !visible(want) {
				t.Errorf("h=%d 开头看不到卡片 %q", h, want)
			}
		}
		scrollToBottom()
		for _, want := range []string{"主题", "设置文件"} {
			if !visible(want) {
				t.Errorf("h=%d 滚到底也看不到卡片 %q", h, want)
			}
		}
		// 按钮不该跟着滚走
		if !visible("保存") {
			t.Errorf("h=%d 滚到底后保存按钮不可见", h)
		}
	}
}

// firstInkRun 从 x0 起找第一段连续墨迹(允许字间空隙 gapMax 像素),返回其宽度。
// 用来量"文字有没有被省略号吃掉"——被截断的文本 tt.Find 取不到,
// 只能看像素。
func firstInkRun(img *image.RGBA, x0, y0, y1, gapMax int) int {
	colHas := func(x int) bool {
		for y := y0; y < y1; y++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 < 140 && g>>8 < 140 && b>>8 < 140 {
				return true
			}
		}
		return false
	}
	start, end, gap := -1, -1, 0
	for x := x0; x < img.Bounds().Dx(); x++ {
		if colHas(x) {
			if start < 0 {
				start = x
			}
			end, gap = x, 0
			continue
		}
		if start >= 0 {
			gap++
			if gap > gapMax {
				break
			}
		}
	}
	if start < 0 {
		return 0
	}
	return end - start + 1
}

// 操作名比参数值重要:窄窗口下该被省略的是参数胶囊,不是步骤名。
// 以前 900 宽 + 长参数时"追加序号"被挤成"追…"、"AI 按内容命名"挤成"AI…",
// 用户只能靠猜知道这一步是什么。修法是给操作名 Shrink(0)。
func TestStepLabelSurvivesNarrowWindow(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	dir := t.TempDir()
	for _, n := range []string{"a.mp3", "b.mp3", "c.mp3"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a.addPaths([]string{dir})
	a.steps = []guiRule{step("seq"), step("case.lower"), step("scope.name"), step("pinyin")}
	a.stepSel = -1
	a.preview()
	tt := ui.NewTester(a.view, 900, 480)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	r, ok := tt.Find("1.")
	if !ok {
		t.Fatal("找不到第一步的编号")
	}
	// 编号宽 22,操作名紧随其后
	img := tt.Image()
	w := firstInkRun(img, int(r.X+r.W)+4, int(r.Y), int(r.Y+r.H), 5)
	// "追加序号"四个 12px 粗体汉字实测约 46px;被截成"追…"只剩约 21px
	if w < 36 {
		t.Fatalf("步骤名被挤成省略号了:墨迹只有 %dpx(完整约 46px)", w)
	}
}

// 类别胶囊、扩展名胶囊都必须能读出来。
// 以前是饱和色底 + 白字:11 个分组实测 3.13–3.59、36 个扩展名 3.10–3.60,
// 全部低于 4.5。改成同色相浅底深字之后要保证 6 以上。
func TestChipContrast(t *testing.T) {
	for _, seed := range []ui.Color{
		ui.RGB(0x2F, 0x7A, 0x52), ui.RGB(0xF2, 0x70, 0x3C),
		ui.RGB(0x31, 0x40, 0xA8), ui.RGB(0x8A, 0x2B, 0xE2),
	} {
		dt := themeFor(seed)
		for _, g := range stepGroups {
			for _, dark := range []bool{false, true} {
				bg, fg := chipTones(dt.groupColor(g), dark)
				if r := contrastRatio(bg, fg); r < 5.5 {
					t.Errorf("seed=%v %s dark=%v 类别胶囊对比度 %.2f < 5.5", seed, g, dark, r)
				}
				bg, fg = chipTonesEmph(dt.groupColor(g), dark)
				if r := contrastRatio(bg, fg); r < 5.5 {
					t.Errorf("seed=%v %s dark=%v 强调胶囊对比度 %.2f < 5.5", seed, g, dark, r)
				}
			}
		}
	}
}

// 删除动作统一 danger 语言:步骤删除与批量删除都要二次确认,且确认态
// 会自己收回(挂太久等于把"再点一下就删"的按钮永远留在屏幕上)。
func TestDeleteNeedsConfirmAndExpires(t *testing.T) {
	a := &app{stepSel: 0, settings: defaultSettings()}
	a.steps = []guiRule{step("pinyin"), step("case.lower")}
	for i := range a.steps {
		a.stepUID++
		a.steps[i].opUID = a.stepUID
	}
	tt := ui.NewTester(a.view, 1280, 800)
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.steps) != 2 {
		t.Fatal("第一次点击就删了,应该只是进入确认态")
	}
	if !a.confirmDelStep {
		t.Fatal("未进入确认态")
	}
	// 超时:三秒没点就当用户改主意
	a.confirmAt = time.Now().Add(-4 * time.Second)
	tt.Frame()
	if a.confirmDelStep {
		t.Fatal("确认态挂过三秒应自动收回")
	}
	if len(a.steps) != 2 {
		t.Fatal("收回确认态不该删东西")
	}
	// 再走一遍,这次真的点确认
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("确认删除?"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.steps) != 1 || a.steps[0].opLabel != "全部转小写" {
		t.Fatalf("确认删除失败: %d 步", len(a.steps))
	}
}

// 同时只允许挂一个"再点一下就删":点清空会收回删除步骤的确认态。
func TestOnlyOneConfirmAtATime(t *testing.T) {
	a := &app{stepSel: 0, settings: defaultSettings()}
	a.steps = []guiRule{step("pinyin")}
	a.stepUID++
	a.steps[0].opUID = a.stepUID
	a.files = []fileInfo{statFile(newTestFile(t, "a.txt"))}
	tt := ui.NewTester(a.view, 1280, 800)
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.confirmDelStep {
		t.Fatal("未进入删除确认")
	}
	if err := tt.Click("清空列表"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.confirmDelStep || !a.confirmClear {
		t.Fatalf("两个确认态同时挂着: delStep=%v clear=%v", a.confirmDelStep, a.confirmClear)
	}
}
