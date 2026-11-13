package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func hasExactText(all []string, want string) bool {
	for _, s := range all {
		if s == want {
			return true
		}
	}
	return false
}

// 图例只在真有范围时出现——没开范围就多一行说明,等于凭空加噪。
// 它存在的理由是范围用色条表示(色条能说明"持续到哪一行",文字不能),
// 而色条必须能自解释:报告原本的批评是"没有图例",不是"色条错了"。
func TestRangeLegendOnlyWhenARangeIsOpen(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: LoadSettings()}
	a.steps = []guiRule{step("case.lower"), step("pinyin")}
	a.steps[0].opUID, a.steps[1].opUID = 1, 2
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	if hasExactText(tt.Texts(), "范围") {
		t.Fatal("没开任何范围时不该有图例")
	}

	a.steps = append(a.steps, step("scope.name"))
	a.steps[2].opUID = 3
	tt.Frame()
	if !hasExactText(tt.Texts(), "范围") {
		t.Fatal("开了选取范围之后应当出现图例")
	}
	if !hasExactText(tt.Texts(), "仅文件名") {
		t.Fatal("图例该说清是哪种范围")
	}
}

// 图例只列此刻生效的那几种,不把三种范围一次列全——列全了又变成
// 一份要读的说明书,而不是"对上颜色"的速查。
func TestRangeLegendListsOnlyOpenScopes(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: LoadSettings()}
	a.steps = []guiRule{step("scope.name")}
	a.steps[0].opUID = 1
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	if !hasExactText(tt.Texts(), "仅文件名") {
		t.Fatal("开着的范围应当在图例里")
	}
	if hasExactText(tt.Texts(), "仅扩展名") {
		t.Fatal("没开的范围不该出现在图例里")
	}
}
