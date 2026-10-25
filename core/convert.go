package core

import (
	"strings"
	"sync"

	"github.com/longbridgeapp/opencc"
)

// ToTraditional / ToSimplified convert between Simplified and Traditional
// Chinese (xTools' CodeConvert, via OpenCC dictionaries).
var (
	ccOnce sync.Once
	s2tCC  *opencc.OpenCC
	t2sCC  *opencc.OpenCC
	ccErr  error
)

func ccInit() {
	ccOnce.Do(func() {
		if s2tCC, ccErr = opencc.New("s2t"); ccErr != nil {
			return
		}
		t2sCC, ccErr = opencc.New("t2s")
	})
}

func ToTraditional(s string) (string, error) {
	ccInit()
	if ccErr != nil {
		return "", ccErr
	}
	return s2tCC.Convert(s)
}

func ToSimplified(s string) (string, error) {
	ccInit()
	if ccErr != nil {
		return "", ccErr
	}
	return t2sCC.Convert(s)
}

// ---- kana → romaji (xTools converted 假名 in its pinyin step; here it
// is its own rule). Longest-match walk over the syllabary. ----

var kanaTable = map[string]string{
	// hiragana
	"あ": "a", "い": "i", "う": "u", "え": "e", "お": "o",
	"か": "ka", "き": "ki", "く": "ku", "け": "ke", "こ": "ko",
	"さ": "sa", "し": "shi", "す": "su", "せ": "se", "そ": "so",
	"た": "ta", "ち": "chi", "つ": "tsu", "て": "te", "と": "to",
	"な": "na", "に": "ni", "ぬ": "nu", "ね": "ne", "の": "no",
	"は": "ha", "ひ": "hi", "ふ": "fu", "へ": "he", "ほ": "ho",
	"ま": "ma", "み": "mi", "む": "mu", "め": "me", "も": "mo",
	"や": "ya", "ゆ": "yu", "よ": "yo",
	"ら": "ra", "り": "ri", "る": "ru", "れ": "re", "ろ": "ro",
	"わ": "wa", "ゐ": "wi", "ゑ": "we", "を": "wo", "ん": "n",
	"が": "ga", "ぎ": "gi", "ぐ": "gu", "げ": "ge", "ご": "go",
	"ざ": "za", "じ": "ji", "ず": "zu", "ぜ": "ze", "ぞ": "zo",
	"だ": "da", "ぢ": "di", "づ": "du", "で": "de", "ど": "do",
	"ば": "ba", "び": "bi", "ぶ": "bu", "べ": "be", "ぼ": "bo",
	"ぱ": "pa", "ぴ": "pi", "ぷ": "pu", "ぺ": "pe", "ぽ": "po",
	// digraphs first by length in ToRomaji
	"きゃ": "kya", "きゅ": "kyu", "きょ": "kyo",
	"しゃ": "sha", "しゅ": "shu", "しょ": "sho",
	"ちゃ": "cha", "ちゅ": "chu", "ちょ": "cho",
	"にゃ": "nya", "にゅ": "nyu", "にょ": "nyo",
	"ひゃ": "hya", "ひゅ": "hyu", "ひょ": "hyo",
	"みゃ": "mya", "みゅ": "myu", "みょ": "myo",
	"りゃ": "rya", "りゅ": "ryu", "りょ": "ryo",
	"ぎゃ": "gya", "ぎゅ": "gyu", "ぎょ": "gyo",
	"じゃ": "ja", "じゅ": "ju", "じょ": "jo",
	"びゃ": "bya", "びゅ": "byu", "びょ": "byo",
	"ぴゃ": "pya", "ぴゅ": "pyu", "ぴょ": "pyo",
	// small kana
	"ぁ": "a", "ぃ": "i", "ぅ": "u", "ぇ": "e", "ぉ": "o",
	"ゃ": "ya", "ゅ": "yu", "ょ": "yo",
	// katakana
	"ア": "a", "イ": "i", "ウ": "u", "エ": "e", "オ": "o",
	"カ": "ka", "キ": "ki", "ク": "ku", "ケ": "ke", "コ": "ko",
	"サ": "sa", "シ": "shi", "ス": "su", "セ": "se", "ソ": "so",
	"タ": "ta", "チ": "chi", "ツ": "tsu", "テ": "te", "ト": "to",
	"ナ": "na", "ニ": "ni", "ヌ": "nu", "ネ": "ne", "ノ": "no",
	"ハ": "ha", "ヒ": "hi", "フ": "fu", "ヘ": "he", "ホ": "ho",
	"マ": "ma", "ミ": "mi", "ム": "mu", "メ": "me", "モ": "mo",
	"ヤ": "ya", "ユ": "yu", "ヨ": "yo",
	"ラ": "ra", "リ": "ri", "ル": "ru", "レ": "re", "ロ": "ro",
	"ワ": "wa", "ヰ": "wi", "ヱ": "we", "ヲ": "wo", "ン": "n",
	"ガ": "ga", "ギ": "gi", "グ": "gu", "ゲ": "ge", "ゴ": "go",
	"ザ": "za", "ジ": "ji", "ズ": "zu", "ゼ": "ze", "ゾ": "zo",
	"ダ": "da", "ヂ": "di", "ヅ": "du", "デ": "de", "ド": "do",
	"バ": "ba", "ビ": "bi", "ブ": "bu", "ベ": "be", "ボ": "bo",
	"パ": "pa", "ピ": "pi", "プ": "pu", "ペ": "pe", "ポ": "po",
	"キャ": "kya", "キュ": "kyu", "キョ": "kyo",
	"シャ": "sha", "シュ": "shu", "ショ": "sho",
	"チャ": "cha", "チュ": "chu", "チョ": "cho",
	"ニャ": "nya", "ニュ": "nyu", "ニョ": "nyo",
	"ヒャ": "hya", "ヒュ": "hyu", "ヒョ": "hyo",
	"ミャ": "mya", "ミュ": "myu", "ミョ": "myo",
	"リャ": "rya", "リュ": "ryu", "リョ": "ryo",
	"ギャ": "gya", "ギュ": "gyu", "ギョ": "gyo",
	"ジャ": "ja", "ジュ": "ju", "ジョ": "jo",
	"ビャ": "bya", "ビュ": "byu", "ビョ": "byo",
	"ピャ": "pya", "ピュ": "pyu", "ピョ": "pyo",
	"ァ": "a", "ィ": "i", "ゥ": "u", "ェ": "e", "ォ": "o",
	"ャ": "ya", "ュ": "yu", "ョ": "yo",
}

// ToRomaji romanizes kana (hiragana and katakana) with longest matching;
// other characters pass through. っ/ッ doubles the next consonant and ー
// repeats the previous vowel.
func ToRomaji(s string) string {
	rs := []rune(s)
	var b strings.Builder
	prevVowel := ""
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == 'っ' || r == 'ッ' {
			// gemination: double the next syllable's consonant
			if next, _ := kanaMatch(rs, i+1); next != "" {
				b.WriteString(next[:1])
			}
			continue
		}
		if r == 'ー' {
			if prevVowel != "" {
				b.WriteString(prevVowel)
			}
			continue
		}
		if m, used := kanaMatch(rs, i); m != "" {
			b.WriteString(m)
			if v := lastVowel(m); v != "" {
				prevVowel = v
			}
			i += used - 1
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// kanaMatch returns the romaji of the kana starting at rs[i] and how many
// runes it consumed (digraph or single), preferring longer matches.
func kanaMatch(rs []rune, i int) (string, int) {
	if i >= len(rs) {
		return "", 0
	}
	if i+1 < len(rs) {
		if m, ok := kanaTable[string(rs[i:i+2])]; ok {
			return m, 2
		}
	}
	if m, ok := kanaTable[string(rs[i])]; ok {
		return m, 1
	}
	return "", 0
}

func lastVowel(s string) string {
	if s == "" {
		return ""
	}
	last := s[len(s)-1]
	if last == 'a' || last == 'i' || last == 'u' || last == 'e' || last == 'o' {
		return string(last)
	}
	return ""
}
