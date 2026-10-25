package core

import (
	"fmt"
	"hash/crc32"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

// timeFormat renders xTools' file/current time metavariables.
func timeFormat(key string, t time.Time) string {
	switch key {
	case "DATE":
		return t.Format("2006-01-02")
	case "TIME":
		return t.Format("15-04-05")
	case "Y":
		return t.Format("2006")
	case "M":
		return t.Format("01")
	case "D":
		return t.Format("02")
	case "H":
		return t.Format("15")
	case "TM":
		return t.Format("04")
	case "S":
		return t.Format("05")
	}
	return ""
}

// randToken returns n random characters; mode: 0 digits, 1 letters, 2 both
// (xTools' <R> variable, default n8t2).
func randToken(n, mode int) string {
	const digits = "0123456789"
	const letters = "abcdefghijklmnopqrstuvwxyz"
	src := digits
	if mode == 1 {
		src = letters
	} else if mode == 2 {
		src = digits + letters
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = src[rand.Intn(len(src))]
	}
	return string(b)
}

// ExpandVars resolves <VAR> placeholders in a template, xTools-style.
// Built-ins first, then <META:...>, then <AI:...>, then <Pinyin:...>.
func ExpandVars(tpl string, ctx *Ctx, dir, name string) string {
	var re = regexp.MustCompile(`<([^<>]+)>`)
	return re.ReplaceAllStringFunc(tpl, func(m string) string {
		key := m[1 : len(m)-1]
		stem, ext := splitExt(name)
		switch {
		case key == "NAME":
			return stem
		case key == "EXT":
			return strings.TrimPrefix(ext, ".")
		case key == ".EXT":
			return ext
		case key == "DIR" || key == "PARENT":
			return filepath.Base(dir)
		case key == "SELF" || key == "NAME":
			return stem
		case key == "SIZE":
			if fi, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return fmt.Sprintf("%d", fi.Size())
			}
			return m
		case key == "R":
			return randToken(8, 2)
		case key == "DATE", key == "TIME", key == "Y", key == "M", key == "D",
			key == "H", key == "TM", key == "S":
			if fi, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return timeFormat(key, fi.ModTime())
			}
			return m
		case key == "CDATE", key == "CTIME", key == "CY", key == "CM", key == "CD",
			key == "CH", key == "CTM", key == "CS":
			return timeFormat(strings.TrimPrefix(key, "C"), time.Now())
		case key == "SEQ":
			return fmt.Sprintf("%d", ctx.Seq)
		case key == "CRC32":
			p := filepath.Join(dir, name)
			b, err := os.ReadFile(p)
			if err != nil {
				return m
			}
			return fmt.Sprintf("%08X", crc32.ChecksumIEEE(b))
		case strings.HasPrefix(key, "META:"):
			k := strings.TrimPrefix(key, "META:")
			if ctx.Meta != nil {
				if v, ok := ctx.Meta[k]; ok {
					return v
				}
			}
			// 未命中回退空串:保留 <META:…> 字面量会让无元数据的
			// 文件全部变成同一个名字,批量互相冲突
			return ""
		case strings.HasPrefix(key, "Pinyin:"):
			inner := strings.TrimPrefix(key, "Pinyin:")
			if inner == "NAME" {
				return ToPinyin(stem)
			}
			return ToPinyin(inner)
		case strings.HasPrefix(key, "AI:"):
			if ctx.Vars != nil {
				if v, ok := ctx.Vars(filepath.Join(dir, name) + "\x00" + key); ok {
					return v
				}
			}
			return m
		}
		return m
	})
}

var pinyinArgs = pinyin.NewArgs()

// ToPinyin converts Han characters to pinyin (first reading, joined with
// nothing), normalizes full-width ASCII to half-width — echoing xTools'
// care for names that must survive non-Chinese devices.
func ToPinyin(s string) string {
	var b strings.Builder
	for _, r := range s {
		r = toHalfWidth(r)
		if unicode.Is(unicode.Han, r) {
			py := pinyin.Pinyin(string(r), pinyinArgs)
			if len(py) > 0 && len(py[0]) > 0 {
				b.WriteString(py[0][0])
				continue
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func toHalfWidth(r rune) rune {
	switch {
	case r == 0x3000:
		return ' '
	case r >= 0xFF01 && r <= 0xFF5E:
		return r - 0xFEE0
	}
	return r
}
