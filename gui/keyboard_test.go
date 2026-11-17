package main

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// keyboardApp builds an app with one file and two steps: enough to have a
// catalog, a step list and a file list to move between.
func keyboardApp(t *testing.T) *app {
	t.Helper()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	dir := t.TempDir()
	for _, n := range []string{"aaa.txt", "bbb.txt"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		a.files = append(a.files, fileInfo{path: p, size: 1, modTime: time.Now(), typ: "文本", order: len(a.files) + 1})
	}
	a.nextOrder = len(a.files)
	a.steps = []guiRule{step("case.lower"), step("case.upper")}
	a.stepUID = a.steps[0].opUID
	a.preview()
	return a
}

func focusedName(tt *ui.Tester) string {
	for _, x := range tt.AccessAll() {
		if x.Focused {
			return x.Name
		}
	}
	return ""
}

// TestTabReachesFileListQuickly pins how many Tab presses it takes to get
// from nowhere to the file list.
//
// The left catalog holds 34 entries plus 11 group headers. Each one used to
// be its own Tab stop, so the file list sat at stop 60 — a keyboard user
// never got there, and Delete (which acts on the file list when it has the
// focus) was unreachable without a mouse click. The catalog is one focus
// group now, so Tab enters it once and the arrows move inside it.
func TestTabReachesFileListQuickly(t *testing.T) {
	a := keyboardApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	at := -1
	for i := 1; i <= 40; i++ {
		tt.Key(0, ui.KeyTab)
		tt.Frame()
		if a.fileFocus {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("按了 40 次 Tab 都没到达文件列表")
	}
	if at > 24 {
		t.Fatalf("文件列表在第 %d 个 Tab 停留点,键盘用户基本到不了(应 ≤24)", at)
	}
}

// TestLeftCatalogIsOneTabStop checks the catalog behaves as a focus group:
// Tab enters it once, and the arrows move between entries.
func TestLeftCatalogIsOneTabStop(t *testing.T) {
	a := keyboardApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	// 走到左栏第一个条目
	at := -1
	// 栏头现在有「流程」「多选」两个按钮,意图卡也可聚焦,前序停留点变多,
	// 放宽到 30 次仍要求能进左栏目录。
	for i := 1; i <= 30; i++ {
		tt.Key(0, ui.KeyTab)
		tt.Frame()
		if n := focusedName(tt); strings.HasPrefix(n, "▾ "+stepGroups[0]+"(") {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("Tab 30 次都没进入左栏目录")
	}
	// 组内方向键应能往下走,而且不该把焦点甩出左栏
	first := focusedName(tt)
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	second := focusedName(tt)
	if second == first {
		t.Fatalf("在左栏按 Down 焦点没动,还停在 %q", first)
	}
	for i := 0; i < 3; i++ {
		tt.Key(0, ui.KeyDown)
		tt.Frame()
	}
	if n := focusedName(tt); n == "" {
		t.Fatalf("方向键把焦点移出了左栏目录")
	}
}

// TestSelectedFileRowIsVisible checks the row the keyboard chose actually
// looks chosen.
//
// a.fileSel was wired into the list (so Up/Down moved it and Delete acted on
// it) but the row rendering never read it: pressing Down selected a file with
// no visible change, and the next Delete removed a row the user could not see
// was selected.
//
// The check reads one pixel of the card's own background rather than counting
// changed pixels: mygo paints the list's chosen row in the accent color by
// itself, so a whole-frame diff stays large even when our cards ignore the
// selection — and that paint is exactly the bug, since the card is opaque and
// only lets the accent through the 8px gutter next to the result column.
func TestSelectedFileRowIsVisible(t *testing.T) {
	a := keyboardApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	// 用文件名胶囊定位第一张卡片:它的左缘就在卡片内边距之后。
	chip, ok := tt.Find("aaa")
	if !ok {
		t.Fatalf("找不到第一个文件名,无法定位卡片")
	}
	// 名字胶囊右侧、卡片右缘之前的空白处。
	px, py := int(chip.X+chip.W+8), int(chip.Y+chip.H/2)
	before := tt.Image().RGBAAt(px, py)

	a.fileSel = 0
	tt.Frame()
	tt.Frame()
	after := tt.Image().RGBAAt(px, py)

	if before == after {
		t.Fatalf("选中文件后卡片底色没变(仍是 %v),用户看不出选了哪一行", before)
	}
	// 选中底色要能看出是"选中",不是随机的轻微色偏。
	th := a.currentTheme().panes(a.isDark())
	if sameRGB(after, th.card) {
		t.Fatalf("选中行的卡片底色仍是普通卡片色 %v", after)
	}
}

func sameRGB(a color.RGBA, b ui.Color) bool {
	return a.R == b.R && a.G == b.G && a.B == b.B
}

// TestStepSwapReportsStatus checks Alt+Up/Down says what it did.
//
// Those keys move the step in the pipeline, not "select the previous step" —
// the same chord means selection in most other software. A silent swap edits
// the pipeline behind the user's back.
func TestStepSwapReportsStatus(t *testing.T) {
	a := keyboardApp(t)
	a.stepSel = 1
	a.status = ""
	a.swapSteps(0, 1)
	if a.stepSel != 0 {
		t.Fatalf("交换后选中应跟着那一步走到 0,实际 %d", a.stepSel)
	}
	if a.status == "" {
		t.Fatalf("Alt+↑↓ 改了管道顺序却什么都没说")
	}
	if !strings.Contains(a.status, "第 1 位") {
		t.Fatalf("状态没说明移到第几位:%q", a.status)
	}
}

var _ = png.Encode
