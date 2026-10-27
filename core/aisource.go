package core

// 内容采样:让 AI 命名真正"看过"文件,而不是只看文件名。
//
// 这里刻意不引 OCR、不引解压库:只取本来就明文躺在那儿的东西——
// PDF 的文本层、EPUB 的 XHTML、压缩包的 Central Directory、JPEG 的 EXIF。
// 7B~9B 的本地模型读这些已经够用,而我们也不用为它们加依赖。

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	maxTextBytes = 8000      // 每个文件最多喂给模型这么多字符
	maxPdfUnzip  = 256 << 10 // 单个流解压上限,防止 zlib 炸弹
	zipListMax   = 60        // 压缩包最多列这么多条目
	zipListName  = 80        // 单个条目名截断长度
)

// 匹配 `N 0 obj << 字典 >> stream ... endstream`。
// 字典里可能出现嵌套的 `>>`(比如 /Matte [0 0 0] 之后的结构),
// 所以用贪婪匹配到 stream 之前的最后一个 `>>`,再靠 "endstream" 定界。
var pdfStreamRe = regexp.MustCompile(`(?s)(<<.*?>>)\s*stream\r?\n(.*?)endstream`)

// 顶层对象头 `N 0 obj` / `N N obj`
var pdfObjRe = regexp.MustCompile(`(\d+)\s+\d+\s+obj\b`)

// 资源字典里的字体项 `/FT8 8 0 R`
var pdfFontRefRe = regexp.MustCompile(`/([A-Za-z0-9_.+-]+)\s+(\d+)\s+\d+\s+R`)

// 任意间接引用 `N 0 R`
var pdfRefRe = regexp.MustCompile(`(\d+)\s+\d+\s+R`)

// CMap 的 bfchar 项:`<0003> <0020>`
var pdfHexPairRe = regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)

// CMap 的 bfrange 项:`<0003> <0005> <0020>`
var pdfHexTripleRe = regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)

func readFileLimit(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

func openFile(path string) (*os.File, error) { return os.Open(path) }

// ---- PDF:解出文本层 ----

// pdfTextLayer 找出内容流,解 FlateDecode,从 Tj/TJ 操作符里抠出文字。
// 排版信息全丢,只留可读的字符——这正是起标题需要的。
func pdfTextLayerLegacy(path string) string {
	b, err := readFileLimit(path, 24<<20)
	if err != nil {
		return ""
	}
	var out strings.Builder
	for _, m := range pdfStreamRe.FindAllSubmatchIndex(b, -1) {
		if out.Len() >= maxTextBytes {
			break
		}
		// group1 = 字典(含外层尖括号), group2 = 流数据
		dict := b[m[2]:m[3]]
		payload := b[m[4]:m[5]]

		// 跳过图片和字体程序:里面没有可读文字,而且很占地方。
		if bytes.Contains(dict, []byte("/Image")) || bytes.Contains(dict, []byte("/FontFile")) ||
			bytes.Contains(dict, []byte("/XObject")) || bytes.Contains(dict, []byte("/ObjStm")) {
			continue
		}
		data := payload
		if bytes.Contains(dict, []byte("/FlateDecode")) {
			if d, ok := inflate(payload, maxPdfUnzip); ok {
				data = d
			} else {
				continue
			}
		}
		// 只处理看起来像内容流的东西:含文本绘制操作符。
		if !bytes.Contains(data, []byte("Tj")) && !bytes.Contains(data, []byte("TJ")) &&
			!bytes.Contains(data, []byte("Td")) {
			continue
		}
		s := pdfStrings(data, nil)
		if strings.TrimSpace(s) != "" {
			out.WriteString(s)
			out.WriteByte('\n')
		}
	}
	return cleanSnippet(out.String())
}

func inflate(b []byte, limit int) ([]byte, bool) {
	zr, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, false
	}
	defer zr.Close()
	d, err := io.ReadAll(io.LimitReader(zr, int64(limit)))
	if err != nil && len(d) == 0 {
		return nil, false
	}
	return d, true
}

// pdfTextLayer 找出内容流,解 FlateDecode,从 Tj/TJ 操作符里抠出文字。
// 中文 PDF 多半是 Type0 字体 + Identity-H 编码,文字是 <03ED> 这种 CID 码,
// 必须先按字体的 /ToUnicode CMap 还原,否则拿出来全是乱码。
func pdfTextLayer(path string) string {
	b, err := readFileLimit(path, 24<<20)
	if err != nil {
		return ""
	}

	// 先把所有 obj 的位置记下来,后面按对象号取 /ToUnicode。
	objs := scanPDFObjects(b)
	cmapCache := map[string]map[uint32]string{} // ToUnicode 对象号 -> 映射

	var out strings.Builder
	textLeft := func() int { return maxTextBytes - out.Len() }
	for _, m := range pdfStreamRe.FindAllSubmatchIndex(b, -1) {
		if textLeft() <= 0 {
			break
		}
		// group1 = 字典(含外层尖括号), group2 = 流数据
		dict := b[m[2]:m[3]]
		payload := b[m[4]:m[5]]

		// 跳过图片和字体程序:里面没有可读文字,而且很占地方。
		if bytes.Contains(dict, []byte("/Image")) || bytes.Contains(dict, []byte("/FontFile")) ||
			bytes.Contains(dict, []byte("/XObject")) || bytes.Contains(dict, []byte("/ObjStm")) {
			continue
		}
		data := payload
		if bytes.Contains(dict, []byte("/FlateDecode")) {
			if d, ok := inflate(payload, maxPdfUnzip); ok {
				data = d
			} else {
				continue
			}
		}
		// 只处理看起来像内容流的东西:含文本绘制操作符。
		if !bytes.Contains(data, []byte("Tj")) && !bytes.Contains(data, []byte("TJ")) &&
			!bytes.Contains(data, []byte("Td")) {
			continue
		}

		// 本页资源里 /FT8 这类名字到字体的映射;以及页面字典里的 BaseFont 反查。
		fonts := pageFontCMaps(b, objs, dict, cmapCache)
		s := pdfStrings(data, fonts)
		if strings.TrimSpace(s) != "" {
			out.WriteString(s)
			out.WriteByte('\n')
		}
	}
	return cleanSnippet(out.String())
}

// pdfObj 是一个顶层对象在文件里的位置。
type pdfObj struct {
	off int
	end int
}

// scanPDFObjects 扫出 `N 0 obj ... endobj` 的位置。
func scanPDFObjects(b []byte) map[string]pdfObj {
	out := map[string]pdfObj{}
	rest := b
	base := 0
	for {
		loc := pdfObjRe.FindSubmatchIndex(rest)
		if loc == nil {
			break
		}
		num := string(rest[loc[2]:loc[3]])
		start := base + loc[0]
		// endobj 可能在流数据里出现,所以只往后找一段
		seg := b[start:]
		e := -1
		if i := bytes.Index(seg, []byte("endobj")); i >= 0 {
			e = start + i
		} else if start+4096 < len(b) {
			e = start + 4096
		} else {
			e = len(b)
		}
		out[num] = pdfObj{off: start, end: e}
		adv := loc[1]
		if adv <= 0 {
			adv = 1
		}
		rest = rest[adv:]
		base += adv
	}
	return out
}

// pageFontCMaps 取得本内容流可用的 字体资源名 -> CID 映射。
// 资源字典的 /Font << /FT8 8 0 R ... >> 里的名字是内容流里 Tf 操作符用的。
func pageFontCMaps(all []byte, objs map[string]pdfObj, dict []byte, cache map[string]map[uint32]string) map[string]map[uint32]string {
	out := map[string]map[uint32]string{}
	// 页面字典有时不在同一个 stream 的字典里,退一步在整个文件里找唯一的 /Font 资源块。
	fontBlocks := [][]byte{}
	if i := bytes.Index(dict, []byte("/Font")); i >= 0 {
		fontBlocks = append(fontBlocks, dict[i:])
	}
	for _, fb := range pdfAllFontBlocks(all) {
		fontBlocks = append(fontBlocks, fb)
	}
	for _, fb := range fontBlocks {
		for _, m := range pdfFontRefRe.FindAllSubmatch(fb, -1) {
			name := string(m[1])
			if _, ok := out[name]; ok {
				continue
			}
			fontObj := string(m[2])
			tu := pdfFontToUnicode(all, objs, fontObj)
			if tu == "" {
				continue
			}
			cm, ok := cache[tu]
			if !ok {
				cm = parseCMap(all[objs[tu].off:objs[tu].end])
				cache[tu] = cm
			}
			if len(cm) > 0 {
				out[name] = cm
			}
		}
	}
	return out
}

// pdfAllFontBlocks 找出文件里所有 `<< ... /Font << ... >> ... >>` 的资源片段。
func pdfAllFontBlocks(all []byte) [][]byte {
	var out [][]byte
	rest := all
	for {
		i := bytes.Index(rest, []byte("/Font"))
		if i < 0 {
			break
		}
		endp := i + 800
		if endp > len(rest) {
			endp = len(rest)
		}
		out = append(out, rest[i:endp])
		rest = rest[i+5:]
		if len(out) > 40 {
			break
		}
	}
	return out
}

// pdfFontToUnicode 顺着 `字体对象 -> /ToUnicode N 0 R` 拿到 CMap 所在对象号。
func pdfFontToUnicode(all []byte, objs map[string]pdfObj, fontObj string) string {
	o, ok := objs[fontObj]
	if !ok || o.end <= o.off {
		return ""
	}
	seg := all[o.off:o.end]
	i := bytes.Index(seg, []byte("/ToUnicode"))
	if i < 0 {
		return ""
	}
	m := pdfRefRe.FindSubmatch(seg[i:])
	if m == nil {
		return ""
	}
	return string(m[1])
}

// parseCMap 解析 ToUnicode CMap 的 bfchar / bfrange。
// 内容本身可能还是 Flate 压缩的,这里先解开再解析。
func parseCMap(seg []byte) map[uint32]string {
	out := map[uint32]string{}
	if len(seg) == 0 {
		return out
	}
	// 取 stream 到 endstream 之间的数据并解压
	if si := bytes.Index(seg, []byte("stream")); si >= 0 {
		body := seg[si+6:]
		body = bytes.TrimLeft(body, "\r\n")
		if ei := bytes.Index(body, []byte("endstream")); ei >= 0 {
			body = body[:ei]
		}
		if bytes.Contains(seg[:si], []byte("/FlateDecode")) {
			if d, ok := inflate(body, 1<<20); ok {
				seg = d
			} else {
				return out
			}
		} else {
			seg = body
		}
	}
	parseBFChar(seg, out)
	parseBFRange(seg, out)
	return out
}

func parseBFChar(seg []byte, out map[uint32]string) {
	rest := seg
	for {
		i := bytes.Index(rest, []byte("beginbfchar"))
		if i < 0 {
			return
		}
		end := bytes.Index(rest[i:], []byte("endbfchar"))
		if end < 0 {
			return
		}
		block := rest[i+len("beginbfchar") : i+end]
		for _, m := range pdfHexPairRe.FindAllSubmatch(block, -1) {
			src, ok1 := hexToUint(string(m[1]))
			if !ok1 {
				continue
			}
			out[src] = hexToUTF16(string(m[2]))
		}
		rest = rest[i+end+len("endbfchar"):]
	}
}

func parseBFRange(seg []byte, out map[uint32]string) {
	rest := seg
	for {
		i := bytes.Index(rest, []byte("beginbfrange"))
		if i < 0 {
			return
		}
		end := bytes.Index(rest[i:], []byte("endbfrange"))
		if end < 0 {
			return
		}
		block := rest[i+len("beginbfrange") : i+end]
		for _, m := range pdfHexTripleRe.FindAllSubmatch(block, -1) {
			lo, ok1 := hexToUint(string(m[1]))
			hi, ok2 := hexToUint(string(m[2]))
			if !ok1 || !ok2 || hi < lo || hi-lo > 65535 {
				continue
			}
			dst := hexToUTF16(string(m[3]))
			rs := []rune(dst)
			if len(rs) == 0 {
				continue
			}
			base := rs[len(rs)-1]
			for k := uint32(0); k <= hi-lo; k++ {
				out[lo+k] = string(base + rune(k))
			}
		}
		rest = rest[i+end+len("endbfrange"):]
	}
}

func hexToUint(s string) (uint32, bool) {
	var v uint32
	if len(s) == 0 || len(s) > 8 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		var d uint32
		switch {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint32(c-'A') + 10
		default:
			return 0, false
		}
		v = v<<4 | d
	}
	return v, true
}

// hexToUTF16 把 CMap 里的 UTF-16BE 十六进制串转成字符串。
func hexToUTF16(s string) string {
	if len(s)%2 == 1 {
		s = "0" + s
	}
	bs := make([]byte, 0, len(s)/2)
	for i := 0; i+1 < len(s); i += 2 {
		v, ok := hexToUint(s[i : i+2])
		if !ok {
			return ""
		}
		bs = append(bs, byte(v))
	}
	rs := make([]rune, 0, len(bs)/2)
	for i := 0; i+1 < len(bs); i += 2 {
		rs = append(rs, rune(uint16(bs[i])<<8|uint16(bs[i+1])))
	}
	return string(rs)
}

// pdfStrings 从内容流里抽 Tj/TJ 的字符串操作数。
// fonts 是当前字体资源名对应的 CID 映射;没有映射时按字面量尽力而为。
func pdfStrings(d []byte, fonts map[string]map[uint32]string) string {
	var out strings.Builder
	var cur map[uint32]string // 当前 Tf 选中的字体映射
	for i := 0; i < len(d) && out.Len() < maxTextBytes; i++ {
		c := d[i]

		// Tf:记录当前字体,决定后面 <...> 怎么解。
		// 写法是 `/FT8 180 Tf`,名字与 "Tf" 之间还隔着字号,不能直接看前缀。
		if c == '/' {
			j := i + 1
			for j < len(d) && isNameByte(d[j]) {
				j++
			}
			name := string(d[i+1 : j])
			// 往后最多看 32 字节,找 Tf
			look := d[j:]
			if len(look) > 32 {
				look = look[:32]
			}
			if k := bytes.Index(look, []byte("Tf")); k >= 0 {
				// 中间只能是空白和数字(字号),否则说明是别的操作符
				okOp := true
				for _, ch := range look[:k] {
					if ch != ' ' && ch != '\r' && ch != '\n' && ch != '\t' &&
						!(ch >= '0' && ch <= '9') && ch != '.' && ch != '-' && ch != '+' {
						okOp = false
						break
					}
				}
				if okOp {
					cur = fonts[name]
				}
			}
			continue
		}

		// <hex> :Type0/Identity-H 下的 CID 码,按当前字体映射还原
		if c == '<' && i+1 < len(d) && d[i+1] != '<' {
			j := bytes.IndexByte(d[i+1:], '>')
			if j < 0 {
				break
			}
			hexs := string(d[i+1 : i+1+j])
			out.WriteString(decodeCID(hexs, cur))
			i += j + 1
			continue
		}

		// (literal) :老式字体的字符串
		if c != '(' {
			continue
		}
		depth, j := 1, i+1
		var lit []byte
		for j < len(d) && depth > 0 {
			ch := d[j]
			switch ch {
			case '\\':
				if j+1 < len(d) {
					lit = append(lit, d[j], d[j+1])
					j += 2
					continue
				}
				j++
			case '(':
				depth++
				lit = append(lit, ch)
				j++
			case ')':
				depth--
				if depth > 0 {
					lit = append(lit, ch)
				}
				j++
			default:
				lit = append(lit, ch)
				j++
			}
		}
		if s := pdfLiteral(lit); s != "" {
			out.WriteString(s)
			out.WriteByte(' ')
		}
		i = j - 1
	}
	return out.String()
}

// decodeCID 把 <03ED> 这样的字形码按 CMap 还原成文字。
// 没有 CMap 时保持沉默——宁可少给,也不给模型一堆乱码。
func decodeCID(hexs string, cm map[uint32]string) string {
	if cm == nil {
		return ""
	}
	if len(hexs)%2 == 1 {
		hexs = "0" + hexs
	}
	var b strings.Builder
	for k := 0; k+4 <= len(hexs); k += 4 {
		v, ok := hexToUint(hexs[k : k+4])
		if !ok {
			continue
		}
		b.WriteString(cm[v])
	}
	return b.String()
}

func isNameByte(c byte) bool {
	if c <= 0x20 || c >= 0x7f {
		return false
	}
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return false
	}
	return true
}

// pdfLiteral 解码 PDF 字符串字面量的常见转义。
func pdfLiteral(b []byte) string {
	var out []byte
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' || i+1 >= len(b) {
			out = append(out, b[i])
			continue
		}
		i++
		switch b[i] {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case '(', ')', '\\':
			out = append(out, b[i])
		default:
			// 八进制 \ddd
			if b[i] >= '0' && b[i] <= '7' {
				v := int(b[i] - '0')
				for k := 0; k < 2 && i+1 < len(b) && b[i+1] >= '0' && b[i+1] <= '7'; k++ {
					i++
					v = v*8 + int(b[i]-'0')
				}
				out = append(out, byte(v))
			} else {
				out = append(out, b[i])
			}
		}
	}
	s := string(out)
	// 中文 PDF 常用 UTF-16BE;顺手把 BOM 版本转回来。
	if strings.HasPrefix(s, "\xfe\xff") {
		bs := []byte(s[2:])
		rs := make([]rune, 0, len(bs)/2)
		for i := 0; i+1 < len(bs); i += 2 {
			rs = append(rs, rune(uint16(bs[i])<<8|uint16(bs[i+1])))
		}
		s = string(rs)
	}
	return strings.TrimSpace(s)
}

// ---- EPUB:读正文文档 ----

func epubText(path string) string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return ""
	}
	defer zr.Close()

	// 优先读正文 XHTML,跳过样式和导航。
	type cand struct {
		name string
		rc   io.ReadCloser
	}
	var out strings.Builder
	for _, f := range zr.File {
		if out.Len() >= maxTextBytes {
			break
		}
		n := strings.ToLower(f.Name)
		if !strings.HasSuffix(n, ".xhtml") && !strings.HasSuffix(n, ".html") && !strings.HasSuffix(n, ".htm") {
			continue
		}
		if strings.Contains(n, "nav") || strings.Contains(n, "toc") || strings.Contains(n, "cover") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(rc, int64(maxTextBytes)))
		rc.Close()
		out.WriteString(stripTags(string(b)))
		out.WriteByte('\n')
	}
	return cleanSnippet(out.String())
}

// ---- 压缩包:列目录 + 尽力脱敏 ----

func zipListing(path string) string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "(压缩包打不开, 仅凭文件名和大小推断)"
	}
	defer zr.Close()

	var b strings.Builder
	b.WriteString("压缩包内有 ")
	b.WriteString(itoa(len(zr.File)))
	b.WriteString(" 个条目:\n")
	shown := 0
	for _, f := range zr.File {
		if shown >= zipListMax {
			b.WriteString("……\n")
			break
		}
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name)
		if f.NonUTF8 {
			// GBK 名字在 Go 眼里是乱码,标出来,别让模型照着乱码编。
			name = filepath.Base(f.Name) + "(编码非 UTF-8)"
		}
		if len(name) > zipListName {
			name = name[:zipListName] + "…"
		}
		b.WriteString("- ")
		b.WriteString(name)
		b.WriteByte('\n')
		shown++
	}
	// 顺手读一个小文本成员的内容,常常是说明文件或歌词。
	for _, f := range zr.File {
		n := strings.ToLower(f.Name)
		if f.UncompressedSize64 > 4096 {
			continue
		}
		if strings.HasSuffix(n, ".txt") || strings.HasSuffix(n, ".nfo") || strings.HasSuffix(n, ".md") {
			if rc, err := f.Open(); err == nil {
				c, _ := io.ReadAll(io.LimitReader(rc, 1200))
				rc.Close()
				if t := cleanSnippet(string(c)); t != "" {
					b.WriteString("\n附带说明文字:\n")
					b.WriteString(t)
					b.WriteByte('\n')
				}
			}
			break
		}
	}
	return b.String()
}

// ---- 图片:只取 EXIF 里人看得懂的那几项 ----

func exifSummary(path string) string {
	f, err := openFile(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	head := make([]byte, 64<<10)
	n, _ := io.ReadFull(f, head)
	head = head[:n]

	i := bytes.Index(head, []byte("Exif\x00\x00"))
	if i < 0 {
		return "(图片, 无 EXIF)"
	}
	tiff := head[i+6:]
	consts := exifFields(tiff)
	if len(consts) == 0 {
		return "(图片, EXIF 里没有可读字段)"
	}
	var parts []string
	for _, k := range []string{"Model", "Make", "DateTimeOriginal", "DateTime"} {
		if v, ok := consts[k]; ok && v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	if len(parts) == 0 {
		return "(图片, EXIF 里没有拍摄信息)"
	}
	return "EXIF: " + strings.Join(parts, "; ")
}

// exifFields 只解析 IFD0 与 ExifIFD 里的 ASCII 字符串字段,够定位机型和时间。
func exifFields(t []byte) map[string]string {
	out := map[string]string{}
	if len(t) < 8 {
		return out
	}
	var bo binary.ByteOrder = binary.LittleEndian
	if string(t[0:2]) == "MM" {
		bo = binary.BigEndian
	}
	readIFD(t, bo, int(bo.Uint32(t[4:8])), out, 0)
	return out
}

func readIFD(t []byte, bo binary.ByteOrder, off int, out map[string]string, depth int) {
	if depth > 2 || off <= 0 || off+2 > len(t) {
		return
	}
	n := int(bo.Uint16(t[off : off+2]))
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return
		}
		tag := bo.Uint16(t[e : e+2])
		typ := bo.Uint16(t[e+2 : e+4])
		cnt := int(bo.Uint32(t[e+4 : e+8]))
		if typ != 2 || cnt <= 0 || cnt > 256 { // 2 = ASCII
			continue
		}
		var raw []byte
		if cnt <= 4 {
			raw = t[e+8 : e+8+cnt]
		} else {
			voff := int(bo.Uint32(t[e+8 : e+12]))
			if voff <= 0 || voff+cnt > len(t) {
				continue
			}
			raw = t[voff : voff+cnt]
		}
		s := strings.TrimRight(string(bytes.TrimRight(raw, "\x00")), "\x00")
		if s == "" {
			continue
		}
		switch tag {
		case 0x0110:
			out["Model"] = s
		case 0x010f:
			out["Make"] = s
		case 0x9003:
			out["DateTimeOriginal"] = s
		case 0x0132:
			out["DateTime"] = s
		case 0x8769: // ExifIFD 指针,typ 会过滤掉,这里不会走到
		}
	}
	// 追一层 ExifIFD(IFD0 里 tag 0x8769)
	for k := 0; k < n; k++ {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return
		}
		if bo.Uint16(t[e:e+2]) != 0x8769 {
			continue
		}
		if sub := int(bo.Uint32(t[e+8 : e+12])); sub > 0 && sub < len(t) {
			readIFD(t, bo, sub, out, depth+1)
		}
	}
}

// ---- 小工具 ----

// stripTags 把 XHTML 变成纯文本,顺手解开常见实体。
func stripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch r {
		case '<':
			in = true
		case '>':
			in = false
			b.WriteByte(' ')
		default:
			if !in {
				b.WriteRune(r)
			}
		}
	}
	out := b.String()
	out = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(out)
	return out
}

// cleanSnippet 压实空白,去掉控制字符,截断到上限。
func cleanSnippet(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		if r < 0x20 {
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	out := strings.TrimSpace(b.String())
	if len(out) > maxTextBytes {
		out = out[:maxTextBytes]
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
