package core

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// allowedHalfWidth lists substrings that may keep half-width punctuation even
// inside a Chinese sentence, because the characters are part of a syntax
// example rather than prose: the variable grammar (<META:…>, <NFO:title>) and
// the Windows reserved-character sample.
var allowedHalfWidth = []string{
	"<META:",
	"<NFO:",
	"如 : / ? *",
}

// syntaxOnlyLiterals lists string literals that are pure regex syntax rather
// than prose. They are handed to fmt.Sprintf as arguments, so the half-width
// characters never appear inside a Chinese sentence — see regexErrHints.
// Keeping them as separate literals is the point: a hint that reads
// "缺少右方括号 ]，…" mixes prose with something the user must type verbatim,
// and neither the punctuation gate nor a reader can tell which is which.
var syntaxOnlyLiterals = []string{
	"]",
	")",
	"{2,5}",
	". ( ) [ ] { }",
	"[:digit:]",
	"(?=",
	"(?<=",
}

// TestChinesePunctuationIsFullWidth enforces the rule the project's own design
// doc states (Design System.md §3.2: use full-width punctuation in Chinese
// text). Without a gate this drifts back within a few commits — it had already
// reached 74 places across 55 lines before it was cleaned up.
func TestChinesePunctuationIsFullWidth(t *testing.T) {
	dirs := []string{".", filepath.Join("..", "cmd", "ncreamer")}
	bad := scanHalfWidthPunct(t, dirs, allowedHalfWidth)
	for _, b := range bad {
		t.Errorf("%s", b)
	}
}

// scanHalfWidthPunct walks every non-test .go file in dirs and reports string
// literals that contain Chinese text plus a half-width , ; : ( or ).
func scanHalfWidthPunct(t *testing.T, dirs, allowed []string) []string {
	t.Helper()
	fset := token.NewFileSet()
	var out []string
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatalf("扫描 %s: %v", dir, err)
		}
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
				if !ok || bl.Kind != token.STRING || !hasHan(bl.Value) {
					return true
				}
				for _, s := range allowed {
					if strings.Contains(bl.Value, s) {
						return true
					}
				}
				// 纯语法字面量不算文案:它们会作为参数填进中文句子,
				// 见 syntaxOnlyLiterals。
				for _, s := range syntaxOnlyLiterals {
					if bl.Value == strconv.Quote(s) || bl.Value == "`"+s+"`" {
						return true
					}
				}
				for _, ch := range []string{",", ";", ":", "(", ")"} {
					if strings.Contains(bl.Value, ch) {
						out = append(out, f+":"+fmt.Sprint(fset.Position(bl.Pos()).Line)+
							" 中文文案里有半角 "+ch+"（应改用全角）: "+bl.Value)
						break
					}
				}
				return true
			})
		}
	}
	return out
}

// hasHan reports whether s contains at least one CJK ideograph — the signal
// that this literal is human-facing Chinese prose rather than a format string
// of symbols or a file path.
func hasHan(s string) bool {
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			return true
		}
	}
	return false
}
