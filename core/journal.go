package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// journal keeps every applied rename so `ncreamer undo` can roll the whole
// batch back — the modern form of xTools' "check before apply" caution.

type journalFile struct {
	Time    string     `json:"time"`
	Renames []RenameOp `json:"renames"`
}

func WriteJournal(path string, ops []RenameOp) error {
	return writeJournalFile(path, journalFile{Time: time.Now().Format(time.RFC3339), Renames: ops})
}

func writeJournalFile(path string, j journalFile) error {
	b, _ := json.MarshalIndent(j, "", "  ")
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// JournalCount reports how many renames a journal can still undo.
// 只判断文件存不存在是不够的:上一次执行如果全失败,日志里是一条空列表,
// 界面却会亮起"回滚上次",点下去一个文件也不动,还报告"已回滚"。
func JournalCount(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var j journalFile
	if err := json.Unmarshal(b, &j); err != nil {
		return 0
	}
	n := 0
	for _, op := range j.Renames {
		if op.Applied {
			n++
		}
	}
	return n
}

// UndoError reports a rollback that restored some files and failed on
// others, so the caller can say exactly how far it got instead of a bare
// "回滚失败".
type UndoError struct {
	Done   int
	Failed []string
}

func (e *UndoError) Error() string {
	return fmt.Sprintf("已回滚 %d 个,%d 个失败: %s", e.Done, len(e.Failed), strings.Join(e.Failed, "; "))
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
	var failed []string
	// reverse order: undo later renames first
	for i := len(j.Renames) - 1; i >= 0; i-- {
		op := j.Renames[i]
		if !op.Applied {
			continue
		}
		old := filepath.Join(op.Dir, op.NewName)
		new := filepath.Join(op.Dir, op.OldName)
		// 尽力而为:一个文件失败不能把整批回滚停在半完成状态,
		// 那样用户既不知道已经退回哪些,也没有办法继续。
		if err := os.Rename(old, new); err != nil {
			failed = append(failed, fmt.Sprintf("%s (%v)", op.NewName, err))
			continue
		}
		fmt.Printf("* %s => %s\n", old, op.OldName)
		n++
		j.Renames[i].Applied = false
	}
	if n > 0 {
		// 标记为已回滚:再点一次"回滚上次"不该把已经退回去的文件重放一遍。
		if err := writeJournalFile(path, j); err != nil {
			return fmt.Errorf("已回滚 %d 个,但日志未能更新: %w", n, err)
		}
	}
	if len(failed) > 0 {
		return &UndoError{Done: n, Failed: failed}
	}
	fmt.Printf("已回滚 %d 个更名\n", n)
	return nil
}
