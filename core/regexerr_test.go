package core

import (
	"errors"
	"strings"
	"testing"
)

var errNotSyntax = errors.New("some other failure")

// 正则写错时给的是中文提示,不是 Go 的英文原串。
//
// 正则已经是这个软件里门槛最高的一项,报错再要求读英文,等于把用户最后
// 一点自救的可能也拿掉。
func TestRegexErrorsSpeakChinese(t *testing.T) {
	cases := []struct{ pat, want string }{
		{`[a-z`, "缺少右方括号"},
		{`(abc`, "缺少右圆括号"},
		{`abc)`, "多了一个右圆括号"},
		{`*abc`, "前面没有东西可以重复"},
		{`a**`, "重复符号叠在一起"},
		{`\q`, "反斜杠后面的字符不认识"},
		{`a{1001}`, "重复次数不合法"},
	}
	for _, c := range cases {
		_, err := ParseRules([]string{"replace=" + c.pat + ",x,regex"})
		if err == nil {
			t.Fatalf("%q 是坏正则,应当报错", c.pat)
		}
		msg := err.Error()
		if !strings.Contains(msg, c.want) {
			t.Errorf("%q 的报错里应说「%s」,实际 %q", c.pat, c.want, msg)
		}
		// 英文原串不能漏出来:那是"没翻译"的证据。
		if strings.Contains(msg, "error parsing regexp") {
			t.Errorf("%q 的报错还是英文原串: %q", c.pat, msg)
		}
	}
}

// 带逗号的正则:界面会用 escArg 把逗号转义成 `\,`,splitArgs 再还原,
// 所以 `a{2,1}` 到 regexp.Compile 那里仍是原样,报的也还是重复次数。
func TestRegexWithCommaStillReportsInChinese(t *testing.T) {
	_, err := ParseRules([]string{`replace=a{2\,1},x,regex`})
	if err == nil {
		t.Fatal("a{2,1} 是坏正则,应当报错")
	}
	if !strings.Contains(err.Error(), "重复次数不合法") {
		t.Fatalf("应报重复次数不合法,实际 %q", err.Error())
	}
}

// 出错的那一小段要指出来:只说"缺右方括号"而不说是哪一处,还得自己找。
func TestRegexErrorPointsAtTheSpot(t *testing.T) {
	_, err := ParseRules([]string{`replace=[a-z,x,regex`})
	if err == nil {
		t.Fatal("坏正则应当报错")
	}
	if !strings.Contains(err.Error(), "[a-z") {
		t.Fatalf("报错应指出出错的片段「[a-z」,实际 %q", err.Error())
	}
}

// 认不出来的错误原样返回:宁可露出英文,也不能因为翻译吞掉真正的信息。
func TestUnknownRegexErrorPassesThrough(t *testing.T) {
	msg := regexErrText(errNotSyntax)
	if msg != errNotSyntax.Error() {
		t.Fatalf("非正则错误应原样返回,实际 %q", msg)
	}
}
