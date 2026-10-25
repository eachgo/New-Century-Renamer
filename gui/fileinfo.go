package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// sniffMagic identifies the real content type from the file's first bytes,
// so a renamed-to-lie extension shows ("photo.mp3" that is really WebP).
func sniffMagic(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	b := make([]byte, 16)
	n, _ := f.Read(b)
	b = b[:n]
	switch {
	case len(b) >= 3 && b[0] == 'I' && b[1] == 'D' && b[2] == '3':
		return "MP3"
	case len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0:
		return "MP3"
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "WebP"
	case len(b) >= 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		return "WAV"
	case len(b) >= 4 && string(b[0:4]) == "ftyp":
		return "MP4"
	case len(b) >= 4 && string(b[0:4]) == "%PDF":
		return "PDF"
	case len(b) >= 4 && string(b[0:4]) == "OggS":
		return "OGG"
	case len(b) >= 4 && string(b[0:4]) == "fLaC":
		return "FLAC"
	case len(b) >= 8 && string(b[0:8]) == "\x89PNG\r\n\x1a\n":
		return "PNG"
	case len(b) >= 3 && string(b[0:3]) == "\xff\xd8\xff":
		return "JPEG"
	case len(b) >= 2 && b[0] == 'B' && b[1] == 'M':
		return "BMP"
	case len(b) >= 4 && b[0] == 'P' && b[1] == 'K':
		return "ZIP"
	case len(b) >= 4 && string(b[0:4]) == "GIF8":
		return "GIF"
	}
	return ""
}

// extCategory buckets an extension for coloring, mirroring how the step
// chips color by category.
func extCategory(ext string) string {
	e := strings.ToLower(ext)
	switch e {
	case "mp3", "flac", "ogg", "wav", "wma", "m4a", "aac":
		return "音频"
	case "mp4", "m4v", "mov", "mkv", "avi", "webm", "m4b":
		return "视频"
	case "jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "ico":
		return "图片"
	case "txt", "md", "log", "csv", "json", "xml", "html", "htm", "srt", "ass":
		return "文本"
	case "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx":
		return "文档"
	case "epub", "mobi", "azw3":
		return "电子书"
	case "zip", "rar", "7z", "tar", "gz":
		return "压缩包"
	}
	return ""
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
