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
| MP3/RM/WMA 标签 | `<META:...>`:EPUB (dc:title/creator)、MP3 (ID3v2)、FLAC/OGG (Vorbis)、MP4/M4V/MOV (ilst)、PDF (/Title /Author) |
| 汉字转拼音(全角/假名处理) | `pinyin` 规则,全角字母数字自动转半角 |
| 保留字符转全角 | `sanitize` 规则,`/ \ : * ? " < > |` → 全角,信息不丢 |
| CRC32 更名 | `<CRC32>` 变量 |
| 重名检查、应用前检查 | 默认 dry-run;重名冲突/目标已存在在预览即标错,失败原因直接写在文件卡片里 |
| 繁简转换 | `cc=s2t` / `cc=t2s`(OpenCC 词典) |
| 假名转罗马音 | `romaji` 规则,平/片假名最长匹配,拗音/促音/长音处理 |
| txt 文件列表导出改回 | **CSV 列表**(sha256 指纹 + 路径 + 文件名 + 新文件名),`reamer export` / `reamer import`,GUI 有同名按钮 |
| — | 流水线步骤:筛选/排除/清除筛选(`filter`/`unfilter`),选取文件名/扩展名(`scope`) |
| 自定义预设 | JSON 预设文件(`-p`),可分享 |
| — | `reamer undo` 用操作日志整批回滚 |
| — | 仅大小写变化的改名在 Windows/macOS 上正确执行(两步改名,可回滚) |

## CLI 用法

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

## GUI

`gui/` 是基于 [mygo](https://github.com/egoist/mygo)(纯 Go 无 cgo 桌面框架)的原生界面,CLI 与 GUI 共享 `core` 包,行为完全一致。

```
cd gui
go build .      # 或 go tool mygo dev 带热重载
```

### 界面

- **三栏布局**:左侧功能目录(点击加入步骤)→ 中间任务步骤 → 右侧文件列表。无编号,顺序不设教条——文件可以最先加,步骤也可以;
- **融合标题栏**:系统标题栏隐藏但窗口按钮/阴影/圆角保留,标题栏与窗口背景一体、整条可拖动(参考 clash-traffic 的原生一体手法);
- **实时预览**:改任何参数立刻看到每个文件的原名 → 新名对照,只有 AI 步骤需要显式点"预览";
- **文件卡**:原名 → 新名、真实类型(扩展名不符时红色警示)、路径、日期、大小;失败原因直接写在卡片里,不只"失败"两个字;
- **主题**:跟随系统深浅模式;主色可跟系统强调色或自选,全部界面色从种子色经 OKLCH 派生(对比度经过校验)。

### 任务步骤

- 点击步骤选中即改参数;Ctrl+点击加选、拖动整组重排(原生 Selection+Reorder);
- **触摸友好**:标题行"多选"按钮开启后每行出现选择圆点,行高 38px 适配触摸;鼠标用户悬停即见圆点;
- 批量删除/全不选;Esc 随时退出多选或关闭对话框;
- 范围控制(筛选/选取)在行右缘以双色条可视化,与同组步骤连续。

### 可靠性

- **CSV 即备份**:改名前导出的 CSV 含 sha256 指纹,文件名改乱之后仍能按指纹找回原名(内容相同的文件也能各自认领);独立于操作日志的第二条退路;
- 单实例锁:第二个实例直接退出,不会互踩操作日志;
- 真实文件端到端测试套件(`gui/e2e_test.go`):添加→预览→执行→磁盘校验→回滚,以及元数据、CSV 备份还原、窄窗口、批量性能。

## 构建

需要 Go 1.27+(老版本 Go 会经 GOTOOLCHAIN=auto 自动下载工具链)。纯 Go 标准库 + go-pinyin,无 cgo。

```powershell
.\build.ps1          # 当前平台:CLI + GUI(GUI 用 -H=windowsgui,双击无黑框)
.\build.ps1 -All     # 交叉编译 windows/linux/darwin 的 amd64+arm64 共 8 个产物到 dist\
```

手工等价:

```
go build -trimpath -ldflags "-s -w -X main.version=0.1.0" -o reamer.exe ./cmd/reamer
cd gui && go build -trimpath -ldflags "-s -w -H=windowsgui" .
```

## 测试

```
go test ./...        # core 规则引擎 + GUI 无头测试
cd gui && go test -run TestE2E -v   # 真实文件端到端(需要 M:\_tmp6\reamer\testfile)
```

CHANGELOG 见 [CHANGELOG.md](CHANGELOG.md)。

## 后续方向

- 更多元数据:MKV、EXIF
- 音频标签写入(xTools 还能写标签,这里暂只读)
- 设置页改进:AI 配置按已配置/未配置折叠、主色实时预览条
