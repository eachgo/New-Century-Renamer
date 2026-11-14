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
	// 步长 2:溢出窗口只发生在很窄的一段(实测 innerW 124~154,即
	// "扩展名胶囊刚被画出来、整组名字已经放不下"之间),步长 20 会整个
	// 跨过去。webm 是最宽的扩展名(胶囊 57.7 DIP),必须包含。
	for _, splitL := range []float32{210, 220, 240, 260, 280, 300, 320, 340, 360, 370, 380, 390, 400, 410, 420, 430, 440, 460, 470} {
		a := degradeApp(t)
		a.splitL, a.splitM = splitL, 330
		tt := ui.NewTester(a.view, 900, 480)
		for i := 0; i < 4; i++ {
			tt.Frame()
		}
		// 走 AccessAll 而不是 tt.Texts()+tt.Find:Find 只返回第一个同名
		// 匹配(左栏目录与详情区常有同名条目),而这里要检查的是"每一个
		// 文本节点"。以前用 Find,同一段文字出现两次时只看得到一处,
		// 另一处被压成竖排也发现不了。
		seen := 0
		for _, n := range tt.AccessAll() {
			if n.Role != ui.RoleText || n.Name == "" {
				continue
			}
			seen++
			// 被压到没有面积:文字被挤出了存在。以前 H<=0.5 直接
			// continue,这类"整段文字消失"的情况反而是最不容易被
			// 看见的——它连竖排都算不上。
			if n.Bounds.W <= 0.5 || n.Bounds.H <= 0.5 {
				t.Fatalf("splitL=%v: 文本 %q 被压到 %.1f×%.1f(整段消失)",
					splitL, n.Name, n.Bounds.W, n.Bounds.H)
			}
			// 多字符文本被压成一列:宽小于高。
			if len([]rune(n.Name)) > 1 && n.Bounds.W < n.Bounds.H-0.5 {
				t.Fatalf("splitL=%v: %q 被压成竖排 %.1f×%.1f",
					splitL, n.Name, n.Bounds.W, n.Bounds.H)
			}
			if n.Bounds.X < -1 || n.Bounds.X+n.Bounds.W > 901 {
				t.Fatalf("splitL=%v: %q 顶出窗口 x=%.1f right=%.1f",
					splitL, n.Name, n.Bounds.X, n.Bounds.X+n.Bounds.W)
			}
		}
		if seen < 10 {
			t.Fatalf("splitL=%v: 只看到 %d 个文本节点,遍历本身可能失效了", splitL, seen)
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
