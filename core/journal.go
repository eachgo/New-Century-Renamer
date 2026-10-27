package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// journal keeps every applied rename so `ncreamer undo` can roll the whole
// batch back — the modern form of xTools' "check before apply" caution.

type journalFile struct {
	Time    string     `json:"time"`
	Renames []RenameOp `json:"renames"`
}

func WriteJournal(path string, ops []RenameOp) error {
	b, _ := json.MarshalIndent(journalFile{Time: time.Now().Format(time.RFC3339), Renames: ops}, "", "  ")
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func UndoJournal(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var j journalFile
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	n := 0
	// reverse order: undo later renames first
	for i := len(j.Renames) - 1; i >= 0; i-- {
		op := j.Renames[i]
		if !op.Applied {
			continue
		}
		old := filepath.Join(op.Dir, op.NewName)
		new := filepath.Join(op.Dir, op.OldName)
		if err := os.Rename(old, new); err != nil {
			return fmt.Errorf("回滚 %s 失败: %w", old, err)
		}
		fmt.Printf("* %s => %s\n", old, op.OldName)
		n++
	}
	fmt.Printf("已回滚 %d 个更名\n", n)
	return nil
}
