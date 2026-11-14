package core

import (
	"fmt"
	"strings"
)

// windowsReserved are the DOS device names that cannot be used as a file
// name on Windows, with or without an extension: "CON.txt" is just as
// reserved as "CON".
var windowsReserved = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// windowsIllegal are the characters Windows refuses in a file name, minus
// the two path separators (checked first, with their own message because
// they mean "move the file", not "bad character"). They are rejected on
// every platform here: a batch that previews fine on Linux and then
// produces unusable names when the folder is copied to Windows is worse
// than one that says up front it cannot do it.
const windowsIllegal = `<>:"|?*`

// CheckName reports why name cannot be used as a file name, or "" when it
// can. It is the single source of truth for that question: preflight calls
// it to mark rows in the preview, and the apply loop calls it again before
// touching the disk, so a plan built by hand (or by a future caller that
// skips preflight) cannot slip an invalid name through.
//
// The rules are Windows', not the host's, because they are a superset: a
// name that passes here is valid on Linux and macOS too.
func CheckName(name string) string {
	if name == "" {
		return "新名字是空的"
	}
	if strings.ContainsAny(name, `/\`) {
		return "新名字里有路径分隔符，会把文件移动到别的目录；更名器只改名不移动文件"
	}
	for _, r := range name {
		// 只报出问题的那个字符:整串里往往只有一处,让用户自己去数
		// 不如直接点名。
		if strings.ContainsRune(windowsIllegal, r) {
			return fmt.Sprintf("新名字里有 Windows 不允许的字符 %q", string(r))
		}
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Sprintf("新名字里有控制字符 U+%04X，资源管理器里会显示成乱码", r)
		}
	}
	// 结尾的空格和点在 Windows 上会被静默吃掉:"改名"看起来成功了,
	// 文件名却和预览里写的不一样,而回滚日志记的是带空格的那个名字。
	if strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".") {
		return "新名字不能以空格或点结尾（Windows 会把它悄悄去掉，改名结果和预览不一致）"
	}
	// 设备名按"去掉扩展名之后的 stem"判:"CON.txt" 同样不可用。
	stem := name
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	if windowsReserved[strings.ToLower(strings.TrimSpace(stem))] {
		return fmt.Sprintf("%q 是 Windows 保留的设备名，不能用作文件名", stem)
	}
	return ""
}
