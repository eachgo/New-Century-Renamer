package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 真实文件字节一路走到界面上的那颗胶囊。
//
// 之前测过 sniffMagic 的每种魔数,也测过 contentCheck 的四档结论,还测过
// 三种胶囊的渲染——但三段之间是各测各的:胶囊那三条直接把 "webp"/""/"mp3"
// 写进 fileInfo.realType,绕过了嗅探。于是"嗅探器对某个格式返回了一个
// 没人认识的新键"这种情况,没有任何一条测试会失败:嗅探测试只查自己那
// 几种,渲染测试只查三个手写的值,中间那层映射(formatTable/containerExts)
// 漏了新键也没人知道。
//
// 这条测试从磁盘上的真实字节出发:写文件 → statFile → sniffMagic →
// contentCheck → 渲染 → 找胶囊。任何一环改了行为都会红。
func TestRealBytesReachTheChip(t *testing.T) {
	// 每种格式:文件名、真实字节、期望的嗅探结果。
	// 字节都是最小可识别的前缀,不需要能播放——嗅探只看开头。
	cases := []struct {
		name  string
		bytes []byte
		sniff string
	}{
		{"song.mp3", []byte("ID3\x04\x00\x00\x00\x00\x00\x00"), "mp3"},
		{"real.flac", append([]byte("fLaC"), make([]byte, 32)...), "flac"},
		{"cover.png", append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...), "png"},
		{"clip.mp4", []byte("\x00\x00\x00\x20ftypisom\x00\x00\x02\x00"), "mp4"},
		{"book.epub", []byte("PK\x03\x04\x14\x00\x00\x00"), "zip"},
		{"page.pdf", []byte("%PDF-1.7\n"), "pdf"},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		path := filepath.Join(dir, tc.name)
		if err := os.WriteFile(path, tc.bytes, 0o644); err != nil {
			t.Fatal(err)
		}
		// 第一环:真实字节 → 嗅探结果。
		fi := statFile(path)
		if fi.realType != tc.sniff {
			t.Errorf("%s: sniffMagic = %q, want %q", tc.name, fi.realType, tc.sniff)
			continue
		}
		// 第二环:嗅探结果必须真的在映射表里。
		//
		// 这里不能只看 contentLabel 非空:它在查不到时会退回
		// strings.ToUpper(real),所以删掉表项它照样返回 "FLAC" ——
		// 反向验证时正是这样漏过去的。查表项本身才守得住。
		// 容器格式(zip/mp4)走 containerLabels,不要求进 formatTable。
		if _, isContainer := containerLabels[fi.realType]; !isContainer {
			if _, known := formatTable[fi.realType]; !known {
				t.Errorf("%s: 嗅探出 %q,但 formatTable 里没有这个键——"+
					"界面会显示一个没人认识的标签(或者退回大写形式)",
					tc.name, fi.realType)
			}
		}
		if got := contentLabel(fi.realType); got == "" {
			t.Errorf("%s: 嗅探出 %q 但 contentLabel 给不出展示名", tc.name, fi.realType)
		}
		// 第三环:扩展名与内容相符时,不该报警。
		ext := filepath.Ext(tc.name)[1:]
		if v := contentCheck(ext, fi.realType, fi.size); v != contentOK {
			t.Errorf("%s: contentCheck(%q, %q) = %d, want %d(相符不该报警)",
				tc.name, ext, fi.realType, v, contentOK)
		}
		// 第四环:整条链路渲染出来,行里不该有"内容为…"的红胶囊。
		a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
		a.settings.Journal = filepath.Join(dir, "journal.json")
		a.files = []fileInfo{fi}
		a.steps = []guiRule{step("case.lower")}
		a.stepUID = a.steps[0].opUID
		a.preview()
		tt := ui.NewTester(a.view, 1280, 720)
		for i := 0; i < 3; i++ {
			tt.Frame()
		}
		if tt.HasText("内容为") {
			t.Errorf("%s: 扩展名与内容相符,却出现了「内容为…」的警告胶囊", tc.name)
		}
	}
}

// 反向:把 .mp3 的内容换成 PNG,界面上必须出现「内容为 PNG 图片」。
// 这条走的是同一条链路,守的是"不符"那一档真的能到界面。
func TestRealMismatchReachesTheChip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lie.mp3")
	if err := os.WriteFile(path, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...), 0o644); err != nil {
		t.Fatal(err)
	}
	fi := statFile(path)
	if fi.realType != "png" {
		t.Fatalf("sniffMagic = %q, want png", fi.realType)
	}
	if v := contentCheck("mp3", fi.realType, fi.size); v != contentMismatch {
		t.Fatalf("contentCheck = %d, want %d", v, contentMismatch)
	}

	a := &app{stepSel: 0, fileSel: -1, settings: defaultSettings()}
	a.settings.Journal = filepath.Join(dir, "journal.json")
	a.files = []fileInfo{fi}
	a.steps = []guiRule{step("case.lower")}
	a.stepUID = a.steps[0].opUID
	a.preview()
	tt := ui.NewTester(a.view, 1280, 720)
	for i := 0; i < 3; i++ {
		tt.Frame()
	}
	if !tt.HasText("内容为 PNG 图片") {
		t.Errorf("名为 .mp3 的 PNG 文件应当显示「内容为 PNG 图片」,界面上没有")
	}
}
