package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckNameRejects(t *testing.T) {
	cases := []struct{ name, want string }{
		{"", "空的"},
		{"a/b.txt", "路径分隔符"},
		{`a\b.txt`, "路径分隔符"},
		{`a<b.txt`, `"<"`},
		{`a>b.txt`, `">"`},
		{`a:b.txt`, `":"`},
		{`a"b.txt`, `"\""`},
		{"a|b.txt", `"|"`},
		{"a?b.txt", `"?"`},
		{"a*b.txt", `"*"`},
		{"trailing.txt ", "空格或点结尾"},
		{"trailing.", "空格或点结尾"},
		{"a\tb.txt", "控制字符"},
		{"a\x00b.txt", "控制字符"},
		{"CON", "保留的设备名"},
		{"con.txt", "保留的设备名"},
		{"NUL.mp3", "保留的设备名"},
		{"com1.log", "保留的设备名"},
		{"lpt9", "保留的设备名"},
	}
	for _, c := range cases {
		got := CheckName(c.name)
		if !strings.Contains(got, c.want) {
			t.Errorf("CheckName(%q) = %q, 应包含 %q", c.name, got, c.want)
		}
	}
}

func TestCheckNameAccepts(t *testing.T) {
	// 中文、空格、括号、点在内侧、看起来像设备名但不是,都要放行
	for _, name := range []string{
		"第01话 标题.mp3", "a.b.c.txt", "console.txt", "com10.txt",
		"我的文件 (1).flac", "..hidden", "CONSOLE", "a-b_c=d.txt",
	} {
		if msg := CheckName(name); msg != "" {
			t.Errorf("CheckName(%q) = %q, 不该拒绝", name, msg)
		}
	}
}

// preflight 必须把非法名字标成错误,否则预览说"将更名"、执行时失败。
func TestPreflightRejectsIllegalNames(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"con.txt", "a<b.txt", "尾点.", "x\x01.txt"} {
		plan := []RenameOp{{Dir: dir, OldName: "a.txt", NewName: bad}}
		preflight(plan)
		if plan[0].Err == nil {
			t.Errorf("preflight 放过了非法名字 %q", bad)
		}
	}
}

// 执行侧是第二道闸:即使调用方跳过 preflight,也不能让非法名字动磁盘。
func TestApplyRejectsIllegalNamesWithoutPreflight(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := []RenameOp{{Dir: dir, OldName: "a.txt", NewName: "con.txt"}}
	rep, err := ApplyPlanReport(plan, filepath.Join(dir, "j.json"), nil)
	if err != nil {
		t.Fatalf("不该返回整体错误: %v", err)
	}
	if rep.Failed != 1 || rep.Renamed != 0 {
		t.Fatalf("rep = %+v, 期望 Failed=1 Renamed=0", rep)
	}
	if plan[0].Err == nil {
		t.Fatal("执行侧没有拦下非法名字")
	}
	// 文件必须还在原处
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("源文件被动了: %v", err)
	}
}

// ApplyReport 的计数必须来自执行者,而不是回头读 Applied。
func TestApplyReportCountsOutcomes(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// 占住 b 的目标名,让它必然失败
	if err := os.WriteFile(filepath.Join(dir, "taken.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := []RenameOp{
		{Dir: dir, OldName: "a.txt", NewName: "A.txt"},                         // 成功
		{Dir: dir, OldName: "b.txt", NewName: "taken.txt"},                     // 目标已存在
		{Dir: dir, OldName: "c.txt", NewName: "c.txt"},                         // 不变
		{Dir: dir, OldName: "gone.txt", NewName: "x.txt", Err: os.ErrNotExist}, // 带旧错
	}
	rep, err := ApplyPlanReport(plan, filepath.Join(dir, "j.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Renamed != 1 || rep.Failed != 1 || rep.Unchanged != 1 || rep.Skipped != 1 {
		t.Fatalf("rep = %+v, 期望 Renamed=1 Failed=1 Unchanged=1 Skipped=1", rep)
	}
}
