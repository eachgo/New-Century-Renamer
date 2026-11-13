package main

import (
	"errors"
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

// 目录里不该再有"立即执行"的动作:目录是"往管道里加东西"的地方,
// 混进一个点了就跑的动作,和它旁边 32 个"点了加一步"的条目长得一样。
// 设置进标题栏、导入CSV 进右栏工具栏(与导出CSV 成对)。
func TestNoActionEntriesInCatalog(t *testing.T) {
	var acts []string
	for _, d := range catalog {
		if d.act != "" {
			acts = append(acts, d.act)
		}
	}
	if len(acts) != 0 {
		t.Fatalf("目录里还有立即执行的动作: %v", acts)
	}
}

// 设置 lives in the title bar now, at any window height without scrolling.
func TestSettingsLivesInTitleBar(t *testing.T) {
	for _, h := range []int{480, 720, 1080} {
		a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
		tt := ui.NewTester(a.view, 1280, h)
		tt.Frame()
		// 齿轮没有文字,用快捷键证明它接在同一条路上;再直接点图标所在位置。
		r, ok := tt.Find("New Century Renamer")
		if !ok {
			t.Fatalf("h=%d 找不到标题栏", h)
		}
		if r.Y > 60 {
			t.Fatalf("h=%d 标题栏不在顶部: y=%.1f", h, r.Y)
		}
		tt.Key(ui.Cmd, ui.KeyComma)
		tt.Frame()
		if !a.settingsOpen {
			t.Fatalf("h=%d Ctrl+, 没打开设置", h)
		}
		if len(a.steps) != 0 {
			t.Fatalf("h=%d 设置 wrongly added a step: %+v", h, a.steps)
		}
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

// 范围用色条而不是文字:文字只能说明"这一行是什么范围",色条还能说明
// "范围持续到哪一行"。这个测试断言的是后者——第 4 行开的选取文件名,
// 到第 12 行才切换,中间每一行(含中间的排除文件行)右槽都必须亮着。
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

	dt := a.currentTheme()
	bars := a.stepRangeBars(dt)
	if len(bars) != len(a.steps) {
		t.Fatalf("色条行数 %d 与步骤数 %d 不符", len(bars), len(a.steps))
	}
	off := ui.Color{}
	// 1..3: 还没开任何范围,两槽全空
	for i := 0; i <= 2; i++ {
		if bars[i][0] != off || bars[i][1] != off {
			t.Fatalf("第 %d 步不该有范围色条", i+1)
		}
	}
	// 4..11: 选取文件名一直开着——这是文字做不到的"持续到哪一行"
	for i := 3; i <= 10; i++ {
		if bars[i][1] != dt.rangeScopeName {
			t.Fatalf("第 %d 步应当仍在仅文件名范围内", i+1)
		}
	}
	// 12..13: 切到扩展名
	if bars[11][1] != dt.rangeScopeExt {
		t.Fatalf("第 12 步(选取扩展名)右槽应是扩展名色")
	}
	if bars[12][1] != dt.rangeScopeExt {
		t.Fatalf("第 13 步(清除筛选)不该结束选取范围")
	}
	// 14: 恢复整体,两槽都空
	if bars[13][0] != off || bars[13][1] != off {
		t.Fatalf("第 14 步(恢复整体)之后不该还有范围")
	}
	// 筛选范围:从第 9 步(排除文件)开始,到第 13 步(清除筛选)结束
	for i := 0; i <= 7; i++ {
		if bars[i][0] != off {
			t.Fatalf("第 %d 步不该有筛选范围", i+1)
		}
	}
	for i := 8; i <= 11; i++ {
		if bars[i][0] != dt.rangeFilter {
			t.Fatalf("第 %d 步应当仍在筛选范围内", i+1)
		}
	}
	if bars[12][0] != off {
		t.Fatalf("第 13 步(清除筛选)之后筛选范围应当结束")
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

// 说明走 Tooltip:目录要在一屏里放下 32 条,每条占两行就只剩一半可见。
// 这里量的是"说明挂在条目上、而且是那个功能自己的说明"——Tooltip 的
// 内容不进 Texts(),所以只能查 app 侧的数据与悬停后的行为。
func TestCatalogDescriptionsAreTooltips(t *testing.T) {
	a := &app{stepSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	if tt.HasText("把鼠标停在功能上,这里会说明它做什么") {
		t.Fatal("底部说明框应当已经删掉")
	}
	// 每一条都得有说明可给,否则 Tooltip 会空着。
	for _, d := range catalog {
		if d.desc == "" {
			t.Errorf("功能 %q 没有说明", d.label)
		}
	}
	// 悬停「筛选文件」之后,它的说明要真的出现在屏幕上。
	rect, ok := tt.Find("筛选文件")
	if !ok {
		t.Fatal("找不到「筛选文件」")
	}
	tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	tt.Advance(700 * time.Millisecond)
	tt.Frame()
	var want string
	for _, d := range catalog {
		if d.id == "filter" {
			want = d.desc
		}
	}
	if !tt.HasText(want) {
		t.Fatalf("悬停「筛选文件」后没看到说明: %q", want)
	}
	// 标题栏那句"点击加入步骤"就是唯一的操作指引。
	if !tt.HasText("功能 · 点击加入步骤") {
		t.Error("缺少栏标题里的操作指引")
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
//
// 嗅探结果现在是 formatTable 的规范键(小写),容器类返回容器名,
// 所以这里用 "zip"/"mp4" 而不是以前的 "ZIP"/"MP4"。
func TestRealMatchesExt(t *testing.T) {
	ok := [][2]string{
		{"epub", "zip"}, {"docx", "zip"}, {"xlsx", "zip"}, {"apk", "zip"},
		{"m4a", "mp4"}, {"m4b", "mp4"}, {"mov", "mp4"}, {"mp4", "mp4"}, {"3gp", "mp4"},
		{"mkv", "mkv"}, {"webm", "mkv"},
		{"ogg", "ogg"}, {"opus", "ogg"}, {"oga", "ogg"}, {"spx", "ogg"},
		{"jpg", "jpeg"}, {"jpeg", "jpeg"},
		{"mp3", "mp3"}, {"flac", "flac"}, {"pdf", "pdf"}, {"png", "png"},
		// 老式 Office 三件套都是 OLE 复合文档
		{"doc", "ole"}, {"xls", "ole"}, {"ppt", "ole"},
		{"azw3", "mobi"}, {"wma", "asf"},
	}
	for _, c := range ok {
		if contentCheck(c[0], c[1], 1<<20) != contentOK {
			t.Errorf("%s 内容 %s 应视为相符", c[0], c[1])
		}
	}
	bad := [][2]string{
		{"mp3", "webp"}, {"jpg", "zip"}, {"pdf", "mp4"}, {"epub", "mp3"},
	}
	for _, c := range bad {
		if contentCheck(c[0], c[1], 1<<20) != contentMismatch {
			t.Errorf("%s 内容 %s 是真不符,应报警", c[0], c[1])
		}
	}
	// 嗅探不出内容(空文件/未知格式)时不报警
	if contentCheck("mp3", "", 1<<20) == contentMismatch {
		t.Error("嗅探不出内容时不该报警")
	}
}

// contentCheck 是 realMatchesExt 背后的三档判断:相符 / 真不符 /
// 声称是二进制格式却读不出魔数。以前第三档被当成"没问题"静默放过,
// 于是一个内容被写坏的 .docx(根本不是 ZIP)看起来和正常文件一样。
func TestContentCheckThreeWay(t *testing.T) {
	const big = int64(1 << 20)
	cases := []struct {
		ext, real string
		size      int64
		want      contentVerdict
		why       string
	}{
		{"mp3", "mp3", big, contentOK, "扩展名与魔数一致"},
		{"epub", "zip", big, contentOK, "EPUB 本来就是 ZIP 容器"},
		{"m4a", "mp4", big, contentOK, "m4a 是 MP4 容器"},
		{"mp3", "webp", big, contentMismatch, "内容是真 WebP"},
		{"docx", "", big, contentUnrecognized, "声称是 ZIP 却读不出魔数"},
		{"flac", "", big, contentUnrecognized, "声称是 FLAC 却读不出魔数"},
		{"txt", "", big, contentNoOpinion, "纯文本本来没有魔数"},
		{"svg", "", big, contentNoOpinion, "SVG 可以是纯文本"},
		{"mp3", "", 3, contentNoOpinion, "3 字节的文件没有嗅探价值"},
		{"", "webp", big, contentNoOpinion, "没有扩展名就无从比较"},
		{"qqq", "", big, contentNoOpinion, "未知格式,不猜也不报"},
	}
	for _, c := range cases {
		if got := contentCheck(c.ext, c.real, c.size); got != c.want {
			t.Errorf("contentCheck(%q,%q,%d) = %d, want %d(%s)",
				c.ext, c.real, c.size, got, c.want, c.why)
		}
	}
}

// 以前 ftyp box 的偏移写错成 b[0:4],而 ISO-BMFF 前面还有 4 字节
// box size,于是真实的 mp4/m4a/mov 一个都嗅探不到——containerExts
// 里六条 MP4 家族白名单在运行时形同虚设。这条测试用真实魔数开头
// 的临时文件守住偏移。
func TestSniffMagicRecognizesContainers(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		want string
	}{
		{"a.m4a", []byte("\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00"), "mp4"},
		{"a.mp4", []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00"), "mp4"},
		{"a.mov", []byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00"), "mp4"},
		{"a.epub", []byte("PK\x03\x04\x14\x00\x00\x00"), "zip"},
		{"a.docx", []byte("PK\x03\x04\x14\x00\x06\x00"), "zip"},
		{"a.doc", []byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"), "ole"},
		{"a.wma", []byte("\x30\x26\xB2\x75\x8E\x66\xCF\x11"), "asf"},
		{"a.rar", []byte("Rar!\x1A\x07\x01\x00"), "rar"},
		{"a.7z", []byte("7z\xBC\xAF\x27\x1C\x00\x04"), "7z"},
		{"a.flac", []byte("fLaC\x00\x00\x00\x22"), "flac"},
		{"a.pdf", []byte("%PDF-1.7\n"), "pdf"},
		{"a.png", []byte("\x89PNG\r\n\x1a\n\x00\x00"), "png"},
		{"a.jpg", []byte("\xff\xd8\xff\xe0\x00\x10JF"), "jpeg"},
		{"a.mp3", []byte("ID3\x04\x00\x00\x00\x00"), "mp3"},
		{"a.mkv", []byte("\x1A\x45\xDF\xA3\x01\x00\x00\x00"), "mkv"},
		{"a.gif", []byte("GIF89a\x01\x00\x01\x00"), "gif"},
	}
	for _, c := range cases {
		p := filepath.Join(t.TempDir(), c.name)
		if err := os.WriteFile(p, c.head, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := sniffMagic(p); got != c.want {
			t.Errorf("%s 嗅探 = %q, want %q", c.name, got, c.want)
		}
	}
}

// 嗅探结果必须都能在 formatTable 里查到标签,否则界面会退回
// 大写英文(以前 sniffMagic 就是直接返回 "MP4"/"ZIP" 这种),
// 同一份内容在两处显示成两种叫法。
func TestSniffResultsHaveLabels(t *testing.T) {
	for key := range containerLabels {
		if _, ok := formatTable[key]; ok {
			continue // 容器键同时是格式键时,标签来自 formatTable
		}
		if contentLabel(key) == "" {
			t.Errorf("容器 %q 没有展示名", key)
		}
	}
	for _, key := range []string{
		"mp3", "flac", "ogg", "opus", "wav", "wma", "m4a", "aac", "mid", "oga", "spx", "mka",
		"mp4", "m4v", "mov", "mkv", "webm", "avi", "flv", "3gp",
		"jpg", "jpeg", "png", "gif", "webp", "bmp", "tiff", "svg", "ico",
		"txt", "md", "log", "csv", "json", "xml", "html", "htm", "srt", "ass",
		"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "ole",
		"epub", "mobi", "azw3", "m4b",
		"zip", "rar", "7z", "tar", "gz", "apk", "jar", "cbz", "odt", "ods", "odp",
	} {
		if _, ok := formatTable[key]; !ok {
			t.Errorf("%q 不在 formatTable 里(会退回成大写扩展名)", key)
			continue
		}
		if formatCategory(key) == "" {
			t.Errorf("%q 没有类别", key)
		}
	}
	// containerExts 里出现的每个容器名都得有展示名:要么在 containerLabels
	// 里(MP4/ZIP/OLE 这类容器),要么本身就是 formatTable 的格式键
	// (jpeg 之于 jpg)。两者都没有说明嗅探返回了一个界面叫不出名字的值。
	for ext, cs := range containerExts {
		if _, ok := formatTable[ext]; !ok {
			t.Errorf("containerExts 的键 %q 不在 formatTable 里", ext)
		}
		for _, c := range cs {
			if _, ok := containerLabels[c]; ok {
				continue
			}
			if _, ok := formatTable[c]; !ok {
				t.Errorf("containerExts[%q] 的容器 %q 没有展示名", ext, c)
			}
		}
	}
}

// 窄窗口(单栏)下设置对话框的每一张卡都必须能被滚到。
// 宽窗口走两栏,四张卡一屏放得下,由 TestSettingsCardsVisibleAtDefaultSize
// 守着——那里断言"不需要滚动"。这里只管窄窗口:单栏排下来总高 890,
// 靠滚动够到后面的卡,而且按钮不该跟着滚走。
func TestSettingsCardsReachableAtAnyHeight(t *testing.T) {
	for _, h := range []int{480, 600, 720, 800} {
		a := &app{stepSel: -1, settings: defaultSettings()}
		tt := ui.NewTester(a.view, 900, h)
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
	// 被 Ellipsis 截断的文本不进 Texts(),所以"完整显示"= 屏幕上同时
	// 存在两处「追加序号」:左栏目录里一条、中栏步骤行里一条。
	// 量像素墨迹在不同平台的字体渲染下不稳(Linux 上实测只有 10px),
	// 数文本则与字体无关。
	n := 0
	for _, txt := range tt.Texts() {
		if txt == "追加序号" {
			n++
		}
	}
	if n < 2 {
		t.Fatalf("步骤名被挤成省略号了:屏幕上只有 %d 处「追加序号」(目录 1 + 步骤行 1 = 2)", n)
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

// 悬停底必须在每一层海拔上都看得出来。
// 改前 SurfaceHover 是固定色调 tone(0.955),而右栏底正好也是 L≈0.955,
// 两者实测 1.00:1——悬停时只有图标和文字变色,底色那层等于没画。
// 三栏四层海拔明度各不相同,任何固定色调都会与其中某一层撞上,所以改用
// 半透明的文字色叠层。这条断言防止以后有人把它改回固定色。
func TestHoverVisibleOnEveryPane(t *testing.T) {
	for _, hex := range []string{"#2F7A52", "#F2703C", "#3140A8", "#8A2BE2", "#000000", "#FFFFFF"} {
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
				if r := contrastRatio(over(th.SurfaceHover, bg), bg); r < 1.12 {
					t.Errorf("%s dark=%v: hover on %s %.3f < 1.12, 悬停底色看不见", hex, dark, pane, r)
				}
				if r := contrastRatio(over(th.SurfacePressed, bg), bg); r < 1.20 {
					t.Errorf("%s dark=%v: pressed on %s %.3f < 1.20", hex, dark, pane, r)
				}
			}
		}
	}
}

// 浅填充(Tonal)按钮上的字必须读得出来。
// 以前直接用 Accent 当字色压 Accent 12% 底:默认绿 3.50:1、橙 2.35:1,
// 都低于 WCAG 4.5。Accent 要够亮才承得住白字,不能再兼作浅底上的字。
func TestTonalButtonInkReadable(t *testing.T) {
	for _, hex := range []string{"#2F7A52", "#F2703C", "#3140A8", "#8A2BE2", "#E22B5C", "#F5C518", "#000000", "#FFFFFF"} {
		dt := themeFor(ui.Hex(hex))
		for _, dark := range []bool{false, true} {
			th := dt.light
			if dark {
				th = dt.dark
			}
			ink := th.Accent
			if !dark {
				_, ac, ah := colorToOklch(th.Accent)
				ink = oklchColor(0.44, ac, ah)
			}
			p := dt.panes(dark)
			// 12% 常态底与 20% 悬停底都要过 4.5
			for _, al := range []float32{0.12, 0.20} {
				bg := over(th.Accent.Alpha(al), p.right)
				if r := contrastRatio(ink, bg); r < 4.5 {
					t.Errorf("%s dark=%v alpha=%.2f: tonal ink %.2f < 4.5", hex, dark, al, r)
				}
			}
		}
	}
}

// 900 宽下日期/大小胶囊不能被压成竖排。
// 改前元数据行既没有 Wrap、胶囊也没有 SingleLine,右栏只剩约 350 时
// 「2026-11-14」被逐字折行,实测「1 B」胶囊 18.9×33.9(宽小于高),
// 读起来像竖排。现在整颗胶囊换行、胶囊内不断行。
func TestMetadataChipsStayHorizontalAt900(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.Local)
	a.files = []fileInfo{
		{path: `C:\x\Awich - THE UNION\01 - Awich - THE UNION.mp3`, modTime: now, size: 1, typ: "MP3 音频"},
		{path: `C:\x\Awich - THE UNION\02 - Awich - THE UNION.mp3`, modTime: now, size: 1048576, typ: "MP3 音频"},
	}
	for i := range a.files {
		a.files[i].group = "Awich - THE UNION"
	}
	a.preview()
	tt := ui.NewTester(a.view, 900, 700)
	tt.Frame()
	tt.Frame()
	for _, s := range []string{"1 B", "1.0 MB", now.Format("2006-01-02")} {
		r, ok := tt.Find(s)
		if !ok {
			t.Errorf("%q 没渲染出来", s)
			continue
		}
		if r.W <= r.H {
			t.Errorf("%q 被压成竖排: %.1f×%.1f(宽应明显大于高)", s, r.W, r.H)
		}
	}
}

// 标题栏右侧的齿轮/问号必须紧贴窗口按钮那一侧,不能停在窗口中间。
// 改前 summary 的 PaddingEnd 又加了一次 bar.Right,让位算两遍,
// 右侧控件组被推左 138,和最小化/关闭之间空出一大段。
func TestTitlebarActionsSitAtRightEdge(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 900, 200)
	tt.SetTitleBar(ui.TitleBar{Height: 38, Right: 138, Bottom: 38})
	tt.Frame()
	tt.Frame()
	r, ok := tt.Find("?")
	if !ok {
		t.Fatal("找不到问号按钮")
	}
	// 行右侧留给三个窗口按钮 138,summary 的 PaddingEnd 8,再往左才是 ?/齿轮。
	// 900 - 138 - 8 - 26(齿轮) - 26(?) ≈ 702 起。留 40 的余量防字体差异。
	if r.X < 660 {
		t.Errorf("问号 x=%.1f,离右缘太远(应 >660),让位算了两遍", r.X)
	}
}

// 三种类型标记必须各自出现,而且只有真不符才标红:
// 扩展名 .mp3 内容是 WebP → 红胶囊「内容为 WebP 图片」;
// 扩展名 .docx 内容读不出 ZIP 魔数 → 中性胶囊「内容未识别」;
// 扩展名 .mp3 内容也像 MP3 → 什么都不画(相符是默认,不该占位置)。
func TestContentChipsRenderThreeStates(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.Local)
	a.files = []fileInfo{
		{path: `C:\x\lie.mp3`, modTime: now, size: 4096, typ: "MP3 音频", realType: "webp"},
		{path: `C:\x\broken.docx`, modTime: now, size: 4096, typ: "Word 文档", realType: ""},
		{path: `C:\x\ok.mp3`, modTime: now, size: 4096, typ: "MP3 音频", realType: "mp3"},
	}
	a.preview()
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	tt.Frame()

	if r, ok := tt.Find("内容为 WebP 图片"); !ok {
		t.Error("真不符的文件没有标红提示")
	} else if r.W <= r.H {
		t.Errorf("标红胶囊被压成竖排: %.1f×%.1f", r.W, r.H)
	}
	if _, ok := tt.Find("内容未识别"); !ok {
		t.Error("声称是二进制格式却读不出魔数,应给中性提示")
	}
	if r, ok := tt.Find("内容为 MP3 音频"); ok {
		t.Errorf("相符的文件不该挂提示,却渲染出 %.0f×%.0f 的胶囊", r.W, r.H)
	}
}

// SVG 不是魔数,是"根元素为 <svg> 的 XML 文档"。以前用 bytes.Contains 判,
// 于是前 512 字节里内联了一段 <svg> 图标的 .html / .md 也被认成 SVG,
// contentCheck 再据此报红「内容为 SVG 图片」——一个二阶假警报:
// 第一个判断(这文件是 SVG 吗)就已经错了,后面那个红胶囊只是它的回声。
func TestInlineSVGInHTMLIsNotAMismatch(t *testing.T) {
	cases := []struct {
		name string
		body string
		real string
		want contentVerdict
	}{
		// 内联图标的 html:嗅探不该说是 svg
		{"page.html", "<!DOCTYPE html>\n<html><body><svg viewBox=\"0 0 10 10\"><circle r=\"5\"/></svg></body></html>", "", contentNoOpinion},
		// 内联图标的 markdown:同上
		{"note.md", "# 标题\n\n图标: <svg width=\"16\"><path d=\"M0 0h16v16z\"/></svg>\n", "", contentNoOpinion},
		// 真的 SVG:根元素就是 <svg>
		{"icon.svg", "<svg xmlns=\"http://www.w3.org/2000/svg\"><rect width=\"8\" height=\"8\"/></svg>", "svg", contentOK},
		// 带 XML 声明与注释的真 SVG
		{"decl.svg", "<?xml version=\"1.0\"?>\n<!-- 生成 -->\n<svg xmlns=\"http://www.w3.org/2000/svg\"/>", "svg", contentOK},
		// <svgfoo> 不是 <svg>
		{"odd.svg", "<svgfoo/>", "", contentNoOpinion},
		// 扩展名说是 html、内容真的是 html:不该报警
		{"plain.html", "<!DOCTYPE html>\n<html><body>hello</body></html>", "", contentNoOpinion},
	}
	for _, c := range cases {
		p := filepath.Join(t.TempDir(), c.name)
		if err := os.WriteFile(p, []byte(c.body), 0o644); err != nil {
			t.Fatal(err)
		}
		real := sniffMagic(p)
		if c.real != "" && real != c.real {
			t.Errorf("%s 嗅探 = %q, want %q", c.name, real, c.real)
		}
		if c.real == "" && real != "" {
			t.Errorf("%s 嗅探 = %q, 但内联的 <svg> 不是 SVG 文档", c.name, real)
		}
		ext := strings.TrimPrefix(filepath.Ext(c.name), ".")
		if got := contentCheck(ext, real, int64(len(c.body))); got != c.want {
			t.Errorf("%s contentCheck = %d, want %d", c.name, got, c.want)
		}
	}
}

// 文本格式之间互不算矛盾:html 里内联 svg、svg 里带 XML 声明都是合法的,
// 这类格式本来就没有魔数,嗅探认出的只是"内容里出现了什么"。
func TestMagiclessFormatsNeverContradictEachOther(t *testing.T) {
	for _, ext := range []string{"html", "htm", "md", "xml", "txt"} {
		if got := contentCheck(ext, "svg", 4096); got != contentNoOpinion {
			t.Errorf("contentCheck(%q, svg) = %d, want %d(文本格式不该互报矛盾)", ext, got, contentNoOpinion)
		}
	}
	// 但文本 vs 二进制仍然要报:一个叫 .html 的文件其实是 PNG
	if got := contentCheck("html", "png", 4096); got != contentMismatch {
		t.Errorf("contentCheck(html, png) = %d, want %d", got, contentMismatch)
	}
}
