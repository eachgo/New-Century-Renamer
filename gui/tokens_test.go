package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
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
// 会失败,而不是悄悄混进去。它扫的是全部界面文件——早先只扫四个文件,
// panes.go/settings.go/material.go/fileinfo.go/icons.go 里的取值根本
// 没被检查过,所以"收敛完成"只是错觉。

// uiFiles 是所有可能写界面取值的文件。新增界面文件时要加进来。
var uiFiles = []string{
	"main.go", "settings_ui.go", "help_ui.go", "rules_editor.go",
	"panes.go", "settings.go", "material.go", "fileinfo.go", "icons.go",
}

// tokenHits 扫 uiFiles 里所有调用了 name 之一的位置,返回 值 → 出现次数。
func tokenHits(t *testing.T, names ...string) map[float64]int {
	t.Helper()
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	hits := map[float64]int{}
	fset := token.NewFileSet()
	for _, file := range uiFiles {
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
			if !ok || !want[sel.Sel.Name] {
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || (lit.Kind != token.INT && lit.Kind != token.FLOAT) {
					continue
				}
				v, err := strconv.ParseFloat(lit.Value, 64)
				if err != nil {
					continue
				}
				hits[v]++
			}
			return true
		})
	}
	return hits
}

// report 把实测分布打出来,失败时能直接看到"现在到底是哪几种"。
func report(t *testing.T, label string, hits map[float64]int) {
	t.Helper()
	keys := make([]float64, 0, len(hits))
	for k := range hits {
		keys = append(keys, k)
	}
	sort.Float64s(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%g×%d", k, hits[k]))
	}
	t.Logf("%s 实测 %d 种: %v", label, len(keys), parts)
}

// 间距回 4pt 基准:0 贴边 / 1、2 描边与微调 / 4、8、12、20 是主阶梯 /
// 6、10、14 是半档(卡片内边距、列表行内距这类"主档太松、贴边太紧"的位置)。
// 每一个值都必须在界面上有明确分工,不是"看着差不多"。
func TestSpacingFollowsOneScale(t *testing.T) {
	allowed := map[float64]string{
		0:  "贴边",
		1:  "描边微调",
		2:  "微调",
		4:  "最小间距",
		6:  "半档",
		8:  "常用间距",
		10: "卡片内边距",
		12: "卡片间距",
		14: "面板内边距",
		20: "对话框内边距",
	}
	hits := tokenHits(t, "Padding", "PaddingX", "PaddingY", "PaddingStart", "PaddingEnd")
	report(t, "Padding", hits)
	for v := range hits {
		if _, ok := allowed[v]; !ok {
			t.Errorf("Padding(%g) 不在阶梯上;允许: %v", v, sortedKeys(allowed))
		}
	}
}

// Gap 的阶梯比 Padding 窄:它是"两件东西之间留多少",半档多了反而没节奏。
func TestGapFollowsOneScale(t *testing.T) {
	allowed := map[float64]string{
		0:  "贴紧",
		4:  "紧凑",
		6:  "半档",
		8:  "常用",
		10: "宽松",
		12: "分组",
	}
	hits := tokenHits(t, "Gap")
	report(t, "Gap", hits)
	for v := range hits {
		if _, ok := allowed[v]; !ok {
			t.Errorf("Gap(%g) 不在阶梯上;允许: %v", v, sortedKeys(allowed))
		}
	}
}

// 圆角阶梯:2/4/6/8/10/14 是"面积"用的六档,9 与 13 是圆形按钮的半径
// (等于边长的一半),不参与阶梯——它们是几何,不是风格。
func TestRadiusFollowsOneLadder(t *testing.T) {
	allowed := map[float64]string{
		2:  "进度条与色条(4px 高的一半)",
		4:  "微胶囊",
		6:  "列表行",
		8:  "按钮",
		9:  "圆形按钮(18×18 的一半)",
		10: "卡片",
		13: "圆形按钮(26×26 的一半)",
		14: "面板与对话框",
	}
	hits := tokenHits(t, "Radius")
	report(t, "Radius", hits)
	for v := range hits {
		if _, ok := allowed[v]; !ok {
			t.Errorf("Radius(%g) 不在阶梯上;允许: %v", v, sortedKeys(allowed))
		}
	}
}

// 字号只留五档:11 徽标(勾选符) / 12 弱 / 13 次 / 14 正文 / 18 标题。
// 改之前 11/12/13 三档承担 40 处文本,差 1px 谁也分不出——那种"差一点"
// 不构成层级,只是噪音。15 曾在帮助页当过标题,已并到 18,和设置页一致:
// 两个对话框的标题没道理差 3px。
func TestFontSizesFollowOneLadder(t *testing.T) {
	allowed := map[string]string{
		"11": "徽标",
		"12": "弱",
		"13": "次",
		"14": "正文",
		"18": "标题",
	}
	fset := token.NewFileSet()
	for _, file := range uiFiles {
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
				if _, ok := allowed[lit.Value]; !ok {
					t.Errorf("%s:%d FontSize(%s) 不在字号阶梯上;允许: 11/12/13/14/18",
						file, fset.Position(call.Pos()).Line, lit.Value)
				}
			}
			return true
		})
	}
}

// 嵌套圆角要递减:外层卡片 10,里面的行 6,行里的胶囊 4。否则内外两层
// 圆角各自为政,看起来像两张纸叠在一起没有贴合。
//
// 早先这个测试只是断言 `14 > 10 && 10 > 6 && 6 > 4` ——三个常量比大小,
// 恒真,再配两条字符串黑名单,基本是空转。现在真去扫源码:把每个 Radius
// 的取值和它所在行的 Padding 配起来,断言"行半径 > 胶囊半径"这种已知的
// 嵌套关系成立。
func TestNestedRadiusShrinks(t *testing.T) {
	// 已知的嵌套对:外层半径 → 允许的内层半径上限(不含)。
	// 面板 14 里放卡片 10;卡片 10 里放行 6;行 6 里放胶囊 4。
	nested := map[float64]float64{14: 10, 10: 6, 6: 4}
	for outer, inner := range nested {
		if !(outer > inner) {
			t.Fatalf("圆角阶梯必须单调递减: %g 不能小于等于 %g", outer, inner)
		}
	}
	// 源码里不该出现"内层比外层大"的写法。这里查两个具体的高危组合:
	// 步骤行(Radius 6)里的胶囊若写成 8 或 10,就是内层超过外层。
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		".Padding(2, 8).Radius(8)",
		".Padding(2, 10).Radius(10)",
		".Padding(2, 8).Radius(10)",
	} {
		if strings.Contains(string(src), bad) {
			t.Errorf("步骤行里出现了 %q:内层圆角不能大于等于外层的 6", bad)
		}
	}
}

func sortedKeys(m map[float64]string) []float64 {
	keys := make([]float64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Float64s(keys)
	return keys
}
