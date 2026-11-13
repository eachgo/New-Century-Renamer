package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 详情编辑器要紧跟选中行,不再钉在栏底。
// 以前 1 步时选中行与编辑器之间隔 290px 空白——中间那一片什么都没有,
// 而参数就在屏幕另一头。
func TestDetailFollowsSelectedStep(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	for i := 0; i < 3; i++ {
		a.steps = append(a.steps, step("case.lower"))
		a.steps[i].opUID = i + 1
	}
	a.stepUID = 3

	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	row, ok := tt.Find("3.")
	if !ok {
		t.Fatal("找不到第 3 行")
	}
	field, ok := tt.Find("转换方式")
	if !ok {
		t.Fatal("找不到详情里的字段")
	}
	gap := field.Y - (row.Y + row.H)
	if gap > 80 {
		t.Fatalf("详情离选中行太远: 空白 %.1fpx(应当紧跟其后)", gap)
	}
	if gap < 0 {
		t.Fatalf("详情跑到选中行上面去了: %.1fpx", gap)
	}
}

// 12 步时列表第 8 行起被分隔线硬切(第 8 行高 11.1、第 9 行起高 0,
// 被 Clip 整行吃掉)。现在列表与详情共用一条滚动轴,行不再被腰斩。
func TestManyStepsAreNotCutInHalf(t *testing.T) {
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	for i := 0; i < 12; i++ {
		a.steps = append(a.steps, step("case.lower"))
		a.steps[i].opUID = i + 1
	}
	a.stepUID = 12

	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	var first ui.Rect
	for i := 1; i <= 12; i++ {
		r, ok := tt.Find(fmt.Sprintf("%d.", i))
		if !ok {
			t.Fatalf("第 %d 行不存在", i)
		}
		if i == 1 {
			first = r
			continue
		}
		// 每行要么完整可见(高度与首行一致),要么在可视区外(高度 0)。
		// "高 11.1" 这种半截状态就是以前被分隔线切掉的样子。
		if r.H > 0 && r.H < first.H-1 {
			t.Fatalf("第 %d 行被切了一半: 高 %.1f,首行 %.1f", i, r.H, first.H)
		}
	}
}

// 滚到底要能看见最后一步的详情,而不是永远够不着。
func TestDetailReachableByScrolling(t *testing.T) {
	a := &app{stepSel: 19, fileSel: -1, settings: defaultSettings()}
	for i := 0; i < 20; i++ {
		a.steps = append(a.steps, step("case.lower"))
		a.steps[i].opUID = i + 1
	}
	a.stepUID = 20

	tt := ui.NewTester(a.view, 1280, 800)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	tt.Scroll(300, 500, 0, 3000)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	field, ok := tt.Find("转换方式")
	if !ok {
		t.Fatal("找不到详情里的字段")
	}
	if field.H <= 0 {
		t.Fatalf("滚到底也看不到最后一步的详情: 高 %.1f", field.H)
	}
}
