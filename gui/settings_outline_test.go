package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// settingsOutlineApp 打开设置面板并返回一个 1280×900 的 tester。
func settingsOutlineApp(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(t.TempDir(), "journal.json")
	a.openSettings()
	tt := ui.NewTester(a.view, 1280, 900)
	tt.SetScale(1)
	tt.Frame()
	tt.Frame()
	return a, tt
}

// 一个控件只许有一条外轮廓。
//
// 「等待秒数」这种"输入框 + 步进器"曾经是两个各自描边的圆角框并排,
// 中间还夹一条底色缝——看上去像两个控件拼在一起,它们本来是一个。
// 现在容器画唯一的框,里面的输入框和步进器都是 Border(0)。
//
// 这条测试量的是"框内部还有没有竖边":有,就说明又退回了并排两个框。
func TestNumberFieldsDrawOneOutline(t *testing.T) {
	a, tt := settingsOutlineApp(t)
	img := tt.Image()
	th := themeFor(a.seedPick)
	borders := []ui.Color{liveBorder(th.light), inertBorder(th.light), th.light.Border}
	isBorder := func(x, y int) bool {
		c := pix(img, x, y)
		for _, tg := range borders {
			if c == tg {
				return true
			}
		}
		return false
	}

	// 带步进器的那两个:容器 = 输入框左边到步进器右边(步进器宽 20,贴在右缘)。
	for _, label := range []string{"等待秒数", "并发路数"} {
		var field ui.AccessInfo
		var found bool
		for _, ai := range tt.AccessAll() {
			if ai.Role == ui.RoleTextField && ai.Name == label {
				field, found = ai, true
				break
			}
		}
		if !found {
			t.Fatalf("设置面板里找不到「%s」", label)
		}
		left := int(field.Bounds.X) - 1
		right := int(field.Bounds.X+field.Bounds.W) + 21
		y := int(field.Bounds.Y + field.Bounds.H/2)

		var edges []int
		for x := left; x <= right; x++ {
			if isBorder(x, y) {
				edges = append(edges, x)
			}
		}
		if len(edges) < 2 {
			t.Fatalf("「%s」找不到外轮廓(x=%d..%d, y=%d 量到 %v)", label, left, right, y, edges)
		}
		// 只有最左和最右两条是合法的;中间任何一条都是"两个框并排"的缝。
		inner := edges[1 : len(edges)-1]
		if len(inner) > 0 {
			t.Errorf("「%s」的框内部还有竖边 %v——输入框和步进器又变成两个各自描边的框了", label, inner)
		}
	}

	// 其余控件(单行输入框、下拉、开关)同样只许有一条外轮廓:一个输入框
	// 里面再套一个描边的框,是"框套框",读起来像两层可编辑区域。
	for _, ai := range tt.AccessAll() {
		switch ai.Role {
		case ui.RoleTextField, ui.RolePopUpButton, ui.RoleComboBox:
		default:
			continue
		}
		b := ai.Bounds
		if b.W < 60 || b.H < 20 {
			continue
		}
		y := int(b.Y + b.H/2)
		var inner []int
		for x := int(b.X) + 2; x < int(b.X+b.W)-2; x++ {
			if isBorder(x, y) {
				inner = append(inner, x)
			}
		}
		if len(inner) > 0 {
			t.Errorf("「%s」里面还有竖边 %v——框套框了", ai.Name, inner)
		}
	}
}
