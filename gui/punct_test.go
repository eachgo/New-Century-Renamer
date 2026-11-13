package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allowedHalfWidthGUI lists substrings that keep half-width punctuation even
// inside Chinese text: the Windows reserved-character sample and the template /
// NFO variable grammar. Everything else must use full-width punctuation, per
// the project's own Design System.md §3.2.
var allowedHalfWidthGUI = []string{
	"如 : / ? *",
	"<META:",
	"<NFO:",
	"<AI:",
}

// TestGUIPunctuationIsFullWidth is the GUI half of the punctuation gate; the
// core half lives in core/punct_test.go. It had drifted to 74 half-width marks
// across 55 lines before the cleanup, so the rule needs a test, not a habit.
func TestGUIPunctuationIsFullWidth(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("扫描当前目录: %v", err)
	}
	bad := 0
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
		ast.Inspect(af, func(node ast.Node) bool {
			bl, ok := node.(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING || !hasHanGUI(bl.Value) {
				return true
			}
			for _, s := range allowedHalfWidthGUI {
				if strings.Contains(bl.Value, s) {
					return true
				}
			}
			for _, ch := range []string{",", ";", ":", "(", ")"} {
				if strings.Contains(bl.Value, ch) {
					t.Errorf("%s:%d 中文文案里有半角 %s（应改用全角）: %s",
						f, fset.Position(bl.Pos()).Line, ch, bl.Value)
					bad++
					break
				}
			}
			return true
		})
	}
	if bad > 0 {
		t.Logf("共 %d 处；全角标点见 Design System.md §3.2", bad)
	}
}

// hasHanGUI reports whether s contains a CJK ideograph.
func hasHanGUI(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}
