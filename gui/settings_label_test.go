package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 设置面板的标签列只该比最长的标签宽一点。
//
// 原来写死 120 DIP,而最长的标签「默认提示词」在 12px 下只有 60 DIP——多出的
// 那一倍是从控件身上白拿走的,而这个面板里控件越宽越好用(接口地址、日志路径
// 都很长,窄一格就只剩开头几个字符)。
func TestSettingsLabelColumnFitsTheLongestLabel(t *testing.T) {
	// 面板里真实出现的标签,改标签时这张表要跟着改。
	labels := []string{"接口地址", "模型", "API Key", "等待秒数", "并发路数", "默认提示词",
		"操作日志", "外观模式", "主色来源", "主色", "配色预览"}
	widest := float32(0)
	for _, l := range labels {
		l := l
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Row(c).Children(func() { ui.Text(c, l).FontSize(12) })
		}, 400, 60)
		tt.Frame()
		r, ok := tt.Find(l)
		if !ok {
			t.Fatalf("量不到标签 %q 的宽度", l)
		}
		if r.W > widest {
			widest = r.W
		}
	}
	// 允许 24 DIP 余量(给将来更长一点的标签),再多就是从控件身上白拿的。
	if settingsLabelW > widest+24 {
		t.Fatalf("标签列 %g 比最长标签(%g)宽出太多,控件白丢 %g DIP",
			settingsLabelW, widest, settingsLabelW-widest-8)
	}
	if settingsLabelW < widest+8 {
		t.Fatalf("标签列 %g 装不下最长标签(%g)+行内边距", settingsLabelW, widest)
	}
}

// 设置面板在最小窗口下不许有控件被裁掉:面板宽度是算出来的(520 或 w-48),
// 标签列一宽,右边缘的控件就会被卡片裁掉,而且裁得一点痕迹都没有。
func TestSettingsFormFitsInsideItsCard(t *testing.T) {
	for _, w := range []int{900, 1000, 1280} {
		a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
		// 长路径 + 长地址:真实设置里最常见的"撑宽"来源。
		a.settings.Journal = "C:/Users/someone/AppData/Roaming/ncreamer/journal-file-with-a-long-name.json"
		a.settings.AI.Base = "https://api.deepseek.com/v1"
		a.openSettings()
		tt := ui.NewTester(a.view, w, 800)
		tt.Frame()
		tt.Frame()

		dw := float32(520)
		if float32(w) >= 980 {
			dw = 880
		}
		if float32(w)-48 < dw {
			dw = float32(w) - 48
		}
		x0 := (float32(w) - dw) / 2
		// 面板内边距 20 + 卡片内边距 12。
		cardRight := x0 + dw - 20 - 12
		// 卡片里的东西按卡片内缘量;面板标题与底排按钮坐在面板内边距上,
		// 按面板内缘量。滚动视口的位置是运行时算的,生产代码里已经存下来了。
		body := a.setBodyRect
		panelRight := x0 + dw - 20
		for _, ai := range tt.AccessAll() {
			if ai.Name == "" || ai.Bounds.W == 0 {
				continue
			}
			limit := panelRight
			if ai.Bounds.Y >= body.Y-1 && ai.Bounds.Y <= body.Y+body.H+1 {
				limit = cardRight
			}
			if r := ai.Bounds.X + ai.Bounds.W; r > limit+0.5 {
				t.Errorf("w=%d: %q 右缘 %.1f 超出可用右缘 %.1f", w, ai.Name, r, limit)
			}
		}
	}
}

// 标签列宽是常量,测试与生产必须同源——否则测试里写 84、代码里还是 120,
// 两边都"通过"。
func TestSettingsLabelWidthIsShared(t *testing.T) {
	src := mustRead(t, "settings_ui.go")
	if !strings.Contains(src, "const settingsLabelW = float32(84)") {
		t.Error("settings_ui.go 里应当有 const settingsLabelW = float32(84)")
	}
	if strings.Contains(src, "labelW := float32(120)") {
		t.Error("标签列宽不该再写死在函数里")
	}
}
