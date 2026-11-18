package core

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 这些测试只验证"能不能从文件里真的抠出内容"——不联网、不调模型。
//
// 原来这两条读 ../testfile/ 里的真实 PDF 与 EPUB，而那个目录被 .gitignore
// 排除：CI 上不存在，于是两条测试永远 t.Skip，从来没守过任何东西。改成
// 当场造夹具——PDF 只要一个带 Tj 的内容流，EPUB 只要一个 .xhtml 正文，
// 解析器要认的就是这两样，不需要文件"像真的"。

// pdfTextFixture 造一个带文字层的最小 PDF：一个内容流，里面一条 Tj。
func pdfTextFixture(text string) []byte {
	body := "BT /F1 12 Tf 72 712 Td (" + text + ") Tj ET"
	return []byte("%PDF-1.7\n" +
		"1 0 obj\n<< /Length " + itoa(len(body)) + " >>\nstream\n" + body + "\nendstream\nendobj\n" +
		"trailer\n<< /Root 1 0 R >>\n%%EOF\n")
}

// epubTextFixture 造一个带正文的最小 EPUB。epubText 只收 .xhtml/.html，
// 并且跳过名字里带 nav/toc/cover 的条目，所以正文文件叫 chapter1.xhtml。
func epubTextFixture(title, body string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			return
		}
		w.Write([]byte(content))
	}
	add("mimetype", "application/epub+zip")
	add("META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)
	add("content.opf", `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>`+title+`</dc:title>
  </metadata>
</package>`)
	add("chapter1.xhtml", `<html><body><h1>`+title+`</h1><p>`+body+`</p></body></html>`)
	zw.Close()
	return buf.Bytes()
}

func TestSnippetPDFTextLayer(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "spec.pdf")
	if err := os.WriteFile(p, pdfTextFixture("Technical Specification rev 1.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := contentSnippet(p)
	if got == "" || strings.Contains(got, "无可用文本") {
		t.Fatalf("PDF 没抠出文字: %q", truncate(got, 200))
	}
	if !strings.Contains(got, "Technical Specification") {
		t.Fatalf("PDF 抠出的不是内容流里的文字: %q", truncate(got, 200))
	}
}

func TestSnippetEPUBText(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "book.epub")
	if err := os.WriteFile(p, epubTextFixture("読書ノート", "第一章 京都の春"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := contentSnippet(p)
	if got == "" || strings.Contains(got, "无可用文本") {
		t.Fatalf("EPUB 没抠出文字: %q", truncate(got, 200))
	}
	if !strings.Contains(got, "京都の春") {
		t.Fatalf("EPUB 抠出的不是正文: %q", truncate(got, 200))
	}
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
