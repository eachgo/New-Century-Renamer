package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// prose 的行距必须真的松一些:12px 的默认行高来自字体自身度量,
// 连续几行中文贴得太紧。
//
// 判据不能写成"prose 比裸 Text 高":Linux 上 12px 的默认行高本来就是
// 1.5 倍,两者一样高,那条断言在 CI 上恒红。这里改成两条与平台无关的:
// 一是 prose 的单行高度至少是字号的 1.45 倍(行距够松),二是一个字都不许
// 比裸 Text 更紧(换了字体也不该倒退)。
func TestProseUsesRoomierLineHeight(t *testing.T) {
	const short = "预览不动磁盘"
	const para = "预览不动磁盘：右栏“原名 → 新名”随时重算，只有“执行更名”会真的改。" +
		"改完不满意，底栏“撤销上次”整批退回去，回来接着调。"

	height := func(s string, useProse bool, w float32) float32 {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Width(w).Children(func() {
				if useProse {
					prose(c, s)
					return
				}
				ui.Text(c, s).FontSize(12).TextColor(c.Theme().TextMuted)
			})
		}, 400, 400)
		tt.Frame()
		r, ok := tt.Find(s)
		if !ok {
			t.Fatalf("找不到那段文字(useProse=%v)", useProse)
		}
		return r.H
	}

	plainLine := height(short, false, 320)
	proseLine := height(short, true, 320)
	if proseLine < plainLine {
		t.Fatalf("prose 的行高不该比默认更紧:默认 %g,prose %g", plainLine, proseLine)
	}
	if proseLine < 12*1.45 {
		t.Fatalf("prose 的行距只有字号的 %.2f 倍(单行高 %g),读起来还是挤",
			proseLine/12, proseLine)
	}

	// 折行之后的高度必须是单行高度的整数倍:行距真的作用在每一行上,
	// 而不是只把最后一行推开。比的是同一平台量出来的单行高,不写死行数。
	paraH := height(para, true, 320)
	lines := float64(paraH) / float64(proseLine)
	if math.Abs(lines-math.Round(lines)) > 0.02 {
		t.Fatalf("折行后高 %g 不是单行高 %g 的整数倍(%.3f 行)", paraH, proseLine, lines)
	}
	if lines < 2 {
		t.Fatalf("这段在 320 宽下应当折成多行,实际只有 %.2f 行", lines)
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

// 一段说明文字不许随弹窗变宽而无限拉长:一行七八十个字,眼睛换行时会跳行。
//
// 弹窗宽度是写死的(520/560/620),所以今天看不出问题;这条约束守的是
// "以后把弹窗加宽到 880"那一刻——那时正文会跟着变宽,而这里会红。
func TestProseWidthStaysInTheComfortableRange(t *testing.T) {
	long := "每行四列：文件指纹（sha256）、所在文件夹、现在的名字、想改成的新名字。" +
		"导入时按指纹找文件——文件就算已经被别的工具改过名，也能对回自己那一行；" +
		"内容被改过，那一行会标成「有问题」，不会硬改。"

	// 12px 中文的舒适区大约 30–40 字一行,换算成宽度是 360–480 DIP。
	if proseMaxWidth < 320 || proseMaxWidth > 520 {
		t.Fatalf("proseMaxWidth=%g 不在 320–520 之间:太窄会折成碎句,太宽会跳行", proseMaxWidth)
	}

	for _, w := range []float32{560, 880, 1200} {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Width(w).Children(func() { prose(c, long) })
		}, int(w)+40, 600)
		tt.Frame()
		r, ok := tt.Find(long)
		if !ok {
			t.Fatalf("w=%g 找不到那段文字", w)
		}
		if r.W > proseMaxWidth+0.5 {
			t.Fatalf("w=%g:正文宽 %g 超过了上限 %g——弹窗一变宽,正文就跟着变宽", w, r.W, proseMaxWidth)
		}
		lines := float64(r.H) / 18
		if perLine := float64(len([]rune(long))) / lines; perLine > 45 {
			t.Fatalf("w=%g:一行 %.0f 个字,超出舒适区(≤45)", w, perLine)
		}
	}
}

// 弹窗里的说明文字必须只有一档字号:正文都是 12px,行高 18。
//
// 曾经是这样,但没有东西守着——下一个人给某段说明加个 FontSize(14)
// 强调一下,整页的节奏就乱了,而且没人会发现(看起来"只是那一段大一点")。
func TestDialogProseHasOneTypography(t *testing.T) {
	const lineH = 18.0 // 12px × 1.5

	check := func(tag string, a *app, w, h int) {
		tt := ui.NewTester(a.view, w, h)
		tt.Frame()
		tt.Frame()
		for _, ai := range tt.AccessAll() {
			// 只看真正的文字节点:流程图是带 Label 的 Box(无障碍里是
			// RoleGroup),它的高度是画布高度,不是行高。
			if ai.Role != ui.RoleText {
				continue
			}
			if ai.Bounds.H < lineH*2-0.5 {
				continue // 单行标签不在"成段正文"之列
			}
			if len([]rune(ai.Name)) < 20 {
				continue // 按钮/标题/短标签
			}
			lines := float64(ai.Bounds.H) / lineH
			if lines-float64(int(lines)) > 0.02 {
				t.Errorf("%s: %q 的行高不是 18 的整数倍(高 %.1f)——弹窗正文只许有一档字号",
					tag, ai.Name, ai.Bounds.H)
			}
		}
	}

	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	a.openHelp()
	a.helpShowFlows = true
	check("帮助", a, 1280, 900)

	b := &app{stepSel: -1, settings: defaultSettings()}
	b.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	b.openSettings()
	check("设置", b, 1280, 900)

	c := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	c.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	c.openFlows()
	check("流程", c, 1280, 900)
}
