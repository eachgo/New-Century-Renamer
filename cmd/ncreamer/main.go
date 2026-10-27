// ncreamer (跨向更名器 / New Century Renamer) - a modern cross-platform
// batch renamer, inheriting the design philosophy of the classic xTools
// renamer: multi-step rule pipeline, metadata variables, pinyin conversion,
// careful preview & undo.
package main

import (
	"ncreamer/core"
)

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// version 为变量以便 build.ps1 用 -ldflags -X 注入
var version = "0.1.0"

func usage() {
	fmt.Print(`ncreamer ` + version + ` - 跨向更名器 New Century Renamer(继承 xTools 巧思的现代版)

用法:
  ncreamer [选项] 文件或目录...
  ncreamer undo -journal 重新命名日志.json

规则(按 -r 顺序依次应用, 可重复):
  template=T     模板重建文件名, T 中可用变量(见下)
  replace=OLD,NEW[,regex]   替换; regex 时 OLD 为正则, NEW 可含 $1
  pinyin         汉字转拼音(全角字母数字转半角, 保留非汉字)
  sanitize       Windows 保留字符转全角, 去控制符/首尾空格/结尾点
  case=upper|lower|title
  insert=POS,TEXT       在文件名 POS 处插入
  delete=POS,LEN        删除文件名从 POS 起 LEN 个字符(负数 POS 表示倒数)
  truncate=MAXLEN       截断文件名(不含扩展名)到 MAXLEN 字符
  ext=NEWEXT            改扩展名(不含点, 空则删除扩展名)
  seq=START[,WIDTH[,STEP]]  追加序号

模板/替换可用变量:
  <NAME> <EXT> <.EXT> <DIR> <CRC32> <Pinyin:NAME> <SEQ>
  <META:title|author|album|...>   来自 EPUB/MP4/PDF 的元数据
  <AI:指示>                       调用 OpenAI 兼容接口按指示生成名字片段

选项:
  -r rule        一条规则, 可重复
  -p preset.json 从 JSON 文件读规则(格式: {"rules":["pinyin", ...]})
  -apply         实际执行(默认只预览)
  -journal F     指定日志文件(默认 .ncreamer-journal.json; 仅 -apply 时写入)
  -recursive     目录参数时递归列举
  -v             版本

AI 配置(环境变量): NCREAMER_AI_BASE / NCREAMER_AI_KEY / NCREAMER_AI_MODEL
                   NCREAMER_AI_TIMEOUT(秒, 默认 300; 本地慢模型可调大)
`)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "export" {
		fs := flag.NewFlagSet("export", flag.ExitOnError)
		out := fs.String("o", "", "输出 CSV 文件(默认 stdout)")
		fs.Parse(os.Args[2:])
		if fs.NArg() == 0 {
			fatal(fmt.Errorf("用法: ncreamer export [-o list.csv] 文件..."))
		}
		files, err := expandInputs(fs.Args(), false)
		if err != nil {
			fatal(err)
		}
		var w io.Writer = os.Stdout
		if *out != "" {
			f, err := os.Create(*out)
			if err != nil {
				fatal(err)
			}
			defer f.Close()
			w = f
		}
		if err := core.ExportCSV(w, files); err != nil {
			fatal(err)
		}
		if *out != "" {
			fmt.Printf("已导出 %d 个文件到 %s(new_name 列编辑后用 ncreamer import 导回)\n", len(files), *out)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "import" {
		fs := flag.NewFlagSet("import", flag.ExitOnError)
		apply := fs.Bool("apply", false, "实际执行")
		journal := fs.String("journal", ".ncreamer-journal.json", "日志文件")
		fs.Parse(os.Args[2:])
		if fs.NArg() != 1 {
			fatal(fmt.Errorf("用法: ncreamer import [-apply] list.csv"))
		}
		plan, errs := core.ImportCSV(fs.Arg(0))
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "问题:", e)
		}
		n := 0
		for _, op := range plan {
			mark := " "
			switch {
			case op.Err != nil:
				mark = "!"
			case op.OldName != op.NewName:
				mark = "*"
				n++
			}
			fmt.Printf("%s %s => %s\n", mark, filepath.Join(op.Dir, op.OldName), op.NewName)
		}
		fmt.Printf("%d 行,%d 个将更名\n", len(plan), n)
		if !*apply {
			fmt.Println("(预览模式, 加 -apply 执行)")
			return
		}
		if err := core.ApplyPlan(plan, *journal); err != nil {
			fatal(err)
		}
		fmt.Printf("已执行, 日志 %s; 可用 ncreamer undo -journal %s 回滚\n", *journal, *journal)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "undo" {
		fs := flag.NewFlagSet("undo", flag.ExitOnError)
		journal := fs.String("journal", ".ncreamer-journal.json", "日志文件")
		fs.Parse(os.Args[2:])
		if err := core.UndoJournal(*journal); err != nil {
			fatal(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "-v" {
		fmt.Println("ncreamer", version)
		return
	}

	var (
		ruleArgs  multiFlag
		preset    string
		apply     bool
		journal   string
		recursive bool
	)
	fs := flag.NewFlagSet("ncreamer", flag.ExitOnError)
	fs.Var(&ruleArgs, "r", "规则, 可重复")
	fs.StringVar(&preset, "p", "", "预设 JSON 文件")
	fs.BoolVar(&apply, "apply", false, "实际执行")
	fs.StringVar(&journal, "journal", ".ncreamer-journal.json", "日志文件")
	fs.BoolVar(&recursive, "recursive", false, "递归")
	fs.Usage = func() { usage() }
	fs.Parse(os.Args[1:])

	if preset != "" {
		b, err := os.ReadFile(preset)
		if err != nil {
			fatal(err)
		}
		var p struct {
			Rules []string `json:"rules"`
		}
		if err := json.Unmarshal(b, &p); err != nil {
			fatal(fmt.Errorf("预设文件 %s: %w", preset, err))
		}
		ruleArgs = append(ruleArgs, p.Rules...)
	}

	rules, err := core.ParseRules(ruleArgs)
	if err != nil {
		fatal(err)
	}
	if len(rules) == 0 {
		usage()
		return
	}

	patterns := fs.Args()
	if len(patterns) == 0 {
		usage()
		return
	}
	files, err := expandInputs(patterns, recursive)
	if err != nil {
		fatal(err)
	}

	plan, errs := core.RunPipeline(rules, files)
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "跳过:", e)
	}

	// preview
	changed := 0
	for _, op := range plan {
		mark := " "
		if op.Err != nil {
			mark = "!"
		} else if op.OldName != op.NewName {
			mark = "*"
			changed++
		}
		fmt.Printf("%s %s  =>  %s\n", mark, filepath.Join(op.Dir, op.OldName), op.NewName)
		if op.Err != nil {
			fmt.Printf("    错误: %v\n", op.Err)
		}
	}
	fmt.Printf("%d 个文件, %d 个将更名, %d 个有问题\n", len(plan), countOK(plan), len(errs)+countErr(plan))

	if !apply {
		fmt.Println("(预览模式, 加 -apply 执行)")
		return
	}
	if err := core.ApplyPlan(plan, journal); err != nil {
		fatal(err)
	}
	fmt.Printf("已执行, 日志写入 %s; 可用 `ncreamer undo -journal %s` 回滚\n", journal, journal)
}

func countOK(plan []core.RenameOp) (n int) {
	for _, op := range plan {
		if op.Err == nil && op.OldName != op.NewName {
			n++
		}
	}
	return
}

func countErr(plan []core.RenameOp) (n int) {
	for _, op := range plan {
		if op.Err != nil {
			n++
		}
	}
	return
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, "; ") }
func (m *multiFlag) Set(s string) error { *m = append(*m, s); return nil }

func expandInputs(patterns []string, recursive bool) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		p = filepath.Clean(p)
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, pat := range patterns {
		// a literal existing path wins over glob interpretation, so
		// filenames that happen to contain [ ] ? * are still usable
		if _, err := os.Stat(pat); err == nil {
			info, _ := os.Stat(pat)
			if info.IsDir() {
				if recursive {
					filepath.WalkDir(pat, func(p string, d os.DirEntry, err error) error {
						if err == nil && !d.IsDir() {
							add(p)
						}
						return err
					})
				} else {
					ents, err := os.ReadDir(pat)
					if err != nil {
						return nil, err
					}
					for _, e := range ents {
						if !e.IsDir() {
							add(filepath.Join(pat, e.Name()))
						}
					}
				}
				continue
			}
			add(pat)
			continue
		}
		// not a literal path: treat as glob pattern
		matches, err := filepath.Glob(pat)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			add(m)
		}
	}
	return out, nil
}
