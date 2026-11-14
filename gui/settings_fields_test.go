package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// settingsApp opens the settings dialog on a fresh app: the AI fields only
// exist while it is open, and every test here drives them the way a user does.
func settingsApp(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	a := &app{stepSel: -1, settings: defaultSettings()}
	a.openSettings()
	tt := ui.NewTester(a.view, 1280, 800)
	tt.Frame()
	tt.Frame()
	return a, tt
}

// fieldRect finds a labelled text field through the accessibility tree: Find
// matches visible text, and an empty field shows its placeholder instead of a
// name, so a field with content has nothing on screen to match.
func fieldRect(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	// AccessByRole,不是 Access:标签本身和输入框同名(标签就在输入框左边),
	// Access 返回的是那行文字,点它当然打不开光标。
	ai, ok := tt.AccessByRole(ui.RoleTextField, label)
	if !ok {
		t.Fatalf("找不到名为 %q 的输入框", label)
	}
	return ai.Bounds
}

func clickField(tt *ui.Tester, r ui.Rect) {
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
}

// API Key 默认必须是掩码:设置面板常被人从背后看到,而 key 是这里唯一
// 不能给旁人看的东西。
func TestAPIKeyIsMaskedByDefault(t *testing.T) {
	_, tt := settingsApp(t)

	ai, ok := tt.AccessByRole(ui.RoleTextField, "API Key")
	if !ok {
		t.Fatal("设置里找不到 API Key 字段")
	}
	if !ai.Password {
		t.Error("API Key 默认应当掩码显示")
	}
	if !tt.HasText("显示") {
		t.Error("掩码时应有「显示」按钮")
	}

	r, ok := tt.Find("显示")
	if !ok {
		t.Fatal("找不到「显示」按钮")
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()

	ai, _ = tt.AccessByRole(ui.RoleTextField, "API Key")
	if ai.Password {
		t.Error("点了「显示」之后应当明文")
	}
	if !tt.HasText("隐藏") {
		t.Error("明文时按钮文案应为「隐藏」")
	}

	r, ok = tt.Find("隐藏")
	if !ok {
		t.Fatal("找不到「隐藏」按钮")
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if ai, _ = tt.AccessByRole(ui.RoleTextField, "API Key"); !ai.Password {
		t.Error("再点一次应回到掩码")
	}
}

// 留空表示"用默认"。这时框里要显示真正生效的数字(300/4),而不是 0 或空白
// ——面板写 300、程序跑 180 这种事,比不写提示更糟。同时不能把默认值
// 写回缓冲:用户没动过手,设置里不该凭空多出一条显式配置。
func TestAIIntFieldsShowEffectiveDefaults(t *testing.T) {
	a, tt := settingsApp(t)

	if !tt.HasText(itoa(core.DefaultAITimeoutSeconds)) {
		t.Errorf("等待秒数未显示默认值 %d", core.DefaultAITimeoutSeconds)
	}
	if !tt.HasText(itoa(core.DefaultAIConcurrency)) {
		t.Errorf("并发路数未显示默认值 %d", core.DefaultAIConcurrency)
	}
	if a.aiTimeoutText != "" || a.aiConcurrencyText != "" {
		t.Errorf("没编辑过就不该写回缓冲: timeout=%q conc=%q", a.aiTimeoutText, a.aiConcurrencyText)
	}
}

// 填了非数字:当场说清楚,而不是等关对话框时静默变成默认值。
func TestAIIntFieldRejectsNonNumberInPlace(t *testing.T) {
	a, tt := settingsApp(t)

	r := fieldRect(t, tt, "等待秒数")
	clickField(tt, r)
	tt.Type("abc")
	tt.Frame()

	if !tt.HasText("只能填整数") {
		t.Error("非数字输入应当就地提示")
	}
	a.commitAITimeout()
	if a.settingsDraft.AI.Timeout != 0 {
		t.Errorf("非法输入应回落默认(0),实际 %d", a.settingsDraft.AI.Timeout)
	}
}

// 超出范围:提示范围,失焦时夹回边界而不是原样写进设置。
func TestAIIntFieldClampsOutOfRange(t *testing.T) {
	a, tt := settingsApp(t)

	r := fieldRect(t, tt, "等待秒数")
	clickField(tt, r)
	for i := 0; i < 6; i++ {
		tt.Key(0, ui.KeyBackspace)
	}
	tt.Type("5000")
	tt.Frame()
	if !tt.HasText("范围 10–3600") {
		t.Error("超出范围应当就地提示")
	}

	tt.ClickAt(900, 700) // 点空白处失焦
	tt.Frame()
	if got := strings.TrimSpace(a.aiTimeoutText); got != "3600" {
		t.Errorf("失焦后应夹到上界 3600,实际 %q", got)
	}
	// 设置面板开着时 commit 写的是草稿(liveSettings),不是已保存的那份。
	a.commitAITimeout()
	if a.settingsDraft.AI.Timeout != 3600 {
		t.Errorf("夹紧后的值应写进草稿,实际 %d", a.settingsDraft.AI.Timeout)
	}
}

// 步进器改的是同一个值,缓冲要跟着走,否则保存时写回旧数字。
func TestAIIntFieldStepperWritesBuffer(t *testing.T) {
	a, tt := settingsApp(t)

	r := fieldRect(t, tt, "并发路数")
	clickField(tt, r)
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if got := strings.TrimSpace(a.aiConcurrencyText); got != "5" {
		t.Errorf("上箭头后缓冲应为 5,实际 %q", got)
	}
	a.commitAIConcurrency()
	if a.settingsDraft.AI.Concurrency != 5 {
		t.Errorf("步进后的值应写进草稿,实际 %d", a.settingsDraft.AI.Concurrency)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
