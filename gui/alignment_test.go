package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// alignApp 构造一个"三栏都有内容"的窗口:左栏有分组,中栏有一步,
// 右栏有三个文件(带序号列)。空栏的标题和有条目的栏标题在不同布局
// 分支里生成,只有三栏都有东西时才能比出基线。
func alignApp(t *testing.T) (*app, *ui.Tester, int, int) {
	t.Helper()
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	for i, n := range []string{"aaa.txt", "bbb.txt", "ccc.txt"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		f := statFile(p)
		f.order = i + 1
		a.files = append(a.files, f)
	}
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	a.preview()
	return a, nil, 1280, 800
}

func findY(t *testing.T, tt *ui.Tester, s string) (ui.Rect, bool) {
	t.Helper()
	r, ok := tt.Find(s)
	if !ok {
		t.Fatalf("找不到 %q", s)
	}
	return r, ok
}

// TestThreePaneTitlesShareOneBaseline 三栏标题必须落在同一条基线上。
//
// 这不是审美问题:三个标题是三栏的入口,它们错开半像素时扫视会先
// 看到"文件"再看到另两个,像是右栏从别处拼过来的。修复前右栏标题
// 高 0.67 DIP——原因是工具栏把 MinHeight(34) 和 Wrap() 挂在同一个
// Row 上,而 mygo 里 Wrap 一旦生效 MinHeight 就不再撑高度。
func TestThreePaneTitlesShareOneBaseline(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	dir := t.TempDir()
	a.settings.Journal = filepath.Join(dir, "journal.json")
	p := filepath.Join(dir, "aaa.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.files = append(a.files, statFile(p))
	a.preview()

	for _, w := range []int{1280, 1120, 1000, 900} {
		tt := ui.NewTester(a.view, w, 700)
		tt.Frame()
		tt.Frame()
		left, _ := findY(t, tt, "功能 · 点击加入步骤")
		mid, _ := findY(t, tt, "任务步骤（0）")
		right, _ := findY(t, tt, "文件")
		if d := mid.Y - left.Y; d > 0.5 || d < -0.5 {
			t.Errorf("w=%d: 中栏标题 y=%.2f,左栏 y=%.2f,差 %.2f DIP(应同基线)", w, mid.Y, left.Y, d)
		}
		if d := right.Y - left.Y; d > 0.5 || d < -0.5 {
			t.Errorf("w=%d: 右栏标题 y=%.2f,左栏 y=%.2f,差 %.2f DIP(应同基线)", w, right.Y, left.Y, d)
		}
		// 标题被压成竖排时 y 会变、宽会缩:一并守住。
		if right.H > right.W {
			t.Errorf("w=%d: 右栏标题被压成竖排 %.2f×%.2f", w, right.W, right.H)
		}
	}
}

// TestCatalogGroupHeaderAlignsWithItems 左栏组标题与条目共用同一条左轴。
//
// 修复前组标题走 ButtonBase 的默认居中(文本 x=84.85),条目左对齐
// (x=32.00):一列折叠列表里,上级和下级左缘差 50 多 DIP,扫下来像
// 两个不相干的列表。
func TestCatalogGroupHeaderAlignsWithItems(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1280, 700)
	tt.Frame()
	tt.Frame()

	// 组标题带条数（「▾ 替换(6)」），条数会随目录变化，不能写死。
	headLabel := groupLabel(tt, "▾", "替换")
	head, ok := tt.Find(headLabel)
	if !ok {
		t.Fatalf("找不到组标题 %q", headLabel)
	}
	// 组标题的 ▾ 与条目的图标占同一个槽位,所以比的是"文本节点的左缘":
	// 组标题已 FillWidth 撑满,靠 TextAlign(Start) 贴左;条目文字前面
	// 有 14 宽的图标。两者都从按钮左缘起算,差的是图标宽度。
	var btnX float32
	for _, x := range tt.AccessAll() {
		if x.Role == 4 && x.Name == headLabel {
			btnX = x.Bounds.X
			break
		}
	}
	if btnX == 0 {
		t.Fatal("找不到组标题按钮")
	}
	if d := head.X - btnX; d > 0.5 || d < -0.5 {
		t.Errorf("组标题文本 x=%.2f,按钮 x=%.2f,差 %.2f DIP——它又被居中了吧", head.X, btnX, d)
	}
	// 条目文字比按钮多一个图标宽度(14),这是设计上的缩进层级。
	item, ok := tt.Find("查找替换")
	if !ok {
		t.Fatal("找不到条目「查找替换」")
	}
	if d := item.X - btnX; d < 10 || d > 18 {
		t.Errorf("条目文字 x=%.2f 相对按钮 x=%.2f 偏了 %.2f DIP(应约一个图标宽 14)", item.X, btnX, d)
	}
}

// TestFileListHeaderSitsOverNames 右栏列头"原名 → 新名"必须压在第一列名字上方。
//
// 行里有序号列(seqW=30 + gap),表头没有,于是表头从 x=598 起、名字从
// x=636 起,差 38 DIP:表头说的是左边那一列,读者看到的是右边那一列。
func TestFileListHeaderSitsOverNames(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	dir := t.TempDir()
	a.settings.Journal = filepath.Join(dir, "journal.json")
	p := filepath.Join(dir, "aaa.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.files = append(a.files, statFile(p))
	a.preview()

	for _, w := range []int{1280, 1000} {
		tt := ui.NewTester(a.view, w, 700)
		tt.Frame()
		tt.Frame()
		head, ok := tt.Find("原名 → 新名")
		if !ok {
			t.Fatalf("w=%d: 找不到列头", w)
		}
		name, ok := tt.Find("aaa")
		if !ok {
			t.Fatalf("w=%d: 找不到文件名胶囊", w)
		}
		if d := name.X - head.X; d > 0.5 || d < -0.5 {
			t.Errorf("w=%d: 列头 x=%.2f,名字 x=%.2f,差 %.2f DIP(表头没让出序号列)", w, head.X, name.X, d)
		}
	}
}

// TestStepListIndentMatchesParamCard 中栏列表编号与参数卡片标签同轴。
//
// 编号原来在 x=254,参数卡片标题/标签在 x=258(栏内边距 10)。差 4 DIP
// 看不出是"两栏拼的",但列表与它下面的编辑器左缘错开时,眼睛会以为
// 参数卡片不属于这一栏。
func TestStepListIndentMatchesParamCard(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	dir := t.TempDir()
	a.settings.Journal = filepath.Join(dir, "journal.json")
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	a.preview()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()

	num, ok := tt.Find("1.")
	if !ok {
		t.Fatal("找不到列表编号「1.」")
	}
	label, ok := tt.Find("转换方式")
	if !ok {
		t.Fatal("找不到参数标签「转换方式」")
	}
	if d := label.X - num.X; d > 0.5 || d < -0.5 {
		t.Errorf("列表编号 x=%.2f,参数标签 x=%.2f,差 %.2f DIP(应同轴)", num.X, label.X, d)
	}
}
