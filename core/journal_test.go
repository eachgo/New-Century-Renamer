package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 全失败的一次执行不该覆盖上一批的回滚记录——那是用户唯一的退路。
func TestApplyPlanKeepsJournalWhenNothingApplied(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "j.json")

	// 第一批:成功一个,日志里有记录
	p1 := filepath.Join(dir, "旧名字.txt")
	if err := os.WriteFile(p1, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan1 := runRules(t, []string{"replace=旧,新"}, []string{p1})
	if err := ApplyPlan(plan1, journal); err != nil {
		t.Fatal(err)
	}
	if n := JournalCount(journal); n != 1 {
		t.Fatalf("第一批后日志应有 1 条, got %d", n)
	}

	// 第二批:目标已存在,必然全部失败
	p2 := filepath.Join(dir, "占用.txt")
	blocked := filepath.Join(dir, "改名后.txt")
	os.WriteFile(p2, []byte("y"), 0o644)
	os.WriteFile(blocked, []byte("z"), 0o644) // 占住目标名
	plan2 := runRules(t, []string{"replace=占用,改名后"}, []string{p2})
	if err := ApplyPlan(plan2, journal); err != nil {
		t.Fatal(err)
	}
	for _, op := range plan2 {
		if op.Applied {
			t.Fatalf("不该有成功的: %+v", op)
		}
	}
	if n := JournalCount(journal); n != 1 {
		t.Fatalf("全失败后日志被覆盖了: 仍有 %d 条(应为上一批的 1 条)", n)
	}
	// 上一批仍然可以回滚
	if err := UndoJournal(journal); err != nil {
		t.Fatalf("上一批应仍可回滚: %v", err)
	}
	if _, err := os.Stat(p1); err != nil {
		t.Fatalf("回滚后旧名应回来: %v", err)
	}
}

// 空日志 / 不存在的日志都不能让界面以为"有东西可回滚"。
func TestJournalCountEmpty(t *testing.T) {
	dir := t.TempDir()
	if n := JournalCount(filepath.Join(dir, "nope.json")); n != 0 {
		t.Fatalf("不存在的日志应为 0, got %d", n)
	}
	empty := filepath.Join(dir, "empty.json")
	if err := WriteJournal(empty, nil); err != nil {
		t.Fatal(err)
	}
	if n := JournalCount(empty); n != 0 {
		t.Fatalf("空日志应为 0, got %d", n)
	}
}

// 回滚遇到单个失败要继续做完其余文件,并报出退了多少、哪些失败。
func TestUndoJournalBestEffort(t *testing.T) {
	dir := t.TempDir()
	journal := filepath.Join(dir, "j.json")

	var files []string
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	plan := runRules(t, []string{"replace=.txt,-new.txt"}, files)
	if err := ApplyPlan(plan, journal); err != nil {
		t.Fatal(err)
	}
	if n := JournalCount(journal); n != 3 {
		t.Fatalf("应有 3 条待回滚, got %d", n)
	}

	// 手动制造一个"改过的文件不见了"的失败:b-new.txt 被删掉
	if err := os.Remove(filepath.Join(dir, "b-new.txt")); err != nil {
		t.Fatal(err)
	}
	err := UndoJournal(journal)
	if err == nil {
		t.Fatal("有文件回滚不了时应当报错")
	}
	ue, ok := err.(*UndoError)
	if !ok {
		t.Fatalf("应返回 *UndoError, got %T: %v", err, err)
	}
	if ue.Done != 2 {
		t.Errorf("应已回滚 2 个, got %d (%v)", ue.Done, err)
	}
	if len(ue.Failed) != 1 || !strings.Contains(ue.Failed[0], "b-new.txt") {
		t.Errorf("失败清单应点名 b-new.txt, got %v", ue.Failed)
	}
	// 其余两个真的回去了
	for _, n := range []string{"a.txt", "c.txt"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s 应当已回滚: %v", n, err)
		}
	}
	// 日志已标记为回滚过,再点一次不会再动任何文件
	if n := JournalCount(journal); n != 1 {
		t.Errorf("回滚后日志应只剩失败的那 1 条, got %d", n)
	}
}
