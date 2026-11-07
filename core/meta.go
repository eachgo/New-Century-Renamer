package core

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
	case "flac":
		m = flacMeta(path)
	case "mp3":
		m = id3Meta(path)
	case "ogg", "oga", "opus":
		m = oggMeta(path)
	case "mkv", "webm", "mka", "mks":
		m = mkvMeta(path)
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
		if title != "" {
			out["title"] = title
		}
		if author != "" {
			out["author"] = author
		}
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
	album := mp4Atom(b, []string{"moov", "udta", "meta", "ilst"}, [4]byte{0xA9, 'a', 'l', 'b'})
	if v := mp4DataString(title); v != "" {
		out["title"] = v
	}
	if v := mp4DataString(artist); v != "" {
		out["artist"] = v
		out["author"] = v // 与 EPUB/FLAC 的 author 同义,模板键统一
	}
	if v := mp4DataString(album); v != "" {
		out["album"] = v
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
		b := []byte(s[2:])
		rs := make([]rune, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			rs = append(rs, rune(uint16(b[i])<<8|uint16(b[i+1])))
		}
		return string(rs)
	}
	return s
}

// ---- FLAC: METADATA_BLOCK_VORBIS_COMMENT (type 4) ----

func flacMeta(path string) map[string]string {
	b := headBytes(path, 256<<10)
	out := map[string]string{}
	if len(b) < 8 || string(b[0:4]) != "fLaC" {
		return out
	}
	off := 4
	for off+4 <= len(b) {
		header := b[off]
		last := header&0x80 != 0
		blockType := header & 0x7f
		size := int(b[off+1])<<16 | int(b[off+2])<<8 | int(b[off+3])
		off += 4
		if off+size > len(b) {
			return out
		}
		if blockType == 4 {
			parseVorbisComment(b[off:off+size], out)
			return out
		}
		off += size
		if last {
			break
		}
	}
	return out
}

// parseVorbisComment reads vendor string then KEY=value pairs (LE lengths,
// UTF-8), keeping ARTIST/TITLE/ALBUM. Shared by FLAC and OGG.
func parseVorbisComment(d []byte, out map[string]string) {
	rd := newLEReader(d)
	vendor := rd.u32()
	if !rd.skip(int(vendor)) {
		return
	}
	n := rd.u32()
	for i := uint32(0); i < n && rd.ok(); i++ {
		l := rd.u32()
		s := rd.str(int(l))
		if s == "" {
			continue
		}
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			continue
		}
		key := strings.ToUpper(k)
		switch key {
		case "ARTIST":
			out["artist"] = v
			out["author"] = v // 与 EPUB 的 dc:creator 同义,模板键统一
		case "TITLE":
			out["title"] = v
		case "ALBUM":
			out["album"] = v
		}
	}
}

type leReader struct {
	b   []byte
	off int
	err bool
}

func newLEReader(b []byte) *leReader { return &leReader{b: b} }

func (r *leReader) ok() bool { return !r.err }

func (r *leReader) u32() uint32 {
	if r.off+4 > len(r.b) {
		r.err = true
		return 0
	}
	v := binary.LittleEndian.Uint32(r.b[r.off:])
	r.off += 4
	return v
}

func (r *leReader) skip(n int) bool {
	if n < 0 || r.off+n > len(r.b) {
		r.err = true
		return false
	}
	r.off += n
	return true
}

func (r *leReader) str(n int) string {
	if n < 0 || r.off+n > len(r.b) {
		r.err = true
		return ""
	}
	s := string(r.b[r.off : r.off+n])
	r.off += n
	return s
}

// headBytes reads at most the first n bytes of a file.
func headBytes(path string, n int) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b := make([]byte, n)
	m, _ := io.ReadFull(f, b)
	return b[:m]
}

// ---- MP3: ID3v2.3/2.4 text frames (TPE1/TIT2/TALB) ----

func id3Meta(path string) map[string]string {
	b := headBytes(path, 512<<10)
	out := map[string]string{}
	if len(b) < 10 || string(b[0:3]) != "ID3" {
		return out
	}
	ver := b[3] // 3 or 4; 2.2 frames differ and are rare — skip
	if ver != 3 && ver != 4 {
		return out
	}
	size := syncsafe(b[6:10])
	if 10+size > len(b) {
		size = len(b) - 10
	}
	body := b[10 : 10+size]
	pos := 0
	for pos+10 <= len(body) {
		id := string(body[pos : pos+4])
		if id[0] == 0 {
			break // padding
		}
		var fs int
		if ver == 4 {
			fs = syncsafe(body[pos+4 : pos+8])
		} else {
			fs = int(binary.BigEndian.Uint32(body[pos+4 : pos+8]))
		}
		if fs < 0 || pos+10+fs > len(body) {
			break
		}
		text := id3Text(body[pos+10 : pos+10+fs])
		switch id {
		case "TPE1":
			out["artist"] = text
			out["author"] = text
		case "TIT2":
			out["title"] = text
		case "TALB":
			out["album"] = text
		}
		pos += 10 + fs
	}
	return out
}

// id3Text decodes a frame's text (first byte = encoding).
func id3Text(d []byte) string {
	if len(d) < 1 {
		return ""
	}
	data := d[1:]
	switch d[0] {
	case 0: // ISO-8859-1
		r := make([]rune, 0, len(data))
		for _, c := range data {
			r = append(r, rune(c))
		}
		return string(r)
	case 1: // UTF-16 with BOM
		return strings.TrimRight(utf16Decode(data, true), "\x00")
	case 2: // UTF-16BE
		return strings.TrimRight(utf16Decode(data, false), "\x00")
	default: // 3 = UTF-8
		return strings.TrimRight(string(data), "\x00")
	}
}

func utf16Decode(b []byte, bom bool) string {
	if bom && len(b) >= 2 {
		if b[0] == 0xFF && b[1] == 0xFE {
			b = b[2:]
		} else if b[0] == 0xFE && b[1] == 0xFF {
			b = b[2:]
			bom = false
		}
	}
	rs := make([]rune, 0, len(b)/2)
	be := !bom // after stripping a LE BOM, remaining is LE; no BOM default BE per spec case 1 is BOM'd
	for i := 0; i+1 < len(b); i += 2 {
		var u uint16
		if be {
			u = uint16(b[i])<<8 | uint16(b[i+1])
		} else {
			u = uint16(b[i]) | uint16(b[i+1])<<8
		}
		rs = append(rs, rune(u))
	}
	return string(rs)
}

func syncsafe(b []byte) int {
	return int(b[0]&0x7f)<<21 | int(b[1]&0x7f)<<14 | int(b[2]&0x7f)<<7 | int(b[3]&0x7f)
}

// ---- OGG: first Vorbis-comment header page ----

func oggMeta(path string) map[string]string {
	b := headBytes(path, 256<<10)
	out := map[string]string{}
	idx := bytes.Index(b, []byte("vorbis"))
	if idx < 0 {
		return out
	}
	parseVorbisComment(b[idx+7:], out)
	return out
}

// ---- MKV / WebM: EBML (Matroska) ----
//
// Matroska nests size-prefixed EBML elements. We only need the title and
// the muxing app out of the Segment > Info element — plus, when the file is
// a whole downloaded film, the "title" tag often already carries
// "Group.Name" (e.g. "FHD x264"), which is exactly what we want to rename by.
// Shared by .mkv and .webm (same container, different CodecIDs).

const (
	ebmlID       = 0x1A45DFA3 // EBML header
	segmentID    = 0x18538067 // Segment
	seekHeadID   = 0x114D9B74
	infoID       = 0x1549A966 // Segment > Info
	tracksID     = 0x1654AE6B
	chaptersID   = 0x1043A770
	clusterID    = 0x1F43B675
	tagsID       = 0x1254C367
	titleID      = 0x7BA9 // Info > Title
	muxAppID     = 0x4D80 // Info > MuxingApp
	writingAppID = 0x5741 // Info > WritingApp
	durationID   = 0x4489 // Info > Duration (float, in TimecodeScale units)
	dateUTCID    = 0x4461 // Info > DateUTC
	maxMKVInfo   = 4 << 20
)

// mkvMeta reads the title/muxer out of a Matroska file's Info element.
func mkvMeta(path string) map[string]string {
	out := map[string]string{}
	b := headBytes(path, maxMKVInfo)
	if len(b) < 4 || !isMKVHeader(b) {
		return out
	}
	// Walk top-level elements until we reach Segment, then inside Segment
	// walk children until Info. Everything before a Cluster is small.
	off := 0
	// Skip the EBML header, then read the Segment element header. Each
	// element gets its own buffer: readEBMLHeader returns the ID as a plain
	// int (e.g. 440786851 for 0x1A45DFA3), so comparing it to the const id
	// works; but reusing `id` from the if-initialiser below is a footgun —
	// keep the two steps separate and re-declare with `=`.
	if eid, esize, ehdr, ok := readEBMLHeader(b, off); ok && eid == ebmlID {
		off += ehdr + esize
	}
	id, size, hdr, ok := readEBMLHeader(b, off)
	if !ok || id != segmentID {
		return out
	}
	body := b[off+hdr:]
	// Segment "size" may be unknown (all-ones); just scan the body we have.
	_ = size
	if m := mkvInfo(body); len(m) > 0 {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func isMKVHeader(b []byte) bool {
	return len(b) >= 4 && b[0] == 0x1A && b[1] == 0x45 && b[2] == 0xDF && b[3] == 0xA3
}

// mkvInfo scans Segment children for Info and pulls Title/MuxingApp.
func mkvInfo(seg []byte) map[string]string {
	off := 0
	for off < len(seg) {
		id, size, hdr, ok := readEBMLHeader(seg, off)
		if !ok {
			break
		}
		body := off + hdr
		if size < 0 || body+size > len(seg) {
			// Unknown size (live/streamed mkv) or the element runs past the
			// bytes we read: fall back to scanning this prefix for Info.
			return mkvScanForInfo(seg[off:])
		}
		if id == infoID {
			return mkvParseInfo(seg[body : body+size])
		}
		if id == clusterID {
			break // media data starts; Info (if any) is before it
		}
		off = body + size
	}
	return map[string]string{}
}

// mkvScanForInfo is the fallback for files whose Segment size is unknown or
// bigger than the window we read: walk child element headers and, the first
// time we hit the Info ID, parse the bytes after it. Like a mini resync.
func mkvScanForInfo(seg []byte) map[string]string {
	off := 0
	for off+2 < len(seg) {
		id, size, hdr, ok := readEBMLHeader(seg, off)
		if !ok {
			off++
			continue
		}
		body := off + hdr
		if id == infoID {
			end := len(seg)
			if size >= 0 && body+size <= len(seg) {
				end = body + size
			}
			return mkvParseInfo(seg[body:end])
		}
		if id == clusterID {
			return map[string]string{}
		}
		if size < 0 || body+size > len(seg) {
			off++
			continue
		}
		off = body + size
	}
	return map[string]string{}
}

func mkvParseInfo(info []byte) map[string]string {
	out := map[string]string{}
	off := 0
	for off < len(info) {
		id, size, hdr, ok := readEBMLHeader(info, off)
		if !ok {
			break
		}
		body := off + hdr
		if size < 0 || body+size > len(info) {
			break
		}
		val := info[body : body+size]
		switch id {
		case titleID:
			if s := mkvString(val); s != "" {
				out["title"] = s
			}
		case muxAppID:
			if s := mkvString(val); s != "" && out["muxer"] == "" {
				out["muxer"] = s
			}
		case writingAppID:
			if s := mkvString(val); s != "" {
				out["app"] = s
			}
		case dateUTCID:
			if s := mkvDate(val); s != "" {
				out["date"] = s
			}
		}
		off = body + size
	}
	return out
}

// readEBMLHeader reads one element ID and size at off. Returns the ID (as
// an int, since EBML IDs are 1-4 bytes), the payload size, the header length
// (ID + size fields), and ok. size = -1 means "unknown/until end".
func readEBMLHeader(b []byte, off int) (id int, size, hdr int, ok bool) {
	id, n, ok := readEBMLID(b, off)
	if !ok {
		return 0, 0, 0, false
	}
	sz, m, ok := readEBMLSize(b, off+n)
	if !ok {
		return 0, 0, 0, false
	}
	return id, sz, n + m, true
}

// readEBMLID decodes a variable-length EBML element ID (the leading bit
// marks its length; the remaining bits plus the rest form the value).
func readEBMLID(b []byte, off int) (id, n int, ok bool) {
	if off >= len(b) {
		return 0, 0, false
	}
	first := b[off]
	if first == 0 {
		return 0, 0, false
	}
	length := 1
	for mask := byte(0x80); mask != 0 && first&mask == 0; mask >>= 1 {
		length++
	}
	if length > 4 || off+length > len(b) {
		return 0, 0, false
	}
	v := 0
	for i := 0; i < length; i++ {
		v = v<<8 | int(b[off+i])
	}
	return v, length, true
}

// readEBMLSize decodes a variable-length size (value bits after the marker
// bit). All-ones means unknown size → -1.
func readEBMLSize(b []byte, off int) (size, n int, ok bool) {
	if off >= len(b) {
		return 0, 0, false
	}
	first := b[off]
	if first == 0 {
		return 0, 0, false
	}
	length := 1
	for mask := byte(0x80); mask != 0 && first&mask == 0; mask >>= 1 {
		length++
	}
	if length > 8 || off+length > len(b) {
		return 0, 0, false
	}
	// value = first byte with marker bit cleared, then remaining bytes
	v := int(first) & (0xFF >> uint(length))
	allOnes := v == (0xFF >> uint(length))
	for i := 1; i < length; i++ {
		v = v<<8 | int(b[off+i])
		if b[off+i] != 0xFF {
			allOnes = false
		}
	}
	if allOnes {
		return -1, length, true
	}
	return v, length, true
}

// mkvString trims trailing NULs from a UTF-8 EBML string.
func mkvString(b []byte) string {
	return strings.TrimRight(string(b), "\x00")
}

// mkvDate converts a Matroska DateUTC (nanoseconds since 2001-01-01 UTC) to
// a YYYY-MM-DD string. Best-effort: only 8-byte signed ints are valid.
func mkvDate(b []byte) string {
	if len(b) != 8 {
		return ""
	}
	ns := int64(binary.BigEndian.Uint64(b))
	if ns == 0 {
		return ""
	}
	// 2001-01-01 00:00:00 UTC in Unix nanos.
	const epoch2001 = 978307200
	unix := epoch2001 + ns/1e9
	return time.Unix(unix, 0).UTC().Format("2006-01-02")
}
