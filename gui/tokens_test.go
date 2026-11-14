package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
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

	// 棘轮:当前实测 10 种,只许减不许增。白名单只说"这个值合法",
	// 于是它允许集合里每一个值都同时存在——收敛到 3 档和保持 10 档
	// 都能通过,那这个测试就只是在记录现状,不是在要求收敛。上限定成
	// 实测值:新增一档立刻失败,删掉一档不用改测试(留出余量让收敛继续)。
	if len(hits) > 10 {
		t.Errorf("Padding 有 %d 种取值(上限 10):只许收敛,不许再分叉。当前 %v",
			len(hits), sortedHitKeys(hits))
	}
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

	// 棘轮:当前实测 6 种,只许减不许增。白名单只说"这个值合法",
	// 于是它允许集合里每一个值都同时存在——收敛到 3 档和保持 6 档
	// 都能通过,那这个测试就只是在记录现状,不是在要求收敛。上限定成
	// 实测值:新增一档立刻失败,删掉一档不用改测试(留出余量让收敛继续)。
	if len(hits) > 6 {
		t.Errorf("Gap 有 %d 种取值(上限 6):只许收敛,不许再分叉。当前 %v",
			len(hits), sortedHitKeys(hits))
	}
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

	// 棘轮:当前实测 8 种,只许减不许增。白名单只说"这个值合法",
	// 于是它允许集合里每一个值都同时存在——收敛到 3 档和保持 8 档
	// 都能通过,那这个测试就只是在记录现状,不是在要求收敛。上限定成
	// 实测值:新增一档立刻失败,删掉一档不用改测试(留出余量让收敛继续)。
	if len(hits) > 8 {
		t.Errorf("Radius 有 %d 种取值(上限 8):只许收敛,不许再分叉。当前 %v",
			len(hits), sortedHitKeys(hits))
	}
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
	used := map[string]bool{}
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
				used[lit.Value] = true
			}
			return true
		})
	}
	// 棘轮:当前实测 5 档,只许减不许增。理由同 Padding 那条——
	// 白名单只说"这一档合法",五档同时存在也算通过,那它记录的是
	// 现状而不是要求。字号是层级,层级越多,层级就越不成立。
	if len(used) > 5 {
		t.Errorf("字号有 %d 档(上限 5):只许收敛,不许再分叉。当前 %v", len(used), used)
	}
}

// 嵌套圆角要递减:外层卡片 10,里面的行 6,行里的胶囊 4。内层圆角大于外层
// 时,两层边线在角上打架,看起来像两张纸没对齐。
//
// 早先这个测试只是断言 `14 > 10 && 10 > 6 && 6 > 4` ——三个常量比大小,
// 恒真;再配三条字符串黑名单,而那三种写法在源码里根本不存在,基本是
// 空转。现在真去扫源码:顺着 `.Children(func(){...})` 建立父子链,内层
// 的 Radius 不许大于外层。
//
// 这条测试第一次真跑起来就抓到了 7 处:6 处是"内外同值"(进度条轨道与
// 填充同为 2、对话框 14 里放 14 的卡片——同值不算错,轨道必须和填充同
// 半径),1 处是真错:步骤行(6)里放了 Radius(8) 的确认按钮。修完是 0 处。
func TestNestedRadiusShrinks(t *testing.T) {
	fset := token.NewFileSet()
	type nestViolation struct {
		file         string
		line         int
		outer, inner float64
	}
	var violations []nestViolation
	var walk func(file string, node ast.Node, stack []float64)
	walk = func(file string, node ast.Node, stack []float64) {
		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// .Children(func(){...}) 是唯一能确立父子关系的写法:
			// 回调体里的元素就是外层元素的子元素。
			if sel.Sel.Name == "Children" && len(call.Args) == 1 {
				if fn, ok := call.Args[0].(*ast.FuncLit); ok {
					next := stack
					if r, has := radiusInChain(call); has {
						next = append(append([]float64{}, stack...), r)
					}
					walk(file, fn.Body, next)
					return false // 回调体已单独走过,不要重复遍历
				}
			}
			if sel.Sel.Name == "Radius" && len(stack) > 0 && len(call.Args) == 1 {
				if lit, ok := call.Args[0].(*ast.BasicLit); ok {
					if v, err := strconv.ParseFloat(lit.Value, 64); err == nil {
						if outer := stack[len(stack)-1]; v > outer {
							violations = append(violations, nestViolation{
								file, fset.Position(call.Pos()).Line, outer, v})
						}
					}
				}
			}
			return true
		})
	}
	for _, file := range uiFiles {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, file, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		walk(file, f, nil)
	}
	for _, v := range violations {
		t.Errorf("%s:%d 内层圆角 %g 大于外层 %g", v.file, v.line, v.inner, v.outer)
	}
}

// radiusInChain 沿着链式调用的接收者往回找 Radius(v)。只认字面量:
// 主题里算出来的圆角由 TestRadiusFollowsOneLadder 管。
func radiusInChain(call *ast.CallExpr) (float64, bool) {
	for e := ast.Expr(call); ; {
		c, ok := e.(*ast.CallExpr)
		if !ok {
			return 0, false
		}
		sel, ok := c.Fun.(*ast.SelectorExpr)
		if !ok {
			return 0, false
		}
		if sel.Sel.Name == "Radius" && len(c.Args) == 1 {
			if lit, ok := c.Args[0].(*ast.BasicLit); ok {
				if v, err := strconv.ParseFloat(lit.Value, 64); err == nil {
					return v, true
				}
			}
		}
		e = sel.X
	}
}

// sortedHitKeys 是 sortedKeys 的计数版:棘轮报错要打印的是"当前有哪些
// 取值",不是"允许哪些取值"。
func sortedHitKeys(m map[float64]int) []float64 {
	keys := make([]float64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Float64s(keys)
	return keys
}

func sortedKeys(m map[float64]string) []float64 {
	keys := make([]float64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Float64s(keys)
	return keys
}
