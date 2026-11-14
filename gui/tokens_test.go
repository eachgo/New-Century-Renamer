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
// 圆角只留三档:6 行内(胶囊、按钮、列表行) / 10 卡片 / 14 面板与对话框。
//
// 原来是 2/4/6/8/9/10/13/14 八档。其中 2 是"4px 高进度条的一半"、9 与 13
// 是"18×18 / 26×26 圆形按钮的一半"——这三个不是风格选择,是几何:半径
// 等于高度的一半才是圆。它们写成 Radius(4 / 2)、Radius(18 / 2) 这样的
// 表达式,由尺寸推导,不再占一档。4 与 8 则纯属"看着差不多"的手调,
// 已分别并入 6(行内) 与 6(按钮——按钮和它所在的胶囊本来就在同一行里,
// 圆角不一致时边线在角上打架)。
func TestRadiusFollowsOneLadder(t *testing.T) {
	allowed := map[float64]string{
		6:  "行内:胶囊、按钮、列表行",
		10: "卡片",
		14: "面板与对话框",
	}
	hits := tokenHits(t, "Radius")
	report(t, "Radius", hits)

	// 棘轮:当前实测 3 种,只许减不许增。
	if len(hits) > 3 {
		t.Errorf("Radius 有 %d 种取值(上限 3):只许收敛,不许再分叉。当前 %v",
			len(hits), sortedHitKeys(hits))
	}
	for v := range hits {
		if _, ok := allowed[v]; !ok {
			t.Errorf("Radius(%g) 不在阶梯上;允许: %v", v, sortedKeys(allowed))
		}
	}
}

// 字号只留三档:12 弱(说明、元数据、状态栏) / 14 正文(标题、按钮、文件名) /
// 18 对话框标题。
//
// 原来是 11/12/13/14/18 五档,而 11/12/13 三档一共承担 40 处文本——
// 差 1px 谁也分不出,那种"差一点"不构成层级,只是噪音:读者找不到
// "哪个更重要"的规律,只会觉得字忽大忽小。15 也曾在帮助页当过标题,
// 已并到 18。现在相邻两档差 2px 与 4px,层级一眼看得出。
func TestFontSizesFollowOneLadder(t *testing.T) {
	allowed := map[string]string{
		"12": "弱",
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
					t.Errorf("%s:%d FontSize(%s) 不在字号阶梯上;允许: 12/14/18",
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
	if len(used) > 3 {
		t.Errorf("字号有 %d 档(上限 3):只许收敛,不许再分叉。当前 %v", len(used), used)
	}
}

// 字重只留两档:默认(400) 与 Bold(700)。500/600 在多数系统字体上没有
// 对应字面,会被合到 400 或 700,写了等于没写——但它让源码看起来像
// 有五个层级,而屏幕上只有两个。
func TestFontWeightsFollowOneLadder(t *testing.T) {
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
			if !ok || sel.Sel.Name != "FontWeight" {
				return true
			}
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					continue
				}
				used[lit.Value] = true
				if lit.Value != "700" {
					t.Errorf("%s:%d FontWeight(%s) 不在字重阶梯上;只允许 700(Bold),其余用默认字重",
						file, fset.Position(call.Pos()).Line, lit.Value)
				}
			}
			return true
		})
	}
	// 也数一下 Bold():只剩两档时,Bold 是"加重"的唯一出口,
	// 它一处都没有就说明这个维度根本没被用过(而不是"收敛干净")。
	bold := 0
	for _, file := range uiFiles {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		bold += strings.Count(string(src), ".Bold()")
	}
	if bold < 5 {
		t.Errorf("全应用只有 %d 处加粗:字重这个维度几乎没被用起来", bold)
	}
	t.Logf("FontWeight 实测 %d 档: %v;.Bold() %d 处", len(used), used, bold)
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

// 相邻两档字号只差 2px 时,光靠字号分不出层级——必须同时有颜色或字重的
// 差异,否则"这一行是主体"和"这一行是说明"在屏幕上就是同一个东西。
//
// 做法:沿链式调用找出每个 FontSize(n) 同一条链上的 TextColor(...),
// 收集"每个字号配了哪些颜色"。相邻两档如果用的是同一组颜色,就说明
// 它们之间只差 2px,层级不成立。
func TestAdjacentFontSizesDifferByMoreThanSize(t *testing.T) {
	fset := token.NewFileSet()
	// size -> 该档用到的颜色表达式原文
	colors := map[string]map[string]bool{}
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
			size, ok := literalArg(call, "FontSize")
			if !ok {
				return true
			}
			if colors[size] == nil {
				colors[size] = map[string]bool{}
			}
			// 同一条链上(往外或往里)找 TextColor
			if col, ok := chainTextColor(call); ok {
				colors[size][col] = true
			} else {
				colors[size]["<默认>"] = true
			}
			return true
		})
	}
	ladder := []string{"12", "14", "18"}
	for i := 0; i+1 < len(ladder); i++ {
		a, b := colors[ladder[i]], colors[ladder[i+1]]
		if len(a) == 0 || len(b) == 0 {
			continue
		}
		// 两档的颜色集合完全相同时,只剩字号差
		same := true
		for k := range a {
			if !b[k] {
				same = false
				break
			}
		}
		if same && len(a) == len(b) {
			t.Errorf("字号 %s 与 %s 用的是同一组颜色 %v:两档只差 %s px,分不出层级",
				ladder[i], ladder[i+1], sortedStrKeys(a), ladder[i+1])
		}
	}
	for _, s := range ladder {
		t.Logf("FontSize(%s) 配的颜色: %v", s, sortedStrKeys(colors[s]))
	}
}

// literalArg 在 call 及其接收者链上找名为 name 的调用,返回它的整数字面量。
func literalArg(call *ast.CallExpr, name string) (string, bool) {
	for c := call; c != nil; {
		if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == name && len(c.Args) == 1 {
			if lit, ok := c.Args[0].(*ast.BasicLit); ok && lit.Kind == token.INT {
				return lit.Value, true
			}
		}
		inner, ok := c.Fun.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		next, ok := inner.X.(*ast.CallExpr)
		if !ok {
			return "", false
		}
		c = next
	}
	return "", false
}

// chainTextColor 在 call 及其接收者链上找 TextColor 的实参原文。
func chainTextColor(call *ast.CallExpr) (string, bool) {
	for c := call; c != nil; {
		if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "TextColor" && len(c.Args) == 1 {
			return exprText(c.Args[0]), true
		}
		inner, ok := c.Fun.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		next, ok := inner.X.(*ast.CallExpr)
		if !ok {
			return "", false
		}
		c = next
	}
	return "", false
}

// exprText 把颜色表达式压成一个可比较的字符串(只用于判等,不求值)。
func exprText(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.SelectorExpr:
		return exprText(v.X) + "." + v.Sel.Name
	case *ast.Ident:
		return v.Name
	case *ast.BasicLit:
		return v.Value
	case *ast.CallExpr:
		return exprText(v.Fun) + "(…)"
	case *ast.BinaryExpr:
		return exprText(v.X) + v.Op.String() + exprText(v.Y)
	}
	return "?"
}

func sortedStrKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
