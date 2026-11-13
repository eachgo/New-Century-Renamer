package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// 数字框:输入越界要夹紧、上下键要能步进、值要真的写回。
// 这是替换 ui.NumberInput 之后新写的控件,三条路径都得自己保证。
func TestNumFieldClampsAndSteps(t *testing.T) {
	v := 2.0
	view := func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).Children(func() {
			numField(c, &v, 1, 9, 1, 90)
		})
	}
	tt := ui.NewTester(view, 300, 100)
	tt.Frame()
	tt.Frame()

	// 光标落进输入框后接着打字:文本框里是 "2",打 7 得到 "27",
	// 超出上限 9,必须夹到 9 而不是把 27 写进值里。
	tt.ClickAt(60, 36)
	tt.Frame()
	tt.Type("7")
	tt.Frame()
	if v != 9 {
		t.Errorf("输入 27 后 v=%v,应夹到上限 9", v)
	}

	// 下键从 9 减到 8
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if v != 8 {
		t.Errorf("下键后 v=%v,应为 8", v)
	}
}
