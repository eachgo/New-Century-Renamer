package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// 三栏的分隔缝都能一路拖到只剩一百多 DIP。那种宽度下如果还按
// "标签 96 + 控件定宽"排,控件会顶出卡片右缘被 Clip 掉一半——
// 用户截图里「数字」后面的箭头、「不分组」的右半、步进器都只剩半个。
// 这两条测试守住"窄到 150 DIP 也不溢出"这条底线。
func narrowApp(t *testing.T, splitL, splitM float32) *app {
	t.Helper()
	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	a.steps = []guiRule{step("seq")}
	a.stepUID = a.steps[0].opUID
	a.splitL, a.splitM = splitL, splitM
	a.files = []fileInfo{{
		path: filepath.Join(t.TempDir(), "2026-11-14 会议录音 很长的名字.mp3"),
		size: 1 << 20, modTime: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC),
		realType: "mp3", typ: "MP3 音频",
	}}
	a.preview()
	return a
}

// middleRight 是中栏内容区的右边界(含分隔缝),控件不该越过它。
func middleRight(splitL, splitM float32) float32 {
	const pad, gripW = float32(8), float32(10)
	return pad + splitL + gripW + splitM
}

func TestParamsFormFitsNarrowMiddlePane(t *testing.T) {
	for _, m := range []float32{330, 218, 150} {
		a := narrowApp(t, 220, m)
		tt := ui.NewTester(a.view, 900, 720)
		tt.Frame()
		tt.Frame()

		limit := middleRight(220, m) - 8 // 卡片自身还有 10 的内边距
		for _, label := range []string{"编号类型", "不足位补齐", "分组方式", "起始 / 步长", "编号模板"} {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("splitM=%v:找不到标签 %q", m, label)
			}
			if r.X+r.W > limit {
				t.Errorf("splitM=%v:标签 %q 右缘 %.1f 超出中栏 %.1f", m, label, r.X+r.W, limit)
			}
		}
		// 控件本身:找到值文字,它的右缘不该越过中栏
		for _, val := range []string{"数字", "不分组"} {
			r, ok := tt.Find(val)
			if !ok {
				t.Fatalf("splitM=%v:找不到控件值 %q", m, val)
			}
			if r.X+r.W > limit {
				t.Errorf("splitM=%v:控件 %q 右缘 %.1f 超出中栏 %.1f(被裁)", m, val, r.X+r.W, limit)
			}
		}
	}
}

// 窄栏下四个操作按钮必须换行,不能靠裁掉"复制"来收场。
func TestParamsButtonsWrapWhenNarrow(t *testing.T) {
	a := narrowApp(t, 220, 150)
	tt := ui.NewTester(a.view, 900, 720)
	tt.Frame()
	tt.Frame()

	limit := middleRight(220, 150) - 8
	up, ok := tt.Find("上移")
	if !ok {
		t.Fatal("找不到「上移」")
	}
	cp, ok := tt.Find("复制")
	if !ok {
		t.Fatal("找不到「复制」")
	}
	if cp.X+cp.W > limit {
		t.Errorf("「复制」右缘 %.1f 超出中栏 %.1f(按钮被裁)", cp.X+cp.W, limit)
	}
	if cp.Y == up.Y {
		t.Errorf("150 DIP 下四个按钮仍在同一行(Y=%.1f),应该换行", up.Y)
	}
}

// 右栏被拖到 150 DIP 时,列头「原名 → 新名」会被压成一列竖排单字,
// 比截断更难看。加了 Wrap 以后它整体换行。
func TestFileListHeaderStaysReadableWhenNarrow(t *testing.T) {
	a := narrowApp(t, 900-330-150, 330)
	tt := ui.NewTester(a.view, 900, 720)
	tt.Frame()
	tt.Frame()

	r, ok := tt.Find("原名 → 新名")
	if !ok {
		t.Fatal("找不到列头「原名 → 新名」")
	}
	if r.H > r.W {
		t.Errorf("列头被压成竖排:%.1f×%.1f", r.W, r.H)
	}
}

// 主色是用户选的,不该由窗口焦点决定。mygo 在 InactiveAccent 留空时
// 会按 AccentText 混出一层灰,切到别的窗口整屏主题色就褪掉。
func TestInactiveAccentKeepsBrandColor(t *testing.T) {
	dt := buildTheme(ui.RGB(0x2F, 0x7A, 0x52))
	for _, th := range []*ui.Theme{dt.light, dt.dark} {
		if th.InactiveAccent != th.Accent {
			t.Errorf("失焦主色 %v 不等于主色 %v", th.InactiveAccent, th.Accent)
		}
	}
}
