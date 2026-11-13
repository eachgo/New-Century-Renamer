package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// 执行成功后改过的行要亮一下:改名是当天最重要的事,
// 以前只有一个静态的结果词。
func TestAppliedRowsFlashThenFade(t *testing.T) {
	dir := t.TempDir()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	writeTestFile(t, filepath.Join(dir, "A.TXT"), "x")
	a.addPaths([]string{filepath.Join(dir, "A.TXT")})
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID

	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	a.apply()

	if a.flashAlpha() <= 0 {
		t.Fatal("执行后没有开始高亮")
	}
	if a.flashUntil.IsZero() {
		t.Fatal("没有记录高亮结束时刻")
	}

	// 把结束时刻拨到过去,模拟 320ms 之后
	a.flashUntil = time.Now().Add(-time.Millisecond)
	tt.Frame()
	if k := a.flashAlpha(); k != 0 {
		t.Fatalf("高亮过了时间还在: %v", k)
	}
}

// 高亮强度必须随时间单调下降,否则看起来是闪烁不是淡出。
func TestFlashAlphaFadesMonotonically(t *testing.T) {
	a := &app{}
	a.flashUntil = time.Now().Add(flashDuration)
	prev := a.flashAlpha()
	if prev <= 0 || prev > 1 {
		t.Fatalf("起始强度应在 (0,1]: %v", prev)
	}
	for i := 0; i < 5; i++ {
		time.Sleep(flashDuration / 10)
		cur := a.flashAlpha()
		if cur > prev+0.001 {
			t.Fatalf("强度反弹了: %v -> %v", prev, cur)
		}
		prev = cur
	}
}

// 没有执行过的时候不该有任何高亮,否则整个列表一直是绿的。
func TestNoFlashBeforeApply(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	if k := a.flashAlpha(); k != 0 {
		t.Fatalf("没执行过却有高亮: %v", k)
	}
}
