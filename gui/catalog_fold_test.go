package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 默认只展开前两组:11 组 32 条全摊开时一屏只放得下前三四组,用户既
// 不知道下面还有多少,也不知道该从哪儿找起。但也不能全折——全折之后
// 左栏只剩 11 行组名,第一次打开的人看不出这里装的是能点的规则。
func TestCatalogGroupsStartPartiallyOpen(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	open, shut := 0, 0
	for _, g := range stepGroups {
		if groupLabel(tt, "▾", g) != "" {
			open++
		}
		if groupLabel(tt, "▸", g) != "" {
			shut++
		}
	}
	if open != defaultOpenGroups {
		t.Fatalf("默认应当只展开 %d 组,实际 %d 组", defaultOpenGroups, open)
	}
	if shut != len(stepGroups)-defaultOpenGroups {
		t.Fatalf("其余 %d 组应当折起,实际 %d 组", len(stepGroups)-defaultOpenGroups, shut)
	}
	// 展开的那两组(筛选/范围、替换)里的条目要真的在
	if !tt.HasText("筛选文件") || !tt.HasText("查找替换") {
		t.Fatal("展开的组里应当看得见条目")
	}
	// 折起的组里条目不该占位(结构已折起,「文件名前添加」在里面)
	if tt.HasText("文件名前添加") {
		t.Fatal("折起的组里条目不该还在")
	}
}

// 折起来之后组里的条目要真的消失,而且组标题还留着——否则用户没有
// 办法再展开。
func TestCatalogGroupCollapses(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	// 编号组默认是折起的,先展开它
	if err := tt.Click(groupLabel(tt, "▸", "编号")); err != nil {
		t.Fatalf("组标题应当可点: %v", err)
	}
	tt.Frame()
	if err := tt.Click(groupLabel(tt, "▾", "编号")); err != nil {
		t.Fatalf("组标题应当可点: %v", err)
	}
	tt.Frame()
	if groupLabel(tt, "▸", "编号") == "" {
		t.Fatal("点过之后组标题应当变成折起状态（▸）")
	}
	if tt.HasText("追加序号") {
		t.Fatal("折起之后组里的条目不该还在")
	}

	// 再点一次展开回来
	if err := tt.Click(groupLabel(tt, "▸", "编号")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("追加序号") {
		t.Fatal("再点一次应当展开回原样")
	}
}

// 折叠状态跟着组走,不是"全局折起":折一个组不该影响别的组。
func TestCatalogCollapseIsPerGroup(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	// 编号默认折起,先展开再折回,确认只影响自己
	if err := tt.Click(groupLabel(tt, "▸", "编号")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(groupLabel(tt, "▾", "编号")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// 元数据组默认也是折起的(它排在第 8 位,不在前两组里),所以这里
	// 断言的是"它没有被连带展开/折起",不是"它是开的"。
	if groupLabel(tt, "▸", "元数据") == "" {
		t.Fatal("折起编号组不该改变元数据组的状态")
	}
	// 展开它,条目应当在;再折起编号组,它不该跟着变
	if err := tt.Click(groupLabel(tt, "▸", "元数据")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("用标题命名") {
		t.Fatal("元数据组展开后条目应当可见")
	}
}

// groupLabel 返回当前帧里某个组标题的完整文字（含条数），找不到返回空串。
//
// 组标题现在带条数（「▾ 替换(6)」），而 tt.Find / tt.Click 要求整串精确
// 匹配。条数会随目录变化，所以测试不该把数字写死——按前缀找，再拿找到
// 的那一串去点。
func groupLabel(tt *ui.Tester, arrow, group string) string {
	prefix := arrow + " " + group + "("
	for _, s := range tt.Texts() {
		if strings.HasPrefix(s, prefix) {
			return s
		}
	}
	return ""
}

// 组标题要带条数。
//
// 11 组折起来时一屏只看得到十一个词：「编号」「日期」「高级」——用户没法
// 判断哪一组里有他要的东西，只能挨个展开。带上条数之后，「元数据(3)」
// 和「日期(1)」的重量一眼分得出来。
func TestCatalogGroupHeadShowsCount(t *testing.T) {
	// 纯函数先守一遍：数字必须来自真实条数，不是写死的。
	if got := groupHead("▾", "替换", 6); got != "▾ 替换(6)" {
		t.Fatalf("组标题格式不对: %q", got)
	}

	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	// 每一组的括号里必须是它在 catalog 里的真实条数。
	for _, g := range stepGroups {
		want := 0
		for _, d := range catalog {
			if d.group == g {
				want++
			}
		}
		label := groupLabel(tt, "▾", g)
		if label == "" {
			label = groupLabel(tt, "▸", g)
		}
		if label == "" {
			t.Fatalf("组 %q 在界面上没有标题", g)
		}
		if got := fmt.Sprintf("%s %s(%d)", string([]rune(label)[0]), g, want); got != label {
			t.Errorf("组 %q 的标题是 %q,想要 %q", g, label, got)
		}
	}
}

// 搜索时括号里的数字要跟着结果变。
//
// 「元数据(3)」在搜「标题」时下面只挂着一条，而标题还写 3——那比不带
// 条数更让人困惑。
func TestGroupCountFollowsSearch(t *testing.T) {
	// 搜「编号」时「元数据」组只留下一条（「用编号命名」之类），而它
	// 本来有 3 条——总数与命中数不同，才分得清括号里数的是哪一个。
	const query, group = "编号", "元数据"
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.palQuery = query
	tt := ui.NewTester(a.view, 1100, 1700)
	tt.Frame()
	tt.Frame()

	label := groupLabel(tt, "·", group)
	if label == "" {
		t.Fatalf("搜索时 %s 组没有标题,界面文字: %v", group, tt.Texts())
	}
	// 数一数这组在过滤后的结果里到底几条，和括号里的数字比。
	want := 0
	total := 0
	for _, d := range catalog {
		if d.group != group {
			continue
		}
		total++
		if catalogMatch(d, query) {
			want++
		}
	}
	if want == total {
		t.Fatalf("这组命中 %d 条 / 共 %d 条,两者相同就分不清括号里数的是哪个", want, total)
	}
	if got := fmt.Sprintf("· %s(%d)", group, want); got != label {
		t.Fatalf("搜索时组标题是 %q,想要 %q(数字必须跟着结果变,不能是总数)", label, got)
	}
}
