package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// 右栏能被一路拖到只剩几十 DIP。那种宽度下"名字 + . + 扩展名 + 结果"
// 排不进一行:允许它压,最先塌掉的是最短的那颗胶囊——实测「flac」被
// 挤成 9×22.6 的竖排一列,结果词「将更名」被挤到视口外只剩一截。
// 这里守住"无论多窄都不出现竖排、也不顶出窗口"这条底线。
func degradeApp(t *testing.T) *app {
	t.Helper()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	dir := t.TempDir()
	now := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	a.files = []fileInfo{
		{path: filepath.Join(dir, "g", "01 - Awich - THE UNION 很长的名字.flac"),
			size: 12 << 20, modTime: now, typ: "FLAC 音频", realType: "flac", group: "g"},
		{path: filepath.Join(dir, "g", "02 - Awich - THE UNION.flac"),
			size: 1, modTime: now, typ: "FLAC 音频", realType: "flac", group: "g"},
	}
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	a.preview()
	return a
}

func TestFileListNeverSqueezesTextVertical(t *testing.T) {
	// splitL 从宽到极窄,覆盖 showResultText/showExt 两条降级线的两侧。
	for _, splitL := range []float32{220, 280, 300, 320, 340, 380, 400, 420, 460} {
		a := degradeApp(t)
		a.splitL, a.splitM = splitL, 330
		tt := ui.NewTester(a.view, 900, 480)
		for i := 0; i < 4; i++ {
			tt.Frame()
		}
		for _, s := range tt.Texts() {
			r, ok := tt.Find(s)
			if !ok || r.H <= 0.5 {
				continue
			}
			// 多字符文本被压成一列:宽小于高
			if len([]rune(s)) > 1 && r.W < r.H-0.5 {
				t.Fatalf("splitL=%v: %q 被压成竖排 %.1f×%.1f", splitL, s, r.W, r.H)
			}
			if r.X < -1 || r.X+r.W > 901 {
				t.Fatalf("splitL=%v: %q 顶出窗口 x=%.1f right=%.1f", splitL, s, r.X, r.X+r.W)
			}
		}
	}
}

// 降级顺序:先丢日期、再丢大小、再收起结果词、最后连扩展名胶囊也不画。
// 每一档都要"更宽的一档包含更窄一档的全部信息",不能乱序。
func TestFileListDegradeOrder(t *testing.T) {
	type state struct{ date, size, result, ext bool }
	get := func(t *testing.T, splitL float32) state {
		t.Helper()
		a := degradeApp(t)
		a.splitL, a.splitM = splitL, 330
		tt := ui.NewTester(a.view, 900, 480)
		for i := 0; i < 4; i++ {
			tt.Frame()
		}
		_, date := tt.Find("2026-11-14")
		_, size := tt.Find("12.0 MB")
		_, result := tt.Find("将更名")
		_, ext := tt.Find("flac")
		return state{date, size, result, ext}
	}
	wide := get(t, 220)
	if !wide.date || !wide.size || !wide.result || !wide.ext {
		t.Fatalf("最宽的一档应该四样都在: %+v", wide)
	}
	narrow := get(t, 460)
	if narrow.date || narrow.size {
		t.Fatalf("极窄的一档应该先丢掉日期与大小: %+v", narrow)
	}
	// 单调:越窄保留的越少
	prev := wide
	for _, splitL := range []float32{280, 320, 360, 400, 460} {
		cur := get(t, splitL)
		for _, pair := range []struct {
			name     string
			was, now bool
		}{
			{"日期", prev.date, cur.date},
			{"大小", prev.size, cur.size},
			{"结果词", prev.result, cur.result},
			{"扩展名", prev.ext, cur.ext},
		} {
			if !pair.was && pair.now {
				t.Fatalf("splitL=%v: %s 在更窄的宽度下反而出现了(降级不单调)", splitL, pair.name)
			}
		}
		prev = cur
	}
}
