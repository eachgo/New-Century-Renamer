package core

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 这些测试只验证"能不能从文件里真的抠出内容"——不联网、不调模型。

func TestSnippetPDFTextLayer(t *testing.T) {
	p := "../testfile/1528NP(I5-10210U)-12 V1.0y规格书.pdf"
	if _, err := os.Stat(p); err != nil {
		t.Skip("no testfile")
	}
	got := contentSnippet(p)
	if got == "" || strings.Contains(got, "无可用文本") {
		t.Fatalf("PDF 没抠出文字: %q", truncate(got, 200))
	}
	// 规格书里总该出现点像人话的东西
	t.Logf("PDF 片段(%d 字符): %.300s", len(got), got)
}

func TestSnippetEPUBText(t *testing.T) {
	var p string
	ents, _ := os.ReadDir("../testfile")
	for _, e := range ents {
		if strings.HasSuffix(strings.ToLower(e.Name()), ".epub") {
			p = filepath.Join("../testfile", e.Name())
			break
		}
	}
	if p == "" {
		t.Skip("no epub")
	}
	got := contentSnippet(p)
	if got == "" || strings.Contains(got, "无可用文本") {
		t.Fatalf("EPUB 没抠出文字: %q", truncate(got, 200))
	}
	t.Logf("EPUB 片段(%d 字符): %.300s", len(got), got)
}

func TestSnippetZipListing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for _, name := range []string{"IMG_0342.JPG", "IMG_0343.JPG", "readme.txt"} {
		w, _ := zw.Create(name)
		if name == "readme.txt" {
			w.Write([]byte("2024 春 京都行"))
		}
	}
	zw.Close()
	f.Close()

	got := contentSnippet(p)
	if !strings.Contains(got, "IMG_0342.JPG") {
		t.Fatalf("压缩包没列出条目: %q", got)
	}
	if !strings.Contains(got, "京都") {
		t.Fatalf("压缩包没读到说明文字: %q", got)
	}
	t.Logf("ZIP 片段: %s", truncate(got, 300))
}

func TestSnippetBinaryFallback(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "blob.bin")
	os.WriteFile(p, []byte{0, 1, 2, 3}, 0o644)
	got := contentSnippet(p)
	if !strings.Contains(got, "无可用文本") {
		t.Fatalf("未知二进制应说明没有文本, 实际: %q", got)
	}
	if !strings.Contains(got, "4 B") {
		t.Fatalf("应带上文件大小, 实际: %q", got)
	}
}

func TestSnippetTextFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "note.md")
	os.WriteFile(p, []byte("# 会议记录\n讨论了 Q4 排期"), 0o644)
	if got := contentSnippet(p); !strings.Contains(got, "Q4 排期") {
		t.Fatalf("纯文本没读进来: %q", got)
	}
}

func TestPDFStringUnescape(t *testing.T) {
	cases := map[string]string{
		`Hello\(world\)`: "Hello(world)",
		`a\\b`:           `a\b`,
		`\101\102`:       "AB",
	}
	for in, want := range cases {
		if got := pdfLiteral([]byte(in)); got != want {
			t.Errorf("pdfLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}
