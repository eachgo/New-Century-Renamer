package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// ebml builds one EBML element: ID bytes as given, then a size field, then
// the payload. Size is written in the minimum number of bytes needed
// (1-8), with the length marker in the top bits.
func ebml(id []byte, payload []byte) []byte {
	var buf bytes.Buffer
	buf.Write(id)
	buf.Write(encodeEBMLSize(len(payload)))
	buf.Write(payload)
	return buf.Bytes()
}

func encodeEBMLSize(n int) []byte {
	// Choose the smallest length L such that n fits in 7L bits.
	for l := 1; l <= 8; l++ {
		max := (1 << (7 * l)) - 1
		if n <= max {
			out := make([]byte, l)
			v := n
			for i := l - 1; i >= 0; i-- {
				out[i] = byte(v & 0xFF)
				v >>= 8
			}
			// set the length marker: top (8-l+1) bits -> bit at position 7-(l-1)
			out[0] |= 0x80 >> uint(l-1)
			return out
		}
	}
	return nil
}

// buildMKV assembles a minimal but structurally valid Matroska file: an EBML
// header, a Segment containing Info (with the given title), and a Cluster
// (so the parser also proves it stops at media data). No real codec needed.
func buildMKV(title string) []byte {
	ebmlHeader := ebml([]byte{0x1A, 0x45, 0xDF, 0xA3},
		ebml([]byte{0x42, 0x86}, []byte{1}) /* EBMLVersion */)

	info := ebml([]byte{0x15, 0x49, 0xA9, 0x66}, // Info
		concat(
			ebml([]byte{0x7B, 0xA9}, []byte(title)),          // Title
			ebml([]byte{0x4D, 0x80}, []byte("libebml test")), // MuxingApp
		),
	)
	cluster := ebml([]byte{0x1F, 0x43, 0xB6, 0x75}, // Cluster (media data)
		ebml([]byte{0xE7}, []byte{1}), // Timecode
	)
	segment := ebml([]byte{0x18, 0x53, 0x80, 0x67}, concat(info, cluster)) // Segment
	return concat(ebmlHeader, segment)
}

func concat(bs ...[]byte) []byte {
	var out []byte
	for _, b := range bs {
		out = append(out, b...)
	}
	return out
}

func TestMKVMetaSynthetic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "video.mkv")
	if err := os.WriteFile(p, buildMKV("Sintel.2010.1080p.BluRay"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := ExtractMeta(p)
	if m == nil {
		t.Fatal("ExtractMeta returned nil for a valid MKV")
	}
	if m["title"] != "Sintel.2010.1080p.BluRay" {
		t.Errorf("title = %q", m["title"])
	}
	if m["muxer"] != "libebml test" {
		t.Errorf("muxer = %q", m["muxer"])
	}
}

func TestMKVMetaNotMatroska(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "not.mkv")
	os.WriteFile(p, []byte("this is not ebml at all"), 0o644)
	if m := ExtractMeta(p); m != nil {
		t.Errorf("want nil for non-matroska, got %v", m)
	}
}

// TestMKVMetaRealFile exercises the parser end-to-end on a file ffmpeg
// produced, so we're not only testing against our own synthetic encoding.
// Skipped when ffmpeg isn't present (CI has no ffmpeg).
func TestMKVMetaRealFile(t *testing.T) {
	p := os.Getenv("NCREAMER_MKV_SAMPLE")
	if p == "" {
		t.Skip("set NCREAMER_MKV_SAMPLE to a .mkv path to run this")
	}
	m := ExtractMeta(p)
	if m == nil || m["title"] == "" {
		t.Fatalf("no title from real mkv: %v", m)
	}
	t.Logf("real mkv meta: %v", m)
}
