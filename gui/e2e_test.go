package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// findTestfile 从当前目录逐级向上找素材目录。仓库根下的 testfile/ 不入库,
// 没有就直接跳过,这样别人 clone 下来跑测试不会红。
func findTestfile() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for i := 0; i < 4; i++ {
		p := filepath.Join(dir, "testfile")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// resetCopy rebuilds the working copy from the pristine testfile directory.
func resetCopy(t *testing.T) (dir string, paths []string) {
	t.Helper()
	src, ok := findTestfile()
	if !ok {
		t.Skip("no testfile dir")
	}
	dir = os.Getenv("NCREAMER_E2E_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "ncreamer-e2e-copy")
	}
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(src)
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			continue
		}
		p := filepath.Join(dir, e.Name())
		os.WriteFile(p, b, 0o644)
		paths = append(paths, p)
	}
	return dir, paths
}

// TestE2EApplyUndo drives the real app over real files: add → preview →
// apply → verify on disk → undo → verify restored.
func TestE2EApplyUndo(t *testing.T) {
	dir, paths := resetCopy(t)
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, ".journal.json")
	a.addPaths(paths)
	if len(a.files) != len(paths) {
		t.Fatalf("added %d of %d files", len(a.files), len(paths))
	}
	tt := ui.NewTester(a.view, 1280, 1300)
	tt.Frame()
	for _, label := range []string{"全部转小写", "追加序号"} {
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %s: %v", label, err)
		}
		tt.Frame()
	}
	if countChanges(a.ops) == 0 {
		t.Fatalf("no preview changes")
	}
	before := snapshot(t, dir)
	a.apply()
	applied := 0
	for _, op := range a.ops {
		if op.Applied {
			applied++
		}
	}
	after := snapshot(t, dir)
	diff := 0
	for name := range before {
		if !after[name] {
			diff++
		}
	}
	t.Logf("applied=%d diskchanged=%d files=%d", applied, diff, len(paths))
	if applied == 0 || diff != applied {
		t.Fatalf("apply mismatch")
	}
	a.undo()
	restored := snapshot(t, dir)
	missing := 0
	for name := range before {
		if !restored[name] {
			missing++
		}
	}
	if missing != 0 {
		t.Fatalf("%d names not restored", missing)
	}
	t.Logf("undo ok, %d names restored", len(before))
}

// TestE2EMetadataFlow uses real FLAC/EPUB/PDF/MP3 files through <META:...>
func TestE2EMetadataFlow(t *testing.T) {
	dir, paths := resetCopy(t)
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, ".journal.json")
	a.addPaths(paths)
	tt := ui.NewTester(a.view, 1280, 1300)
	tt.Frame()

	// 作者 - 标题 preset step
	if err := tt.Click("作者 - 标题"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	withMeta := 0
	for i, op := range a.ops {
		if i < 3 || op.NewName != op.OldName {
			t.Logf("meta: %q -> %q", op.OldName, op.NewName)
		}
		if op.NewName != op.OldName && !strings.Contains(op.NewName, "META") {
			withMeta++
		}
	}
	t.Logf("files with metadata resolution: %d of %d", withMeta, len(a.ops))
	if withMeta == 0 {
		t.Error("no file resolved metadata (flac/mp3/epub/pdf expected)")
	}
	// no literal placeholders may leak into names
	for _, op := range a.ops {
		if strings.Contains(op.NewName, "<META") || strings.Contains(op.NewName, "<") {
			t.Errorf("literal variable leaked: %q", op.NewName)
		}
	}
}

// TestE2ENarrowWindow checks the touch-sized layout still works: switch
// panel sizes, pick-mode, editing a step.
func TestE2ENarrowWindow(t *testing.T) {
	dir, paths := resetCopy(t)
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, ".journal.json")
	a.addPaths(paths)
	tt := ui.NewTester(a.view, 900, 640) // 窄窗口
	tt.Frame()
	tt.SetSize(760, 560)
	tt.Frame()
	if !tt.HasText("文件") {
		t.Error("files pane missing in narrow window")
	}
	// add a step through the visible catalog area (scroll is needed; use state)
	a.steps = append(a.steps, func() guiRule { r := step("pinyin"); a.stepUID++; r.opUID = a.stepUID; return r }())
	tt.Frame()
	if !tt.HasText("多选") {
		t.Error("多选 entry missing in narrow window")
	}
	if err := tt.Click("多选"); err != nil {
		t.Errorf("click 多选: %v", err)
	}
	tt.Frame()
	if !a.pickMode {
		t.Error("pick mode did not engage")
	}
}

// TestE2EBulkPerformance: many files, live preview must stay responsive.
func TestE2EBulkPerformance(t *testing.T) {
	dir, paths := resetCopy(t)
	// multiply to ~600 files
	var many []string
	for k := 0; k < 20; k++ {
		for _, p := range paths {
			base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			ext := filepath.Ext(p)
			dst := filepath.Join(dir, base+"_"+string(rune('a'+k))+ext)
			if b, err := os.ReadFile(p); err == nil {
				os.WriteFile(dst, b, 0o644)
				many = append(many, dst)
			}
		}
	}
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, ".journal.json")
	start := time.Now()
	a.addPaths(many)
	addDur := time.Since(start)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	start = time.Now()
	a.steps = append(a.steps, func() guiRule { r := step("pinyin"); a.stepUID++; r.opUID = a.stepUID; return r }())
	tt.Frame()
	previewDur := time.Since(start)
	t.Logf("%d files: add=%v preview=%v changes=%d", len(a.files), addDur, previewDur, countChanges(a.ops))
	if previewDur > 3*time.Second {
		t.Errorf("preview too slow: %v", previewDur)
	}
}

func snapshot(t *testing.T, dir string) map[string]bool {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]bool{}
	for _, e := range ents {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			m[e.Name()] = true
		}
	}
	return m
}

// TestE2ECsvBackupAndRestore: 用 CSV 备份原始文件名 → 按 CSV 更名 →
// 再用备份 CSV 原生还原。CSV 是独立于 journal 的第二条退路。
func TestE2ECsvBackupAndRestore(t *testing.T) {
	dir, paths := resetCopy(t)
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, ".journal.json")
	a.addPaths(paths)

	// 1) 备份:导出即原始文件名快照(new_name 与 name 相同)
	backup := filepath.Join(dir, "backup.csv")
	if err := a.exportTo(backup); err != nil {
		t.Fatal(err)
	}
	origRows := readCSV(t, backup)
	if len(origRows) != len(paths) {
		t.Fatalf("backup rows %d != files %d", len(origRows), len(paths))
	}

	// 2) 模拟用户在编辑器里写新名字
	edited := filepath.Join(dir, "edited.csv")
	writeCSV(t, backup, edited, func(row []string) []string {
		stem := strings.TrimSuffix(row[2], filepath.Ext(row[2]))
		row[3] = "改后_" + stem + filepath.Ext(row[2])
		return row
	})

	// 3) 导入并按 CSV 更名,校验磁盘
	plan, errs := a.importFrom(edited)
	for _, e := range errs {
		t.Errorf("import: %v", e)
	}
	if len(plan) != len(paths) {
		t.Fatalf("imported %d rows", len(plan))
	}
	a.apply()
	ok := 0
	for _, op := range a.ops {
		if op.Applied {
			ok++
		}
		if _, err := os.Stat(filepath.Join(op.Dir, op.NewName)); err != nil {
			t.Errorf("missing after apply: %s", op.NewName)
		}
	}
	if ok != len(paths) {
		t.Fatalf("applied %d of %d", ok, len(paths))
	}
	t.Logf("CSV rename applied to %d files", ok)

	// 4) 用备份 CSV 还原(文件名已变,靠 sha256 指纹定位)
	plan2, errs2 := a.importFrom(backup)
	for _, e := range errs2 {
		t.Errorf("restore import: %v", e)
	}
	a.apply()
	restored := snapshot(t, dir)
	missing := 0
	for _, row := range origRows {
		if !restored[row[2]] {
			missing++
			if missing < 4 {
				t.Logf("not restored: %s", row[2])
			}
		}
	}
	if missing != 0 {
		t.Fatalf("%d files not restored from backup CSV", missing)
	}
	t.Logf("restored all %d names from backup CSV (sha256 指纹定位改后文件)", len(origRows))
	_ = plan2
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) > 0 {
		rows = rows[1:] // header
	}
	return rows
}

// writeCSV rewrites src through edit using a real CSV codec, as an editor does.
func writeCSV(t *testing.T, src, dst string, edit func([]string) []string) {
	t.Helper()
	f, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(rows); i++ {
		rows[i] = edit(rows[i])
	}
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	out.WriteString("\ufeff")
	w := csv.NewWriter(out)
	if err := w.WriteAll(rows); err != nil {
		t.Fatal(err)
	}
	w.Flush()
}

// 失败原因必须在界面上可见(不能只写"失败")
func TestFailureReasonVisible(t *testing.T) {
	dir, paths := resetCopy(t)
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, ".journal.json")
	a.addPaths(paths)
	// 造一个必然冲突的步骤:所有文件改成同一个名字
	r := step("tpl")
	r.tpl = "同名.txt"
	a.stepUID++
	r.opUID = a.stepUID
	a.steps = []guiRule{r}
	tt := ui.NewTester(a.view, 1280, 1000)
	tt.Frame()
	if !tt.HasText("失败") {
		t.Fatal("未出现失败状态")
	}
	if !tt.HasText("⚠ ") {
		texts := tt.Texts()
		for _, s := range texts {
			if strings.Contains(s, "相同") || strings.Contains(s, "覆盖") {
				return // 原因以别的方式可见,也算通过
			}
		}
		t.Fatalf("失败原因不可见, texts=%q", texts)
	}
}
