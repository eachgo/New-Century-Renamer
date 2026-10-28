# 跨向更名器 New Century Renamer

> 让你找回世纪初管理文件的感觉。

![界面截图](docs/screenshot.png)

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

大多数改名工具把你变成一个正则表达式工程师。跨向更名器走的是另一条路：

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
- [x] **一键回滚**：操作日志记录了每一次改名，`ncreamer undo` 整批退回去
- [x] **纯 64 位 / ARM 的现代重制**：单个可执行文件，无安装器、无 cgo，交叉编译一份全平台通用（GUI 是原生界面，由程序自己在 GPU 上绘制，不依赖任何浏览器内核）
- [ ] 更多元数据格式：MKV、EXIF（见[路线图](#路线图)）
- [ ] 音频标签写入（目前只读不写）

---

## 安装

不提供安装器——它就是一个可执行文件，删掉即卸载。

### Windows

从 [Releases](https://github.com/eachgo/New-Century-Renamer/releases) 下载对应架构的两个文件，放在同一个文件夹里：

- `ncreamer-gui-windows-amd64.exe` —— 双击打开界面（改名叫 `跨向更名器.exe` 也行，界面不受影响）
- `ncreamer-windows-amd64.exe` —— 命令行版，适合写进批处理

ARM 版（如骁龙本）把 `amd64` 换成 `arm64`。

### macOS / Linux

一条命令装 CLI 到 `/usr/local/bin`：

```bash
curl -fsSL https://github.com/eachgo/New-Century-Renamer/releases/latest/download/install.sh | bash
```

`install.sh` 会按 `uname -s` / `uname -m` 挑对应的二进制，**并且用发布时生成的 `SHA256SUMS.txt` 校验**——下载坏了或者被中途替换过，脚本不会往下装。想先看看它做什么：

```bash
curl -fsSL https://github.com/eachgo/New-Century-Renamer/releases/latest/download/install.sh | less
```

不放心管道的话也可以手动来：

```bash
curl -LO https://github.com/eachgo/New-Century-Renamer/releases/latest/download/ncreamer-darwin-arm64
curl -LO https://github.com/eachgo/New-Century-Renamer/releases/latest/download/SHA256SUMS.txt
shasum -a 256 -c SHA256SUMS.txt --ignore-missing    # Linux 上用 sha256sum -c
chmod +x ncreamer-darwin-arm64 && mv ncreamer-darwin-arm64 /usr/local/bin/ncreamer
```

macOS 版 GUI（`ncreamer-gui-darwin-arm64`）同理，下载后 `chmod +x` 直接跑。

> macOS 上没签名没公证，首次打开会被 Gatekeeper 拦一下：右键点图标 → 打开，或者到"系统设置 → 隐私与安全性"里放行一次。

Linux 只发布 CLI：图形界面要连系统的显示服务（X11/Wayland）和 OpenGL，各发行版的桌面环境差异较大，暂不提供通用二进制（想用界面可以自己编译，见[构建](#构建)）。界面是程序自己在 GPU 上画的，不需要额外装浏览器内核。

### 从源码构建

需要 Go 1.22 以上（`gui/` 里要求 1.27，老版本 Go 会经 `GOTOOLCHAIN=auto` 自动下载工具链）。

```bash
git clone https://github.com/eachgo/New-Century-Renamer && cd New-Century-Renamer
go build -o ncreamer ./cmd/ncreamer         # CLI
cd gui && go build .                        # GUI(纯 Go,无 cgo)
```

> 命令行程序叫 `ncreamer`，界面程序叫 `ncreamer-gui`。
> 仓库名与 Go 模块名也都是 `ncreamer`——原来叫 `reamer`，重名风险太大，改名时一并处理了。

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
ncreamer -r pinyin -r sanitize 某目录/

# 用专辑标签重命名，非法字符自动转全角
ncreamer -r 'template=<META:artist> - <META:title><.EXT>' -r 'seq=1,2' 音乐/

# 确认无误，加 -apply 才真的执行
ncreamer -apply -r pinyin 某目录/

# 后悔了
ncreamer undo -journal .ncreamer-journal.json
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
ncreamer -r 'if:*.mp3>>pinyin' 某目录/    # 只有 mp3 转拼音，其他文件原样通过
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

**这是本地小模型最擅长的活，也是这个功能存在的理由。**

看文件名的机器只能猜；看内容的机器才叫得出名字。7B、9B 这一档的本地模型做不了写代码那种硬活,但"读一段文字、起一个标题"恰恰是它们做得好、而且不需要大模型的事:

- **PDF** —— 读文本层。中文 PDF 大多是 Type0/Identity-H 嵌入式字体,名字是一串 CID 码,我们按字体自带的 ToUnicode 表还原成正常汉字,标题页、目录、正文都读得到(不需要 OCR,也不需要联网);
- **EPUB** —— 读正文 XHTML 和版权页,作者、出版社、版次都出来了;
- **压缩包** —— 读内部文件列表,一堆 `IMG_0342.JPG` 也能认出是"2024 春 京都行";包里带的小说明文件(`.txt` / `.nfo`)也会捎上一段;
- **纯文本 / Markdown / 字幕** —— 直接读前若干字;
- **图片** —— 读 EXIF:拍摄时间、机型,不用再对着文件夹猜;
- **音视频 / 电子书** —— 顺带带上容器里的 title / artist / album / author。

读不出文字的场合(PDF 是扫描件、压缩包加过密)会老实说"没有可用文本",只给文件名和大小——**宁可少给,也不编**。

**隐私优先,连得上才连。** AI 那一步可以指向任意 OpenAI 兼容接口,包括你自己机器上跑的 Ollama / llama.cpp / LM Studio:

```bash
# 全本地,一个字节都不出网
export NCREAMER_AI_BASE=http://127.0.0.1:11434/v1
export NCREAMER_AI_MODEL=qwen2.5:7b
ncreamer -r 'template=<AI:根据内容起一个简短描述性文件名>_<SEQ><.EXT>' -r sanitize 笔记/
```

本地小模型读完一整份 PDF 再起名,慢起来要一两分钟是常事。默认给足 **300 秒**(5 分钟)的等待时间;跑更慢的模型(比如 32B、或在低速机器上)可以调长:

```bash
export NCREAMER_AI_TIMEOUT=600   # 单位:秒
```

用远程 API 也行(GUI 设置里填,优先级高于环境变量),但**默认设计是把你家硬盘上的东西留在家里的机器上**——文件内容只在你开启那一刻发一次,结果缓存下来,反复改别的参数不会重复请求、也不会重复上传。

> 这一步是唯一会联网的步骤,所以只有你显式点"预览"或运行命令时才会发生一次。

### CSV 清单：改乱了也能找回来

```bash
ncreamer export -o list.csv 某目录/     # 导出：sha256 + 路径 + 原名 + 新名
# 在 Excel/编辑器里改 new_name 列，然后导回
ncreamer import list.csv               # 先预览
ncreamer import -apply list.csv        # 执行
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
ncreamer -p presets/book-meta.json 书库/
```

仓库里的 [`presets/`](presets/) 放了一些现成的。

---

## 构建

```powershell
.\build.ps1          # 当前平台：CLI + GUI（GUI 用 -H=windowsgui，双击无黑框）
.\build.ps1 -All     # 交叉编译 windows/linux/darwin 的 amd64+arm64 到 dist\
```

手工等价：

```bash
go build -trimpath -ldflags "-s -w -X main.version=0.1.0" -o ncreamer ./cmd/ncreamer
cd gui && go build -trimpath -ldflags "-s -w -H=windowsgui" .
```

打 tag 推送（`v0.1.0`）会触发 Release 工作流，自动交叉编译并按上面的命名发布。

## 测试

```bash
go test ./...                              # 规则引擎
cd gui && go test -skip '^TestE2E' .       # 界面无头测试
cd gui && go test -run TestE2E -v .        # 真实文件端到端
```

前两条在任何机器上都能跑。端到端测试会真的建目录、真的改名、真的回滚，再逐个核对磁盘上的结果——其中包括 `the union.mp3` → `THE UNION.MP3` 这类大小写改名；它需要一份带音频/PDF 的素材目录，没有就自动跳过。

## 路线图

- 更多元数据：MKV
- 设置页改进：AI 配置按已配置/未配置折叠、主色实时预览条

## 致谢

- 界面基于 [mygo](https://github.com/egoist/mygo)，纯 Go 无 cgo 的桌面框架——我们用它的原生 UI（自己绘制在 GPU 上，无 HTML/JS、不占用浏览器内核），所以启动快、内存开销小，也不必额外装浏览器运行时；
- 拼音 [go-pinyin](https://github.com/mozillazg/go-pinyin)，繁简 [OpenCC](https://github.com/longbridgeapp/opencc)，图标 [Flowbite](https://flowbite.com)(MIT)。

## 许可证

[MIT](LICENSE)。

用这个软件改名是你自己的事：改名操作只发生在本机磁盘上，不会向任何服务器回报，也没有遥测。
