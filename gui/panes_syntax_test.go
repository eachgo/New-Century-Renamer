package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 三栏要用同一套容器语法。左栏以前是"文档语法"——内容直接铺在窗口底色上,
// 没有容器;中/右两栏是"卡片语法"——圆角 + 投影 + 一阶更亮的底。同一屏里
// 两套语言,读者要重新学一遍"这块区域的边界在哪"。
//
// 视觉报告建议的是用**颜色层级**区分区块(而不是再加竖线去对齐):三栏都浮,
// 两侧同色、中间最亮,一眼看出"中间是工作区、两侧是辅助"。
func TestThreePanesShareOneContainerSyntax(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.addPaths([]string{"../testfile"})
	a.steps = []guiRule{step("case.lower")}
	a.stepSel = 0
	tt := ui.NewTester(a.view, 1100, 700)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}

	// 三栏的标题都在同一高度上(以前左栏 y=61、中/右栏 y=67-68)。
	titles := []string{"功能 · 点击加入步骤", "任务步骤（1）", "文件"}
	var ys []float32
	for _, s := range titles {
		r, ok := tt.Find(s)
		if !ok {
			t.Fatalf("找不到栏标题 %q", s)
		}
		ys = append(ys, r.Y)
	}
	lo, hi := ys[0], ys[0]
	for _, y := range ys {
		if y < lo {
			lo = y
		}
		if y > hi {
			hi = y
		}
	}
	// 三个标题的字号不同(13 Bold),文字框高也略有差别,允许 2px 的
	// 亚像素误差;超过就不是"同一条基线"了。
	if hi-lo > 2 {
		t.Errorf("三栏标题不在同一基线上: y=%v", ys)
	}

	// 左栏内部只该有两条竖线:栏标题/分组标题/提示 18,条目文字 32。
	// 以前是五条(12/19/22/26/42),每条都要眼睛解析一次。
	for _, s := range []string{"▾ 筛选/范围", "筛选文件"} {
		if _, ok := tt.Find(s); !ok {
			t.Fatalf("找不到左栏条目 %q", s)
		}
	}
	title, _ := tt.Find("功能 · 点击加入步骤")
	group, _ := tt.Find("▾ 筛选/范围")
	item, _ := tt.Find("筛选文件")
	if group.X != title.X {
		t.Errorf("分组标题 x=%.1f 与栏标题 x=%.1f 不共线", group.X, title.X)
	}
	if d := item.X - title.X; d < 8 || d > 20 {
		t.Errorf("条目文字相对栏标题缩进 %.1f,应当在 8~20 之间(图标列 + gap)", d)
	}
}
