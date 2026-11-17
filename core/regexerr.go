package core

import (
	"errors"
	"fmt"
	"regexp/syntax"
)

// regexErrText 把 Go 的正则解析错误翻成一句能照着改的中文。
//
// 原来正则写错时,界面直接吐 Go 的英文原串:
//
//	error parsing regexp: missing closing ]: `[a-z`
//
// 对一个想改文件名的人来说,这句话既没说"哪个字错了",也没说"该补什么"。
// 正则本身已经是这个软件里门槛最高的一项,报错再要求读英文,等于把最后
// 一点自救的可能也拿掉。
//
// 翻不出来的错误原样返回:宁可露出英文,也不能因为翻译吞掉真正的信息。
func regexErrText(err error) string {
	var se *syntax.Error
	if !errors.As(err, &se) {
		return err.Error()
	}
	h, ok := regexErrHints[se.Code]
	if !ok {
		return err.Error()
	}
	hint := h.text
	if len(h.syntax) > 0 {
		args := make([]any, len(h.syntax))
		for i, s := range h.syntax {
			args[i] = s
		}
		hint = fmt.Sprintf(hint, args...)
	}
	// 指出出错的那一小段。Go 给的 Expr 就是原文里最相关的那一截
	// (缺右括号时是整个 `(abc`,重复符号出错时只是那个 `**`)。
	if se.Expr != "" {
		return hint + "（出在「" + se.Expr + "」）"
	}
	return hint
}

// regexHint 是一条中文说法,外加要填进去的语法片段。
//
// 语法片段单独存,不写进中文句子里:那些半角符号是"用户要照着打的东西",
// 不是标点。混在句子里既会被标点规范误判,也读不出"这一段是字面量"。
type regexHint struct {
	text   string
	syntax []string
}

// regexErrHints 是 syntax.ErrorCode 到中文说法的对照表。
//
// 每条都写成"缺什么/多什么/哪里不对 + 该怎么做",而不是复述 Go 的措辞:
// 「missing closing ]」翻成「缺少右方括号」只是换了个语言,用户仍然不知道
// 要去哪一行补。
var regexErrHints = map[syntax.ErrorCode]regexHint{
	syntax.ErrMissingBracket: {
		text:   "缺少右方括号%s，检查一下方括号有没有配对",
		syntax: []string{"]"},
	},
	syntax.ErrMissingParen: {
		text:   "缺少右圆括号%s，检查一下圆括号有没有配对",
		syntax: []string{")"},
	},
	syntax.ErrUnexpectedParen: {
		text:   "多了一个右圆括号%s，它前面没有对应的左括号",
		syntax: []string{")"},
	},
	syntax.ErrMissingRepeatArgument: {
		text: "重复符号（* + ?）前面没有东西可以重复",
	},
	syntax.ErrInvalidRepeatSize: {
		text:   "重复次数不合法：写成 %s 这种形式，前面不能大于后面，也不能超过 1000",
		syntax: []string{"{2,5}"},
	},
	syntax.ErrInvalidRepeatOp: {
		text: "重复符号叠在一起了（如 ** 或 +*），只能用一个",
	},
	syntax.ErrTrailingBackslash: {
		text: "结尾多了一个反斜杠 \\，它后面没有字符了",
	},
	syntax.ErrInvalidEscape: {
		text:   "反斜杠后面的字符不认识：要匹配 %s 这些符号本身时，写 \\ 加那个符号",
		syntax: []string{". ( ) [ ] { }"},
	},
	syntax.ErrInvalidCharRange: {
		text: "字符范围不合法：方括号里的 a-z 要小的在前、大的在后",
	},
	syntax.ErrInvalidCharClass: {
		text:   "方括号里的字符类写法不认识（形如 %s 的写法只有固定的几种）",
		syntax: []string{"[:digit:]"},
	},
	syntax.ErrInvalidNamedCapture: {
		text: "命名分组的名字不合法：只能用字母、数字和下划线，且不能以数字开头",
	},
	syntax.ErrInvalidPerlOp: {
		text:   "用了 Go 不支持的正则语法（如 %s 前瞻、%s 后顾），这些在这里用不了",
		syntax: []string{"(?=", "(?<="},
	},
	syntax.ErrNestingDepth: {
		text: "表达式嵌套太深了，拆成几步试试",
	},
	syntax.ErrLarge: {
		text: "表达式太长了，拆成几步试试",
	},
	syntax.ErrInternalError: {
		text: "正则引擎内部出错，换个写法试试",
	},
}
