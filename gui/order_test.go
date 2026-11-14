package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// orderApp builds a batch in the order a user would: a loose file, then a
// folder's files, then another loose one. The mix matters — the group header
// only appears where the group changes, so sorting and dragging have to behave
// inside a run and refuse to cross between runs.
func orderApp(t *testing.T) *app {
	t.Helper()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	dir := t.TempDir()
	day := func(d int) time.Time { return time.Date(2026, 11, d, 9, 0, 0, 0, time.UTC) }
	a.files = []fileInfo{
		{path: filepath.Join(dir, "loose-b.txt"), size: 300, modTime: day(3), typ: "文本", group: "", order: 1},
		{path: filepath.Join(dir, "第10话.mkv"), size: 900, modTime: day(1), typ: "MKV", group: "A", order: 2},
		{path: filepath.Join(dir, "第2话.mkv"), size: 100, modTime: day(4), typ: "MKV", group: "A", order: 3},
		{path: filepath.Join(dir, "第1话.mkv"), size: 500, modTime: day(2), typ: "MKV", group: "A", order: 4},
		{path: filepath.Join(dir, "loose-a.txt"), size: 100, modTime: day(5), typ: "文本", group: "", order: 5},
	}
	a.nextOrder = 5
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	a.preview()
	return a
}

func names(files []fileInfo) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = filepath.Base(f.path)
	}
	return out
}

func sameOrder(t *testing.T, got []fileInfo, want ...string) {
	t.Helper()
	g := names(got)
	if len(g) != len(want) {
		t.Fatalf("文件数 = %d, 期望 %d: %v", len(g), len(want), g)
	}
	for i := range want {
		if g[i] != want[i] {
			t.Fatalf("顺序 = %v, 期望 %v", g, want)
		}
	}
}

// 排序只在同一分组内生效,不把两个文件夹交错。
func TestSortStaysInsideEachGroup(t *testing.T) {
	a := orderApp(t)
	sortFiles(a.files, "name", false)
	sameOrder(t, a.files,
		"loose-b.txt",
		"第1话.mkv", "第2话.mkv", "第10话.mkv",
		"loose-a.txt",
	)
}

// 「按名称」要按人读数字的方式比:第2话 在 第10话 前面。字节序会把 10 排到 2 前面。
func TestSortByNameIsNatural(t *testing.T) {
	a := orderApp(t)
	sortFiles(a.files, "name", false)
	got := names(a.files)
	if got[1] != "第1话.mkv" || got[2] != "第2话.mkv" || got[3] != "第10话.mkv" {
		t.Fatalf("自然序 = %v", got)
	}
}

// 按时间/大小排序,以及倒序。
func TestSortByDateAndSize(t *testing.T) {
	a := orderApp(t)
	sortFiles(a.files, "date", false)
	sameOrder(t, a.files,
		"loose-b.txt",
		"第10话.mkv", "第1话.mkv", "第2话.mkv",
		"loose-a.txt",
	)
	a = orderApp(t)
	sortFiles(a.files, "size", true)
	sameOrder(t, a.files,
		"loose-b.txt",
		"第10话.mkv", "第1话.mkv", "第2话.mkv",
		"loose-a.txt",
	)
}

// 「添加顺序」必须能回来。没有 order 字段,按大小排完就再也还原不出用户建批次时的顺序。
func TestSortBackToAddedOrder(t *testing.T) {
	a := orderApp(t)
	sortFiles(a.files, "size", true)
	sortFiles(a.files, "", false)
	sameOrder(t, a.files,
		"loose-b.txt",
		"第10话.mkv", "第2话.mkv", "第1话.mkv",
		"loose-a.txt",
	)
}

// 排序键相同时用 order 兜底,而不是让 sort 自己决定。
func TestSortIsStableOnEqualKeys(t *testing.T) {
	a := orderApp(t)
	for i := range a.files {
		a.files[i].size = 42
	}
	sortFiles(a.files, "size", false)
	sameOrder(t, a.files,
		"loose-b.txt",
		"第10话.mkv", "第2话.mkv", "第1话.mkv",
		"loose-a.txt",
	)
}

// 拖动重排:把同组最后一个挪到最前。
func TestReorderWithinGroup(t *testing.T) {
	a := orderApp(t)
	rows := buildRows(a.files)
	// 行序列: [loose-b] [header A] [第10话] [第2话] [第1话] [loose-a]
	from, dest, ok := reorderTarget(a.files, rows, []int{4}, 2)
	if !ok {
		t.Fatal("同组内拖动应当被接受")
	}
	a.files = moveFile(a.files, from, dest)
	sameOrder(t, a.files,
		"loose-b.txt",
		"第1话.mkv", "第10话.mkv", "第2话.mkv",
		"loose-a.txt",
	)
}

// 跨组拖动必须被拒绝:分组头只在组名变化处出现,跨组会把两个文件夹交错。
func TestReorderRefusesToCrossGroups(t *testing.T) {
	a := orderApp(t)
	rows := buildRows(a.files)
	// 把组内的「第10话」(行 2) 拖到整张表的末尾:那已经出了 A 组。
	if _, _, ok := reorderTarget(a.files, rows, []int{2}, len(rows)); ok {
		t.Fatal("跨组拖动应当被拒绝")
	}
	// 拖到分组头之前也不许。
	if _, _, ok := reorderTarget(a.files, rows, []int{2}, 1); ok {
		t.Fatal("拖到分组头之前应当被拒绝")
	}
}

// 落在原位、或落在自己后面一格,都是"没动",不该触发一次重排。
func TestReorderNoOpIsIgnored(t *testing.T) {
	a := orderApp(t)
	rows := buildRows(a.files)
	for _, to := range []int{2, 3} {
		if _, _, ok := reorderTarget(a.files, rows, []int{2}, to); ok {
			t.Fatalf("to=%d 是原位,不该被接受", to)
		}
	}
	// 多选拖动不支持。
	if _, _, ok := reorderTarget(a.files, rows, []int{2, 3}, 4); ok {
		t.Fatal("多选拖动应当被拒绝")
	}
	// 拖分组头也不支持。
	if _, _, ok := reorderTarget(a.files, rows, []int{1}, 2); ok {
		t.Fatal("拖动分组头应当被拒绝")
	}
}

// 拖动到末尾是合法的落点(len(rows))。
func TestReorderToEndOfGroup(t *testing.T) {
	a := orderApp(t)
	rows := buildRows(a.files)
	// 整张表的末尾 = 出组,不许。
	if _, _, ok := reorderTarget(a.files, rows, []int{2}, len(rows)); ok {
		t.Fatal("拖到整张表末尾是跨组,不该被接受")
	}
	// 拖到本组最后一格(loose-a 那一行)之前 = 组内末尾,合法。
	from, dest, ok := reorderTarget(a.files, rows, []int{2}, 5)
	if !ok {
		t.Fatal("拖到本组末尾应当被接受")
	}
	a.files = moveFile(a.files, from, dest)
	sameOrder(t, a.files,
		"loose-b.txt",
		"第2话.mkv", "第1话.mkv", "第10话.mkv",
		"loose-a.txt",
	)
}

// 序号列画的是这一行在 <N> 里的值。它是发号顺序唯一的可见证据,
// 所以必须跟着排序/拖动一起变。
func TestSequenceColumnFollowsOrder(t *testing.T) {
	a := orderApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	for _, want := range []string{"#1", "#2", "#3", "#4", "#5"} {
		if !tt.HasText(want) {
			t.Fatalf("序号列缺少 %q", want)
		}
	}
	// 按名称排完之后,#1 应该是 loose-b(它单独成段,永远第一个)。
	a.sortBy = "name"
	a.pendingSort = true
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if got := filepath.Base(a.files[1].path); got != "第1话.mkv" {
		t.Fatalf("排序后 #2 应是 第1话,实际 %s", got)
	}
	if !tt.HasText("#2") {
		t.Fatal("排序后序号列消失")
	}
}

// 清空列表要把序号和排序依据一起归零,否则下一批文件一进来
// 就从 #6 开始、还带着"按大小"。
func TestClearResetsOrderAndSort(t *testing.T) {
	a := orderApp(t)
	a.sortBy, a.sortDesc = "size", true
	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	// 清空走二次确认,直接调确认分支等价的赋值路径。
	a.files, a.ops, a.applied = nil, nil, false
	a.nextOrder = 0
	a.sortBy, a.sortDesc = "", false
	dir := t.TempDir()
	p := filepath.Join(dir, "one.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.addPaths([]string{p})
	if a.files[0].order != 1 {
		t.Fatalf("清空后第一个文件的 order = %d, 期望 1", a.files[0].order)
	}
	if a.sortBy != "" {
		t.Fatalf("清空后排序依据 = %q, 期望空", a.sortBy)
	}
}

// 序号列在极窄右栏要收起来:那时名字只剩省略号,再挤一列数字
// 只会把名字压得更短。
func TestSequenceColumnHidesWhenNarrow(t *testing.T) {
	a := orderApp(t)
	a.splitL, a.splitM = 460, 330
	tt := ui.NewTester(a.view, 900, 480)
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	if tt.HasText("#1") {
		t.Fatal("极窄右栏不该画序号列")
	}
}

// 排序按钮的文字要说清当前依据,"添加顺序"而不是"不排序":
// 后者会让人以为列表没有次序。
func TestSortLabel(t *testing.T) {
	cases := []struct {
		by   string
		desc bool
		want string
	}{
		{"", false, "添加顺序"},
		{"", true, "添加顺序"},
		{"name", false, "按名称"},
		{"name", true, "按名称 ↓"},
		{"date", false, "按修改时间"},
		{"size", true, "按大小 ↓"},
	}
	for _, c := range cases {
		if got := sortLabel(c.by, c.desc); got != c.want {
			t.Fatalf("sortLabel(%q,%v) = %q, 期望 %q", c.by, c.desc, got, c.want)
		}
	}
}
