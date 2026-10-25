# reamer

跨平台批量更名器 —— 继承经典 Win32 更名器 xTools 的设计巧思,面向纯 64 位 / ARM 平台的现代重制。

xTools 的三层核心抽象被完整延续:

1. **多步规则管道**:规则按顺序依次应用,可自由组合;
2. **元变量系统**:`<NAME>`、`<META:...>`、`<AI:...>` 在模板中展开;
3. **谨慎哲学**:默认只预览,`-apply` 才执行,日志可一键回滚。

## 现代化部分

| xTools 的巧思 | reamer 的延续 |
|---|---|
| 文本第一行更名 | `<AI:指示>` 变量,OpenAI 兼容接口读文件内容命名 |
| MP3/RM/WMA 标签 | `<META:...>`:EPUB (dc:title/creator)、MP4/M4V/MOV (ilst ©nam/©ART)、PDF (/Title /Author) |
| 汉字转拼音(全角/假名处理) | `pinyin` 规则,全角字母数字自动转半角 |
| 保留字符转全角 | `sanitize` 规则,`/ \ : * ? " < > |` → 全角,信息不丢 |
| CRC32 更名 | `<CRC32>` 变量 |
| 重名检查、应用前检查 | 默认 dry-run;重名冲突/目标已存在在预览即标错 |
| 繁简转换 | `cc=s2t` / `cc=t2s`(OpenCC 词典) |
| 假名转罗马音 | `romaji` 规则,平/片假名最长匹配,拗音/促音/长音处理 |
| txt 文件列表导出改回 | **CSV 列表**(sha256 指纹 + 路径 + 文件名 + 新文件名),`reamer export` / `reamer import`,GUI 有同名按钮 |
| — | 流水线步骤:筛选/排除/清除筛选(`filter`/`unfilter`),选取文件名/扩展名(`scope`) |
| 自定义预设 | JSON 预设文件(`-p`),可分享 |
| — | `reamer undo` 用操作日志整批回滚 |

## 用法

```
# 预览(默认,不动文件)
reamer -r pinyin -r sanitize 某目录/*.mp3

# 用 EPUB 元数据重命名,含非法字符的书名自动转全角
reamer -r 'template=<META:author> - <META:title><.EXT>' -r sanitize 书库/

# AI 按内容命名文本文件
export REAMER_AI_BASE=https://api.deepseek.com/v1
export REAMER_AI_KEY=sk-...
export REAMER_AI_MODEL=deepseek-chat
reamer -r 'template=<AI:根据内容起一个简短描述性文件名>_<SEQ><.EXT>' -r sanitize 笔记/*.txt

# 执行并回滚
reamer -apply -r seq=1,3 文件...
reamer undo -journal .reamer-journal.json
```

规则和变量完整列表见 `reamer`(无参数)的帮助输出。

## 构建

纯 Go 标准库 + go-pinyin,无 cgo,任意机器可交叉编译:

```
GOOS=windows GOARCH=arm64 go build .
GOOS=linux   GOARCH=arm64 go build .
GOOS=darwin  GOARCH=arm64 go build .
```

## GUI

`gui/` 是基于 [mygo](https://github.com/egoist/mygo)(纯 Go 无 cgo 桌面框架)的原生界面:

```
cd gui
go build .      # 或 go tool mygo dev 带热重载
```

窗口布局:添加文件/目录 → 规则编辑(每行一条,与 CLI 同语法)→ 预览表格(原文件名/新文件名/状态)→ 执行/回滚。CLI 与 GUI 共享 `core` 包,行为完全一致。需要 Go 1.27+(老版本 Go 会经 GOTOOLCHAIN=auto 自动下载工具链)。

## 后续方向

- 更多元数据:MKV、FLAC/OPUS 标签、EXIF
- 音频标签写入(xTools 还能写标签,这里暂只读)
- GUI:规则预设选择、拖拽添加文件、目录递归开关
