package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 空态得给出一个看得见的落点:虚线框 + 大号按钮。以前正文区只有两行灰字,
// "添加文件…"只是工具栏上一个 14px 的次要按钮,于是整窗都能接文件、却
// 没有任何地方告诉用户该往哪儿拖。
func TestEmptyStateShowsDropZone(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	for _, want := range []string{"把文件或文件夹拖到这里", "添加文件…", "添加文件夹…"} {
		if !tt.HasText(want) {
			t.Fatalf("空态缺 %q", want)
		}
	}
}

// 三处引导文案不能再互相矛盾:右栏说"先放文件"、底栏说"拖入文件开始"、
// 中栏说"点一张卡片"——三条路指向不同的起点,用户只能猜哪条才对。
// 现在的口径是:两条路都行,顺序不影响结果。
func TestGuidanceDoesNotContradictItself(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	for _, stale := range []string{
		"批量更名：左边挑处理步骤", // 旧版右栏空态,句首还是以前的说法
		"拖入文件开始",       // 旧版底栏,把"先放文件"说成唯一入口
		"把文件或文件夹拖进窗口",  // 旧版右栏第二句,指整窗而不是落点
	} {
		if tt.HasText(stale) {
			t.Fatalf("互相矛盾的旧文案还在: %q", stale)
		}
	}
	// 右栏明说两条路都行
	if !tt.HasText("也可以点下面的按钮挑。先放文件或先排步骤都行，两边会自动接上。") {
		t.Fatal("右栏没说清两条路都行")
	}
	// 中栏也说两条路都行,并说明步骤会等文件
	if !tt.HasText("还没放文件也可以先排——文件加进来时，这些步骤会立刻套上去。") {
		t.Fatal("中栏没说清可以先排步骤")
	}
}

// 拖到虚线框里要真的收下文件,并且不能和窗口级 OnFileDrop 重复添加。
func TestDropZoneAcceptsFiles(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	r, ok := tt.Find("把文件或文件夹拖到这里")
	if !ok {
		t.Fatal("找不到拖放区")
	}
	p := filepath.Join(t.TempDir(), "A.TXT")
	if err := os.WriteFile(p, []byte("hello world, long enough"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !tt.DropFiles(r.X+r.W/2, r.Y+r.H/2+40, p) {
		t.Fatal("拖放没被拖放区吃掉")
	}
	tt.Frame()
	tt.Frame()
	if len(a.files) != 1 {
		t.Fatalf("拖放后文件数 = %d, 期望 1(重复添加?)", len(a.files))
	}
	if filepath.Base(a.files[0].path) != "A.TXT" {
		t.Fatalf("加进来的不是拖的那个: %q", a.files[0].path)
	}
}

// 虚线框是这个空态唯一的落点提示,对比度不够就等于没画。
func TestDropZoneBorderIsVisible(t *testing.T) {
	dt := buildTheme(ui.RGB(0x2F, 0x7A, 0x52))
	for _, th := range []*ui.Theme{dt.light, dt.dark} {
		pt := dt.panes(th.Dark)
		dash := dropZoneDash(th)
		if got := contrastRatio(over(dash, pt.card), pt.card); got < 3 {
			t.Fatalf("虚线框对比度 %.2f,低于非文本界面元素要求的 3:1", got)
		}
	}
}
