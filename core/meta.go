package core

import (
	"archive/zip"
	"encoding/binary"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ExtractMeta pulls metadata from modern containers — the successor of
// xTools' MP3/RM/WMA tag support: EPUB (dc:title/dc:creator), MP4/M4V/MOV
// (©nam/©ART ilst atoms), PDF (/Title /Author).
func ExtractMeta(path string) map[string]string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	meta := map[string]string{}
	var m map[string]string
	switch ext {
	case "epub":
		m = epubMeta(path)
	case "mp4", "m4v", "m4a", "mov", "m4b":
		m = mp4Meta(path)
	case "pdf":
		m = pdfMeta(path)
	}
	for k, v := range m {
		if v != "" {
			meta[k] = v
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

// ---- EPUB ----

func epubMeta(path string) map[string]string {
	out := map[string]string{}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return out
	}
	defer zr.Close()
	// container.xml -> rootfile
	var opfName string
	for _, f := range zr.File {
		if f.Name == "META-INF/container.xml" {
			rc, err := f.Open()
			if err == nil {
				b, _ := io.ReadAll(rc)
				rc.Close()
				var c struct {
					Rootfiles []struct {
						FullPath string `xml:"full-path,attr"`
					} `xml:"rootfiles>rootfile"`
				}
				if xml.Unmarshal(b, &c) == nil && len(c.Rootfiles) > 0 {
					opfName = c.Rootfiles[0].FullPath
				}
			}
			break
		}
	}
	if opfName == "" {
		for _, f := range zr.File {
			if strings.HasSuffix(f.Name, ".opf") {
				opfName = f.Name
				break
			}
		}
	}
	if opfName == "" {
		return out
	}
	for _, f := range zr.File {
		if f.Name != opfName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return out
		}
		defer rc.Close()
		dec := xml.NewDecoder(rc)
		var cur string
		var title, author string
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "title" || t.Name.Local == "creator" {
					cur = t.Name.Local
				}
			case xml.CharData:
				if cur == "title" && title == "" {
					title = strings.TrimSpace(string(t))
				}
				if cur == "creator" && author == "" {
					author = strings.TrimSpace(string(t))
				}
			case xml.EndElement:
				if t.Name.Local == cur {
					cur = ""
				}
			}
		}
		out["title"], out["author"] = title, author
		break
	}
	return out
}

// ---- MP4 ----

func mp4Meta(path string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil || len(b) < 12 {
		return out
	}
	title := mp4Atom(b, []string{"moov", "udta", "meta", "ilst"}, [4]byte{0xA9, 'n', 'a', 'm'})
	artist := mp4Atom(b, []string{"moov", "udta", "meta", "ilst"}, [4]byte{0xA9, 'A', 'R', 'T'})
	if v := mp4DataString(title); v != "" {
		out["title"] = v
	}
	if v := mp4DataString(artist); v != "" {
		out["artist"] = v
	}
	return out
}

// mp4Atom walks atom boxes along path and returns the box payload of the
// 4CC name inside (raw "ilst" child box bytes, including its header).
func mp4Atom(b []byte, path []string, name [4]byte) []byte {
	cur := b
	for _, seg := range path {
		next, ok := findBox(cur, str4(seg))
		if !ok {
			return nil
		}
		cur = next
		// 'meta' box carries a 4-byte version/flags before children
		if seg == "meta" {
			if len(cur) >= 4 {
				cur = cur[4:]
			}
		}
	}
	// scan children for the named box
	off := 0
	for off+8 <= len(cur) {
		size := int(binary.BigEndian.Uint32(cur[off : off+4]))
		if size < 8 || off+size > len(cur) {
			return nil
		}
		if cur[off+4] == name[0] && cur[off+5] == name[1] && cur[off+6] == name[2] && cur[off+7] == name[3] {
			return cur[off+8 : off+size]
		}
		off += size
	}
	return nil
}

func findBox(b []byte, n [4]byte) (payload []byte, ok bool) {
	off := 0
	for off+8 <= len(b) {
		size := int(binary.BigEndian.Uint32(b[off : off+4]))
		if size < 8 || off+size > len(b) {
			return nil, false
		}
		if b[off+4] == n[0] && b[off+5] == n[1] && b[off+6] == n[2] && b[off+7] == n[3] {
			return b[off+8 : off+size], true
		}
		if size == 1 { // 64-bit size
			if off+16 > len(b) {
				return nil, false
			}
			size64 := int(binary.BigEndian.Uint64(b[off+8 : off+16]))
			if size64 < 16 || off+size64 > len(b) {
				return nil, false
			}
			off += int(size64)
			continue
		}
		off += size
	}
	return nil, false
}

func str4(s string) [4]byte {
	var n [4]byte
	copy(n[:], s)
	return n
}

// mp4DataString extracts the UTF-8 string from an ilst box's `data` child.
func mp4DataString(box []byte) string {
	// find 'data' child
	off := 0
	for off+8 <= len(box) {
		size := int(binary.BigEndian.Uint32(box[off : off+4]))
		if size < 8 || off+size > len(box) {
			return ""
		}
		if box[off+4] == 'd' && box[off+5] == 'a' && box[off+6] == 't' && box[off+7] == 'a' {
			payload := box[off+8 : off+size]
			if len(payload) < 8 {
				return ""
			}
			return strings.TrimRight(string(payload[8:]), "\x00")
		}
		off += size
	}
	return ""
}

// ---- PDF (literal /Title and /Author strings) ----

var pdfStrRe = regexp.MustCompile(`(?i)/(Title|Author)\s*\(((?:[^()\\]|\\.)*)\)`)

func pdfMeta(path string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	if len(b) > 4<<20 {
		b = b[:4<<20] // metadata lives near the start usually
	}
	for _, m := range pdfStrRe.FindAllSubmatch(b, -1) {
		key, val := strings.ToLower(string(m[1])), pdfUnescape(string(m[2]))
		if val == "" || strings.HasPrefix(val, "microsoft") || strings.HasPrefix(val, "word") {
			continue
		}
		if key == "title" || key == "author" {
			out[key] = val
		}
	}
	return out
}

func pdfUnescape(s string) string {
	s = strings.ReplaceAll(s, `\(`, "(")
	s = strings.ReplaceAll(s, `\)`, ")")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	// UTF-16BE with BOM -> Go string
	if strings.HasPrefix(s, "\xfe\xff") {
		b := []byte(s[3:])
		rs := make([]rune, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			rs = append(rs, rune(uint16(b[i])<<8|uint16(b[i+1])))
		}
		return string(rs)
	}
	return s
}
