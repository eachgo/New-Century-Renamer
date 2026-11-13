package main

import (
	"fmt"
	"testing"

	"ncreamer/core"
)

// 行里的字和底栏的数字必须来自同一个判定。
//
// 以前是两条各自独立、顺序还相反的 switch:resultChip 先判 Err,
// apply 的统计先判 Applied。于是一个"上一轮成功、这一轮失败"的文件
// 会在行里写"失败",却被底栏算进"成功"——用户没法知道该信哪一个。
func TestRowTextAndCountersAgree(t *testing.T) {
	dir := t.TempDir()
	applied, pending, unchanged, failed := core.RenameOp{Dir: dir, OldName: "a", NewName: "b", Applied: true},
		core.RenameOp{Dir: dir, OldName: "a", NewName: "b"},
		core.RenameOp{Dir: dir, OldName: "a", NewName: "a"},
		core.RenameOp{Dir: dir, OldName: "a", NewName: "b", Err: fmt.Errorf("目标已存在同名文件：b")}

	// 最要命的那一条:上一轮成功、这一轮失败,两个标记同时存在。
	stale := failed
	stale.Applied = true

	for _, after := range []bool{false, true} {
		for _, op := range []core.RenameOp{applied, pending, unchanged, failed, stale} {
			a := &app{settings: defaultSettings()}
			a.applied = after
			a.ops = []core.RenameOp{op}
			text, _ := a.resultChip(op)
			ok, bad := a.applyOutcome()
			switch text {
			case "失败":
				if bad != 1 || ok != 0 {
					t.Fatalf("行里写失败,底栏却算 ok=%d failed=%d (after=%v, op=%+v)", ok, bad, after, op)
				}
			case "已更名":
				if ok != 1 || bad != 0 {
					t.Fatalf("行里写已更名,底栏却算 ok=%d failed=%d (after=%v, op=%+v)", ok, bad, after, op)
				}
			default:
				// 将更名/不变/跳过 都既不算成功也不算失败
				if ok != 0 || bad != 0 {
					t.Fatalf("行里写%s,底栏却算 ok=%d failed=%d (after=%v, op=%+v)", text, ok, bad, after, op)
				}
			}
		}
	}
}

// 上一轮成功、这一轮失败的项,行里必须是"失败",不能被 Applied 盖住。
func TestStaleAppliedDoesNotHideFailure(t *testing.T) {
	dir := t.TempDir()
	op := core.RenameOp{
		Dir: dir, OldName: "冲突.txt", NewName: "已占用.txt",
		Err: fmt.Errorf("目标已存在同名文件：已占用.txt"), Applied: true,
	}
	a := &app{settings: defaultSettings()}
	a.applied = true
	a.ops = []core.RenameOp{op}
	if text, _ := a.resultChip(op); text != "失败" {
		t.Fatalf("带错误的一行应显示失败, got %q", text)
	}
	if ok, bad := a.applyOutcome(); ok != 0 || bad != 1 {
		t.Fatalf("底栏应算 0 成功 1 失败, got ok=%d failed=%d", ok, bad)
	}
}
