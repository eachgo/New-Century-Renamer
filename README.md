# 跨向更名器 New Century Renamer

> 让你找回世纪初管理文件的感觉。

![界面截图](gui/audit-titlebar.png)

---

## 写在前面

这两年，CD 和磁带又回来了。

HI-FI 厂商开始出新的离线播放器，唱片店重新排队，有人开始把专辑一张张存进硬盘。我们忽然发现，当年那些被流媒体"优化"掉的东西——文件夹、专辑封面、一首一首整理好的曲目——原来是我们一直在想念的。

因为我们想回到那个**自己拥有音乐**的时代。

那个时代的音乐不是一串随时可能消失的链接，而是你硬盘上一个个具体的文件。你给它们建文件夹，起名字，按专辑、按年份、按心情排列。凌晨两点对着资源管理器，把 `track01.mp3` 改成真正的歌名——现在想来，那不叫麻烦，那叫拥有。

**跨向更名器**，就是为这种感觉而做的。

当然，它也不止于媒体文件。**它可以用来更名一切文件**：照片、文档、下载的压缩包、扫描件、课程资料、工作归档……任何一堆你想好好收起来的文件。而且，很方便。

---

## 它和别的改名软件不一样的地方

大多数改名工具把你变成一个正则表达式工程师。跨向更名器继承的是二十年前那个 xTools 的思路：

**把"改名字"当成一条可以反复走的流水线。**

```
添加文件 → 加步骤 → 加步骤 → 加步骤 → 预览 → 执行 → 后悔了回滚
```

每一步只做一件小事——转拼音、提取专辑信息、把非法字符换成全角、加上序号——但步骤可以任意排列、任意组合，而且**你随时能看见每个文件最后会变成什么**。

最关键的一条：**默认不动文件**。所有操作先出预览，逐行告诉你原名和新名；点了执行才真的改；改完不满意，一条命令整批回滚。

---

## 功能列表

- [x] **批量更名**：拖进来多少文件都行，逐个文件显示原名 → 新名对照
- [x] **多步规则管道**：十几条小规则自由串联，顺序即逻辑；可以筛选/排除一部分文件单独处理（`filter` / `unfilter`），也可以只改文件名或只改扩展名（`scope`）
- [x] **读取文件自带的"名字"**：EPUB 的标题作者、MP3 的 ID3v2 标签、FLAC/OGG 的 Vorbis 注释、MP4/M4V/MOV 的 ilst、PDF 的 `/Title` `/Author`——文件名直接来自专辑和书的元数据
- [x] **AI 按内容命名**：一个 `<AI:根据内容起个名字>` 变量，接任意 OpenAI 兼容接口，让它读完正文再给文件起名
- [x] **中文友好**：汉字转拼音、繁简互转（OpenCC）、假名转罗马音、全角半角处理
- [x] **非法字符改全角**：`/ \ : * ? " < > |` 不会粗暴删掉（删掉就丢信息了），而是换成 `／＼：＊？＂＜＞｜`
- [x] **CSV 清单即备份**：导出列表带每个文件的 sha256 指纹——文件名被改乱了，也能靠"内容没变"找回原名
- [x] **一键回滚**：操作日志记录了每一次改名，`reamer undo` 整批退回去
- [x] **纯 64 位 / ARM 的现代重制**：单个可执行文件，无运行时、无安装器、无 cgo
- [ ] 更多元数据格式：MKV、EXIF（见[路线图](#路线图)）
- [ ] 音频标签写入（目前只读不写）

---

## 安装

不提供安装器——它就是一个可执行文件，删掉即卸载。

### Windows

从 [Releases](https://github.com/OWNER/reamer/releases) 下载对应架构的两个文件，放在同一个文件夹里：

- `reamer-gui-windows-amd64.exe` —— 双击打开界面（改名叫 `跨向更名器.exe` 也行，界面不受影响）
- `reamer-windows-amd64.exe` —— 命令行版，适合写进批处理

ARM 版（如骁龙本）把 `amd64` 换成 `arm64`。

### macOS / Linux

一条命令装 CLI 到 `/usr/local/bin`：

```bash
curl -fsSL https://github.com/OWNER/reamer/releases/latest/download/install.sh | bash
```

`install.sh` 会按 `uname -s` / `uname -m` 挑对应的二进制，**并且用发布时生成的 `SHA256SUMS.txt` 校验**——下载坏了或者被中途替换过，脚本不会往下装。想先看看它做什么：

```bash
curl -fsSL https://github.com/OWNER/reamer/releases/latest/download/install.sh | less
```

不放心管道的话也可以手动来：

```bash
curl -LO https://github.com/OWNER/reamer/releases/latest/download/reamer-darwin-arm64
curl -LO https://github.com/OWNER/reamer/releases/latest/download/SHA256SUMS.txt
shasum -a 256 -c SHA256SUMS.txt --ignore-missing    # Linux 上用 sha256sum -c
chmod +x reamer-darwin-arm64 && mv reamer-darwin-arm64 /usr/local/bin/reamer
```

macOS 版 GUI（`reamer-gui-darwin-arm64`）同理，下载后 `chmod +x` 直接跑。

> macOS 上没签名没公证，首次打开会被 Gatekeeper 拦一下：右键点图标 → 打开，或者到"系统设置 → 隐私与安全性"里放行一次。

Linux 只发布 CLI：GUI 依赖桌面环境里的 WebKitGTK，各发行版差异太大，不提供通用二进制（想用界面可以自己编译，见[构建](#构建)）。

### 从源码构建

需要 Go 1.22 以上（`gui/` 里要求 1.27，老版本 Go 会经 `GOTOOLCHAIN=auto` 自动下载工具链）。

```bash
git clone https://github.com/OWNER/reamer && cd reamer
go build -o reamer ./cmd/reamer          # CLI
cd gui && go build .                      # GUI(纯 Go,无 cgo)
```

或者用自带脚本一次出全套（Windows PowerShell）：

```powershell
.\build.ps1          # 只构建当前平台
.\build.ps1 -All     # 交叉编译 windows/linux/darwin × amd64/arm64 到 dist\
```

---

## 使用方法

### 图形界面

三栏：左边是**功能目录**，中间是**任务步骤**，右边是**文件列表**。

1. 把文件或文件夹拖进来（也可以点"添加"）；
2. 在左边点功能，它会变成右边的一步——"转拼音""提取元数据""加序号"……想加几步加几步；
3. 点中某一步改它的参数，右边的列表**实时**跟着变，每个文件一行：原名 → 新名；
4. 看着对了，点执行；
5. 万一不对，点回滚。

几个不显眼但有用的地方：

- **文件可以最先加，步骤也可以最先加**，顺序不设教条；
- 步骤行可以 **Ctrl+点击多选**、拖动整组重排；标题行的"多选"按钮打开后每行出现圆点，触摸屏上也能点；
- 参数有问题或目标冲突时，**红字原因直接写在文件卡片上**，不会只说一句"失败"；
- 只有 AI 那一步需要你显式点"预览"（因为它要联网、要花钱），其余步骤都是即时预览；
- 同一时间只允许开一个窗口——两个实例会互相覆盖操作日志。

### 命令行

```bash
# 预览（默认，绝不改文件）
reamer -r pinyin -r sanitize 某目录/

# 用专辑标签重命名，非法字符自动转全角
reamer -r 'template=<META:artist> - <META:title><.EXT>' -r 'seq=1,2' 音乐/

# 确认无误，加 -apply 才真的执行
reamer -apply -r pinyin 某目录/

# 后悔了
reamer undo -journal .reamer-journal.json
```

输出的样子：

```
* 音乐/track01.mp3  =>  Awich - THE UNION01.mp3
* 音乐/track02.mp3  =>  Awich - THE UNION02.mp3
! 音乐/track03.mp3  =>  同一个名字.mp3
    错误: 新名字与其他 1 个文件相同,批量执行会互相覆盖
3 个文件, 2 个将更名, 1 个有问题
(预览模式, 加 -apply 执行)
```

`*` 是会改的，`!` 是不会改的——**预览里标了错的文件，执行时会被跳过，不会改一半**。

### 规则一览

规则按 `-r` 出现的顺序依次应用，可以重复用同一个规则。

| 规则 | 作用 | 例子 |
|---|---|---|
| `template=T` | 用模板重建文件名，T 里可用变量 | `template=<META:artist> - <META:title><.EXT>` |
| `replace=OLD,NEW[,regex]` | 替换；带 `regex` 时 OLD 是正则，NEW 可用 `$1`；参数里的 `,` 和 `\` 用 `\` 转义 | `replace=（,(` |
| `seq=START[,WIDTH[,STEP]]` | 追加序号，按宽度补零；也支持键值写法 `seq=start:1,width:2,step:1`，另有 `base:letter`（字母编号）、`group:ext\|dir`（分组重新计数）、`tpl:……`（自定义拼法，其中 `<N>` 是序号文本） | `seq=1,2` → `01`、`02` |
| `pinyin` | 汉字转拼音，全角字母数字转半角 | `张三丰.txt` → `zhangsanfeng.txt` |
| `sanitize` | Windows 保留字符转全角，去控制符/首尾空格/结尾点 | `a:b` → `a：b` |
| `cc=s2t` / `cc=t2s` | 繁简互转（OpenCC 词典） | `蓝莲花` → `藍蓮花` |
| `romaji` | 假名转罗马音，处理拗音/促音/长音 | `るろうに剣心` → `rurounikenshin` |
| `case=upper\|lower\|title` | 大小写 | `the union` → `THE UNION` |
| `insert=POS,TEXT` | 在 POS 处插入 | |
| `delete=POS,LEN` | 删除字符，负数 POS 从末尾数 | |
| `truncate=MAXLEN` | 截断文件名（不含扩展名） | |
| `ext=NEWEXT` | 改扩展名，留空则去掉扩展名 | |
| `filter=模式[,invert]` / `unfilter` | 只让名字匹配通配符的文件走后面的步骤（加 `invert` 反向） | `filter=*.mp3` |
| `scope=name\|ext` | 后面的步骤只作用于文件名或只作用于扩展名 | |

单条规则也可以直接加前缀只作用于部分文件，写法是 `if:模式>>规则`：

```bash
reamer -r 'if:*.mp3>>pinyin' 某目录/    # 只有 mp3 转拼音，其他文件原样通过
```

### 变量

在 `template` 和 `replace` 的 NEW 里展开：

| 变量 | 展开成 |
|---|---|
| `<NAME>` | 当前文件名（不含扩展名） |
| `<EXT>` / `<.EXT>` | 扩展名（带点） |
| `<DIR>` | 所在目录 |
| `<CRC32>` | 文件内容 CRC32 |
| `<SEQ>` | 当前文件在处理队列里的序号 |
| `<Pinyin:NAME>` | 变量内容转拼音 |
| `<META:title>`、`<META:author>`、`<META:album>`…… | 从 EPUB / MP3 / FLAC / OGG / MP4 / PDF 里读到的元数据 |
| `<AI:指示>` | 调用 AI 按指示生成名字片段 |

元数据全部为空时，模板会**保持原名**，不会产出 `" - .ext"` 这种残骸。

### AI 命名

任意 OpenAI 兼容接口都行，用环境变量配置（GUI 里也可以在设置中填，优先级更高）：

```bash
export REAMER_AI_BASE=https://api.deepseek.com/v1
export REAMER_AI_KEY=sk-...
export REAMER_AI_MODEL=deepseek-chat

reamer -r 'template=<AI:根据内容起一个简短描述性文件名>_<SEQ><.EXT>' -r sanitize 笔记/
```

它会读文件内容（文本、PDF 的文本层等）再生成名字。**这一步要联网、要花钱，所以只在你点"预览"或运行命令时发生一次**，之后结果会缓存，反复改别的参数不会重复请求。

### CSV 清单：改乱了也能找回来

```bash
reamer export -o list.csv 某目录/     # 导出：sha256 + 路径 + 原名 + 新名
# 在 Excel/编辑器里改 new_name 列，然后导回
reamer import list.csv               # 先预览
reamer import -apply list.csv        # 执行
```

关键在于那一列 sha256：**文件名改乱了，内容没变**，所以每个文件都能按指纹认领自己的原名，内容完全相同的多个文件也能各自对上号。这是独立于操作日志的第二条退路。

### 预设

一串规则存成 JSON 就能分享：

```json
{
  "rules": [
    "template=<META:author> - <META:title><.EXT>",
    "sanitize",
    "truncate=120"
  ]
}
```

```bash
reamer -p presets/book-meta.json 书库/
```

仓库里的 [`presets/`](presets/) 放了一些现成的。

---

## 几条设计上的取舍

写下来，免得以后自己忘了为什么这么做。

- **默认 dry-run**：`-apply` 之前一个字节都不会动。改名器离用户的数据太近了，多一次确认不算麻烦。
- **非法字符改全角而不是删掉**：`AC/DC` 删掉斜杠变成 `ACDC`，信息就没了；变成 `AC／DC` 还认得出来。
- **冲突在预览阶段就标出来**：两个文件改成同一个名字、目标已经被占用——这些在执行前逐个文件标红，而不是执行到一半才发现。
- **只改大小写不做"目标已存在"报错**：Windows/macOS 的文件系统不区分大小写，`the union.mp3` 改成 `THE UNION.mp3` 目标是同一个文件。这种情况用 `os.SameFile` 认出来，执行时走临时名两步改名，回滚也照样有效。
- **纯 Go、无 cgo**：一个二进制文件，交叉编译不求人，老机器上也不会缺 DLL。

---

## 构建

```powershell
.\build.ps1          # 当前平台：CLI + GUI（GUI 用 -H=windowsgui，双击无黑框）
.\build.ps1 -All     # 交叉编译 windows/linux/darwin 的 amd64+arm64 到 dist\
```

手工等价：

```bash
go build -trimpath -ldflags "-s -w -X main.version=0.1.0" -o reamer ./cmd/reamer
cd gui && go build -trimpath -ldflags "-s -w -H=windowsgui" .
```

打 tag 推送（`v0.1.0`）会触发 Release 工作流，自动交叉编译并按上面的命名发布。

## 测试

```bash
go test ./...                              # 规则引擎
cd gui && go test -skip '^TestE2E' .       # 界面无头测试
cd gui && go test -run TestE2E -v .        # 真实文件端到端(需要 testfile/ 素材)
```

端到端测试会真的建目录、真的改名、真的回滚，再逐个核对磁盘上的结果——其中包括 `the union.mp3` → `THE UNION.MP3` 这类大小写改名。

`testfile/` 是本地素材目录（约 200MB 的音频/PDF），**不入库**；没有它时 E2E 测试会自己跳过。

## 路线图

- 更多元数据：MKV、EXIF
- 音频标签写入（xTools 还能写标签，这里暂只读）
- 设置页改进：AI 配置按已配置/未配置折叠、主色实时预览条

## 致谢

- 设计思路来自二十年前的 **xTools 批量更名器**——多步管道、元变量、谨慎预览，这些巧思都值得留下来；
- 界面基于 [mygo](https://github.com/egoist/mygo)，纯 Go 无 cgo 的桌面框架；
- 拼音 [go-pinyin](https://github.com/mozillazg/go-pinyin)，繁简 [OpenCC](https://github.com/longbridgeapp/opencc)，图标 [Flowbite](https://flowbite.com)(MIT)。

## 许可证

<!-- 待定:发布前把下面一行换成实际选择(MIT / Apache-2.0 / 其它),并补上 LICENSE 文件。
     GitHub 只有在仓库里有 LICENSE 文件时才会在页面上显示许可证徽标。 -->

尚未选定。在确定之前,代码按"保留所有权利"处理;确定后会补上 `LICENSE` 文件并更新本节。
