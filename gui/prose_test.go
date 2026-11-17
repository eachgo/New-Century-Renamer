package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// prose 的行高必须真的比默认大:12px 的默认行高来自字体自身度量,
// 连续几行中文贴得太紧。
func TestProseUsesRoomierLineHeight(t *testing.T) {
	const para = "预览不动磁盘：右栏“原名 → 新名”随时重算，只有“执行更名”会真的改。" +
		"改完不满意，底栏“撤销上次”整批退回去，回来接着调。"

	height := func(useProse bool) float32 {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Width(320).Children(func() {
				if useProse {
					prose(c, para)
					return
				}
				ui.Text(c, para).FontSize(12).TextColor(c.Theme().TextMuted)
			})
		}, 400, 400)
		tt.Frame()
		r, ok := tt.Find(para)
		if !ok {
			t.Fatalf("找不到那段文字(useProse=%v)", useProse)
		}
		return r.H
	}

	plain := height(false)
	p := height(true)
	if p <= plain {
		t.Fatalf("prose 的行高应当更高:默认 %g,prose %g", plain, p)
	}
	// 这段在 320 宽下折成三行:默认 16px 一行(48),1.5 倍是 18px 一行(54)。
	// 比的是每行高度,不写死行数——换了字体或文案,断言不该跟着崩。
	if got, want := p/plain, proseLineHeight/1.3333; got < want*0.95 {
		t.Fatalf("行高倍数没生效: prose/默认 = %.3f,期望约 %.3f", got, want)
	}
}

// 成段正文不许再用裸 ui.Text:那样会回到 1.33 倍行高,而且下一个人加说明
// 时还是会顺手写 ui.Text。
//
// 判据是"够长的中文字面量"。链式调用了 SingleLine 的跳过——单行文字不折行,
// 行高对它没有意义(状态条、就地警示都属这类)。
func TestLongChineseProseGoesThroughProseHelper(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var bad []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读 %s: %v", f, err)
		}
		af, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatalf("解析 %s: %v", f, err)
		}
		// 先把"链上带 SingleLine"的文字节点位置收起来:它们不折行。
		single := map[token.Pos]bool{}
		ast.Inspect(af, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !chainEndsWith(call, "SingleLine") {
				return true
			}
			if base := chainBase(call); base != nil {
				single[base.Pos()] = true
			}
			return true
		})

		ast.Inspect(af, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Text" {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "ui" {
				return true
			}
			if single[call.Pos()] {
				return true
			}
			// ui.Text(c, s):第一个参数是 context,要看的文字在第二个。
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if len([]rune(s)) >= 28 {
				bad = append(bad, fset.Position(call.Pos()).String()+"  "+s)
			}
			return true
		})
	}
	for _, b := range bad {
		t.Errorf("成段正文应走 prose(): %s", b)
	}
}

// chainEndsWith reports whether the outermost call in the chain is name,
// i.e. `ui.Text(...).FontSize(12).SingleLine()`.
func chainEndsWith(call *ast.CallExpr, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name
}

// chainBase walks down `a.B().C()` to the innermost call `a.B()`.
func chainBase(e ast.Expr) *ast.CallExpr {
	var last *ast.CallExpr
	for {
		call, ok := e.(*ast.CallExpr)
		if !ok {
			return last
		}
		last = call
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return last
		}
		e = sel.X
	}
}
