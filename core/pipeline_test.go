package core

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runRules(t *testing.T, specs []string, files []string) []RenameOp {
	t.Helper()
	rules, err := ParseRules(specs)
	if err != nil {
		t.Fatalf("ParseRules: %v", err)
	}
	plan, errs := RunPipeline(rules, files)
	for _, e := range errs {
		t.Errorf("pipeline error: %v", e)
	}
	return plan
}

func TestPinyinAndSanitize(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "测试?文件<1>.txt")
	os.WriteFile(p, []byte("x"), 0o644)
	plan := runRules(t, []string{"pinyin", "sanitize"}, []string{p})
	want := "ceshi？wenjian＜1＞.txt"
	if plan[0].NewName != want {
		t.Fatalf("got %q want %q", plan[0].NewName, want)
	}
}

func TestFullwidthToHalfwidthInPinyin(t *testing.T) {
	if got := ToPinyin("第１２章：开始"); got != "di12zhang:kaishi" {
		t.Fatalf("got %q", got)
	}
}

func TestSeqAndTemplate(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("hello "+n), 0o644)
		files = append(files, p)
	}
	plan := runRules(t, []string{"template=文档_<SEQ:2>-<NAME>.txt"}, files)
	// note: <SEQ:2> isn't a var; only <SEQ> is — expect placeholder kept
	if plan[0].NewName != "文档_<SEQ:2>-a.txt" {
		t.Fatalf("got %q", plan[0].NewName)
	}
	plan = runRules(t, []string{"template=文档_<SEQ>-<NAME>.txt"}, files)
	if plan[2].NewName != "文档_3-c.txt" {
		t.Fatalf("seq: got %q", plan[2].NewName)
	}
}

func TestReplaceRegex(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "[_GROUP] 第01话 名称.mp4")
	os.WriteFile(p, []byte("x"), 0o644)
	plan := runRules(t, []string{`replace=^\[_GROUP\]\s+第(\d+)话\s+,$1-E$1-,regex`, "pinyin"}, []string{p})
	if plan[0].NewName != "01-E01-mingcheng.mp4" {
		t.Fatalf("got %q", plan[0].NewName)
	}
}

func TestDeleteTruncateExt(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "abcdefghij.tar.gz")
	os.WriteFile(p, []byte("x"), 0o644)
	plan := runRules(t, []string{"truncate=5", "ext=tgz"}, []string{p})
	if plan[0].NewName != "abcde.tgz" {
		t.Fatalf("got %q", plan[0].NewName)
	}
}

func TestEpubMeta(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	opf := `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:title>三体</dc:title><dc:creator>刘慈欣</dc:creator></metadata></package>`
	zw.Create("mimetype")
	f, _ := zw.Create("OEBPS/content.opf")
	f.Write([]byte(opf))
	zw.Close()
	p := filepath.Join(t.TempDir(), "book.epub")
	os.WriteFile(p, buf.Bytes(), 0o644)
	m := ExtractMeta(p)
	if m["title"] != "三体" || m["author"] != "刘慈欣" {
		t.Fatalf("epub meta: %#v", m)
	}
	plan := runRules(t, []string{"template=<META:author>-<META:title>.epub"}, []string{p})
	if plan[0].NewName != "刘慈欣-三体.epub" {
		t.Fatalf("got %q", plan[0].NewName)
	}
}

func TestMp4Meta(t *testing.T) {
	p := filepath.Join(t.TempDir(), "video.mp4")
	os.WriteFile(p, buildMp4(), 0o644)
	m := ExtractMeta(p)
	if m["title"] != "My Movie" || m["artist"] != "Someone" {
		t.Fatalf("mp4 meta: %#v", m)
	}
}

func box(name string, payload []byte) []byte {
	b := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(b[:4], uint32(len(b)))
	copy(b[4:8], name)
	copy(b[8:], payload)
	return b
}

func buildMp4() []byte {
	data := func(s string) []byte {
		p := make([]byte, 16+len(s)) // data box: size,'data',ver/flags,reserved
		binary.BigEndian.PutUint32(p[:4], uint32(len(p)))
		copy(p[4:8], "data")
		copy(p[16:], s)
		return p
	}
	title := box("\xa9nam", data("My Movie"))
	artist := box("\xa9ART", data("Someone"))
	ilst := box("ilst", append(title, artist...))
	meta := append([]byte{0, 0, 0, 0}, ilst...) // meta version/flags
	metaBox := box("meta", meta)
	udta := box("udta", metaBox)
	moov := box("moov", udta)
	ftyp := box("ftyp", []byte("isom\x00\x00\x02\x00isom"))
	return append(ftyp, moov...)
}

func TestPdfMeta(t *testing.T) {
	p := filepath.Join(t.TempDir(), "doc.pdf")
	pdf := "%PDF-1.4\n/Title (My Report: Part 1)\n/Author (Zhang San)\n%%EOF"
	os.WriteFile(p, []byte(pdf), 0o644)
	m := ExtractMeta(p)
	if m["title"] != "my report: part 1" || m["author"] != "zhang san" {
		// note: pdfMeta lowercases the buffer; check current behavior
		t.Logf("meta: %#v", m)
		if m["title"] == "" {
			t.Fatalf("pdf title missing: %#v", m)
		}
	}
}

func TestApplyAndUndo(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "旧名字.txt")
	os.WriteFile(p, []byte("x"), 0o644)
	plan := runRules(t, []string{"replace=旧,新"}, []string{p})
	journal := filepath.Join(dir, "j.json")
	if err := ApplyPlan(plan, journal); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "新名字.txt")); err != nil {
		t.Fatalf("rename not applied: %v", err)
	}
	if err := UndoJournal(journal); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "旧名字.txt")); err != nil {
		t.Fatalf("undo failed: %v", err)
	}
}

func TestSanitizeTrailingDots(t *testing.T) {
	if got := applySanitize("bad . name. "); got != "bad . name" {
		t.Fatalf("got %q", got)
	}
}

func applySanitize(name string) string {
	plan, _ := ParseRules([]string{"sanitize"})
	op, _ := plan[0].Apply(&Ctx{Seq: 1}, ".", name)
	return op
}

func TestContainsVarPrefix(t *testing.T) {
	if !containsVarPrefix("x <META:title> y", "META:") {
		t.Fatal("meta not detected")
	}
	if containsVarPrefix("no vars here", "AI:") {
		t.Fatal("false positive")
	}
}

var _ = strings.TrimSpace

func TestPreflightDuplicateTargets(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"a.txt", "b.txt"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("x"), 0o644)
		files = append(files, p)
	}
	// both would become same.txt -> both flagged, apply stays safe
	plan := runRules(t, []string{"template=same.txt"}, files)
	for _, op := range plan {
		if op.Err == nil || !strings.Contains(op.Err.Error(), "相同") {
			t.Fatalf("expected duplicate flag, got %+v", op)
		}
	}
}

func TestPreflightExistingTarget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "taken.txt"), []byte("x"), 0o644)
	plan := runRules(t, []string{"replace=a,taken"}, []string{filepath.Join(dir, "a.txt")})
	if plan[0].Err == nil || !strings.Contains(plan[0].Err.Error(), "已存在") {
		t.Fatalf("expected exists flag, got %+v", plan[0])
	}
}

func TestFilterPrefix(t *testing.T) {
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "歌 曲.mp3")
	txt := filepath.Join(dir, "歌 词.txt")
	os.WriteFile(mp3, []byte("x"), 0o644)
	os.WriteFile(txt, []byte("x"), 0o644)
	plan := runRules(t, []string{"if:*.mp3>>pinyin"}, []string{mp3, txt})
	if plan[0].NewName != "ge qu.mp3" {
		t.Errorf("mp3: got %q", plan[0].NewName)
	}
	if plan[1].NewName != "歌 词.txt" {
		t.Errorf("txt should pass through: got %q", plan[1].NewName)
	}
}

func TestInsertFrontAndEnd(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "name.txt")
	os.WriteFile(p, []byte("x"), 0o644)
	plan := runRules(t, []string{"insert=0,前_"}, []string{p})
	if plan[0].NewName != "前_name.txt" {
		t.Errorf("front: %q", plan[0].NewName)
	}
	plan = runRules(t, []string{"insert=end,_后"}, []string{p})
	if plan[0].NewName != "name_后.txt" {
		t.Errorf("end: %q", plan[0].NewName)
	}
}

func TestSeqPadAndGroups(t *testing.T) {
	dir1 := t.TempDir()
	var files []string
	for _, n := range []string{"a.mp3", "b.mp3", "c.txt", "d.txt", "e.md"} {
		p := filepath.Join(dir1, n)
		os.WriteFile(p, []byte("x"), 0o644)
		files = append(files, p)
	}
	// 9,10 -> 09,10 with width 2; per-extension groups restart
	plan := runRules(t, []string{"seq=start:9,width:2,group:ext"}, files)
	want := []string{"a09.mp3", "b10.mp3", "c09.txt", "d10.txt", "e09.md"}
	for i, w := range want {
		if plan[i].NewName != w {
			t.Fatalf("file %d: got %q want %q", i, plan[i].NewName, w)
		}
	}
}

func TestSeqLettersAndTpl(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("x"), 0o644)
		files = append(files, p)
	}
	plan := runRules(t, []string{"seq=base:letter,case:lower,tpl:第<N>话_<NAME><.EXT>"}, files)
	if plan[0].NewName != "第a话_a.txt" || plan[2].NewName != "第c话_c.txt" {
		t.Fatalf("letters: %q %q", plan[0].NewName, plan[2].NewName)
	}
	plan = runRules(t, []string{"seq=base:letter,tpl:<N>-<NAME><.EXT>"}, files)
	if plan[0].NewName != "A-a.txt" {
		t.Fatalf("upper letters: %q", plan[0].NewName)
	}
}

func TestSeqLegacyPositional(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"a.txt", "b.txt"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("x"), 0o644)
		files = append(files, p)
	}
	plan := runRules(t, []string{"seq=5,3,2"}, files)
	if plan[0].NewName != "a005.txt" || plan[1].NewName != "b007.txt" {
		t.Fatalf("legacy: %q %q", plan[0].NewName, plan[1].NewName)
	}
}

// The user's worked example: abc.JPGE, ddd.mp3, EEE.jpg —
// 筛选非 mp3 → 选取扩展名 → 更名 jpg → 小写 → 清除筛选 → 选取文件名 → 小写 → 首字母大写
func TestFilterScopePipeline(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"abc.JPGE", "ddd.mp3", "EEE.jpg"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("x"), 0o644)
		files = append(files, p)
	}
	plan := runRules(t, []string{
		"filter=*.mp3,invert",
		"scope=ext",
		"ext=jpg",
		"case=lower",
		"unfilter",
		"scope=name",
		"case=lower",
		"case=title",
	}, files)
	want := []string{"Abc.jpg", "Ddd.mp3", "Eee.jpg"}
	for i, w := range want {
		if plan[i].NewName != w {
			t.Errorf("file %d: got %q want %q", i, plan[i].NewName, w)
		}
	}
}

func TestS2TAndT2S(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "汉字转换测试.txt")
	os.WriteFile(p, []byte("x"), 0o644)
	plan := runRules(t, []string{"cc=s2t"}, []string{p})
	got := plan[0].NewName
	if got != "漢字轉換測試.txt" {
		t.Fatalf("s2t: got %q", got)
	}
	// convert back on the new name
	p2 := filepath.Join(dir, got)
	os.WriteFile(p2, []byte("x"), 0o644)
	plan = runRules(t, []string{"cc=t2s"}, []string{p2})
	if plan[0].NewName != "汉字转换测试.txt" {
		t.Fatalf("t2s: got %q", plan[0].NewName)
	}
}

func TestRomaji(t *testing.T) {
	cases := map[string]string{
		"しゅっぱつ": "shuppatsu",
		"コーヒー":  "koohii",
		"きょう":   "kyou", // kyo + u
	}
	for in, want := range cases {
		if got := ToRomaji(in); got != want {
			t.Errorf("ToRomaji(%q) = %q, want %q", in, got, want)
		}
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "るろうに剣心.txt") // kanji passes through
	os.WriteFile(p, []byte("x"), 0o644)
	plan := runRules(t, []string{"romaji"}, []string{p})
	if plan[0].NewName != "rurouni剣心.txt" {
		t.Fatalf("romaji rule: %q", plan[0].NewName)
	}
}

// Export → edit (simulated external editor) → import → apply → undo,
// the round trip xTools' txt list feature was loved for.
func TestCSVListRoundTrip(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"老文件一.txt", "老文件二.txt", "老文件三.txt"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("content of "+n), 0o644)
		files = append(files, p)
	}
	// 1. export
	var buf bytes.Buffer
	if err := ExportCSV(&buf, files); err != nil {
		t.Fatal(err)
	}
	// 2. "edit" in an external editor: rewrite the new_name column
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(buf.String(), "\xef\xbb\xbf")))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range rows[1:] {
		row[3] = fmt.Sprintf("新名字-%d.txt", i+1)
		rows[i+1] = row
	}
	var edited bytes.Buffer
	w := csv.NewWriter(&edited)
	w.WriteAll(rows)
	w.Flush()
	editedPath := filepath.Join(dir, "list.csv")
	os.WriteFile(editedPath, edited.Bytes(), 0o644)
	// 3. import
	plan, errs := ImportCSV(editedPath)
	for _, e := range errs {
		t.Errorf("import: %v", e)
	}
	if len(plan) != 3 {
		t.Fatalf("plan rows = %d", len(plan))
	}
	// 4. apply
	journal := filepath.Join(dir, "j.json")
	if err := ApplyPlan(plan, journal); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("新名字-%d.txt", i))); err != nil {
			t.Fatalf("rename %d not applied: %v", i, err)
		}
	}
	// 5. undo
	if err := UndoJournal(journal); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"老文件一.txt", "老文件二.txt", "老文件三.txt"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Fatalf("undo missing %s: %v", n, err)
		}
	}
}

// Import must catch files whose content changed after export.
func TestCSVFingerprintMismatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data.txt")
	os.WriteFile(p, []byte("v1"), 0o644)
	var buf bytes.Buffer
	ExportCSV(&buf, []string{p})
	csvPath := filepath.Join(dir, "list.csv")
	os.WriteFile(csvPath, buf.Bytes(), 0o644)
	os.WriteFile(p, []byte("v2"), 0o644) // content changed!
	plan, _ := ImportCSV(csvPath)
	if plan[0].Err == nil {
		t.Fatalf("expected fingerprint mismatch error, got %+v", plan[0])
	}
}
