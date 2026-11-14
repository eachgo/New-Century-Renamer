package main

import (
	"os"
	"path/filepath"
	"testing"
)

// 外部入口必须真的把文件加进列表,而且和拖放走同一条路。
func TestOpenExternalAddsFile(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	p := filepath.Join(t.TempDir(), "从资源管理器打开.mp3")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 外部入口不该当场改列表:它排进队列,由帧首统一追加。
	a.openExternal(p)
	if len(a.files) != 0 {
		t.Fatalf("openExternal 当场改了列表: %d 个文件", len(a.files))
	}
	if len(a.pendingDrop) != 1 {
		t.Fatalf("排队数 = %d, 期望 1", len(a.pendingDrop))
	}
	a.drainPendingDrop()
	if len(a.files) != 1 || a.files[0].path != p {
		t.Fatalf("帧首没有把外部文件加进来: %+v", a.files)
	}
}

// 同一个文件从两条路各来一次,只能算一个。
func TestOpenExternalDeduplicates(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	p := filepath.Join(t.TempDir(), "同一个.mp3")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.openExternal(p)
	a.openExternal(p)
	a.drainPendingDrop()
	if len(a.files) != 1 {
		t.Fatalf("同一个文件被加了 %d 次", len(a.files))
	}
}

// 空路径不该进队列(某些平台在"打开方式"里会送一个空串)。
func TestOpenExternalIgnoresEmpty(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.openExternal("")
	if len(a.pendingDrop) != 0 {
		t.Fatalf("空路径进了队列: %v", a.pendingDrop)
	}
}

// 窗口还没建好就收到外部文件时不能崩:路径留着,等窗口起来再加。
func TestOpenExternalBeforeWindowIsSafe(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.win = nil // 模拟 WhenReady 之前
	p := filepath.Join(t.TempDir(), "早到.mp3")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.openExternal(p) // update() 在 win==nil 时是空操作
	if len(a.pendingDrop) != 1 {
		t.Fatalf("路径丢了: %v", a.pendingDrop)
	}
	a.drainPendingDrop()
	if len(a.files) != 1 {
		t.Fatalf("窗口起来后没补上: %d", len(a.files))
	}
}

// 第二个实例必须把窗口捞出来,不能只是"什么也没发生"。
func TestSecondInstanceBringsWindowForward(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	// 真正的 Show/Focus 是平台调用,headless 下没有可观测的效果,所以这里
	// 只能守住"窗口还没建好时不能崩"这一条——而它恰恰是最容易踩的:
	// 第二个实例可能在 WhenReady 之前就撞上来。
	a.bringToFront() // a.win == nil,不该 panic
}
