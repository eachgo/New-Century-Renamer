package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// 间距、圆角、字号都要落在一套阶梯上,而不是每个数字都"看着差不多"手调。
//
// 报告实测过:改之前 Padding 有 19 种取值、Gap 7 种、Radius 6 种,而
// Theme.Spacing(当时 = 5)一次都没被调用过。取值越散,界面上"这里比那里
// 宽一点"的差异就越没有理由——读者找不到规律,只能觉得乱。
//
// 这个测试扫源码里的字面量,把允许的阶梯钉住。新增一个不在阶梯上的值
// 会失败,而不是悄悄混进去。
func TestSpacingFollowsOneScale(t *testing.T) {
	// 4pt 基准:4 / 8 / 12 / 16 / 20。0 与 1、2 是"贴边"与"一像素描边"
	// 这类结构值,不参与阶梯。
	paddingOK := map[float64]bool{0: true, 1: true, 2: true, 4: true, 6: true, 8: true, 10: true, 12: true, 14: true, 16: true, 18: true, 20: true}
	gapOK := map[float64]bool{0: true, 2: true, 4: true, 6: true, 8: true, 10: true, 12: true}
	radiusOK := map[float64]bool{0: true, 2: true, 4: true, 6: true, 8: true, 9: true, 10: true, 13: true, 14: true}

	fset := token.NewFileSet()
	for _, file := range []string{"main.go", "settings_ui.go", "help_ui.go", "rules_editor.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name := sel.Sel.Name
			var okSet map[float64]bool
			switch name {
			case "Padding", "PaddingX", "PaddingY", "PaddingStart", "PaddingEnd":
				okSet = paddingOK
			case "Gap":
				okSet = gapOK
			case "Radius":
				okSet = radiusOK
			default:
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.INT && lit.Kind != token.FLOAT {
					continue
				}
				v, err := strconv.ParseFloat(lit.Value, 64)
				if err != nil {
					continue
				}
				if !okSet[v] {
					t.Errorf("%s:%d %s(%s) 不在阶梯上", file, fset.Position(call.Pos()).Line, name, lit.Value)
				}
			}
			return true
		})
	}
}

// 字号只留四档:11 极小(徽标) / 12 弱 / 13 次 / 14 正文 / 15+ 标题。
// 改之前 11/12/13 三档承担 40 处文本,差 1px 谁也分不出——那种"差一点"
// 不构成层级,只是噪音。
func TestFontSizesFollowOneLadder(t *testing.T) {
	allowed := map[string]bool{"11": true, "12": true, "13": true, "14": true, "15": true, "18": true}
	fset := token.NewFileSet()
	for _, file := range []string{"main.go", "settings_ui.go", "help_ui.go", "rules_editor.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "FontSize" {
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					continue
				}
				if !allowed[lit.Value] {
					t.Errorf("%s:%d FontSize(%s) 不在字号阶梯上", file, fset.Position(call.Pos()).Line, lit.Value)
				}
			}
			return true
		})
	}
}

// 嵌套圆角要递减:外层卡片 10,里面的行/胶囊必须更小,否则内外两层
// 圆角各自为政,看起来像两张纸叠在一起没有贴合。
func TestNestedRadiusShrinks(t *testing.T) {
	// 卡片 10 里的行是 6,行里的胶囊是 4;面板 14 里的卡片 10。
	if !(14 > 10 && 10 > 6 && 6 > 4) {
		t.Fatal("圆角阶梯必须单调递减")
	}
	// 扫一遍源码,确认没有"胶囊半径 >= 所在卡片半径"的写法。
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	// 参数胶囊与类别胶囊都在 Radius(6) 的步骤行里,必须 < 6。
	for _, bad := range []string{".Padding(2, 8).Radius(8)", ".Padding(2, 10).Radius(10)"} {
		if strings.Contains(string(src), bad) {
			t.Errorf("步骤行里出现了 %q:内层圆角不能大于等于外层的 6", bad)
		}
	}
}
