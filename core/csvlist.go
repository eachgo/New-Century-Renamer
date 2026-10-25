package core

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The export/edit/import round trip, the modern form of xTools' "export
// file list, edit in your editor, import back to rename":
//
//   reamer export -o list.csv files...   (CSV columns: sha256,dir,name,new_name)
//   ... edit the new_name column anywhere ...
//   reamer import list.csv -apply        (files located by sha256)
//
// The fingerprint lets the import find the file even if its name column
// was renamed in the meantime, and detects edits to the file's content.

// FileSHA256 returns the hex sha256 of a file's content.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ExportCSV writes the list: sha256,dir,name,new_name. new_name starts as
// the current name, so an unedited row means "no change".
func ExportCSV(w io.Writer, files []string) error {
	bw := bomWriter(w)
	cw := csv.NewWriter(bw)
	if err := cw.Write([]string{"sha256", "dir", "name", "new_name"}); err != nil {
		return err
	}
	for _, f := range files {
		sum, err := FileSHA256(f)
		if err != nil {
			return err
		}
		dir, name := filepath.Split(f)
		dir = filepath.Clean(dir)
		if err := cw.Write([]string{sum, dir, name, name}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// UTF-8 BOM so Excel and editors pick the right encoding.
func bomWriter(w io.Writer) io.Writer {
	io.WriteString(w, "\xef\xbb\xbf")
	return w
}

// ImportCSV reads an edited list back and builds rename ops. Each row's
// file is located by trying dir/name first, then scanning the directory
// for a file whose sha256 matches. Rows whose content hash no longer
// matches, or whose new_name is empty or unchanged, become errors or no-ops.
func ImportCSV(path string) (plan []RenameOp, errs []error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, []error{err}
	}
	b = trimBOM(b)
	r := csv.NewReader(strings.NewReader(string(b)))
	r.FieldsPerRecord = 4
	rows, err := r.ReadAll()
	if err != nil {
		return nil, []error{fmt.Errorf("CSV 格式: %w", err)}
	}
	if len(rows) == 0 || strings.EqualFold(rows[0][0], "sha256") {
		rows = rows[1:] // header
	}
	for i, row := range rows {
		sum, dir, name, newName := row[0], row[1], row[2], row[3]
		op := RenameOp{Dir: dir, NewName: newName}
		found, err := locateByFingerprint(sum, dir, name)
		if err != nil {
			op.Err = fmt.Errorf("第 %d 行: %w", i+2, err)
			op.ErrText = op.Err.Error()
			errs = append(errs, op.Err)
			plan = append(plan, op)
			continue
		}
		op.OldName = found
		if newName == "" {
			op.Err = fmt.Errorf("第 %d 行: new_name 为空(留空不改名请与 name 相同)", i+2)
			op.ErrText = op.Err.Error()
			errs = append(errs, op.Err)
		}
		plan = append(plan, op)
	}
	preflight(plan)
	return plan, errs
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xef && b[1] == 0xbb && b[2] == 0xbf {
		return b[3:]
	}
	return b
}

// locateByFingerprint finds the file with the given sha256 in dir: the
// recorded name if it still matches, else any file in the directory.
func locateByFingerprint(sum, dir, name string) (string, error) {
	p := filepath.Join(dir, name)
	if s, err := FileSHA256(p); err == nil && s == sum {
		return name, nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("目录不可读 %s: %w", dir, err)
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if s, err := FileSHA256(filepath.Join(dir, e.Name())); err == nil && s == sum {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("找不到指纹 %s… 的文件(内容已改动?)", sum[:12])
}
