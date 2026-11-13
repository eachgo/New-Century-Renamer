package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// ---- 类型判定:一张表 ----
//
// 以前这里有三个各说各话的口径:detectType 按扩展名给中文标签,
// sniffMagic 按魔数给大写英文,extCategory 又按扩展名分中文类别
// (而且全仓库没人调用它)。同一个 .m4a 在前两套里分别叫「MP4 音频」
// 和「MP4」,想加一种格式得改三处,漏一处就出现「标签说音频、嗅探说
// 视频」这种自相矛盾的界面。现在只留 formatTable 一张表:键是规范
// 格式名(小写、不带点,和扩展名同形),值是展示标签与类别;扩展名
// 标签、魔数嗅探结果、相符判断全部从它派生。
type formatInfo struct {
	label string // 展示标签,如「MP3 音频」
	cat   string // 类别,如「音频」
}

var formatTable = map[string]formatInfo{
	// 音频
	"mp3":  {"MP3 音频", "音频"},
	"flac": {"FLAC 音频", "音频"},
	"ogg":  {"OGG 音频", "音频"},
	"opus": {"Opus 音频", "音频"},
	"wav":  {"WAV 音频", "音频"},
	"wma":  {"WMA 音频", "音频"},
	"m4a":  {"MP4 音频", "音频"},
	"aac":  {"AAC 音频", "音频"},
	"mid":  {"MIDI 音乐", "音频"},
	"oga":  {"OGG 音频", "音频"},
	"spx":  {"Speex 音频", "音频"},
	"mka":  {"Matroska 音频", "音频"},
	// 视频
	"mp4":  {"MP4 视频", "视频"},
	"m4v":  {"MP4 视频", "视频"},
	"mov":  {"MP4 视频", "视频"},
	"mkv":  {"MKV 视频", "视频"},
	"webm": {"WebM 视频", "视频"},
	"avi":  {"AVI 视频", "视频"},
	"flv":  {"FLV 视频", "视频"},
	"3gp":  {"3GP 视频", "视频"},
	// 图片
	"jpg":  {"JPEG 图片", "图片"},
	"jpeg": {"JPEG 图片", "图片"},
	"png":  {"PNG 图片", "图片"},
	"gif":  {"GIF 图片", "图片"},
	"webp": {"WebP 图片", "图片"},
	"bmp":  {"BMP 图片", "图片"},
	"tiff": {"TIFF 图片", "图片"},
	"svg":  {"SVG 图片", "图片"},
	"ico":  {"ICO 图标", "图片"},
	// 文本
	"txt":  {"文本", "文本"},
	"md":   {"Markdown", "文本"},
	"log":  {"文本", "文本"},
	"csv":  {"CSV 表格", "文本"},
	"json": {"JSON", "文本"},
	"xml":  {"XML", "文本"},
	"html": {"HTML", "文本"},
	"htm":  {"HTML", "文本"},
	"srt":  {"字幕", "文本"},
	"ass":  {"字幕", "文本"},
	// 文档
	"pdf":  {"PDF 文档", "文档"},
	"doc":  {"Word 文档", "文档"},
	"docx": {"Word 文档", "文档"},
	"xls":  {"Excel 表格", "文档"},
	"xlsx": {"Excel 表格", "文档"},
	"ppt":  {"PowerPoint", "文档"},
	"pptx": {"PowerPoint", "文档"},
	"ole":  {"OLE 复合文档", "文档"},
	// 电子书
	"epub": {"EPUB 电子书", "电子书"},
	"mobi": {"MOBI 电子书", "电子书"},
	"azw3": {"AZW3 电子书", "电子书"},
	"m4b":  {"MP4 有声书", "电子书"},
	// 压缩包
	"zip": {"压缩包", "压缩包"},
	"rar": {"RAR 压缩包", "压缩包"},
	"7z":  {"7z 压缩包", "压缩包"},
	"tar": {"tar 归档", "压缩包"},
	"gz":  {"gzip 压缩", "压缩包"},
	"apk": {"Android 应用", "压缩包"},
	"jar": {"Java 归档", "压缩包"},
	"cbz": {"漫画归档", "压缩包"},
	"odt": {"ODT 文档", "文档"},
	"ods": {"ODS 表格", "文档"},
	"odp": {"ODP 演示", "文档"},
}

// formatLabel 是唯一的中文标签出口:表里有就查表,没有就退回大写扩展名
// (未知格式至少还能看出原来是什么后缀)。
func formatLabel(key string) string {
	k := strings.ToLower(strings.TrimPrefix(key, "."))
	if k == "" {
		return ""
	}
	if fi, ok := formatTable[k]; ok {
		return fi.label
	}
	return strings.ToUpper(k)
}

// formatCategory 给类别;未知格式返回空串,调用方自己决定怎么兜底。
func formatCategory(key string) string {
	if fi, ok := formatTable[strings.ToLower(strings.TrimPrefix(key, "."))]; ok {
		return fi.cat
	}
	return ""
}

// containerExts 记录"这个扩展名本来就建立在某个容器之上"。
// sniffMagic 只能看到容器层,EPUB/DOCX/XLSX/APK 全都是 ZIP,.m4a/.mov 全都
// 是 MP4——按字符串判等会把整屏正常文件标成红色错误,假警报一多,
// 真正"扩展名在撒谎"的那一个就被淹掉了。
var containerExts = map[string][]string{
	// ZIP 家族
	"epub": {"zip"}, "docx": {"zip"}, "xlsx": {"zip"}, "pptx": {"zip"},
	"odt": {"zip"}, "ods": {"zip"}, "odp": {"zip"}, "apk": {"zip"},
	"jar": {"zip"}, "cbz": {"zip"},
	// MP4 家族
	"m4a": {"mp4"}, "m4b": {"mp4"}, "m4v": {"mp4"}, "mov": {"mp4"},
	"mp4": {"mp4"}, "3gp": {"mp4"},
	// Matroska 家族
	"mkv": {"mkv"}, "webm": {"mkv"}, "mka": {"mkv"},
	// Ogg 家族
	"ogg": {"ogg"}, "oga": {"ogg"}, "opus": {"ogg"}, "spx": {"ogg"},
	// 图片别名
	"jpg": {"jpeg"}, "jpeg": {"jpeg"},
	// 电子书别名
	"azw3": {"mobi"},
	// OLE 复合文档:老式 .doc/.xls/.ppt 都是它
	"doc": {"ole"}, "xls": {"ole"}, "ppt": {"ole"},
	// WMA 与 ASF 同容器
	"wma": {"asf"},
}

// containerLabels 给"内容其实是某个容器"这种情况一个不会误导的名字。
// 一个扩展名写成 .mp3、内容却是 m4a 的文件,说"内容为 MP4 视频"会让人
// 以为里面是视频;说"MP4 容器"才对——它确实只是个 MP4 盒子。
var containerLabels = map[string]string{
	"mp4":  "MP4 容器",
	"mkv":  "Matroska 容器",
	"ogg":  "Ogg 容器",
	"zip":  "ZIP 容器",
	"asf":  "ASF 容器",
	"ole":  "OLE 复合文档",
	"mobi": "MOBI 容器",
}

// contentLabel 是"内容是什么"的展示名:容器优先用容器名,其余用格式标签。
func contentLabel(real string) string {
	if s, ok := containerLabels[real]; ok {
		return s
	}
	if s := formatLabel(real); s != "" {
		return s
	}
	return strings.ToUpper(real)
}

// contentVerdict 是"扩展名与内容是否说得通"的结论。
type contentVerdict int

const (
	// contentNoOpinion 表示无从判断:没有扩展名、文件太小(空文件、
	// 占位文件),或者这个格式本来就没有魔数(纯文本类)。无从判断不
	// 等于"没问题",只是没资格报警。
	contentNoOpinion contentVerdict = iota
	contentOK
	contentMismatch
	// contentUnrecognized 表示扩展名声称自己是二进制格式,但内容里
	// 读不出任何已知魔数——以前这种文件被静默放过,于是一个改坏的
	// .docx(不是 ZIP)在界面上看起来完全正常。
	contentUnrecognized
)

// magiclessExts 是"内容本来就没有固定魔数"的格式:纯文本类,以及可以
// 用文本形式写出来的 SVG。对它们嗅探返回空是正常结果,不该报未识别。
var magiclessExts = map[string]bool{
	"txt": true, "md": true, "log": true, "csv": true, "json": true,
	"xml": true, "html": true, "htm": true, "srt": true, "ass": true,
	"svg": true,
}

// contentCheck 判断嗅探到的内容类型与扩展名是否一致。
// size 是文件字节数:小于 8 字节的文件没有嗅探价值,一律算无从判断。
func contentCheck(ext, real string, size int64) contentVerdict {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	real = strings.ToLower(strings.TrimPrefix(real, "."))
	if ext == "" || size < 8 {
		return contentNoOpinion
	}
	if real != "" {
		if ext == real {
			return contentOK
		}
		for _, ok := range containerExts[ext] {
			if ok == real {
				return contentOK
			}
		}
		// 文本格式之间的"嗅探结果"不构成矛盾:html、md、xml 里可以合法地
		// 内联一段 <svg>,反过来 .svg 也常常就是被 XML 声明包着。这类格式
		// 本来就没有魔数,嗅探器能认出来的只是"内容里出现了什么",不是
		// "这个文件是什么"。拿它去报警,就成了二阶假警报。
		if magiclessExts[ext] && magiclessExts[real] {
			return contentNoOpinion
		}
		return contentMismatch
	}
	if magiclessExts[ext] {
		return contentNoOpinion
	}
	if _, known := formatTable[ext]; known {
		return contentUnrecognized
	}
	return contentNoOpinion
}

// sniffMagic identifies the real content type from the file's first bytes,
// so a renamed-to-lie extension shows ("photo.mp3" that is really WebP).
// 返回值是 formatTable 的规范键(小写、不带点),容器类统一返回容器名。
//
// 以前这里读 16 字节、判 b[0:4]=="ftyp",而 ISO-BMFF 的 ftyp box 前面
// 还有 4 字节 box size,所以真实的 mp4/m4a/mov 一个都嗅探不到,
// containerExts 里六条 MP4 家族白名单在运行时形同虚设。
func sniffMagic(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	// 512 字节:tar 的 "ustar" 在偏移 257,MOBI 的 "BOOKMOBI" 在偏移 60。
	b := make([]byte, 512)
	n, _ := f.Read(b)
	b = b[:n]
	switch {
	case len(b) >= 8 && string(b[4:8]) == "ftyp":
		return "mp4" // ISO-BMFF:mp4/m4a/m4b/m4v/mov/3gp 共用一个容器
	case len(b) >= 3 && b[0] == 'I' && b[1] == 'D' && b[2] == '3':
		return "mp3" // ID3v2 标签开头
	case len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0 && (b[1]&0x06) != 0:
		return "mp3" // MPEG 帧同步
	case len(b) >= 2 && b[0] == 0xFF && (b[1] == 0xF1 || b[1] == 0xF9):
		return "aac" // ADTS
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "webp"
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		return "wav"
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "AVI ":
		return "avi"
	case len(b) >= 4 && string(b[0:4]) == "%PDF":
		return "pdf"
	case len(b) >= 4 && string(b[0:4]) == "OggS":
		return "ogg"
	case len(b) >= 4 && string(b[0:4]) == "fLaC":
		return "flac"
	case len(b) >= 8 && string(b[0:8]) == "\x89PNG\r\n\x1a\n":
		return "png"
	case len(b) >= 3 && string(b[0:3]) == "\xff\xd8\xff":
		return "jpeg"
	case len(b) >= 2 && b[0] == 'B' && b[1] == 'M':
		return "bmp"
	case len(b) >= 4 && b[0] == 0x1A && b[1] == 0x45 && b[2] == 0xDF && b[3] == 0xA3:
		return "mkv" // EBML / Matroska(mkv, webm, mka)
	case len(b) >= 4 && b[0] == 'P' && b[1] == 'K' && (b[2] == 3 || b[2] == 5 || b[2] == 7):
		return "zip"
	case len(b) >= 4 && string(b[0:4]) == "GIF8":
		return "gif"
	case len(b) >= 4 && string(b[0:4]) == "Rar!":
		return "rar"
	case len(b) >= 6 && b[0] == '7' && b[1] == 'z' && b[2] == 0xBC && b[3] == 0xAF && b[4] == 0x27 && b[5] == 0x1C:
		return "7z"
	case len(b) >= 2 && b[0] == 0x1F && b[1] == 0x8B:
		return "gz"
	case len(b) >= 8 && b[0] == 0xD0 && b[1] == 0xCF && b[2] == 0x11 && b[3] == 0xE0:
		return "ole" // 老式 doc/xls/ppt 共用的复合文档
	case len(b) >= 4 && b[0] == 0x30 && b[1] == 0x26 && b[2] == 0xB2 && b[3] == 0x75:
		return "asf" // wma/wmv 容器
	case len(b) >= 3 && string(b[0:3]) == "FLV":
		return "flv"
	case len(b) >= 4 && string(b[0:4]) == "MThd":
		return "mid"
	case len(b) >= 4 && (string(b[0:4]) == "II*\x00" || string(b[0:4]) == "MM\x00*"):
		return "tiff"
	case len(b) >= 4 && b[0] == 0 && b[1] == 0 && (b[2] == 1 || b[2] == 2) && b[3] == 0:
		return "ico"
	case len(b) >= 68 && string(b[60:68]) == "BOOKMOBI":
		return "mobi" // mobi/azw3
	case len(b) >= 262 && string(b[257:262]) == "ustar":
		return "tar"
	case looksLikeSVG(b):
		return "svg"
	}
	return ""
}

// svgProlog 剥掉 SVG 文档开头允许出现的东西,让调用方只看"第一个真元素
// 是不是 <svg>"。
//
// 不能用 bytes.Contains 判 SVG:那不是魔数,是一次子串搜索,于是任何在
// 前 512 字节里内联了一段 <svg> 图标的 .html / .md 都会被认成 SVG,
// contentCheck 再据此报红「内容为 SVG 图片」——一个二阶假警报。
// SVG 的定义是"根元素为 <svg> 的 XML 文档",按这个判就对了:
// 跳过 BOM、空白、XML 声明、注释和 DOCTYPE,再看第一个标签。
func svgProlog(b []byte) []byte {
	// UTF-8 BOM
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	for {
		b = bytes.TrimLeft(b, " \t\r\n")
		switch {
		case bytes.HasPrefix(b, []byte("<?")):
			// <?xml version="1.0"?> 之类的处理指令,到 ?> 为止
			end := bytes.Index(b, []byte("?>"))
			if end < 0 {
				return nil
			}
			b = b[end+2:]
		case bytes.HasPrefix(b, []byte("<!--")):
			end := bytes.Index(b, []byte("-->"))
			if end < 0 {
				return nil
			}
			b = b[end+3:]
		case len(b) > 9 && bytes.HasPrefix(bytes.ToLower(b[:9]), []byte("<!doctype")):
			end := bytes.IndexByte(b, '>')
			if end < 0 {
				return nil
			}
			b = b[end+1:]
		default:
			return b
		}
	}
}

// looksLikeSVG 判断内容是不是一个 SVG 文档:根元素为 <svg>。
func looksLikeSVG(b []byte) bool {
	rest := svgProlog(b)
	if len(rest) < 4 {
		return false
	}
	if rest[0] != '<' {
		return false
	}
	// <svg 后面必须紧跟空白、'>' 或 '/'——否则 <svgfoo> 也会命中。
	if !bytes.HasPrefix(bytes.ToLower(rest), []byte("<svg")) {
		return false
	}
	if len(rest) == 4 {
		return true
	}
	switch rest[4] {
	case ' ', '\t', '\r', '\n', '>', '/':
		return true
	}
	return false
}

// commonExts 按序占用色相环:同一扩展名在色彩管线下永远同色,
// 常见的都互不相同
var commonExts = []string{
	"mp3", "flac", "wav", "ogg", "m4a", "wma",
	"mp4", "mkv", "mov", "avi", "webm",
	"jpg", "jpeg", "png", "gif", "webp", "bmp", "svg",
	"txt", "md", "pdf", "epub", "docx", "xlsx", "pptx",
	"zip", "rar", "7z", "tar", "gz",
	"html", "json", "xml", "csv", "srt", "log",
}

var commonExtHue = func() map[string]float64 {
	m := make(map[string]float64, len(commonExts))
	for i, e := range commonExts {
		m[e] = float64(i) * 360 / float64(len(commonExts))
	}
	return m
}()

// assignExtColors pre-computes every extension's chip color on the main
// thread: the render path must never call ui.Oklch — list rows build on
// raster threads, and Oklch's state is not safe there (deadlocked the
// rasterizer). Misses at render time fall back to a plain gray.
func (a *app) assignExtColors(dt *derivedTheme) {
	if a.extColors == nil {
		a.extColors = map[string]ui.Color{}
	} else {
		a.extColors = map[string]ui.Color{} // seed may have changed
	}
	_, _, base := colorToOklch(dt.selBg)
	hueOf := func(e string, taken []float64) float64 {
		if fixed, ok := commonExtHue[e]; ok {
			return fixed
		}
		off := hashHue(e)
		for n := 0; n < 36; n++ {
			clash := false
			for _, t := range taken {
				if hueClose(off, t) {
					clash = true
					break
				}
			}
			if !clash {
				break
			}
			off = mod360(off + 10)
		}
		return off
	}
	var taken []float64
	assign := func(e string) {
		if e == "" {
			return
		}
		if _, ok := a.extColors[e]; ok {
			return
		}
		off := hueOf(e, taken)
		taken = append(taken, off)
		oc := oklchColor(0.64, 0.125, mod360(base+off)).SRGB()
		a.extColors[e] = ui.RGB(oc.R, oc.G, oc.B)
	}
	for i := range a.files {
		assign(strings.ToLower(strings.TrimPrefix(filepath.Ext(a.files[i].path), ".")))
		assign(strings.ToLower(a.files[i].realType))
	}
	assign("无扩展名")
}

// fileColor is the render-side read: pure map lookup, no Oklch (raster
// threads must not build wide colors), plain gray fallback on miss.
func (a *app) fileColor(dt *derivedTheme, ext string) ui.Color {
	e := strings.ToLower(strings.TrimPrefix(ext, "."))
	if e == "" {
		e = "无扩展名"
	}
	if c, ok := a.extColors[e]; ok {
		return c
	}
	if fixed, isCommon := commonExtHue[e]; isCommon {
		_ = fixed // cold path: fall through to gray rather than Oklch here
	}
	return ui.RGB(128, 128, 128)
}

func hashHue(s string) float64 {
	h := float64(0)
	for i := 0; i < len(s); i++ {
		h = h*31 + float64(s[i])
	}
	return mod360(h * 7.3)
}

func hueClose(a, b float64) bool {
	d := math.Abs(a - b)
	if d > 180 {
		d = 360 - d
	}
	return d < 8
}

func mod360(v float64) float64 {
	v = math.Mod(v, 360) // 大数值下加减循环会退化成天文迭代
	if v < 0 {
		v += 360
	}
	return v
}

// pathTail3 returns the last three folders of a directory, joined with ⁄.
func pathTail3(dir string) string {
	dir = filepath.Clean(dir)
	var parts []string
	for i := 0; i < 3; i++ {
		if dir == "" || dir == "." || dir == string(filepath.Separator) {
			break
		}
		parts = append([]string{filepath.Base(dir)}, parts...)
		dir = filepath.Dir(dir)
	}
	return strings.Join(parts, " ⁄ ")
}
