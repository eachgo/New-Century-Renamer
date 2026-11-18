package main

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 窗口边框色是"唯一一块不是我们画的像素":DWM 在 Win11 上会给每个窗口画
// 一圈 1px 的线,聚焦时强调色、失焦时灰白。用户把桌面设成纯黑之后,它变成
// 一道白边(实测窗口最外 1px 恒为 (255,255,255),左/右/下三边都有、顶部
// 没有),看上去像渲染 bug。
//
// 我们没法让 DWM 不画(那个属性在部分 build 上不生效),只能让它画成我们的
// 底色——画出来和旁边一模一样,等于没有。这组测试守的就是"那个颜色必须
// 是内容底色,而且必须跟着明暗一起变"。

// TestWindowBorderMatchesTheContent 断言边框色就是基底色,不是某个写死的
// 白/灰。用多个种子(强调色不同)与明暗两档,防止它被改成常量。
func TestWindowBorderMatchesTheContent(t *testing.T) {
	for _, seed := range []string{"#2F7A52", "#CC9B29", "#1E5AA8", "#8E44AD"} {
		dt := themeFor(ui.Hex(seed))
		for _, dark := range []bool{false, true} {
			want := dt.panes(dark).base
			got := borderColorFor(dt, dark)
			if got != want {
				t.Errorf("%s(dark=%v): 窗口边框色是 %v,应当就是基底色 %v", seed, dark, got, want)
			}
			// 顺便守住"不是纯白也不是纯黑":那两个值说明它退回了系统默认。
			if got.R == 255 && got.G == 255 && got.B == 255 {
				t.Errorf("%s(dark=%v): 边框色退成了纯白——正是用户截图里那道线", seed, dark)
			}
			if got.R == 0 && got.G == 0 && got.B == 0 {
				t.Errorf("%s(dark=%v): 边框色是纯黑,在亮色外观下会变成一圈黑框", seed, dark)
			}
		}
	}
}

// TestWindowBorderFollowsAppearance 断言亮/暗两档给出的边框色不同——底色
// 本身差一档,如果边框色不变,切外观之后那圈线就会露出来。
func TestWindowBorderFollowsAppearance(t *testing.T) {
	dt := themeFor(ui.Hex("#2F7A52"))
	light, dark := borderColorFor(dt, false), borderColorFor(dt, true)
	if light == dark {
		t.Fatalf("亮色与暗色的窗口边框色相同(%v)——切外观之后那圈线会露出来", light)
	}
}

// TestSyncWindowBorderOnlyCallsWhenChanged 断言每帧只比一次、颜色没变就
// 不碰系统调用。syncWindowBorder 在 view() 里每帧都跑,而 setWindowBorder
// 要跨进程进 DWM。
func TestSyncWindowBorderOnlyCallsWhenChanged(t *testing.T) {
	calls := 0
	var last ui.Color
	oldBorder, oldHandle := setWindowBorder, windowHandle
	setWindowBorder = func(hwnd uintptr, col ui.Color) { calls++; last = col }
	windowHandle = func(w *mygo.Window) uintptr { return 42 }
	t.Cleanup(func() { setWindowBorder, windowHandle = oldBorder, oldHandle })

	dt := themeFor(ui.Hex("#2F7A52"))
	a := &app{}
	// 没有窗口时不该调(测试里 a.win 是 nil)。
	a.syncWindowBorder(dt, false)
	if calls != 0 {
		t.Fatalf("还没有窗口就调了系统调用 %d 次", calls)
	}

	a.win = &mygo.Window{}
	a.syncWindowBorder(dt, false)
	if calls != 1 {
		t.Fatalf("第一次同步应当调 1 次,实际 %d 次", calls)
	}
	if want := dt.panes(false).base; last != want {
		t.Fatalf("写进去的颜色是 %v,应当是基底色 %v", last, want)
	}
	// 同一档再跑几帧:一次都不该再调。
	for i := 0; i < 5; i++ {
		a.syncWindowBorder(dt, false)
	}
	if calls != 1 {
		t.Fatalf("底色没变却又调了 %d 次——每帧都在跨进程进 DWM", calls-1)
	}
	// 切到暗色:必须重新调一次。
	a.syncWindowBorder(dt, true)
	if calls != 2 {
		t.Fatalf("切到暗色后应当再调 1 次,实际总共 %d 次", calls)
	}
	if want := dt.panes(true).base; last != want {
		t.Fatalf("切暗色后写进去的是 %v,应当是 %v", last, want)
	}
}
