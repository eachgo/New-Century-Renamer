package main

import (
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

// fileCatColor colors a file-category chip from the seed's hue.
func fileCatColor(dt *derivedTheme, cat string) ui.Color {
	hues := map[string]float64{"音频": 130, "视频": 280, "图片": 210,
		"文本": 60, "文档": 20, "电子书": 160, "压缩包": 30, "": 90}
	base, _, _ := colorToOklch(dt.selBg)
	_, _, h := colorToOklch(dt.selBg)
	_ = base
	return ui.Oklch(0.64, 0.125, float32(mod360(h+hues[cat])))
}

func mod360(v float64) float64 {
	for v < 0 {
		v += 360
	}
	for v >= 360 {
		v -= 360
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
