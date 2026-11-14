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
	// 日志默认落在用户配置目录下,那个子目录可能还不存在;
	// 写文件前先把它建出来,否则第一次执行就会报"no such file or directory"。
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// JournalSummary 是"回滚上次"该说给人听的话:什么时候改的、几个文件、都是谁。
// 日志里本来就写着 Time 和完整的 Renames 列表,界面却只问"文件存不存在",
// 于是按钮永远只写着"回滚上次"——用户不知道自己即将撤销哪一次、几个文件。
type JournalSummary struct {
	Time  time.Time
	Names []string // 只含仍可回滚(Applied)的那些,按执行顺序
}

// ReadJournalSummary 读出可回滚的一批更名。第二个返回值为 false 表示
// 没有可回滚的内容(文件不存在、坏了、或者已经全部回滚过)。
func ReadJournalSummary(path string) (JournalSummary, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return JournalSummary{}, false
	}
	var j journalFile
	if err := json.Unmarshal(b, &j); err != nil {
		return JournalSummary{}, false
	}
	var s JournalSummary
	if t, err := time.Parse(time.RFC3339, j.Time); err == nil {
		s.Time = t
	}
	for _, op := range j.Renames {
		if op.Applied {
			s.Names = append(s.Names, op.NewName)
		}
	}
	return s, len(s.Names) > 0
}

// JournalCount reports how many renames a journal can still undo.
// 只判断文件存不存在是不够的:上一次执行如果全失败,日志里是一条空列表,
// 界面却会亮起"回滚上次",点下去一个文件也不动,还报告"已回滚"。
func JournalCount(path string) int {
	s, ok := ReadJournalSummary(path)
	if !ok {
		return 0
	}
	return len(s.Names)
}

// UndoReport 说清一次回滚到底退回了哪些文件。
//
// 只给一个 Done 计数是不够的:界面要按行更新——退回去的那几行显示旧名,
// 没退成的那几行必须继续显示新名(它们确实还叫新名字)。以前界面只能
// 遍历"所有 Applied 的行"一起改成旧名,部分失败时就把没退成的文件也
// 画成了旧名,和磁盘上真实的名字对不上。
type UndoReport struct {
	Restored []string // 已成功退回旧名的那些"新名字"
	Failed   []string // 没退成的,格式 "新名字 (原因)"
}

// UndoError reports a rollback that restored some files and failed on
// others, so the caller can say exactly how far it got instead of a bare
// "回滚失败".
type UndoError struct {
	Done   int
	Failed []string
}

func (e *UndoError) Error() string {
	return fmt.Sprintf("已回滚 %d 个，%d 个失败：%s", e.Done, len(e.Failed), strings.Join(e.Failed, "; "))
}

func UndoJournal(path string) error {
	rep, err := UndoJournalReport(path)
	if err != nil {
		return err
	}
	if len(rep.Failed) > 0 {
		return &UndoError{Done: len(rep.Restored), Failed: rep.Failed}
	}
	return nil
}

// UndoJournalReport is UndoJournal that also reports which files were
// actually restored, for callers that must keep per-row state in sync.
func UndoJournalReport(path string) (UndoReport, error) {
	var rep UndoReport
	b, err := os.ReadFile(path)
	if err != nil {
		return rep, err
	}
	var j journalFile
	if err := json.Unmarshal(b, &j); err != nil {
		return rep, err
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
		// 尽力而为:一个文件失败不能把整批回滚停在半完成状态,
		// 那样用户既不知道已经退回哪些,也没有办法继续。
		if err := os.Rename(old, new); err != nil {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s (%v)", op.NewName, err))
			continue
		}
		fmt.Printf("* %s => %s\n", old, op.OldName)
		n++
		rep.Restored = append(rep.Restored, op.NewName)
		j.Renames[i].Applied = false
	}
	if n > 0 {
		// 标记为已回滚:再点一次"回滚上次"不该把已经退回去的文件重放一遍。
		if err := writeJournalFile(path, j); err != nil {
			return rep, fmt.Errorf("已回滚 %d 个，但日志未能更新：%w", n, err)
		}
	}
	if len(rep.Failed) == 0 {
		fmt.Printf("已回滚 %d 个更名\n", n)
	}
	return rep, nil
}
