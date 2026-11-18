package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// pinStarApp 起一个空应用,左栏目录用默认展开的前两组。
// 设置文件重定向到临时目录,否则钉选会写进开发者本机的那份。
func pinStarApp(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	dir := t.TempDir()
	settingsFilePath = func() string { return filepath.Join(dir, "settings.json") }
	t.Cleanup(func() { settingsFilePath = settingsPath })
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	return a, tt
}

// 钉选原来只有右键菜单。而「常用」这个组只有钉过东西之后才存在——没钉过的
// 人看不到这个功能,也就不会想到去右键试。行内星标是那个功能的可见入口。
//
// 星标不能常驻:目录一屏要放下几十条,每行多一颗常驻的星,等于每行多一个
// 可点区域,还抢走规则名的宽度。所以它只在悬停这一行、或这条已经钉住时出现。
func TestPinStarAppearsOnHover(t *testing.T) {
	a, tt := pinStarApp(t)

	// 一开始不该有星标:既没钉过,鼠标也不在任何一行上。
	if _, ok := tt.Find("☆"); ok {
		t.Fatal("没有悬停也没有钉选,却已经画出了星标")
	}

	r, ok := tt.Find("筛选文件")
	if !ok {
		t.Fatal("找不到目录条目「筛选文件」")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	tt.Frame()

	star, ok := tt.Find("☆")
	if !ok {
		t.Fatal("悬停目录条目时应当出现钉选星标")
	}
	// 星标要落在条目右端,不能压在规则名上。
	if star.X < r.X+r.W*0.7 {
		t.Errorf("星标 x=%.1f 落在规则名中间(条目文字右缘 %.1f),它该在最右侧",
			star.X, r.X+r.W)
	}

	// 点星标只钉住,不能顺手把这条规则加进步骤——它是条目里的另一个按钮。
	//
	// 先把指针移到星标上再点:用户就是这么点的,而且"移上去"这一步让星标
	// 从"因为整行悬停才画出来"变成"自己悬停着",跨平台的字宽差异不会再
	// 决定点击落在星标上还是落在行上。
	clickStar(tt, star)
	tt.Frame()
	tt.Frame()

	if len(a.pinned) != 1 || a.pinned[0] != "filter" {
		t.Fatalf("点星标后应当钉住 filter,实际 %v", a.pinned)
	}
	if len(a.steps) != 0 {
		t.Fatalf("点星标不该顺手加一步,实际加了 %d 步", len(a.steps))
	}

	// 鼠标移开:实星必须还在,否则用户找不到"取消钉住"。
	tt.Move(5, 5)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("★"); !ok {
		t.Fatal("钉住之后,鼠标移开实星就消失了——没法取消钉住")
	}
	// 「常用」组跟着出现,并且带条数。
	if !tt.HasText("▾ 常用(1)") && !tt.HasText("▸ 常用(1)") {
		t.Fatalf("钉住之后左栏顶部应当出现「常用」组,实际组标题是 %v", groupTitles(tt))
	}
}

// 再点一次要取消钉住,而且「常用」组跟着消失。
func TestPinStarTogglesOff(t *testing.T) {
	a, tt := pinStarApp(t)
	a.pinned = []string{"filter"}
	a.settings.Pinned = a.pinned
	tt.Frame()

	star, ok := tt.Find("★")
	if !ok {
		t.Fatal("已经钉住的条目,实星应当一直可见(不需要悬停)")
	}
	clickStar(tt, star)
	tt.Frame()
	tt.Frame()

	if len(a.pinned) != 0 {
		t.Fatalf("再点一次应当取消钉住,实际 %v", a.pinned)
	}
	if tt.HasText("▾ 常用(0)") || tt.HasText("▸ 常用(0)") {
		t.Error("一条都没钉的时候不该留一个空的「常用」组")
	}
}

// 钉住要落盘:这是设置里的一项,重启后还得在。
func TestPinStarSavesToSettings(t *testing.T) {
	_, tt := pinStarApp(t)
	r, _ := tt.Find("筛选文件")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	star, ok := tt.Find("☆")
	if !ok {
		t.Fatal("找不到星标")
	}
	clickStar(tt, star)
	tt.Frame()

	got := LoadSettings()
	if len(got.Pinned) != 1 || got.Pinned[0] != "filter" {
		t.Fatalf("钉选没有写进设置文件,读回 %v", got.Pinned)
	}
}

// groupTitles 收集当前界面上所有"箭头 + 组名(条数)"形式的组标题。
func groupTitles(tt *ui.Tester) []string {
	var out []string
	for _, s := range tt.Texts() {
		if strings.HasPrefix(s, "▾ ") || strings.HasPrefix(s, "▸ ") || strings.HasPrefix(s, "· ") {
			out = append(out, s)
		}
	}
	return out
}

// clickStar 把指针移到星标上再点它。
//
// 先移再点不只是"更像用户":目录右端浮着一条竖滚动条,而滚动条压在所有
// 内容之上。星标贴着右边缘时,它的中心正好落在滚动条轨道里,点下去命中的
// 是滚动条,钉不住也取消不了(实测 tt.Click 报 "covered by a scroll bar")。
// 这条路径同时守着"星标没有被滚动条盖住"。
func clickStar(tt *ui.Tester, star ui.Rect) {
	cx, cy := star.X+star.W/2, star.Y+star.H/2
	tt.Move(cx, cy)
	tt.Frame()
	tt.ClickAt(cx, cy)
}
