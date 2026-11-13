package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 详情编辑器固定在中栏下方,不跟着列表滚。
//
// 它编辑的永远是"当前选中的那一步",所以位置也该固定——以前它接在列表
// 最后一行之后、跟着整块一起滚:1 步时选中行与编辑器之间隔 290px 空白,
// 12 步时要滚到底才看得见。用户的原话是"应该悬浮中部下方,因为更改的是
// 选中的步骤而不是永远最下的设置"。
func TestDetailStaysAtBottomOfMiddlePane(t *testing.T) {
	for _, n := range []int{1, 3, 12, 20} {
		a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
		for i := 0; i < n; i++ {
			a.steps = append(a.steps, step("case.lower"))
			a.steps[i].opUID = i + 1
		}
		a.stepUID = n

		tt := ui.NewTester(a.view, 1280, 800)
		for i := 0; i < 3; i++ {
			tt.Frame()
		}
		field, ok := tt.Find("转换方式")
		if !ok {
			t.Fatalf("%d 步:找不到详情里的字段", n)
		}
		if field.H <= 0 {
			t.Fatalf("%d 步:详情不可见(高 %.1f)", n, field.H)
		}
		// 无论几步,详情都停在同一位置(中栏底部)。
		if field.Y < 500 {
			t.Errorf("%d 步:详情跑到 y=%.1f,应当固定在中栏下方", n, field.Y)
		}
	}
}

// 选中别的行时详情跟着换内容,但位置不变——这正是"悬浮"的意义。
func TestDetailFollowsSelectionNotPosition(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	for i, id := range []string{"case.lower", "pinyin", "truncate"} {
		a.steps = append(a.steps, step(id))
		a.steps[i].opUID = i + 1
	}
	a.stepUID = 3

	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	// 左栏目录里本来就有同名条目,所以用"出现次数"判断详情卡片里的是哪一步:
	// 选中的那一步名字会出现两次(目录 + 详情)。
	count := func(label string) int {
		n := 0
		for _, s := range tt.Texts() {
			if s == label {
				n++
			}
		}
		return n
	}
	before := count("全部转小写")
	if before < 2 {
		t.Fatalf("第 1 步选中时详情里应当也有它:只出现 %d 次", before)
	}

	// 选第 3 步:详情换内容,位置不变。
	tt.Click("3.")
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if got := count("全部转小写"); got >= before {
		t.Errorf("切到第 3 步后详情里还是第 1 步:仍出现 %d 次(切前 %d)", got, before)
	}
	if count("截断长度") < 2 {
		t.Error("第 3 步的详情没出现")
	}
}

// 20 步时最后一行滚得出来,详情不被列表挤没。
func TestDetailSurvivesLongList(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	for i := 0; i < 20; i++ {
		a.steps = append(a.steps, step("case.lower"))
		a.steps[i].opUID = i + 1
	}
	a.stepUID = 20

	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	tt.Scroll(300, 400, 0, 3000)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	last, ok := tt.Find(fmt.Sprintf("%d.", 20))
	if !ok || last.H <= 0 {
		t.Fatalf("滚到底也看不到第 20 行: %v 高 %.1f", ok, last.H)
	}
	field, ok := tt.Find("转换方式")
	if !ok || field.H <= 0 {
		t.Fatal("滚到底后详情不见了")
	}
}
