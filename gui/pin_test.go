package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func pinApp(t *testing.T) *app {
	t.Helper()
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = t.TempDir() + "/journal.json"
	return a
}

// 钉住的条目要出现在最上面那个「常用」组里。
func TestPinnedEntriesAppearOnTop(t *testing.T) {
	a := pinApp(t)
	a.pinned = []string{"seq", "meta.nfo"}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("▾ 常用") {
		t.Fatal("钉了条目之后应有「常用」组")
	}
	// 两条都在
	if !tt.HasText("追加序号") {
		t.Fatal("常用组里应有「追加序号」")
	}
	if !tt.HasText("按 NFO 编号命名") {
		t.Fatal("常用组里应有「按 NFO 编号命名」")
	}
}

// 没钉东西的时候不该凭空长出一个空组。
func TestNoPinGroupWhenNothingPinned(t *testing.T) {
	a := pinApp(t)
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	if tt.HasText("常用") {
		t.Fatal("一条都没钉时不该有「常用」组")
	}
}

// 钉住的条目同时留在原分组里——钉是"再加一个入口",不是"搬家"。
func TestPinnedEntryKeepsItsOriginalGroup(t *testing.T) {
	a := pinApp(t)
	a.pinned = []string{"seq"}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	// 同一个标签在「常用」和「编号」两组里各出现一次
	n := 0
	for _, s := range tt.Texts() {
		if s == "追加序号" {
			n++
		}
	}
	if n < 2 {
		t.Fatalf("钉住的条目应在常用与原分组各出现一次,实际出现 %d 次", n)
	}
}

// togglePin 是幂等的:再钉一次就是取消。
func TestTogglePinTwiceUnpins(t *testing.T) {
	a := pinApp(t)
	a.togglePin("seq")
	if !a.isPinned("seq") {
		t.Fatal("钉选失败")
	}
	a.togglePin("seq")
	if a.isPinned("seq") {
		t.Fatal("再钉一次应取消")
	}
	if len(a.pinned) != 0 {
		t.Fatalf("取消后列表应为空,实际 %v", a.pinned)
	}
}

// 设置里存了已经不存在的 id(旧版本删过规则)时,跳过而不是画个空条目。
func TestPinnedUnknownIDIsSkipped(t *testing.T) {
	a := pinApp(t)
	a.pinned = []string{"seq", "this.op.does.not.exist"}
	got := a.pinnedEntries()
	if len(got) != 1 || got[0].id != "seq" {
		t.Fatalf("未知 id 应被跳过,实际 %d 条: %v", len(got), got)
	}
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame() // 不该崩
}

// 钉选顺序就是用户钉的顺序,不按目录顺序重排。
func TestPinOrderFollowsUser(t *testing.T) {
	a := pinApp(t)
	a.pinned = []string{"meta.nfo", "seq"} // 与目录里的先后相反
	got := a.pinnedEntries()
	if len(got) != 2 || got[0].id != "meta.nfo" || got[1].id != "seq" {
		t.Fatalf("顺序应跟随钉选顺序,实际 %v", []string{got[0].id, got[1].id})
	}
}

// 搜索时不显示「常用」组:否则同一条结果会出现两次。
func TestPinGroupHiddenWhileSearching(t *testing.T) {
	a := pinApp(t)
	a.pinned = []string{"seq"}
	a.palQuery = "序号"
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	if tt.HasText("▾ 常用") {
		t.Fatal("搜索时不该显示「常用」组")
	}
}
