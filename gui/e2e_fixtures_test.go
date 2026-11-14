package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// 端到端测试原来依赖仓库根下的 testfile/(约 200MB 的真实音频/PDF/EPUB),
// 而那个目录被 .gitignore 排除。结果是 CI 上 resetCopy 找不到素材,
// 六个 TestE2E* 全部 t.Skip——"端到端"在 CI 里恒不执行,只在开发者本机
// 有素材时才跑。测试绿了,但它守的东西一天都没被守过。
//
// 这里改成:没有 testfile/ 就当场生成一套最小夹具。每个文件只需要
// "够让对应的解析器认出自己"——嗅探看开头几个字节,元数据解析看结构头,
// 不需要能真的播放。
//
// 夹具刻意覆盖四种元数据解析器(MP3 的 ID3 / FLAC 的 Vorbis comment /
// EPUB 的 OPF / PDF 的 Info 字典)加一个纯嗅探格式(PNG),这样
// TestE2EMetadataFlow 里那句 "withMeta == 0 就报错" 才真的守得住:
// 以前在 CI 上它连跑都跑不到。
func makeTestfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string][]byte{
		"01. THE UNION.mp3":   id3Fixture("THE UNION", "Awich"),
		"02. RASEN.mp3":       id3Fixture("RASEN in OKINAWA", "Awich"),
		"03. Guerilla.mp3":    id3Fixture("Guerilla", "Awich"),
		"04. Ali Baba.flac":   flacFixture("Ali Baba", "Awich"),
		"05. Call On Me.flac": flacFixture("Call On Me", "Awich"),
		"06. 読書ノート.epub":      epubFixture("読書ノート", "誰か"),
		"07. notes.pdf":       pdfFixture("notes", "someone"),
		"08. cover.png":       pngFixture(),
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// id3Fixture builds a minimal ID3v2.3 tag with TIT2/TPE1 and an empty frame
// area after it. The audio payload is irrelevant: nothing decodes it.
func id3Fixture(title, artist string) []byte {
	frame := func(id string, text string) []byte {
		body := append([]byte{0x03}, []byte(text)...) // 3 = UTF-8
		var b bytes.Buffer
		b.WriteString(id)
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], uint32(len(body)))
		b.Write(size[:])
		b.Write([]byte{0, 0}) // flags
		b.Write(body)
		return b.Bytes()
	}
	var frames bytes.Buffer
	frames.Write(frame("TIT2", title))
	frames.Write(frame("TPE1", artist))
	body := frames.Bytes()

	var out bytes.Buffer
	out.WriteString("ID3")
	out.Write([]byte{0x03, 0x00, 0x00}) // v2.3, no flags
	var size [4]byte
	// syncsafe
	n := len(body)
	size[0] = byte((n >> 21) & 0x7f)
	size[1] = byte((n >> 14) & 0x7f)
	size[2] = byte((n >> 7) & 0x7f)
	size[3] = byte(n & 0x7f)
	out.Write(size[:])
	out.Write(body)
	// 一点静音帧,让文件看起来像音频
	out.Write(bytes.Repeat([]byte{0xFF, 0xFB, 0x90, 0x00}, 64))
	return out.Bytes()
}

// flacFixture builds "fLaC" + STREAMINFO + VORBIS_COMMENT with TITLE/ARTIST.
func flacFixture(title, artist string) []byte {
	var vc bytes.Buffer
	vendor := []byte("ncreamer-test")
	var l [4]byte
	binary.LittleEndian.PutUint32(l[:], uint32(len(vendor)))
	vc.Write(l[:])
	vc.Write(vendor)
	pairs := []string{"TITLE=" + title, "ARTIST=" + artist}
	binary.LittleEndian.PutUint32(l[:], uint32(len(pairs)))
	vc.Write(l[:])
	for _, p := range pairs {
		binary.LittleEndian.PutUint32(l[:], uint32(len(p)))
		vc.Write(l[:])
		vc.WriteString(p)
	}

	var out bytes.Buffer
	out.WriteString("fLaC")
	// STREAMINFO: type 0, not last, 34 bytes
	out.Write([]byte{0x00, 0x00, 0x00, 34})
	out.Write(make([]byte, 34))
	// VORBIS_COMMENT: type 4, last
	payload := vc.Bytes()
	out.Write([]byte{0x84, byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload))})
	out.Write(payload)
	return out.Bytes()
}

// epubFixture builds a real (minimal) EPUB: mimetype + container.xml + opf.
func epubFixture(title, author string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, content string) {
		w, err := zw.Create(name)
		if err != nil {
			return
		}
		w.Write([]byte(content))
	}
	add("mimetype", "application/epub+zip")
	add("META-INF/container.xml", `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`)
	add("content.opf", `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>`+title+`</dc:title>
    <dc:creator>`+author+`</dc:creator>
  </metadata>
</package>`)
	zw.Close()
	return buf.Bytes()
}

// pdfFixture writes a PDF whose Info dictionary carries /Title and /Author.
func pdfFixture(title, author string) []byte {
	return []byte("%PDF-1.7\n" +
		"1 0 obj\n<< /Title (" + title + ") /Author (" + author + ") >>\nendobj\n" +
		"trailer\n<< /Root 1 0 R >>\n%%EOF\n")
}

// pngFixture is an 8-byte PNG signature plus filler: enough for sniffMagic,
// and nothing tries to decode it.
func pngFixture() []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
}
