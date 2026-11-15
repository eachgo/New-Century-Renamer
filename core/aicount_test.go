package core

import "testing"

// 没有 AI 步骤时一次请求都不该发。
func TestAICallCountWithoutAIStep(t *testing.T) {
	rules, err := ParseRules([]string{"case=lower"})
	if err != nil {
		t.Fatal(err)
	}
	if n := AICallCount(rules, []string{"a.mp3", "b.mp3"}); n != 0 {
		t.Fatalf("没有 AI 步骤时应为 0,实际 %d", n)
	}
}

// 每个文件一次 AI 请求。
func TestAICallCountPerFile(t *testing.T) {
	rules, err := ParseRules([]string{"template=<AI:起名>"})
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"a.mp3", "b.mp3", "c.mp3"}
	if n := AICallCount(rules, files); n != len(files) {
		t.Fatalf("%d 个文件应估 %d 次请求,实际 %d", len(files), len(files), n)
	}
}

// 同一个文件出现在列表里两次只算一次——和真正执行时的去重一致,
// 否则对话框报的数字比实际多,用户会以为程序在重复花钱。
func TestAICallCountDeduplicatesSameFile(t *testing.T) {
	rules, err := ParseRules([]string{"template=<AI:起名>"})
	if err != nil {
		t.Fatal(err)
	}
	if n := AICallCount(rules, []string{"a.mp3", "a.mp3"}); n != 1 {
		t.Fatalf("同一个文件应只算一次,实际 %d", n)
	}
}

// 估算值必须和真正跑起来时的 total 一致。
func TestAICallCountMatchesPrefetchTotal(t *testing.T) {
	rules, err := ParseRules([]string{"template=<AI:起名>"})
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"a.mp3", "b.flac", "a.mp3"}
	want := AICallCount(rules, files)

	var got int
	// prefetchAIProgress 会真的发请求;这里只看 total,不看结果,
	// 所以传一个必然失败的配置让它立刻返回。total 在发请求之前
	// 就已经算好并通过 progress 报出来了。
	SetAIConfig("http://127.0.0.1:1/v1", "x", "m")
	t.Cleanup(func() { SetAIConfig("", "", "") })
	prefetchAIProgress(rules, files, func(done, total int) { got = total })
	if got != want {
		t.Fatalf("估算 %d 与实际 total %d 不一致", want, got)
	}
}

// filter 不参与 AI 计数:analyze 只看规则里有没有 <AI:>,不看文件名。
// 真正跑的时候也一样(被 filter 排除的文件仍会被预取),所以估算与实际
// 仍然一致——这条断言的是"两边同源",不是"估算更聪明"。
func TestAICallCountIgnoresFilterLikeTheRealRun(t *testing.T) {
	rules, err := ParseRules([]string{"filter=*.mp3", "template=<AI:起名>"})
	if err != nil {
		t.Fatal(err)
	}
	files := []string{"a.txt", "b.txt"}
	want := AICallCount(rules, files)

	var got int
	SetAIConfig("http://127.0.0.1:1/v1", "x", "m")
	t.Cleanup(func() { SetAIConfig("", "", "") })
	prefetchAIProgress(rules, files, func(done, total int) { got = total })
	if got != want {
		t.Fatalf("filter 存在时估算 %d 与实际 %d 不一致", want, got)
	}
}
