package core

import (
	"strings"
	"testing"
)

// 空查找串不是"没填",它是一条会执行成功的规则:ReplaceAll 会把替换
// 文本插到每个字符之间,正则在每个位置匹配一次。预览会显示每个文件名
// 被切碎,执行出来就是一堆谁也不认识的名字。
func TestEmptyFindIsRejected(t *testing.T) {
	if _, err := ParseRules([]string{"replace=,X"}); err == nil {
		t.Fatal("空查找串被接受了")
	} else if !strings.Contains(err.Error(), "查找内容不能为空") {
		t.Fatalf("错误信息没说清原因: %v", err)
	}
	if _, err := ParseRules([]string{"replace=,X,regex"}); err == nil {
		t.Fatal("空查找串(正则)被接受了")
	}
}

// 第二道闸:规则可能来自导入的 JSON,也可能由调用方直接构造,
// 不经过 newReplaceRule。这时空查找串必须是"什么都不做"。
func TestEmptyFindRuleIsNoOp(t *testing.T) {
	r := &replaceRule{old: "", new: "X"}
	got, err := r.Apply(&Ctx{}, "", "abc.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc.mp3" {
		t.Fatalf("空查找串把名字改成了 %q,应该是原样返回", got)
	}
}
