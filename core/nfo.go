package core

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---- NFO sidecars: the "metadata and numbering" middle ground ----
//
// A folder ripped from a disc or a series usually holds
//
//	01.mkv  02.mkv  03.mkv      ← numbers in the names
//	movie.nfo / tvshow.nfo      ← title, year, and often a URL to the source
//
// Renaming by metadata alone fails here: the container (mkv) carries no
// title tag, so <META:title> is empty; but the sidecar NFO knows the real
// title. What ties a file to the NFO is its *number* — but only if the
// number in the filename is already the correct episode/disc index. So this
// rule reads the folder's .nfo, resolves the title, and stamps it onto files
// whose leading number matches. If a number is missing or ambiguous it says
// so instead of guessing.

// nfoInfo is what we could pull out of a folder's .nfo files.
type nfoInfo struct {
	title     string
	showtitle string
	year      string
	season    int
	episodes  map[int]nfoEpisode // by episode number
	raw       string
}

type nfoEpisode struct {
	title    string
	season   int
	episode  int
	hasTitle bool
}

// nfoExtPriority lists the canonical whole-folder sidecar names, highest
// priority first. A bare number.nfo (01.nfo next to 01.mkv) also counts, but
// ranks below these.
var nfoExtPriority = []string{"tvshow.nfo", "movie.nfo"}

// loadNFO reads and parses the folder's NFO sidecar(s). Returns nil when the
// folder has none.
func loadNFO(dir string) *nfoInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".nfo") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return nil
	}
	// Prefer the canonical whole-folder sidecars, then a numbered one.
	sort.SliceStable(names, func(i, j int) bool {
		return nfoRank(names[i]) < nfoRank(names[j])
	})
	info := &nfoInfo{episodes: map[int]nfoEpisode{}}
	for _, n := range names {
		p := filepath.Join(dir, n)
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			continue
		}
		parseNFO(b, info)
	}
	if info.title == "" && info.showtitle == "" && len(info.episodes) == 0 {
		return nil
	}
	return info
}

func nfoRank(name string) int {
	lower := strings.ToLower(name)
	for i, want := range nfoExtPriority {
		if lower == want {
			return i
		}
	}
	// numbered sidecar (01.nfo) — lowest priority, but still usable
	if _, err := strconv.Atoi(strings.TrimSuffix(lower, ".nfo")); err == nil {
		return 100
	}
	return 50
}

// parseNFO accumulates Kodi/NFO-style fields. The format is loose: TMDb/Kodi
// <movie><title>…</title><year>…</year></movie>, or an episodedetails record,
// or multi-episode <episodedetails> blocks. We also fall back to regexes when
// the XML is malformed (real NFOs sometimes carry junk).
func parseNFO(b []byte, info *nfoInfo) {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '<' {
		if parseNFOXML(trimmed, info) == nil {
			return
		}
	}
	nfoRegexFallback(string(b), info)
}

// parseNFOXML walks the whole stream, accumulating every movie/episodedetails
// block it sees (multi-episode NFOs contain several).
func parseNFOXML(b []byte, info *nfoInfo) error {
	dec := xml.NewDecoder(bytes.NewReader(b))
	var stack []string
	var cur *nfoEpisode
	var field string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			stack = append(stack, name)
			field = name
			if name == "episodedetails" {
				cur = &nfoEpisode{}
			}
		case xml.CharData:
			s := strings.TrimSpace(string(t))
			if s == "" {
				continue
			}
			switch field {
			case "title":
				if cur != nil && cur.title == "" {
					cur.title = s
				} else if cur == nil && info.title == "" {
					info.title = s
				}
			case "showtitle":
				if info.showtitle == "" {
					info.showtitle = s
				}
			case "year", "premiered", "aired", "dateadded":
				if info.year == "" {
					if y := firstYear(s); y != "" {
						info.year = y
					}
				}
			case "season":
				if cur != nil {
					cur.season, _ = strconv.Atoi(s)
				} else {
					info.season, _ = strconv.Atoi(s)
				}
			case "episode":
				if cur != nil {
					cur.episode, _ = strconv.Atoi(s)
				}
			}
		case xml.EndElement:
			name := strings.ToLower(t.Name.Local)
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if name == "episodedetails" && cur != nil {
				if cur.episode > 0 && cur.title != "" {
					info.episodes[cur.episode] = *cur
				}
				cur = nil
			}
			if len(stack) > 0 {
				field = stack[len(stack)-1]
			} else {
				field = ""
			}
		}
	}
	return nil
}

var (
	nfoTitleRe   = regexp.MustCompile(`(?is)<title>(.*?)</title>`)
	nfoShowRe    = regexp.MustCompile(`(?is)<showtitle>(.*?)</showtitle>`)
	nfoYearRe    = regexp.MustCompile(`(?is)<year>(\d{4})</year>`)
	nfoSeasonRe  = regexp.MustCompile(`(?is)<season>(\d+)</season>`)
	nfoEpisodeRe = regexp.MustCompile(`(?is)<episode>(\d+)</episode>`)
)

func nfoRegexFallback(s string, info *nfoInfo) {
	if info.title == "" {
		if m := nfoTitleRe.FindStringSubmatch(s); m != nil {
			info.title = strings.TrimSpace(stripTagsOK(m[1]))
		}
	}
	if info.showtitle == "" {
		if m := nfoShowRe.FindStringSubmatch(s); m != nil {
			info.showtitle = strings.TrimSpace(stripTagsOK(m[1]))
		}
	}
	if info.year == "" {
		if m := nfoYearRe.FindStringSubmatch(s); m != nil {
			info.year = m[1]
		}
	}
	// Single-episode NFO with <episodedetails>: capture the pairing.
	ep := nfoEpisodeRe.FindStringSubmatch(s)
	se := nfoSeasonRe.FindStringSubmatch(s)
	if ep != nil {
		var e nfoEpisode
		e.episode, _ = strconv.Atoi(ep[1])
		if se != nil {
			e.season, _ = strconv.Atoi(se[1])
		}
		// title captured above (last <title> in the doc) is the episode title
		if e.episode > 0 && info.title != "" {
			e.title = info.title
			e.hasTitle = true
			if _, exists := info.episodes[e.episode]; !exists {
				info.episodes[e.episode] = e
			}
		}
	}
}

func stripTagsOK(s string) string {
	var out strings.Builder
	in := false
	for _, r := range s {
		switch r {
		case '<':
			in = true
		case '>':
			in = false
		default:
			if !in {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}

var yearRe = regexp.MustCompile(`(19|20)\d{2}`)

func firstYear(s string) string {
	return yearRe.FindString(s)
}

// leadingNumber pulls the leading run of digits out of a filename stem.
// "01 - Pilot" -> (1, true); "S01E05" -> we don't try here (needs a pattern);
// "1080p" -> (1080, true) but that's a resolution, so callers must be careful.
func leadingNumber(stem string) (int, bool) {
	digits := ""
	for _, r := range stem {
		if r >= '0' && r <= '9' {
			digits += string(r)
		} else {
			break
		}
	}
	if digits == "" {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return n, true
}

// episodeTagRe matches SxxExx in any common spelling, e.g. S01E05, s1e5.
var episodeTagRe = regexp.MustCompile(`(?i)s(\d{1,3})[\s._-]*e(\d{1,4})`)

// fileNumber returns the file's index for NFO matching: either a leading
// number or an SxxExx tag. label is a short Chinese description used in the
// error message when it can't be determined.
func fileNumber(stem string) (num int, kind string, ok bool) {
	if m := episodeTagRe.FindStringSubmatch(stem); m != nil {
		e, _ := strconv.Atoi(m[2])
		return e, "集", true
	}
	if n, ok := leadingNumber(stem); ok {
		return n, "编号", true
	}
	return 0, "", false
}

// addNFOSnippet appends a folder's NFO title (if any) to an AI content
// snippet, so a numbered media file with a sidecar but no container title
// still gives the model something to name by.
func addNFOSnippet(dir string, parts []string) []string {
	if s := nfoSummary(dir); s != "" {
		parts = append(parts, "NFO: "+s)
	}
	return parts
}

// newNFORule parses `nfo[=TEMPLATE][,file=PATH]`. A bare `file=…` (no tpl)
// or a first positional arg that isn't a key:value leaves the template empty.
// NB: splitArgs only unescapes commas/backslashes, so a Windows path like
// C:\TV	vshow.nfo keeps its backslashes verbatim here.
func newNFORule(argstr string) (Rule, error) {
	r := nfoRule{}
	if argstr == "" {
		return r, nil
	}
	for _, a := range splitArgs(argstr) {
		if v, ok := strings.CutPrefix(a, "file="); ok {
			r.file = strings.TrimSpace(v)
		} else {
			// first non-key token is the template
			if r.tpl == "" {
				r.tpl = a
			}
		}
	}
	return r, nil
}

// loadNFOAt parses one specific NFO file (used when the rule names a
// path explicitly rather than relying on the folder's own .nfo).
func loadNFOAt(path string) *nfoInfo {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return nil
	}
	info := &nfoInfo{episodes: map[int]nfoEpisode{}}
	parseNFO(b, info)
	if info.title == "" && info.showtitle == "" && len(info.episodes) == 0 {
		return nil
	}
	return info
}

// ---- the rule ----

// nfoRule renames a media file by the title found in its folder's NFO,
// keyed on the file's number. tpl may reference <NFO:title>, <NFO:showtitle>,
// <NFO:year>, <NFO:season>, <NFO:episode>, or the convenience <NFOName>
// (which picks showtitle·SxxExx·title for episodes, or title+year otherwise).
// If tpl is empty, <NFOName><.EXT> is assumed.
type nfoRule struct {
	tpl  string
	file string // specific NFO path; "" = use the folder's own .nfo
}

func (r nfoRule) Apply(ctx *Ctx, dir, name string) (string, error) {
	if IsSidecarFile(name) {
		return name, nil // never rename the .nfo itself
	}
	var info *nfoInfo
	if r.file != "" {
		info = loadNFOAt(r.file)
		if info == nil {
			return name, fmt.Errorf("指定的 NFO 文件读不出内容: %s", r.file)
		}
	} else {
		info = loadNFO(dir)
		if info == nil {
			return name, fmt.Errorf("文件夹里没有 .nfo 文件,无法按编号命名")
		}
	}
	stem, _ := splitExt(name)
	num, kind, ok := fileNumber(stem)
	if !ok {
		return name, fmt.Errorf("文件名里没有可用的%s,无法与 NFO 对应", "编号")
	}
	ep, isEp := info.episodes[num]
	season := info.season
	if isEp && ep.season > 0 {
		season = ep.season
	}
	// Resolve the title for this file. When the NFO carries per-episode
	// titles, we demand an exact match on the number — otherwise a typo'd or
	// stale number would silently borrow the wrong episode's title, which is
	// the whole hazard this feature has to avoid. Only when the NFO has no
	// episode titles at all do we fall back to the show/movie title.
	title := ""
	hasEpisodes := len(info.episodes) > 0
	switch {
	case isEp:
		title = ep.title
	case hasEpisodes:
		return name, fmt.Errorf("NFO 里没有第 %d %s的标题(编号必须与 NFO 准确对应)", num, kind)
	default:
		title = info.title
		if title == "" {
			title = info.showtitle
		}
	}
	if title == "" {
		return name, fmt.Errorf("NFO 里找不到对应%s %d 的标题(编号必须与 NFO 准确对应)", kind, num)
	}
	vars := map[string]string{
		"title":     title,
		"showtitle": info.showtitle,
		"year":      info.year,
		"season":    pad2(season),
		"episode":   pad2(num),
	}
	tpl := r.tpl
	if strings.TrimSpace(tpl) == "" {
		tpl = "<NFOName><.EXT>"
	}
	out := expandNFOVars(tpl, vars, info, num, season, isEp && ep.title != "", name)
	if strings.TrimSpace(splitExtStem(out)) == "" {
		return name, nil
	}
	return out, nil
}

// expandNFOVars resolves <NFO:...>, <NFOName> and the usual built-in vars via
// ExpandVars for anything else (so <NAME>, <.EXT>, <SEQ> still work).
func expandNFOVars(tpl string, vars map[string]string, info *nfoInfo, num, season int, epTitled bool, name string) string {
	// Handle our own tokens first, then hand the rest to ExpandVars.
	var b strings.Builder
	rest := tpl
	for {
		i := strings.Index(rest, "<")
		if i < 0 {
			b.WriteString(rest)
			break
		}
		j := strings.IndexByte(rest[i:], '>')
		if j < 0 {
			b.WriteString(rest)
			break
		}
		j += i
		token := rest[i : j+1]
		b.WriteString(rest[:i])
		if rep, ok := nfoToken(token, vars, info, num, season, epTitled); ok {
			b.WriteString(rep)
		} else {
			// not ours: leave for ExpandVars
			b.WriteString(token)
		}
		rest = rest[j+1:]
	}
	// ExpandVars with a nil ctx meta still resolves <NAME>/<.EXT>/etc.
	ctx := &Ctx{}
	return ExpandVars(b.String(), ctx, filepath.Dir(name), name)
}

func nfoToken(token string, vars map[string]string, info *nfoInfo, num, season int, epTitled bool) (string, bool) {
	inner := token[1 : len(token)-1]
	switch {
	case inner == "NFOName":
		return nfoComposeName(vars, info, num, season, epTitled), true
	case strings.HasPrefix(inner, "NFO:"):
		k := strings.TrimPrefix(inner, "NFO:")
		if v, ok := vars[k]; ok {
			return v, true
		}
		return "", true // unknown NFO: key → empty, don't leak the literal
	}
	return "", false
}

// nfoComposeName builds a sensible default: "Show - SxxExx - Title" for an
// episode, or "Title (Year)" for a movie.
func nfoComposeName(vars map[string]string, info *nfoInfo, num, season int, epTitled bool) string {
	if epTitled && info.showtitle != "" {
		s := fmt.Sprintf("%s - S%sE%s - %s", info.showtitle, pad2(season), pad2(num), vars["title"])
		return sanitizeFileName(s)
	}
	if info.year != "" {
		return sanitizeFileName(fmt.Sprintf("%s (%s)", vars["title"], info.year))
	}
	return sanitizeFileName(vars["title"])
}

// sanitizeFileName replaces characters Windows forbids in a name with safe
// look-alikes, so a title straight from an NFO can't produce an invalid path.
func sanitizeFileName(s string) string {
	rep := strings.NewReplacer(
		"/", "／", "\\", "＼", ":", "：", "*", "＊", "?", "？",
		"\"", "＂", "<", "＜", ">", "＞", "|", "｜",
	)
	return strings.TrimSpace(rep.Replace(s))
}

func pad2(n int) string {
	if n < 0 {
		return ""
	}
	return fmt.Sprintf("%02d", n)
}

// nfoSummary lets the AI snippet mention a folder's NFO title when present.
func nfoSummary(dir string) string {
	info := loadNFO(dir)
	if info == nil {
		return ""
	}
	var parts []string
	if info.showtitle != "" {
		parts = append(parts, "剧名: "+info.showtitle)
	}
	if info.title != "" {
		parts = append(parts, "标题: "+info.title)
	}
	if info.year != "" {
		parts = append(parts, "年份: "+info.year)
	}
	if len(info.episodes) > 0 {
		parts = append(parts, fmt.Sprintf("NFO 含 %d 集标题", len(info.episodes)))
	}
	return strings.Join(parts, "; ")
}

// IsSidecarFile reports whether name is a metadata sidecar (an .nfo). Such
// files are inputs to the nfo rule, not media to be renamed, so "no number to
// match" on them is expected and shouldn't be counted as a problem.
func IsSidecarFile(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".nfo")
}
