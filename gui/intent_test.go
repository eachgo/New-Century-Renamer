package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// 空管道时中栏给的是"想做什么",不是"从左侧功能里点一个"——后者把
// 32 项规则摆在用户面前,却没说清点哪个才能得到想要的结果。
func TestEmptyStepsShowIntentCards(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 620)
	tt.Frame()
	tt.Frame()

	for _, in := range intents {
		if !tt.HasText(in.label) {
			t.Fatalf("空步骤时看不到意图卡片 %q", in.label)
		}
	}
}

// 每张卡片点下去都要真的生成一条能跑的管道——文案说得漂亮、生成的
// spec 却解析不了,是最糟的情况。
//
// 用真素材跑:meta.title / meta.author 这两张卡片的意义就是"读文件
// 自带的元数据",拿一个空文件去测等于什么都没测。素材不在就跳过
// (CI 上没有 testfile/),与 E2E 的取舍一致。
func TestIntentBuildsRunnablePipeline(t *testing.T) {
	dir, ok := findTestfile()
	if !ok {
		t.Skip("no testfile dir")
	}
	for _, in := range intents {
		// 每张卡片用适合它的输入:括号那张需要一个真带括号的名字,
		// 否则"没变化"是正确行为,测不出东西。拷到临时目录,避免
		// 剥完括号后正好撞上 testfile 里已有的 01. THE UNION.mp3。
		file := filepath.Join(dir, "01. THE UNION.mp3")
		if in.label == "去掉括号和特殊字符" {
			tmp := t.TempDir()
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			file = filepath.Join(tmp, "01. THE UNION (Live)【FLAC】.mp3")
			if err := os.WriteFile(file, src, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		files := []string{file}
		a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
		tt := ui.NewTester(a.view, 1100, 620)
		tt.Frame()
		tt.Frame()

		if err := tt.Click(in.label); err != nil {
			t.Fatalf("%s: %v", in.label, err)
		}
		tt.Frame()
		if len(a.steps) == 0 {
			t.Fatalf("%s 点下去没有生成步骤", in.label)
		}

		rules, err := core.ParseRules(a.stepSpecs())
		if err != nil {
			t.Fatalf("%s 生成的规则解析失败: %v (specs=%v)", in.label, err, a.stepSpecs())
		}
		ops, _ := core.RunPipeline(rules, files)
		if len(ops) != 1 {
			t.Fatalf("%s: 期望 1 个结果,得到 %d", in.label, len(ops))
		}
		if ops[0].Err != nil {
			t.Fatalf("%s 跑出来报错: %v", in.label, ops[0].Err)
		}
		if ops[0].NewName == "" || ops[0].NewName == ops[0].OldName {
			t.Fatalf("%s 没有改变文件名: %q", in.label, ops[0].NewName)
		}
	}
}

// 卡片生成的步骤必须能继续编辑:它是起点,不是模式。
func TestIntentStepsAreEditable(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 620)
	tt.Frame()
	tt.Frame()

	if err := tt.Click("用文件里的标题改名"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.steps) != 1 {
		t.Fatalf("期望 1 步,得到 %d", len(a.steps))
	}
	// 模板步骤的详情表单里就是那个模板,改它等于改结果
	if a.steps[0].tpl != "<META:title><.EXT>" {
		t.Fatalf("模板不是详情表单编辑的那个字段: %q", a.steps[0].tpl)
	}
	// 加完卡片就消失,换成正常的步骤列表
	if tt.HasText("想改成什么样？点一张，我把步骤配好") {
		t.Fatal("已经有步骤了,意图卡片应当让位给步骤列表")
	}
}

// 每张卡片生成的步骤都要有唯一 opUID,否则列表会 panic
// (mygo: two elements under one parent have the key …)。
func TestIntentStepsHaveUniqueIDs(t *testing.T) {
	a := &app{stepSel: -1, fileSel: -1, settings: defaultSettings()}
	tt := ui.NewTester(a.view, 1100, 620)
	tt.Frame()
	tt.Frame()

	// 连点两张卡片。每点一张卡片就消失(管道不再为空),所以点完第一张
	// 要把步骤清掉,才能再看到第二张。
	for _, label := range []string{"改成 作者 - 标题", "去掉括号和特殊字符"} {
		if err := tt.Click(label); err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		tt.Frame()
		if label == "改成 作者 - 标题" {
			a.steps, a.stepSel = nil, -1
			tt.Frame()
			tt.Frame()
		}
	}
	seen := map[int]bool{}
	for _, s := range a.steps {
		if seen[s.opUID] {
			t.Fatalf("opUID 重复: %d", s.opUID)
		}
		seen[s.opUID] = true
	}
	if len(a.steps) != 2 {
		t.Fatalf("“去掉括号和特殊字符”应当生成 2 步（删括号 + 转全角）,得到 %d", len(a.steps))
	}
}
