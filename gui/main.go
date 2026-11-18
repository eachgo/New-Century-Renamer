package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"ncreamer/core"
)

// fileInfo is one file in the list, with the facts the table shows.
type fileInfo struct {
	path     string
	size     int64
	modTime  time.Time
	typ      string // by extension
	realType string // sniffed from content; differs → warning chip
	// group keys the file to a tree node when it was dragged in as part of a
	// folder; the tree header shows the folder's name. Empty = a loose file
	// added on its own (shown without a group header).
	group string
	// order is the position the file had when it entered the list. It outlives
	// sorting and dragging, which is what makes "按添加顺序" a way back instead
	// of a one-way trip: after sorting by size there is nothing else left to
	// reconstruct the order the user built the batch in.
	order int
}

// app is the state the window shows: three panes — catalog, task steps,
// file preview.
type app struct {
	win       *mygo.Window
	files     []fileInfo
	ops       []core.RenameOp // preview or apply result, aligned with files
	status    string
	busy      bool
	busyAI    bool      // busy because an AI (network) call is in flight — show spinner
	busyStart time.Time // when the current busy op began (for the elapsed timer)
	applied   bool      // true after 执行/撤销, until steps or files change
	// appliedAt/appliedCount 是"刚才这一批"的时间与成功数,执行完的横幅用它。
	// 一个 applied 布尔量说明不了"什么时候改了哪一批"。
	appliedAt    time.Time
	appliedCount int
	// busyDone/busyTotal 是批量执行走到第几个:500 个文件时一个转圈
	// 说明不了还要多久。
	busyDone, busyTotal int
	// aiDone/aiTotal 是联网预取走到第几个。单独一组而不复用 busyDone:
	// 预取的回调来自多个 goroutine,写的时候要加锁;而执行那组只在主线程
	// 里写,混在一起会让加锁范围变得难以判断。
	aiMu    sync.Mutex
	aiDone  int
	aiTotal int
	// flashUntil 是"刚改名的那几行"高亮结束的时刻。执行成功是当天最重要
	// 的事,却只有一个静态的结果词——让改过的行亮一下再淡出,是唯一一处
	// 值得动的动效。
	flashUntil time.Time
	lastSpec   string
	// planSpec 是"算出现在这份 a.ops 时,步骤长什么样"。
	//
	// 含 <AI:> 的步骤不能实时重算(会联网),所以改了模板之后 a.ops 会停在
	// 旧计划上;而"执行更名"只看 a.ops 有没有变更,于是它照样能点——按下
	// 去执行的是改之前的计划。把 spec 记下来,和当前 spec 一比就知道这份
	// 计划是不是已经过期;过期时主按钮置灰并说清"先点预览"。
	planSpec string
	// aiCancel 是"这一轮联网命名"的取消句柄。有它才能给「停止」:
	// 一次 300 个文件的批量配上慢模型是十几分钟,而窗口只能干等——
	// 用户唯一的出路是杀进程,那会把已经算出来的结果一起丢掉。
	aiCancel context.CancelFunc
	// aiStopped 记录"这一轮是被用户停掉的"。取消会让还没发的请求
	// 以 errAICanceled 结束,但底栏不该把整批文件标成红字:用户自己
	// 按的停止,不是接口坏了。
	aiStopped bool
	// planFiles 是算计划时的文件数。计划是按"这些文件"算出来的:含 AI 时
	// 加入新文件不会自动重算,计划就只覆盖老的那几个,而列表里已经多了
	// 一行没有新名字的文件。空串/零值表示"还没有计划",不算过期。
	planFiles int

	settings      Settings
	settingsOpen  bool
	helpOpen      bool
	helpShowFlows bool // 帮助页里流程图默认收起,点按钮才展开
	helpScroll    ui.ScrollState
	setScroll     ui.ScrollState      // 设置对话框表单的滚动位置
	setBody       ui.Handle           // 设置表单滚动区,用来判断要不要淡出底边
	setBodyRect   ui.Rect             // 上一帧的滚动视口位置,测试用来量淡出
	settingsDraft Settings            // 设置对话框里正在编辑的副本;取消即丢弃
	extColors     map[string]ui.Color // per-extension chip colors (main-thread)
	seedFollow    bool                // theme seed follows the desktop accent
	seedPick      ui.Color            // theme seed when not following
	confirmClear  bool                // 清空列表 needs a second click
	confirmAt     time.Time           // 二次确认的截止时间:挂太久自动收回
	// stepUndo 是"从流水线里拿掉的步骤"的栈,最近一批在末尾。
	//
	// 删除单个步骤原来和"清空列表"一样要点两下,但两者差着数量级:
	// 清空是"一整批文件从列表里消失",删一步是"从管道里拿掉一条,想
	// 加回来点一下就行"。同样的红色二次确认套在两件事上,用久了会变成
	// "看到红按钮就点第二下"——防错反而失效。所以低危的这个改成可撤销:
	// 删完在步骤区顶部留一条「已删除「X」 撤回」,几秒后自己消失。
	// 高危的两个(撤销、清空列表)仍然保留二次确认。
	//
	// 槽改成栈:原来只有一个槽,连着删两步第一步就永远回不来——而
	// "删错了"从来不是一次性的,人多半是连删几下之后才发现删过头。
	stepUndo []stepUndo
	// clearedFiles 是"刚被清空的那一整个文件清单"。
	//
	// 清空列表原来只有"再点一下"的确认,点完就没了:31 个文件得重新
	// 拖一遍,连添加顺序和当时的排序也回不来。删一步能撤回,清空一整
	// 表更该能——保存的是整份快照,不是"再猜一遍刚才有什么"。
	clearedFiles  []fileInfo
	clearedOps    []core.RenameOp
	clearedOrder  int
	clearedSort   string
	clearedDesc   bool
	clearedAt     time.Time
	confirmChosen bool // 删除所选 needs a second click
	confirmUndo   bool // 撤销上次 needs a second click
	// confirmApply 是"执行更名"的二次确认。它是全产品唯一不可逆的
	// 写盘动作,却原来反而是唯一没有确认的——清空列表和撤销都要点两下。
	// 守卫装错了地方:越是改磁盘的越该拦。
	confirmApply      bool
	saveErr           string // settings dialog save error, shown in-dialog
	aiTimeoutText     string // 设置里"等待秒数"输入框的文本缓冲
	aiConcurrencyText string // 设置里"并发路数"输入框的文本缓冲
	aiPromptText      string // 设置里"默认提示词"多行输入框的缓冲
	keyShown          bool   // 设置里 API Key 是否明文显示(默认掩码)

	steps   []guiRule // ordered task steps
	stepSel int       // selected step, -1 = none
	// stepScrollTo 是"下一帧要把这一行滚进视野",-1 = 不用滚。
	//
	// 加一步之后总要能看见它:21 步时再加第 22 步,stepSel 指到 21,
	// 但列表已经滚不动了的话那一行就落在视口外(实测 bounds 高度 0)。
	// ScrollIntoView 只能在 view 里调(它在布局完成后才生效),所以把
	// 目标行排到下一帧的 stepsView 里执行。
	stepScrollTo int
	// revealLeft 让"把这一行滚进视野"多求几帧。ScrollIntoView 用的是
	// 上一帧的布局,只求一次常常只露出一半(实测第一次后行仍被 Clip
	// 在下边缘,h=0)。连续求几帧直到布局跟上。
	revealLeft int
	stepList   ui.ListState
	stepUID    int               // 自增步骤身份
	chosen     ui.Selection[int] // multi-selected step uids
	pickMode   bool              // 显示行内多选圆点(触摸入口)
	pickOpen   bool              // 设置里的主色选择器是否展开
	dialogs    dialogHooks       // 可注入的对话框(测试用; nil=真实对话框)
	palQuery   string            // 目录过滤框的内容;空=不过滤
	// 目录里被折起来的分组。只记"用户手动改过的那几组";没记过的按
	// defaultOpenGroups 算,这样默认状态和用户改过的状态互不覆盖。
	folded map[string]bool
	// pinned 是左栏顶部"常用"组的条目 id,与 settings.Pinned 同步。
	// 单独存一份是因为目录每帧重建,而设置只在对话框里改;两处各读
	// 各的会有一帧延迟,拖动排序时会看到条目跳一下。
	pinned    []string
	palScroll ui.ScrollState
	catScroll ui.Handle // 目录滚动位置;用来判断"下面还有没有"
	catRect   ui.Rect   // 上一帧的目录视口位置,测试用来量底缘渐隐

	splitL, splitM float32 // pane divider positions
	// splitDragging 记着"这一帧之前缝还被抓着"。松手那一刻才落盘,
	// 拖动途中每帧写文件没有意义。
	splitDragging bool
	table         ui.ListState
	fileSel       int // 文件列表里选中的行(rows 下标,不是 a.files 下标),-1 = 没选
	// nextOrder 是下一个 fileInfo.order 的值,单调递增;清空列表时归零。
	nextOrder int
	// sortBy 是当前的排序依据:"" = 添加顺序,"name"/"date"/"size"。
	// sortDesc 只对后三者有意义。
	sortBy   string
	sortDesc bool
	// pendingSort 表示"排序依据变了,下一帧开头重排"。和 pendingMove
	// 同理:菜单点击发生在构建途中,当场 sort 会让正在逐行构建的列表
	// 与数据错位。
	pendingSort bool
	// pendingMove 是拖动重排的结果 {from, before},nil 表示没有。
	// 排到下一帧开头再动 a.files:重排是在构建这一帧的途中发生的,
	// 当场挪动会让正在逐行构建的列表与数据错位——同一帧里后面的行
	// 会指向刚被换走的那个文件。
	pendingMove *[2]int
	// pendingRemove 是"下一帧要移除的文件下标 + 1",0 表示没有。
	// 不能在构建这一帧的途中直接删 a.files:列表正按旧长度逐行构建,
	// 删掉一个会让后面每一行的下标越界(实测直接 panic)。
	pendingRemove int
	// pendingDrop 是拖放区收到的路径,排到下一帧开头再加——理由同上:
	// 拖放是在构建这一帧的途中发现的,当场 append 会让正在逐行构建的
	// 列表长度对不上。
	pendingDrop []string
	// fileFocus 记着"上一帧结束时文件列表是否拿着焦点",见 handleShortcuts
	fileFocus bool
	// dragOver 是"此刻有文件正被拖在这个窗口上方"。整窗的根节点是拖放
	// 落点(见 view),它每帧把 FileDragOver 记在这里;空态的虚线框只在
	// 它为真时画出来——平时摆一个虚线框在空列表里,等于把"可以往这儿拖"
	// 说成"你正在往这儿拖"。
	dragOver bool
	// previewErr 是"这一轮预览整体算不出来"的原因(规则本身有问题,
	// 比如模板括号不配对)。它和 op.Err 不是一回事:op.Err 是"某个文件
	// 改不成",previewErr 是"一个都算不出来"。
	//
	// 以前这条分支只把 a.ops 置空,于是文件行还在、新名列和结果列整片
	// 消失,看起来像"预览全没了"却又不说是为什么。现在留着原因,
	// 右栏顶部明说,每行也标一句"等修好步骤"。
	previewErr string
	// previewErrStep 是 previewErr 指向的那一步(下标),-1 表示认不出
	// 是哪一步。有它才能给一个「去修这一步」的按钮:只说"第 3 步有
	// 问题",用户还得自己在列表里数到第三行。
	previewErrStep int
	// importPending 是"已经读进内存、还没落盘"的导入设置。
	// 导入是整份替换:文件里没写的字段会回到默认值,API Key 会被清空,
	// 日志路径会被改掉——这些都不该在点下"导入设置…"的那一下发生。
	// 先拿在手里,让用户看清要换掉什么,再决定要不要写盘。
	importPending *Settings
	importPath    string
	importOpen    bool
	// flowOpen 是"流程"弹窗。它替代了原来只在空态出现的四张意图卡:
	// 排到一半想换做法时,那四张卡已经不在屏幕上了。
	flowOpen   bool
	flowName   string
	flowScroll ui.ScrollState
	// confirmFlow / pendingFlow 是"套用会替换现有步骤"的二次确认。
	pendingFlow flow
	confirmFlow bool
	// recoveredSteps 是刚被流程套用替换掉的旧步骤,供「撤回」整批放回。
	// 与 stepUndo(单步可撤回)同一条思路:删一步能撤回,换掉一整条
	// 流水线凭什么不能。
	recoveredSteps []guiRule
	recoveredAt    time.Time
}

// dragOverProbe 让测试造出"文件正被拖在窗口上方"这一帧。Tester 没有只发
// FileDragOver 而不松手的入口(DropFiles 两件事一起发,而 fileDrop 会先把
// dropOver 清零),所以真实悬停状态在无头测试里造不出来。生产路径永远是 nil。
var dragOverProbe func() bool

func (a *app) view(c *ui.Context) {
	dt := a.currentTheme()
	if a.isDark() {
		c.SetTheme(dt.dark)
	} else {
		c.SetTheme(dt.light)
	}
	// 逃生键:模式类状态必须能被一键解除,用户不会被困在里面。
	// 导入确认排在最前:它盖在设置面板上面,按 Esc 该先关最上面那层,
	// 而不是连下面那个一起关掉。
	if a.importOpen && c.Shortcut(0, ui.KeyEscape) {
		a.importPending, a.importPath, a.importOpen = nil, "", false
		a.status = "已取消导入，设置没有变化"
	} else if a.pickOpen && c.Shortcut(0, ui.KeyEscape) {
		a.pickOpen = false
	} else if a.pickMode && c.Shortcut(0, ui.KeyEscape) {
		a.pickMode = false
		a.chosen.Clear()
	} else if a.flowOpen && c.Shortcut(0, ui.KeyEscape) {
		a.flowOpen = false
	} else if a.helpOpen && c.Shortcut(0, ui.KeyEscape) {
		a.helpOpen = false
	} else if a.settingsOpen && c.Shortcut(0, ui.KeyEscape) {
		a.closeSettings(false)
	} else if strings.TrimSpace(a.palQuery) != "" && c.Shortcut(0, ui.KeyEscape) {
		// 搜索框的 Esc:清空并复位折叠状态。放在整条逃生链最后——有弹窗
		// 开着时,Esc 先关那个弹窗,不该顺手把搜索也清了。
		a.palQuery = ""
		a.folded = nil
	}
	a.handleShortcuts(c)
	// 上一帧排下的移除,等这一帧开头再动 a.files
	if a.pendingRemove > 0 {
		i := a.pendingRemove - 1
		a.pendingRemove = 0
		a.removeFile(i)
	}
	a.drainPendingDrop()
	// 排序与拖动重排也排到帧首:两者都在构建这一帧的途中被触发,
	// 当场重排 a.files 会让已经建好的行指向错位的文件。
	if a.pendingSort {
		a.pendingSort = false
		sortFiles(a.files, a.sortBy, a.sortDesc)
		a.preview()
	}
	if mv := a.pendingMove; mv != nil {
		a.pendingMove = nil
		a.files = moveFile(a.files, mv[0], mv[1])
		a.preview()
	}
	// live preview on any change (AI steps need the explicit button)
	if spec := a.specKey(); spec != a.lastSpec {
		if !strings.Contains(spec, "<AI:") {
			a.preview()
		}
		a.lastSpec = spec
	}
	if a.splitL == 0 {
		a.splitL, a.splitM = 220, 330
		// 上一次运行拖过的位置优先于默认值:分栏宽度是"调过一次就不想
		// 再调"的东西,而它原来只活在内存里。
		if a.settings.SplitL > 0 {
			a.splitL = a.settings.SplitL
		}
		if a.settings.SplitM > 0 {
			a.splitM = a.settings.SplitM
		}
	}
	// 顶部自绘标题栏:与窗口背景一体,整条可拖动;右侧留窗口按钮的位置。
	// 窗口用 TitleBarHidden 保留按钮/阴影/圆角,系统标题栏则不再出现
	bar := c.TitleBar()
	titleH := float32(38)
	if bar.Height > titleH {
		titleH = bar.Height
	}
	t := c.Theme()
	pt := a.currentTheme().panes(a.isDark())
	// 基底:整个窗口先铺 base 色;顶栏、底栏、左栏、四周留白都在这个底色上。
	//
	// 根节点同时是整窗的拖放落点。mygo 的窗口级 OnFileDrop 只在真正松手时
	// 触发,没有"文件正悬在窗口上方"的窗口级事件——要画"拖进来才出现的
	// 虚线框",就得有个每帧都被问到的元素。根节点正好:它覆盖整窗,任何
	// 位置的拖拽都会沿 hitChain 走到它(见 ui/input.go 的 appendHitChain)。
	// 注意不能用 PassThrough:那样它根本不进 rt.hits,也就永远不是落点。
	root := ui.Column(c).Fill().Background(pt.base)
	if paths := root.DroppedFiles(); len(paths) > 0 {
		a.pendingDrop = append(a.pendingDrop, paths...)
	}
	a.dragOver = root.FileDragOver()
	if dragOverProbe != nil && dragOverProbe() {
		a.dragOver = true
	}
	root.Children(func() {
		ui.Row(c).Height(titleH).Padding(0, float32(bar.Right), 0, float32(bar.Left)).DragWindow().
			AlignItems(ui.Center).Children(func() {
			brand := ui.Text(c, "New Century Renamer").Bold().FontSize(14).Padding(0, 8)
			if bar.Left == 0 {
				brand.PaddingStart(float32(bar.Left) + 8)
			}
			ui.Spacer(c)
			// 帮助入口:一整个目录的功能、模板变量表、NFO 编号、清单回填,
			// 全靠试是学不会的。放标题栏而不是目录里——它不属于管道。
			// (不写具体条数:目录会增删,注释里的数字迟早对不上。)
			help := ui.ButtonBase(c).Size(26, 26).Radius(26 / 2).Padding(0).Tooltip("帮助 F1")
			if help.Hovered() {
				help = help.Background(t.SurfaceHover)
			}
			help.Children(func() {
				ui.Text(c, "?").FontSize(14).TextColor(t.TextMuted)
			})
			if help.Clicked() {
				a.openHelp()
			}
			// 设置是应用级入口,不该藏在"往管道里加东西"的目录末尾——
			// 那里它排在 y≈1385,任何窗口高度都要先滚到底才能看见。
			// 桌面应用的约定是设置不进内容列表,所以放标题栏右侧。
			// 齿轮只有图标,没有文字:不显式给 Label 的话它的可读名是空串
			// (实测 Name="" Desc="Ctrl+,"),读屏念到它只会念出快捷键。
			gear := ui.ButtonBase(c).Size(26, 26).Radius(26 / 2).Padding(0).
				Label("设置").Tooltip(shortcutTip("Ctrl+,"))
			if gear.Hovered() {
				gear = gear.Background(t.SurfaceHover)
			}
			gear.Children(func() {
				if ic := opIcon("act.settings"); ic != nil {
					ui.Icon(c, ic).FontSize(14).TextColor(t.TextMuted)
				}
			})
			if gear.Clicked() {
				a.openSettings()
			}
			// 右上角是用户扫视时第一个落脚点,放"这批有多少文件、几个会改名"
			// 比放一个和窗口标题重复的名字有用。
			// 右侧让位只由这一行的 Padding(0, bar.Right, ...) 负责。
			// 以前这里又加了一次 bar.Right,让位被算两遍,于是齿轮和
			// 问号被推到窗口中间,和最小化/关闭之间空出一大段。
			ui.Text(c, a.batchSummary()).FontSize(12).TextColor(t.TextMuted).PaddingEnd(8)
		})
		// 只 Grow 不 Fill:Fill 会让 Split 想占满整列(含标题栏那几十像素),
		// 于是内容画到标题栏底下。Grow(1) 只吃标题栏之外的剩余空间。
		// Split 外面留一点内边距,让中/右两张圆角卡片浮在 base 底色上、
		// 四周露出基底(和左栏同色),读起来就是"卡片浮在底板上"。
		// 三栏之间留空缝,不画分隔线:见 panes.go 里 splitPanes 的注释——
		// mygo 的 Split 把线色写死成 Theme.Border,而 Border 要留给输入框
		// 的边框,深了以后那两条线比它们分隔的内容还显眼。
		// 中/右栏那一层要知道自己实际有多宽:外层减掉两侧内边距与缝、
		// 再减掉左栏,剩下的才是它能分的。用窗口宽度做夹紧会把右栏
		// 压到几十 DIP。
		winW, _ := c.Size()
		secondW := winW - 2*8 - 10 - a.splitL
		a.splitPanes(c, &a.splitL,
			func() { a.catalogView(c) },
			func() {
				// pad=0:内边距已经由外层加过,这一层再缩进就会让中/右
				// 两栏的卡片顶边比左栏低 8px,三栏不在同一条水平线上。
				a.splitPanesInset(c, &a.splitM, 0, secondW,
					func() { a.stepsView(c) },
					func() { a.filesView(c) })
			})
		// ③ 应用级底部状态栏:这里是"对整个软件"的操作与反馈,与右栏
		// "对文件列表"的操作分开。左边是警示/状态文字(多少将更名、报错),
		// 右边是主操作"执行更名"和"撤销上次"——执行是主按钮实心,撤销镂空。
		a.statusBar(c)
	})
	a.settingsDialog(c)
	a.importDialog(c)
	a.helpDialog(c)
	a.flowDialog(c)
	// 焦点要等这一帧构建完才有值:列表在构建时才把行标成可聚焦,
	// 所以记在帧尾,给下一帧的 handleShortcuts 用。
	a.fileFocus = a.table.FocusWithin(c)
	// 目录被裁掉的那半行字,要淡出而不是硬切在一条直线上。
	if a.catalogHasMore() {
		a.fadeEdge(c, a.catScroll, a.currentTheme().panes(a.isDark()).base)
	}
}

// fadeEdge 在滚动视口的底边盖一条渐变色,让被裁掉的那半行字淡出而不是
// 硬切在一条直线上——硬切看起来像渲染坏了。
//
// 必须走 Overlay:遮罩要画在文字之上,而正常流里的 Absolute 元素永远排在
// 文字下面(实测红色遮罩只在字缝里透出来)。Overlay 是窗口级的一层,盖在
// 所有内容之上,位置用 Handle.Bounds 把滚动视口的坐标换算到窗口坐标。
// to 是遮罩要淡入的颜色:它必须和视口底下那层底色一致,否则渐变到一半
// 就换了个颜色,看起来像一块脏印子而不是"内容还在继续"。
func (a *app) fadeEdge(c *ui.Context, h ui.Handle, to ui.Color) {
	r := h.Bounds(c)
	if r.W <= 0 || r.H <= 0 {
		return
	}
	const fadeH float32 = 20
	ui.Overlay(c, func() {
		ui.Box(c).Absolute().Left(r.X).Top(r.Y+r.H-fadeH).Width(r.W).Height(fadeH).
			Gradient(to.Alpha(0), to, 180)
	})
}

// statusBar 是整个应用底部的固定状态栏(所有窗格之下,横贯全宽)。
// 左侧状态/警示:多少将更名、几个有问题、正在等模型的秒数;右侧:主操作。
func (a *app) statusBar(c *ui.Context) {
	t := c.Theme()
	// 顶部一条极细的分隔线,和上方内容区分开。用 outline 而不是 Border:
	// Border 是输入框的边框色,为了看得见必须够深;装饰线用同一个色会
	// 比它分隔的内容还显眼。
	ui.Box(c).FillWidth().Height(1).Background(a.currentTheme().panes(a.isDark()).outline)
	ui.Row(c).FillWidth().Height(46).Padding(0, 12).Gap(10).AlignItems(ui.Center).
		Background(a.currentTheme().panes(a.isDark()).left).Children(func() {
		// ---- 左:状态 / 警示文字 ----
		if a.busy {
			ui.Spinner(c)
			label := "处理中…"
			if a.busyAI {
				label = "正在联网命名，模型思考中…"
			}
			if !a.busyStart.IsZero() {
				if sec := int(time.Since(a.busyStart).Seconds()); sec >= 1 {
					label = fmt.Sprintf("%s 已等待 %d 秒", label, sec)
				}
			}
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
			// 批量执行时给一条真进度:一个转圈说不出"还要多久",
			// 而 500 个文件能跑十几秒。
			if a.busyTotal > 0 {
				frac := float32(a.busyDone) / float32(a.busyTotal)
				ui.Box(c).Width(120).Height(4).Radius(4 / 2).Background(neutralStroke(t)).Children(func() {
					ui.Box(c).FillWidth().Height(4).Radius(4 / 2).Background(t.Accent).Width(120 * frac)
				})
				ui.Text(c, fmt.Sprintf("%d/%d", a.busyDone, a.busyTotal)).FontSize(12).TextColor(t.TextMuted)
			} else if done, total := a.aiProgress(); total > 0 {
				// 联网预取是整次预览里唯一可能跑上几分钟的一段。没有这条
				// 进度,用户分不清"本地模型慢"和"网关已经死了"——而 3/40
				// 正是"再等等"与"直接关掉"之间的区别。
				frac := float32(done) / float32(total)
				ui.Box(c).Width(120).Height(4).Radius(4 / 2).Background(neutralStroke(t)).Children(func() {
					ui.Box(c).FillWidth().Height(4).Radius(4 / 2).Background(t.Accent).Width(120 * frac)
				})
				ui.Text(c, fmt.Sprintf("%d/%d", done, total)).FontSize(12).TextColor(t.TextMuted)
			}
		} else if a.status != "" {
			ui.Text(c, a.status).FontSize(12).TextColor(a.statusColor(t)).SingleLine().Ellipsis("…")
		} else {
			// 空态只陈述事实,不指挥用户点某个按钮——此刻"执行更名"是禁用的,
			// 让文案指向它等于把人引到一堵墙上。
			// 也不再说"先拖文件":中栏这时正摆着四张意图卡,两条路都通,
			// 底栏再指定一条就等于和旁边那条打架。
			ui.Text(c, "还没有文件").FontSize(12).TextColor(t.TextMuted)
		}
		ui.Spacer(c)
		// 联网命名可能跑上十几分钟,而这段时间窗口里没有任何出口:唯一
		// 的办法是杀进程,那会把已经算出来的结果一起丢掉。停止放在底栏
		// 右侧、紧挨主操作,是因为它此刻就是用户最想按的那一个。
		if a.busyAI && a.aiCancel != nil {
			if softButton(c, "停止", true).Radius(6).
				Tooltip("不再等剩下的请求。已经算出来的名字会保留，可以直接执行。").Clicked() {
				a.stopAI()
			}
		}
		// ---- 右:主操作(执行=主按钮,回滚=次要镂空) ----
		// "撤销上次"必须说清是"哪一次、几个文件"。只写三个字,用户不
		// 知道自己即将撤销的是刚才那批,还是上周某个目录里的一批。
		sum, canUndo := a.journalInfo()
		if a.confirmUndo && a.confirmStale() {
			a.dropConfirm()
		}
		if a.confirmUndo {
			label := fmt.Sprintf("确认撤销 %d 个?", len(sum.Names))
			confirm := ui.ButtonBase(c).Radius(6).Padding(8, 12).Background(t.Danger).TextColor(ui.RGB(255, 255, 255))
			if confirm.Hovered() {
				confirm = confirm.Background(t.Danger.Alpha(0.85))
			}
			confirm.Children(func() { ui.Text(c, label).SingleLine() })
			if confirm.Clicked() {
				a.dropConfirm()
				go a.undo()
			}
		} else {
			undoLabel := "撤销上次"
			if canUndo {
				undoLabel = fmt.Sprintf("撤销上次（%d 个）", len(sum.Names))
			}
			undoBtn := outlinedButton(c, undoLabel, !a.busy && canUndo).Tooltip(a.undoTip(sum))
			if undoBtn.Clicked() {
				// 回滚会动磁盘,和"清空列表"一样属于"再点一下才真做"
				a.armConfirm("undo")
			}
		}
		// 计划过期时"执行更名"必须点不了:含 AI 的步骤改动不触发实时预览,
		// a.ops 还停在改之前那份计划上,能点就等于按旧计划改磁盘。
		stale := a.planStale()
		applyEnabled := !a.busy && !stale && a.hasChanges()
		// 执行前把 AI 请求量说出来。一次 300 个文件的批量,配上 4 路并发
		// 和慢模型,是十几分钟的事;等到点下去才发现,用户能做的只有
		// 干等或者中断。这条数字在点之前就有,所以放在按钮左边。
		//
		// 只在真的有 AI 步骤且要动磁盘时才占位:没配 AI 的批量里凭空
		// 出现"0 次请求"只会让人以为哪里坏了。
		if applyEnabled {
			if n := a.aiCallCount(); n > 0 {
				ui.Text(c, fmt.Sprintf("将调用 AI %d 次", n)).FontSize(12).
					TextColor(t.TextMuted).SingleLine().
					Tooltip(fmt.Sprintf("这次执行会向 AI 接口发 %d 次请求（%d 路并发）。\n本地模型可能要等一会儿，云端按次计费。", n, core.AIConcurrency()))
			}
		}
		// 过期时把"下一步该做什么"写在按钮左边。按钮自己是灰的,而灰按钮
		// 上的 Tooltip 在 mygo 里不弹(禁用元素不收悬停),所以提示不能只挂
		// 在按钮上,必须有一句摆在外面。
		if why := a.planStaleWhy(); why != "" && !a.busy {
			ui.Text(c, why).FontSize(12).TextColor(t.TextMuted).SingleLine().
				Tooltip("含 AI 步骤时不自动重算（会联网，一改就发请求）。点右边「预览」按当前步骤重新算一遍，再执行。")
		}
		// 执行更名:动磁盘的一次操作,和"清空列表""撤销上次"同一套语言——
		// 第一下只挂确认态,第二下才真改。确认态用实心主色而不是红:它
		// 不是危险动作,只是不可逆;红色留给"会删东西"的。
		//
		// 确认态挂着的时候步骤被改了,也要收回:否则第二下点下去,确认的
		// 是"刚才那份计划",执行的却是别的。
		if a.confirmApply && (a.confirmStale() || stale) {
			a.dropConfirm()
		}
		if a.confirmApply {
			confirm := ui.ButtonBase(c).Radius(6).Padding(8, 12).
				Background(t.Accent).TextColor(t.AccentText)
			if confirm.Hovered() {
				confirm = confirm.Background(t.AccentHover)
			}
			confirm.Label("确认执行")
			confirm.Children(func() { ui.Text(c, "确认执行?").SingleLine() })
			if confirm.Clicked() {
				a.dropConfirm()
				go a.apply()
			}
		} else {
			applyBtn := primaryButton(c, "执行更名", applyEnabled).Tooltip(shortcutTip("Ctrl+Enter"))
			if applyEnabled && applyBtn.Clicked() {
				a.armConfirm("apply")
			}
		}
	})
}

// intentCards fills the empty middle column with four "what am I trying to
// do" cards. The catalog next door answers "which rule do I need" — 32
// entries, in pipeline vocabulary. Someone who just wants "歌手 - 歌名"
// should not have to work out that this means template=<META:artist> …
//
// Each card inserts a ready-made pipeline that stays editable, so this is a
// starting point and not a mode: there is no "wizard" to escape from.
func (a *app) intentCards(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).FillWidth().Gap(8).Padding(2, 2).Children(func() {
			// 一句话说清这一屏的两个入口:左边的「功能」目录(answer "哪个规则")与
			// 下面的四张卡(answer "我想干什么")。以前这里分两句,一句在右栏又说
			// 一遍"先放文件还是先排步骤",同一件事说两遍反而像在教用户怎么做。
			ui.Text(c, "从左边挑选功能，或从下面的卡片选择预设好的").
				FontSize(12).TextColor(t.TextMuted).Padding(0, 2)
			for _, in := range intents {
				// 卡片底与中栏底只差 0.005 的明度(0.995 vs 0.990),
				// 不加描边就看不出这里有几张卡——实测截图里它们像四行
				// 普通文字,点了才知道是按钮。
				//
				// ButtonBase 而不是 Column:空态里这四张卡是唯一的入口,
				// 用 Column 它们不进 Tab 序列(实测 0 站)、Enter 也按不动,
				// 键盘用户被挡在中栏外面。换成 ButtonBase 后能聚焦、能回车,
				// 鼠标路径的观感不变(内层 Column 照旧竖排两行)。
				// 底用带主题色的半透明叠层,不用 Border:描边是按钮的语言
				// (见 control_look.go),四张卡描上边就成了四颗大按钮。
				// 悬停把同一层加深,而不是换一条主色描边。
				// 底是"抬高一层的那个中性色",不掺主题色——掺了就会读成
				// 一颗淡底按钮(详见 control_look.go 的说明)。"能点"由
				// 悬停时亮起的主题色描边承担。
				ground := a.currentTheme().panes(a.isDark()).card
				card := ui.ButtonBase(c).FillWidth().Radius(10).Padding(8, 10).
					Background(ground).Border(1, cardBorder(t))
				if card.Hovered() {
					card = card.Border(1, cardBorderHover(t))
				}
				card.Label(in.label + "：" + in.desc)
				card.Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, in.label).FontSize(14).TextColor(t.Text).SingleLine()
						ui.Text(c, in.desc).FontSize(12).TextColor(t.TextMuted).SingleLine().Ellipsis("…")
					})
				})
				if card.Clicked() {
					a.addIntent(in)
				}
			}
		})
	})
}

// addIntent appends the intent's steps and selects the first one, so the
// detail form below is already showing something to tweak.
func (a *app) addIntent(in intent) {
	steps := in.build()
	if len(steps) == 0 {
		return
	}
	for i := range steps {
		a.stepUID++
		steps[i].opUID = a.stepUID
	}
	a.steps = append(a.steps, steps...)
	a.stepSel = len(a.steps) - len(steps)
	a.stepScrollTo = a.stepSel
	a.revealLeft = 8
	a.update()
}

// catalogHasMore reports whether the catalog still has rows below the
// visible part, as the last frame laid it out: the fade at the bottom only
// shows then, so it means something instead of being decoration.
func (a *app) catalogHasMore() bool {
	return a.palScroll.MaxY > 0 && a.palScroll.Y < a.palScroll.MaxY-1
}

// ---- left pane: grouped catalog of named operations ----

// catalogSel 返回左栏目录里该高亮的那一条:当前选中步骤对应的功能。
//
// 派生而不是另存一份:两套高亮迟早会各说各话(见 catalogView 里的注释)。
// 选中项不存在、或者那一步不是从目录里加的(opID 为空)时返回空串,
// 目录里就一条都不亮——那正是实情:此刻没有哪条规则"正在被改"。
func (a *app) catalogSel() string {
	if a.stepSel < 0 || a.stepSel >= len(a.steps) {
		return ""
	}
	return a.steps[a.stepSel].opID
}

// groupHead 是左栏组标题的文本:箭头 + 组名 + 条数。
//
// 十来个组折起来时一屏只看得到十来个词:「编号」「日期」「高级」——用户没法
// 判断哪一组里有他要的东西,只能挨个展开。带上条数之后,「元数据(3)」和
// 「日期(1)」的重量一眼分得出来,想找"那条只有一个的"也有据可依。
//
// 箭头与组名之间用空格、组名与括号之间不留空格:中文括号紧跟名词是惯例,
// 而「▾ 替换(3)」比「▾ 替换 (3)」少一处要扫的空隙。
func groupHead(arrow, group string, n int) string {
	return fmt.Sprintf("%s %s(%d)", arrow, group, n)
}

// groupCount 数某个组在当前(可能被搜索过滤过的)结果里有几条。
//
// 数的是 shown 而不是 catalog:搜索时括号里的数字要跟着结果变,否则
// 「元数据(3)」在搜「标题」时仍然写 3,而下面只挂着一条——那比不带条数
// 更让人困惑。钉选组数的是钉了几个,不是 catalog 里有几条。
func (a *app) groupCount(shown []opDef, group string) int {
	if group == pinGroup {
		return len(a.pinnedEntries())
	}
	n := 0
	for _, d := range shown {
		if d.group == group {
			n++
		}
	}
	return n
}

func (a *app) catalogView(c *ui.Context) {
	t := c.Theme()
	// 过滤:几十项、十来个组正落在"需要搜索"的量级(Colborne 第 5 章专讲
	// 搜索)。输入框收 label / desc / 组名三处,任一命中即留下。
	shown := append([]opDef(nil), catalog...)
	if q := strings.ToLower(strings.TrimSpace(a.palQuery)); q != "" {
		shown = nil
		for _, d := range catalog {
			if catalogMatch(d, q) {
				shown = append(shown, d)
			}
		}
	} else {
		// 不搜索时按 stepGroups 的顺序排(用户找东西的频率),而不是
		// catalog 那个历史书写顺序——默认展开的正好是前两组,那两组
		// 必须是最高频的。
		rank := make(map[string]int, len(stepGroups))
		for i, g := range stepGroups {
			rank[g] = i
		}
		sort.SliceStable(shown, func(i, j int) bool {
			if ri, rj := rank[shown[i].group], rank[shown[j].group]; ri != rj {
				return ri < rj
			}
			return false
		})
	}
	var groups []string
	for _, d := range shown {
		if len(groups) == 0 || groups[len(groups)-1] != d.group {
			groups = append(groups, d.group)
		}
	}
	// 目录高亮跟着"当前选中的那一步"走,不再自己记一份"刚点了哪条"。
	//
	// 原来 palSel 与 stepSel 是两套状态:点步骤行换的是 stepSel,左栏
	// 却还停在很久以前加进来的那条上——两处高亮说着两件不同的事,用户
	// 没法从左边读出"我正在改的是哪条规则"。而"刚加进来的那条"本来就
	// 是选中项(加完 a.stepSel 指向新行),派生出来的结果一样,少一份
	// 要同步的状态。
	sel := a.catalogSel()
	// 钉住的条目另起一组放在最前,名字叫「常用」。
	//
	// 不把它们从原分组里搬走:一个条目可以同时在「常用」和「转换」里
	// 出现——用户钉它是因为常用,不是因为它改了归属。搬走会让原来那
	// 组少一条,熟悉位置的人反而找不到。
	// 搜索时不显示「常用」组:搜索的结果是一份"匹配了什么"的清单,
	// 钉住的条目已经在下面的分组里出现过,再置顶一份就成了同一个结果
	// 出现两次,看起来像两条不同的规则。
	filtering := strings.TrimSpace(a.palQuery) != ""
	if !filtering && len(a.pinnedEntries()) > 0 && len(groups) > 0 {
		groups = append([]string{pinGroup}, groups...)
	}
	// a hand-built list rather than ui.Sidebar: its internal scroll
	// container never received wheel events here, hiding the lower groups
	//
	// 左栏以前是"文档语法":内容直接铺在窗口底色上,没有容器;而中/右两栏
	// 是"卡片语法":圆角 + 投影 + 一阶更亮的底。同一屏里两套容器语言,读者
	// 要重新学一遍"这块区域的边界在哪"。视觉报告的讽刺点就在这——它建议
	// 靠"颜色层级"区分区块,不是再加竖线去对齐(那只会多一条要眼睛解析的线)。
	// 所以左栏也套一层同样的卡片:两侧同色(0.955)、中间最亮(0.990),
	// 一眼看出"中间是工作区、两侧是辅助"。
	pt := a.currentTheme().panes(a.isDark())
	ui.Column(c).Fill().Gap(8).Padding(10).Radius(14).Background(pt.right).Clip().
		Shadow(0, 2, 10, 0, pt.shadow).Disabled(a.busy).DimDisabled(false).Children(func() {
		// 栏头与中/右栏同构:一行 Row,标题靠左,不再用 Padding 去凑基线。
		// 栏头统一 MinHeight(34) + 垂直居中:三栏的标题就落在同一条基线上。
		// 不固定高度是因为右栏的工具栏在窄窗口下会折行,折了就该长高。
		paneHeader(c, "功能 · 点击加入步骤", nil)
		// 搜索框包一层 Row:有内容时右侧挂一颗 ×,点一下清空。Esc 也清,
		// 但鼠标用户得有个看得见的出口——"0 命中"是个死路,没有出口就
		// 只剩"猜是哪个字错了"。
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.palQuery).Placeholder("筛选功能…").Grow(1)
			if strings.TrimSpace(a.palQuery) != "" {
				if softButton(c, "×", true).Radius(6).Tooltip("清空搜索").Clicked() {
					a.palQuery = ""
					a.folded = nil
				}
			}
		})
		// TrackScroll 记住滚动位置:底缘渐隐只在真还有内容时出现,
		// 而这个判断要读它,不能靠猜。
		a.catRect = a.catScroll.Bounds(c)
		// FocusGroup:左栏这一整块只算一个 Tab 停留点,方向键在条目之间移动。
		// 以前每条规则 + 每个组标题各占一个停留点,三十多个条目下来 Tab
		// 要走四十多次才出得了左栏——文件列表排在那之后,键盘用户基本
		// 到不了。Tab 遍历本身是 mygo 内建的(ui/input.go 的 moveFocus),
		// 这里只是告诉它"这一堆是一个整体"。搜索框留在组外,自己是一个
		// 停留点。
		//
		// 数字不写死:catalog 条目数会变,写死的数字过一阵就又对不上了。
		ui.Scroll(c).Fill().TrackScroll(&a.palScroll).Bind(&a.catScroll).FocusGroup(ui.Vertical).Children(func() {
			if len(shown) == 0 {
				ui.Column(c).FillWidth().Gap(6).Padding(8, 10).Children(func() {
					ui.Text(c, "没有匹配「"+strings.TrimSpace(a.palQuery)+"」的功能").FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, "试试更短的关键词，或换个说法（如「改名」「后缀」）。").FontSize(12).TextColor(t.TextMuted)
					if softButton(c, "清空搜索", true).Radius(6).Clicked() {
						a.palQuery = ""
						a.folded = nil
					}
				})
			}
			for _, g := range groups {
				// 组标题本身就是折叠开关:全收起后一屏放得下,而且
				// 分组名("编号""元数据")比一长串规则名更好认。
				open := a.groupOpen(g, groups)
				arrow := "▾"
				if !open {
					arrow = "▸"
				}
				// 搜索期分组不可折叠:groupOpen 在搜索时一律返回 true(结果
				// 本来就少),但组标题仍是个可点按钮,点下去会往 a.folded 写
				// 一个"折叠"标记——界面因为 groupOpen 恒真毫无反应,状态却被
				// 写坏了。实测点「▾ 编号」后 folded=map[编号:true] 而界面不变。
				// 搜索期把它降成静态行,箭头也换成点,不再假装能点。
				if filtering {
					arrow = "·"
				}
				var head ui.Element
				if filtering {
					head = ui.Row(c).Fill().Padding(8, 0)
				} else {
					head = ui.ButtonBase(c).Fill().Radius(6).Padding(8, 0)
					if head.Hovered() {
						head.Background(t.SurfaceHover)
					}
				}
				label := groupHead(arrow, g, a.groupCount(shown, g))
				head.Children(func() {
					// FillWidth + Start:ButtonBase 默认把内容居中,而下面的条目
					// 全是左对齐。组标题居中、条目左对齐,两者左缘差了 50 多 DIP
					// (实测组标题文本 x=84.85,条目 x=32.00),扫下来像两组不相干
					// 的列表。组标题是条目的上级,它该和条目共用同一条左轴。
					ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).
						FillWidth().TextAlign(ui.Start).SingleLine()
				})
				if !filtering && head.Clicked() {
					if a.folded == nil {
						a.folded = map[string]bool{}
					}
					a.folded[g] = open
					a.persistFolded()
				}
				if !open {
					continue
				}
				items := shown
				if g == pinGroup {
					items = a.pinnedEntries()
				}
				for _, d := range items {
					if g != pinGroup && d.group != g {
						continue
					}
					selBg, selFg := a.selColors()
					// 上下各 2 的留白:条目之间要有缝,否则几十条挤成一片,
					// 眼睛没法判断一行到哪结束。悬停底色跟着长高,这也是
					// "可点区域比文字大"的一层。
					// Tooltip 挂在外层按钮上,不是里面那行文字。挂子元素时
					// 气泡只跟着鼠标走,键盘 Tab 到这一条、焦点环亮了,说明
					// 却永远不出现——整份目录的说明对键盘和读屏用户等于不存在
					// (实测条目 Description 为空)。
					// PaddingEnd 是给滚动条让位:目录一屏放不下,右端会浮一条竖滚动条,
					// 而滚动条压在所有内容之上——星标画到 x=206 就正好落在它的轨道里,
					// 点下去命中的是滚动条,星标根本按不动(实测 tt.Click 报
					// "covered by a scroll bar")。留出 10 让星标整颗都在轨道左边。
					// 这里不要加 .Label():条目的可访问名已经从里面的文字自动
					// 取到了,再挂一个同名 Label 会多出一个覆盖整行的标签节点,
					// tt.Find 就匹配到那一层(x 等于按钮左缘),而条目里的文字
					// 本来才是它的位置。
					item := ui.ButtonBase(c).Fill().Radius(6).Padding(2, 0).PaddingEnd(10).Tooltip(d.desc)
					pinned := a.isPinned(d.id)
					switch {
					case sel == d.id:
						item.Background(selBg)
					case item.Hovered():
						item.Background(t.SurfaceHover)
					}
					// 全部条目统一左对齐。原来"居中即流程控制"那套编码
					// 没有图例,而且中栏早就放弃了它(见 stepsView 的注释),
					// 同一条视觉规则出了左栏就失效——这类暗示比不暗示更糟。
					align := ui.Start
					item.Children(func() {
						// 图标走色彩管线:按分组类别色(与步骤行类别胶囊同源),
						// 选中时白——图标即类别指示,扫色即知
						if ic := opIcon(d.icon); ic != nil {
							ie := ui.Icon(c, ic).FontSize(14)
							if sel == d.id {
								ie.TextColor(selFg)
							} else {
								ie.TextColor(a.currentTheme().groupColorFor(d.group, a.isDark()))
							}
						}
						labelCol := ui.Color{}
						if sel == d.id {
							labelCol = selFg
						}
						// 说明走 Tooltip:目录要在一屏里放下几十条,每条占两行
						// 就只剩一半可见;悬停即得,和系统里其它按钮一致。
						txt := ui.Text(c, d.palName()).SingleLine().FillWidth().TextAlign(align)
						if labelCol.A != 0 {
							txt.TextColor(labelCol)
						}
						// 钉住的星标:平时不占地方,悬停这一行或已经钉住时才出现。
						// 曾经只有右键菜单能钉,而"常用"这个组只有钉过东西之后
						// 才存在——没钉过的人根本看不到这个功能,也就不会去右键
						// 试。星标只在需要时出现,扫视时目录仍是干净的一列。
						if pinned || item.Hovered() {
							a.pinStar(c, t, d.id, sel == d.id, selFg)
						}
					})
					// 右键菜单保留:星标要悬停才出现,右键是它的备份入口,
					// 也是唯一能解释"钉住是什么意思"的地方(菜单项文字)。
					item.ContextMenu(func(m *ui.Menu) {
						label := "钉到常用"
						if a.isPinned(d.id) {
							label = "取消钉住"
						}
						if m.Item(label).Chosen() {
							a.togglePin(d.id)
							a.settings.Pinned = a.pinned
							if err := SaveSettings(a.settings); err != nil {
								a.status = "钉住没能存下来：" + err.Error()
							}
						}
					})
					if item.Clicked() {
						// opens 是选择器入口(目前只有"自定义流程"),不直接
						// 加步骤:它把流程弹窗摊开,用户在那里挑完才落地。
						if d.opens != "" {
							a.openFlows()
							return
						}
						r := d.make()
						r.opID, r.opLabel, r.opDesc, r.opFlow, r.opGroup = d.id, d.label, d.desc, d.flow, d.group
						a.stepUID++
						r.opUID = a.stepUID
						a.steps = append(a.steps, r)
						a.stepSel = len(a.steps) - 1
						a.stepScrollTo = a.stepSel
						a.revealLeft = 8
					}
				}
			}
		})
	})
}

// pinGroup 是左栏顶部那个"常用"分组的名字。用一个不可能与真实分组
// 撞名的字符串:真实分组名都来自 catalog 的 group 字段(筛选/范围、
// 转换、编号……),不会有人取这个名字。
const pinGroup = "常用"

// pinnedEntries 按用户钉的顺序返回目录条目。id 在 catalog 里找不到时
// 跳过——设置是磁盘上的 JSON,可能来自删掉了某条规则的旧版本。
func (a *app) pinnedEntries() []opDef {
	out := make([]opDef, 0, len(a.pinned))
	for _, id := range a.pinned {
		if d, ok := opByID(id); ok {
			out = append(out, d)
		}
	}
	return out
}

// pinEntry 把一条目录条目钉到「常用」组末尾,已钉过就取消。
// 返回新的钉选列表,调用方负责存盘——设置写盘只在对话框与这里发生,
// 两处都要留痕。
// persistFolded 把当前的折叠状态写回设置文件。用户手一松就落盘,
// 与钉选一致——这一天里再打开软件,折起来的那几组还折着。
func (a *app) persistFolded() {
	a.settings.Folded = make(map[string]bool, len(a.folded))
	for k, v := range a.folded {
		a.settings.Folded[k] = v
	}
	_ = SaveSettings(a.settings)
}

func (a *app) togglePin(id string) {
	for i, v := range a.pinned {
		if v == id {
			a.pinned = append(a.pinned[:i], a.pinned[i+1:]...)
			return
		}
	}
	a.pinned = append(a.pinned, id)
}

// pinStar 画条目右端那颗钉选星标。
//
// 平时不出现,只有悬停这一行或它已经钉住时才画——目录要在一屏里放下
// 几十条,每行多一颗常驻的星等于给每一行都加一个可点区域,而钉住是
// 低频动作。已经钉住的必须一直看得见,否则用户找不到"取消钉住"。
//
// 星标是条目内部的一个按钮:mygo 的点击不会冒泡到外层,所以点星星不会
// 顺手把这条规则加进步骤(实测 outer=0 inner=1)。
func (a *app) pinStar(c *ui.Context, t *ui.Theme, id string, selected bool, selFg ui.Color) {
	glyph := "☆"
	label := "钉住"
	tip := "钉到「常用」组，以后不用每次翻目录"
	if a.isPinned(id) {
		glyph = "★"
		label = "取消钉住"
		tip = "已钉在「常用」组，点一下取消"
	}
	// Label 是给读屏的:星标画的是一个字形,没有名字时读屏只会念出 "☆"。
	// 它会被拼进外层条目的可访问名(读出来是"筛选文件 钉住"),所以这里
	// 只写动作,不再重复条目名。
	st := ui.ButtonBase(c).Shrink(0).Radius(6).Padding(0, 2).Label(label).Tooltip(tip)
	col := t.TextMuted
	if selected {
		col = selFg
	} else if a.isPinned(id) {
		col = t.Accent
	}
	if st.Hovered() && !selected {
		st.Background(t.SurfacePressed)
	}
	st.Children(func() {
		ui.Text(c, glyph).FontSize(12).TextColor(col)
	})
	if st.Clicked() {
		a.togglePin(id)
		a.settings.Pinned = a.pinned
		if err := SaveSettings(a.settings); err != nil {
			a.status = "钉住没能存下来：" + err.Error()
			return
		}
		if a.isPinned(id) {
			a.status = "已钉到「常用」组，左栏顶部就能找到"
		} else {
			a.status = "已取消钉住"
		}
	}
}

// isPinned reports whether the catalog entry id is in the 常用 group.
func (a *app) isPinned(id string) bool {
	for _, v := range a.pinned {
		if v == id {
			return true
		}
	}
	return false
}

// stepRangeBars returns, per step, the two fixed range-bar slots:
// [0] = left slot for the 筛选 range (seed complement), [1] = right slot for
// the 选取 range — filename and extension share the brand hue and differ in
// lightness, so a switch from 选取文件名 to 选取扩展名 shows as a value change
// (visible in grayscale too) instead of one continuous bar. A zero Color
// means the slot is off (transparent).
// Slots never move: at most two ranges can be open, each owning its place.
//
// 为什么是色条而不是文字:文字只能说明"这一行是什么范围",色条还能说明
// "范围持续到哪一行"——第 4 行设了"仅文件名"、第 12 行才"恢复整体",
// 中间每一行右边都有一条同色竖线连成一竖,一眼看出这 8 行都在范围内;
// 写成文字则每行都要读一遍,还占宽度。所以这里保留色条,另配图例。
func (a *app) stepRangeBars(dt *derivedTheme, dark bool) [][2]ui.Color {
	rp := dt.ranges(dark)
	out := make([][2]ui.Color, len(a.steps))
	filterOn := false
	scopeCol := ui.Color{} // zero = off
	for i := range a.steps {
		s := &a.steps[i]
		key, _ := kindByLabel[s.kind]
		switch key {
		case "filter":
			filterOn = true
		case "unfilter":
			filterOn = false
		case "scope":
			switch s.scopeSel {
			case "整体":
				scopeCol = ui.Color{}
			case "扩展名":
				scopeCol = rp.ext
			default: // 文件名
				scopeCol = rp.name
			}
		}
		if filterOn {
			out[i][0] = rp.filter
		}
		out[i][1] = scopeCol
	}
	return out
}

// rangeLegend 说明色条的含义——只列此刻真正生效的范围,没开的范围不占地方。
// 报告原本的批评是"色条没有图例",不是"色条错了":编码本身最省宽度,缺的
// 只是让它自解释。图例与色条同色同形(4px 竖条),看一次就能对上。
func (a *app) rangeLegend(c *ui.Context, t *ui.Theme, dt *derivedTheme) {
	bars := a.stepRangeBars(dt, t.Dark)
	rp := dt.ranges(t.Dark)
	var filt, name, ext bool
	for _, b := range bars {
		if b[0].A != 0 {
			filt = true
		}
		if b[1].A != 0 {
			if b[1] == rp.ext {
				ext = true
			} else {
				name = true
			}
		}
	}
	if !filt && !name && !ext {
		return
	}
	chip := func(col ui.Color, label string) {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Width(4).Height(13).Radius(4 / 2).Background(col)
			ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).SingleLine()
		})
	}
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "范围").FontSize(12).TextColor(t.TextMuted)
		if filt {
			chip(rp.filter, "筛选")
		}
		if name {
			chip(rp.name, "仅文件名")
		}
		if ext {
			chip(rp.ext, "仅扩展名")
		}
	})
}

// ---- middle pane: ordered task steps + selected step's detail ----

// undoStrips 画中栏顶部那两条"刚发生了什么、怎么退回去"的提示。
//
// 位置选在列表上方而不是底栏:动作的效果就发生在这一栏里(那一行不见了、
// 整条流水线被换掉了),退路的入口也该在同一个视线落点上。底栏已经承担
// 了"执行/撤销"这类全局动作,再塞一条局部撤销会让"哪个按钮撤销哪个
// 东西"变成要读文字才能分辨。
func (a *app) undoStrips(c *ui.Context, t *ui.Theme) {
	if last, ok := a.lastStepUndo(); ok && !a.undoDeleteExpired() {
		// 栈里多于一步时,条上要说出"还有几步",并给出"全部恢复":
		// 连删三下之后一步步点撤回,等于让用户把刚才的误操作再演一遍。
		label := fmt.Sprintf("已删除「%s」", last.step.opLabel)
		if n := len(a.stepUndo); n > 1 {
			label = fmt.Sprintf("已删除「%s」等 %d 步", last.step.opLabel, n)
		}
		ui.Row(c).FillWidth().Gap(8).Padding(8, 10).Radius(6).
			Background(t.SurfaceHover).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).
				FontSize(12).TextColor(t.Text).Grow(1).SingleLine().Ellipsis("…")
			if n := len(a.stepUndo); n > 1 {
				if softButton(c, fmt.Sprintf("全部撤回（%d）", n), true).Radius(6).Clicked() {
					a.undoAllDeleted()
				}
			}
			if softButton(c, "撤回", true).Radius(6).Clicked() {
				a.undoDelete()
			}
		})
	} else if len(a.stepUndo) > 0 {
		// 过期的栈要在帧里清掉,否则它会在下一次删除之前一直占着
		// "有东西可撤回"这个状态。
		a.clearDeleted()
	}
	// 流程套用替换掉整条流水线时的撤回条。与删单步同一个位置、同一个
	// 时限:被换掉的是一整批,更该给一条明确的退路。
	if len(a.recoveredSteps) > 0 {
		if a.flowReplaceExpired() {
			a.recoveredSteps = nil
			a.recoveredAt = time.Time{}
		} else {
			ui.Row(c).FillWidth().Gap(8).Padding(8, 10).Radius(6).
				Background(t.SurfaceHover).AlignItems(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprintf("已替换原来的 %d 步", len(a.recoveredSteps))).
					FontSize(12).TextColor(t.Text).Grow(1).SingleLine().Ellipsis("…")
				if softButton(c, "撤回", true).Radius(6).Clicked() {
					a.undoFlowReplace()
				}
			})
		}
	}
}

func (a *app) stepsView(c *ui.Context) {
	t := c.Theme()
	if a.stepSel >= len(a.steps) {
		a.stepSel = len(a.steps) - 1
	}
	// 加到第 22 步时要把这一行滚进视野。列表本身不是滚动容器(它嵌在
	// stepsView 的 ui.Scroll 里,Grow(1) 的列表自认为全部可见),所以
	// ListState.ScrollIntoView 不起作用——得让那一行的元素自己向所有
	// 外层滚动容器申请露出来。
	// 多选开着、步骤却被删光了(或整批清掉):开关该自己退回关。
	// 否则栏头仍写着「完成」,批量条也还挂着,而一条可选的行都没有。
	if a.pickMode && len(a.steps) == 0 {
		a.pickMode = false
		a.chosen.Clear()
	}
	revealRow := -1
	if a.stepScrollTo >= 0 && a.revealLeft > 0 {
		revealRow = a.stepScrollTo
		a.revealLeft--
		if a.revealLeft == 0 {
			a.stepScrollTo = -1
		}
	}
	// 中间栏:核心工作区,浮得最高的一张白色圆角卡片(暗色下也比两侧亮一阶),
	// 压在基底之上;给它圆角 + 一层柔投影,像一张单独浮起的纸。
	pt := a.currentTheme().panes(a.isDark())
	ui.Column(c).Fill().Gap(8).Padding(10).Radius(14).Background(pt.mid).Clip().
		Shadow(0, 3, 14, 0, pt.shadow).Disabled(a.busy).DimDisabled(false).Children(func() {
		paneHeader(c, fmt.Sprintf("任务步骤（%d）", len(a.steps)), func() {
			// 流程入口放在栏头,和「多选」并列。以前它藏在左栏「高级」
			// 目录的最后一条(y≈1400,要滚到底)——而它是"整套做法"的
			// 入口,比"往管道里加一条规则"高一档,不该排在规则目录末尾。
			flowLabel := "流程"
			narrowFlow := a.splitM < 260
			if narrowFlow {
				flowLabel = "▤"
			}
			fb := softButton(c, flowLabel, true).Radius(6)
			if narrowFlow {
				fb = fb.Tooltip("流程：套用预设，或把当前步骤存成自己的流程")
			}
			if fb.Clicked() {
				a.openFlows()
			}
			// 触摸入口:点开后才显示行内勾选框(Ctrl+点击亦可)
			label := "多选"
			if a.pickMode {
				label = "完成"
			}
			narrowHead := a.splitM < 210
			if narrowHead {
				label = "⋯"
			}
			// 0 步时没有东西可勾,这颗按钮和「上移」一样按不动。开着多选
			// 再删光步骤的路径由 stepsView 开头的守卫生效。
			btn := softButton(c, label, len(a.steps) > 0)
			if narrowHead {
				btn = btn.Tooltip("多选：点开后在每步前面显示勾选框")
			}
			if btn.Clicked() {
				a.pickMode = !a.pickMode
				// 关掉开关 = 退出多选:选中集合一并清掉,勾选框与批量条
				// 随之消失。留着集合会让"开关是关的、多选却还在"。
				if !a.pickMode {
					a.chosen.Clear()
				}
			}
		})
		// 选中行自绘浅色底(Selection),不用 List 的深 Accent 绘制:
		// 选中时行内的彩色胶囊必须保持可读
		// 撤回条要在两个分支之上画:把最后一步删掉之后清单就空了,而
		// "刚删掉一步"恰恰是用户最需要退路的那一刻——把它留在 else 分支
		// 里,删光步骤的同时退路也一起消失。
		a.undoStrips(c, t)
		if len(a.steps) == 0 {
			a.intentCards(c)
		} else {
			// 不变量:选中集合只在多选开关打开时存在。开关关掉的地方
			// 已经清过一次,这里再兜一次——集合只要非空,勾选框与批量条
			// 就还有理由出现,而那时开关是关的。
			if !a.pickMode && a.chosen.Len() > 0 {
				a.chosen.Clear()
			}
			dt := a.currentTheme()
			bars := a.stepRangeBars(dt, t.Dark)
			// 单选随时可用;多选只在开关打开时接上。
			//
			// mygo 的 List 只要挂了 Selection,普通单击就会走 choose()——
			// 那是"清空整个集合、只留被点的那行"。于是开关关着的时候,
			// 随手点两行也会让 chosen 非空,而勾选框的显示条件里又有
			// "已经有选中项"这一条,屏幕上就冒出勾选框:开关看着是关的,
			// 多选却随时可用,这个开关等于没有。现在 Selection 只在
			// pickMode 为真时挂上,关掉开关 = 集合清空 + 退回单选,
			// 勾选框与批量条随之消失。
			a.stepList.Key = func(i int) any { return a.steps[i].opUID }
			a.stepList.Selected = &a.stepSel
			if a.pickMode {
				a.stepList.Selection = &a.chosen
			} else {
				a.stepList.Selection = nil
			}
			a.stepList.Reorder = func(rows []int, to int) {
				a.moveSteps(rows, to)
			}
			// 列表与详情共用同一条滚动轴:详情紧跟最后一行,不再钉在栏底。
			// 以前是"列表 Grow(1) 吃满高度 + 详情钉底",结果是 1 步时选中行
			// 与编辑器之间隔 290px 空白,12 步时第 8 行起被分隔线硬切——
			// 列表没有自己的可视边界,行被排进详情区地盘然后整行 Clip 掉。
			// 一行能放多少:中栏可以拖到只剩一百多 DIP,那时"编号 + 操作名 +
			// 参数胶囊 + 类别胶囊 + 色条 + 多选圆点"六段并排必然溢出,胶囊被
			// 切掉一半。按宽度分三档降级,先丢最不承载信息的:类别胶囊(颜色
			// 仍由行首色条表达)、参数胶囊、再是编号。
			rowW := a.splitM - 20
			showGroupChip, showSummary := rowW >= 300, rowW >= 210
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					// 图例:色条只在真有范围时出现,所以图例也只列此刻生效的那几种。
					// 报告原本说"色条没有图例、全应用找不到任何解释"——补的是这一句。
					a.rangeLegend(c, t, dt)
					ui.List(c, &a.stepList, len(a.steps), func(i int) {
						s := a.steps[i]
						cat := dt.groupColorFor(s.opGroup, a.isDark())
						// 行结构:编号 · 操作名 · 参数胶囊 · 类别胶囊 · 右缘两条范围色条。
						// 四套编码挤在 38px 里确实密,但它们各答一个不同的问题:
						// 参数=改什么,类别色=哪一类处理(靠色相分,不靠读字),
						// 范围色条=这条规则管到哪一行。报告批评的是"没有图例",
						// 所以补的是图例(stepsView 顶部),不是删掉编码。
						// 当前步骤与多选集合都高亮
						hot := i == a.stepSel || a.chosen.Has(s.opUID)
						// 横向内边距 10 而不是 6:下面参数卡片的标题与标签
						// 都在 x=258(栏内边距 10),而编号原来在 x=254,
						// 差 4 DIP——列表和它下面的编辑器看起来是两块拼的。
						row := ui.Column(c).Fill().Padding(8, 10).MinHeight(38).Radius(6)
						if hot {
							row.Background(t.Selection)
						}
						if i == revealRow {
							row.ScrollIntoView()
						}
						// 范围色条自绘:两槽固定——右槽=选取范围,左槽=筛选范围;
						// 未激活透明占位,槽位永不移动
						row.DrawOver(func(p *ui.Painter, r ui.Rect) {
							for k := 0; k < 2; k++ {
								col := bars[i][1-k] // [1] 右槽,[0] 左槽
								if col.A == 0 {
									continue
								}
								x := r.X + r.W - 4 - float32(k)*6
								p.Fill(ui.Rect{X: x, Y: r.Y, W: 4, H: r.H}, col, 0)
							}
							// 选中行左缘一条主色竖条。
							//
							// 行底色已经会变,但底色是"这一行被点亮"的整体感,
							// 在 38px 高的窄行里它和相邻行的差别很淡;而下面那张
							// 详情卡说的是"我正在改第几步",两者需要一个显式的
							// 连线。左缘竖条就是那条线:它指向的那一行,正是
							// 卡片里编辑的那一步。
							if i == a.stepSel {
								p.Fill(ui.Rect{X: r.X, Y: r.Y + 4, W: 3, H: r.H - 8}, t.Accent, 0)
							}
						})
						row.Children(func() {
							// 高亮行:mygo 会把选中行的文字染成强调色(浅),显式
							// 声明深色,不靠继承
							numCol, labCol := t.TextMuted, t.Text
							if hot {
								numCol, labCol = t.Text, t.Text
							}
							ui.Row(c).Gap(6).Children(func() {
								// 勾选框在最左、编号之前。以前它在行尾——类别胶囊、
								// 范围色条之后,读一行流程要在半路绕过它。移到前面
								// 之后,"[ ] 1. 操作名 参数 类别"读起来是一条直线。
								//
								// 槽位在"多选模式开着"或"已经有选中项"时恒占着,
								// 不管这一行选没选:否则勾选一行会让整行文字横移
								// 16 DIP,读流程时最刺眼的正是这种跳动。
								picked := a.chosen.Has(s.opUID)
								if a.pickMode {
									// 勾选框是控件:没勾时也走"能点"的主题色描边,
									// 不是中性灰(灰 = 按不动,见 control_look.go)。
									dotColor := liveBorder(t)
									if picked {
										dotColor = t.Accent
									}
									pick := ui.ButtonBase(c).Size(16, 16).Radius(6).Border(1, dotColor).Shrink(0)
									if picked {
										pick.Background(t.Accent)
									}
									pick.Label("选中第 " + strconv.Itoa(i+1) + " 步")
									pick.Children(func() {
										if picked {
											ui.Text(c, "✓").FontSize(12).TextColor(t.AccentText)
										}
									})
									if pick.Clicked() {
										if picked {
											a.chosen.Remove(s.opUID)
										} else {
											a.chosen.Add(s.opUID)
										}
									}
								}
								ui.Text(c, fmt.Sprintf("%d.", i+1)).FontSize(12).TextColor(numCol).Width(22).TextAlign(ui.End)
								// 全部步骤统一左对齐(设计书:不要在同一列表里混用对齐);
								// 流程控制靠类别胶囊 + 范围色条区分,不再靠居中
								if s.opFlow {
									ui.Row(c).Fill().Gap(6).Children(func() {
										// Shrink(0):操作名不参与压缩,要压缩就让后面的参数
										// 胶囊去省略。以前没有它,"追加序号"会被挤成"追…"、
										// "AI 按内容命名"挤成"AI…"——名字比参数重要。
										ui.Text(c, s.opLabel).FontSize(12).Bold().TextColor(labCol).SingleLine().Shrink(0)
										if v := s.summary(); v != "" && showSummary {
											// SingleLine 才能让 Ellipsis 生效,否则长参数会折成两行撑高行。
											// Shrink(1):行宽不够时让胶囊让步,而不是把操作名挤没。
											ui.Text(c, v).FontSize(12).Padding(2, 8).Radius(6).MaxWidth(150).SingleLine().Shrink(1).Ellipsis("…").
												Background(cat.Alpha(0.16)).TextColor(t.Text).Tooltip(v)
										}
									})
								} else {
									ui.Row(c).Fill().Gap(6).Children(func() {
										ui.Text(c, s.opLabel).FontSize(12).Bold().TextColor(labCol).SingleLine().Shrink(0)
										if v := s.summary(); v != "" && v != s.opLabel && showSummary {
											// 同上:一行 + 可省略,长参数不撑高这一行。
											ui.Text(c, v).FontSize(12).Padding(2, 8).Radius(6).MaxWidth(150).SingleLine().Shrink(1).Ellipsis("…").
												Background(cat.Alpha(0.16)).TextColor(t.Text).Tooltip(v)
										}
									})
								}
								// 胶囊区:固定宽度单独管理,行内不再被挤。
								// 窄栏下整列收起——类别颜色由行尾色条与行底色承担,
								// 挤着画只会把操作名压没。
								if showGroupChip {
									catBg, catFg := chipTones(cat, a.isDark())
									ui.Row(c).Width(84).Gap(4).Children(func() {
										ui.Spacer(c)
										// 连续同类别:胶囊退化为同色圆点,换组才显示文字
										if i > 0 && a.steps[i-1].opGroup == s.opGroup {
											ui.Box(c).Size(9, 9).Background(catFg)
										} else {
											ui.Text(c, s.opGroup).FontSize(12).Padding(2, 8).Radius(6).
												Background(catBg).TextColor(catFg)
										}
									})
								}
								// 色条保留区:两条竖线永远画在这里,不压胶囊
								ui.Box(c).Width(12)
							})
						})
					}).Gap(0) // 行相接,范围竖线连续;行距由行内 Padding 提供
				})
			})
			// 选中步骤的参数悬浮在中栏下方,是一张压在列表上的小卡片。
			// 以前它接在列表最后一行之后、跟着整块一起滚:列表长的时候要
			// 滚到底才看得见,短的时候又和选中行隔着大半个空屏。它编辑的
			// 永远是"当前选中的那一步",所以位置也该固定,不跟着列表跑。
			if a.stepSel >= 0 && a.stepSel < len(a.steps) {
				r := &a.steps[a.stepSel]
				// 矮窗口(900×480)下中栏只有 260 高,详情不限高就会把底部的
				// 上移/删除/复制顶出可视区。做法是参数区自己滚、按钮钉在
				// 卡片底部:参数可以滚,按钮必须一直在。
				_, paneH := c.Size()
				ui.Column(c).FillWidth().Gap(6).Padding(8, 10).Radius(10).
					MaxHeight(paneH * 0.45).
					Background(dt.panes(a.isDark()).card).Children(func() {
					// 首行先报位置:「第 3 步 / 共 7 步」。
					//
					// 卡片浮在列表下方,它编辑的永远是"当前选中的那一步",
					// 但卡片自己不说这件事——列表滚到中段时,读者得回头
					// 数一遍才能确认自己改的是第几步。参数改错一项的代价
					// 是整批文件名,这一步不该靠数。
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, fmt.Sprintf("第 %d 步 / 共 %d 步", a.stepSel+1, len(a.steps))).
							FontSize(12).TextColor(t.TextMuted).SingleLine().Shrink(0)
						ui.Text(c, r.opLabel).Bold().SingleLine().Shrink(1).Ellipsis("…")
					})
					if r.opDesc != "" {
						ui.Text(c, r.opDesc).FontSize(12).TextColor(t.TextMuted)
					}
					// AI 步骤没配 Key 时,先在这里说清楚。
					//
					// 「AI 按内容命名」本质是一条 template 规则,参数面板里
					// 只有模板文本——第一次用的人看到的是「<AI:根据文件内容
					// 起一个简短的名字><.EXT>」,既看不出这是要联网的,也看
					// 不出缺什么。等到点「执行更名」才发现整列变红,那时他
					// 已经对着这段模板猜了半天。
					//
					// 所以:模板里出现 <AI: 且没有可用 Key,就在卡片顶部
					// 直接给一条提示 + 一个「去配置」入口。配好之后这条
					// 自然消失,不需要用户再去别处确认状态。
					if strings.Contains(r.tpl, "<AI:") && !core.AIConfigured() {
						ui.Row(c).FillWidth().Gap(8).Padding(8, 8).Radius(6).
							Background(t.Danger.Alpha(0.10)).AlignItems(ui.Center).Children(func() {
							ui.Text(c, "这一步要调用 AI 接口，但还没填 API Key。").
								FontSize(12).TextColor(t.Danger).Grow(1).SingleLine().Ellipsis("…")
							if softButton(c, "去配置", true).Radius(6).Clicked() {
								a.openSettings()
							}
						})
					}
					// 参数表单要知道自己实际有多宽:中栏的分隔缝可以一路拖到
					// 几乎关闭,固定 96 的标签加上控件在那种宽度下会顶出栏外。
					// 减去卡片左右各 10 的内边距,再留出滚动条的余量。
					formW := a.splitM - 20 - 12
					ui.Scroll(c).Grow(1).Children(func() { r.paramsEditor(c, t, formW) })
					// Wrap:中栏拖窄时四个按钮横排必然出界,"复制"会被卡片
					// 右缘裁掉一半。宁可换行,也不能让按钮消失。
					ui.Row(c).Gap(8).Wrap().Children(func() {
						if plainButton(c, "上移", a.stepSel > 0).Tooltip(shortcutTip("Alt+↑")).Clicked() {
							a.swapSteps(a.stepSel-1, a.stepSel)
						}
						if plainButton(c, "下移", a.stepSel < len(a.steps)-1).Tooltip(shortcutTip("Alt+↓")).Clicked() {
							a.swapSteps(a.stepSel, a.stepSel+1)
						}
						// 删除单步不需要二次确认:它删的是"管道里的一条",
						// 不是磁盘上的文件,而且删完立刻在列表上方给出
						// 「已删除「X」 撤回」。以前它和"清空列表"共用
						// 同一套红按钮 + 两次点击,把两种不同量级的动作
						// 说成一样重——用久了人会形成"看到红就点第二下"
						// 的肌肉记忆,那时确认框已经不再拦任何东西。
						// 真正不可逆的两个(撤销、清空列表)仍然保留确认。
						if dangerButton(c, "删除", true).Tooltip(shortcutTip("Delete")).Clicked() {
							a.deleteStep(a.stepSel)
						}
						if softButton(c, "复制", true).Tooltip(shortcutTip("Ctrl+D")).Clicked() {
							a.duplicateStep(a.stepSel)
						}
					})
				})
			}
		}
		// 多选时的批量操作,跟着开关走。
		//
		// 条件里原来还有"或已有选中项"这一条兜底,但那条兜底本身就是
		// bug 的一半:选中项只可能在开关开着时产生,关掉开关又不清空,
		// 屏幕就留下一条没有归属的批量条。现在关开关必清集合,兜底
		// 不再需要——留着它只会掩盖"关不干净"。
		if a.pickMode {
			n := a.chosen.Len()
			ui.Box(c).FillWidth().Height(1).Background(a.currentTheme().panes(a.isDark()).outline)
			// 标签独占一行、按钮成组:挤在一行里时"删除所选"会被
			// Wrap 甩到第二行单独一个,看着像排版出错而不是分组。
			ui.Text(c, fmt.Sprintf("已选 %d 步", n)).Bold().SingleLine()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
				// 全选/反选:步骤多起来之后,一个个点比拖一次还慢,而"删掉
				// 中间那几步"通常等于"留下两头"——先全选再点掉不要的,比
				// 逐个勾更接近人想做的事。
				if plainButton(c, "全选", n < len(a.steps)).Clicked() {
					for _, st := range a.steps {
						a.chosen.Add(st.opUID)
					}
				}
				if plainButton(c, "反选", len(a.steps) > 0).Clicked() {
					for _, st := range a.steps {
						if a.chosen.Has(st.opUID) {
							a.chosen.Remove(st.opUID)
						} else {
							a.chosen.Add(st.opUID)
						}
					}
				}
				if plainButton(c, "全不选", n > 0).Clicked() {
					a.chosen.Clear()
				}
				// 批量删除同样是"丢东西",和单步删除、清空列表共用一套语言
				if a.confirmChosen && a.confirmStale() {
					a.dropConfirm()
				}
				if a.confirmChosen {
					confirm := ui.ButtonBase(c).Radius(6).Padding(8, 12).Background(t.Danger).TextColor(ui.RGB(255, 255, 255))
					confirm.Children(func() { ui.Text(c, "确认删除所选?").SingleLine() })
					if confirm.Clicked() {
						a.deleteChosen()
						a.dropConfirm()
					}
				} else if dangerButton(c, "删除所选", n > 0).Clicked() {
					a.armConfirm("chosen")
				}
			})
		}
	})
}

// moveSteps is the List's Reorder callback: the user dragged rows (the
// multi-selection, or a single row) to before index to.
func (a *app) moveSteps(rows []int, to int) {
	sort.Ints(rows)
	moved := make([]guiRule, 0, len(rows))
	kept := make([]guiRule, 0, len(a.steps))
	shift := 0
	isDragged := map[int]bool{}
	for _, r := range rows {
		isDragged[r] = true
	}
	for i, s := range a.steps {
		if isDragged[i] {
			moved = append(moved, s)
			if i < to {
				shift++
			}
			continue
		}
		kept = append(kept, s)
	}
	at := to - shift
	if at < 0 {
		at = 0
	}
	if at > len(kept) {
		at = len(kept)
	}
	out := append(kept[:at:at], moved...)
	out = append(out, kept[at:]...)
	a.steps = out
}

// swapSteps 交换第 i 步与第 j 步。
// 选中必须跟着"那一步"走,而不是跟着"那个位置":把第 2 步上移之后
// 还停在第 2 行,用户再点一次"上移"动的就是原来第 1 步了。
func (a *app) swapSteps(i, j int) {
	if i < 0 || j < 0 || i >= len(a.steps) || j >= len(a.steps) {
		return
	}
	a.steps[i], a.steps[j] = a.steps[j], a.steps[i]
	switch a.stepSel {
	case i:
		a.stepSel = j
	case j:
		a.stepSel = i
	}
	// Alt+↑↓ 动的是管道顺序,不是"选中上一个/下一个"——这两个语义在别的
	// 软件里常落在同一组键上,误按一次就会静默改掉执行顺序(预览会跟着变,
	// 但预览变了不等于用户知道是自己按出来的)。所以每次交换都在底栏
	// 说一句,让"我按错了"当场可见,而不是等到执行完才发现顺序不对。
	if a.stepSel >= 0 && a.stepSel < len(a.steps) {
		a.status = fmt.Sprintf("「%s」移到第 %d 位", a.steps[a.stepSel].opLabel, a.stepSel+1)
	}
}

// stepUndo 是撤回栈里的一项:被拿掉的那一步、它原来的位置、以及删除时刻。
//
// 位置必须跟着一起存:撤回的意义是"放回原处",不是"接到末尾"。删掉
// 管道中间那一步之后,末尾已经不是它该待的地方了。
type stepUndo struct {
	step guiRule
	idx  int
	at   time.Time
}

// pushStepUndo 把一步压进撤回栈。
func (a *app) pushStepUndo(st guiRule, idx int) {
	a.stepUndo = append(a.stepUndo, stepUndo{step: st, idx: idx, at: time.Now()})
}

// lastStepUndo 取栈顶(最近一次删除)。ok=false 表示栈空。
func (a *app) lastStepUndo() (stepUndo, bool) {
	if len(a.stepUndo) == 0 {
		return stepUndo{}, false
	}
	return a.stepUndo[len(a.stepUndo)-1], true
}

// deleteStep 删掉第 i 步,并把它压进撤回栈。
//
// 鼠标的「删除」和键盘的 Delete 走同一条路——两条路径各写一遍,迟早
// 会长歪。删除本身不再需要二次确认(见 stepUndo 的注释),所以这里
// 必须留下足够的信息把它原样放回去:内容、位置、以及选中哪一行。
func (a *app) deleteStep(i int) {
	if i < 0 || i >= len(a.steps) {
		return
	}
	st := a.steps[i]
	a.pushStepUndo(st, i)
	a.steps = append(a.steps[:i], a.steps[i+1:]...)
	if a.stepSel >= len(a.steps) {
		a.stepSel = len(a.steps) - 1
	}
	if n := len(a.stepUndo); n > 1 {
		a.status = fmt.Sprintf("已删除「%s」，连这次共 %d 步可以撤回", st.opLabel, n)
	} else {
		a.status = fmt.Sprintf("已删除「%s」，可以撤回", st.opLabel)
	}
	// 界面只在有事件时重画。撤回条要在几秒后自己消失,而用户删完
	// 通常就撒手了——没有事件,那条就会一直挂在屏幕上。
	a.scheduleUndoExpiry()
}

// deleteChosen 删掉多选的每一步,并把它们全部压进撤回栈。
//
// 批量删除原来是一条死路:31 步里点错两个勾,删完没有任何退路。既然
// 单步删除可以撤回,批量删除更该可以——被一次拿掉的东西更多。
func (a *app) deleteChosen() {
	if a.chosen.Len() == 0 {
		return
	}
	drop := map[int]bool{}
	for uid := range a.chosen.All() {
		drop[uid] = true
	}
	// 先记下每个被删项的原下标,压栈时按下标**降序**:撤回是从栈顶往回
	// 放的(后删的先归位),降序压进去正好让下标小的先归位。删 [B,C] 时
	// 先插回 B 再插回 C 才得到 [A,B,C,D];反过来的话 C 先落位,插入 B
	// 又把它顶到后面去,恢复出来是 [A,B,D,C]。
	var dropped []stepUndo
	kept := make([]guiRule, 0, len(a.steps))
	for i, s := range a.steps {
		if drop[s.opUID] {
			dropped = append(dropped, stepUndo{step: s, idx: i, at: time.Now()})
			continue
		}
		kept = append(kept, s)
	}
	for i := len(dropped) - 1; i >= 0; i-- {
		a.stepUndo = append(a.stepUndo, dropped[i])
	}
	n := len(a.steps) - len(kept)
	a.steps = kept
	a.chosen.Clear()
	if a.stepSel >= len(a.steps) {
		a.stepSel = len(a.steps) - 1
	}
	if n == 0 {
		a.status = "没有选中任何步骤"
		return
	}
	a.status = fmt.Sprintf("已删除 %d 个步骤，可以撤回", n)
	a.scheduleUndoExpiry()
}

// undoDelete 把栈顶那一步放回原位并选中它。
func (a *app) undoDelete() {
	last, ok := a.lastStepUndo()
	if !ok {
		return
	}
	a.stepUndo = a.stepUndo[:len(a.stepUndo)-1]
	a.restoreStep(last)
	a.status = fmt.Sprintf("已恢复「%s」", last.step.opLabel)
	if n := len(a.stepUndo); n > 0 {
		a.status += fmt.Sprintf("，还有 %d 步可以撤回", n)
	}
}

// undoAllDeleted 把撤回栈里的每一步都放回去。
//
// 从栈顶往回放:栈顶是最后删的、下标最大,先归位它,前面那些的下标
// 才仍然指向它们原来的位置。
func (a *app) undoAllDeleted() {
	if len(a.stepUndo) == 0 {
		return
	}
	n := len(a.stepUndo)
	for len(a.stepUndo) > 0 {
		last := a.stepUndo[len(a.stepUndo)-1]
		a.stepUndo = a.stepUndo[:len(a.stepUndo)-1]
		a.restoreStep(last)
	}
	a.status = fmt.Sprintf("已恢复 %d 步", n)
}

// restoreStep 把一项撤回内容插回它原来的位置并选中它。
func (a *app) restoreStep(u stepUndo) {
	i := u.idx
	if i < 0 {
		i = 0
	}
	if i > len(a.steps) {
		i = len(a.steps)
	}
	a.steps = append(a.steps, guiRule{})
	copy(a.steps[i+1:], a.steps[i:])
	a.steps[i] = u.step
	a.stepSel = i
	a.stepScrollTo = i
	a.revealLeft = 8
}

// focusStep 把某一步选中并滚进视野,让右侧编辑器直接对着它。
//
// 「去修这一步」要的是"人已经站在那个编辑器前面",不是"列表里多亮了一
// 行":中栏可能滚到别处去了,选中却看不见。
func (a *app) focusStep(i int) {
	if i < 0 || i >= len(a.steps) {
		return
	}
	a.stepSel = i
	a.stepScrollTo = i
	a.revealLeft = 8
	a.status = fmt.Sprintf("已选中第 %d 步，右边可以改参数", i+1)
}

// clearDeleted 清空撤回栈。
func (a *app) clearDeleted() { a.stepUndo = nil }

// undoClear 把刚被清空的文件清单整份放回去。
//
// 恢复的不只是"哪几个文件":添加顺序(order)、序号计数器、当时的排序
// 依据都一起回来。只把文件塞回去的话,用户看到的是"文件回来了,但编号
// 和顺序全乱了"——那和没恢复差不多。
func (a *app) undoClear() {
	if len(a.clearedFiles) == 0 {
		return
	}
	n := len(a.clearedFiles)
	a.files = a.clearedFiles
	a.ops = a.clearedOps
	a.nextOrder = a.clearedOrder
	a.sortBy, a.sortDesc = a.clearedSort, a.clearedDesc
	a.clearedFiles, a.clearedOps = nil, nil
	a.clearedOrder = 0
	a.clearedSort, a.clearedDesc = "", false
	a.clearedAt = time.Time{}
	a.fileSel = -1
	a.assignExtColors(a.currentTheme())
	a.status = fmt.Sprintf("已恢复 %d 个文件", n)
	a.update()
}

// clearUndoExpired reports whether the clear-undo offer has outlived its window.
func (a *app) clearUndoExpired() bool {
	return !a.clearedAt.IsZero() && time.Since(a.clearedAt) > undoDeleteWindow
}

// dropClearUndo 丢掉"刚清空"的快照。清单被别的东西改过之后(加入新文件、
// 导入清单),旧的快照就不再是"上一个状态"了,留着只会让撤回覆盖掉
// 用户刚做的事。
func (a *app) dropClearUndo() {
	a.clearedFiles, a.clearedOps = nil, nil
	a.clearedOrder = 0
	a.clearedSort, a.clearedDesc = "", false
	a.clearedAt = time.Time{}
}

// scheduleClearExpiry 排一次重画,让"已清空"的撤回条到点自己消失。
func (a *app) scheduleClearExpiry() {
	at := a.clearedAt
	go func() {
		time.Sleep(undoDeleteWindow + 100*time.Millisecond)
		if a.clearedAt.Equal(at) {
			a.update()
		}
	}()
}

// undoDeleteExpired reports whether the undo offer has outlived its window.
//
// 以栈顶(最近一次删除)为准:连着删几步时,窗口从最后一次删除重新计时
// ——否则删到第三步时前两条已经过期,而用户正看着"可以撤回"的提示。
func (a *app) undoDeleteExpired() bool {
	last, ok := a.lastStepUndo()
	if !ok || last.at.IsZero() {
		return false
	}
	return time.Since(last.at) > undoDeleteWindow
}

// scheduleUndoExpiry 排一次重画,让撤回条到点自己消失。
func (a *app) scheduleUndoExpiry() {
	last, ok := a.lastStepUndo()
	if !ok {
		return
	}
	at := last.at
	go func() {
		time.Sleep(undoDeleteWindow + 100*time.Millisecond)
		if cur, still := a.lastStepUndo(); still && cur.at.Equal(at) {
			a.update()
		}
	}()
}

// duplicateStep 复制第 i 步到列表末尾并选中它。
// 鼠标的「复制」和 Ctrl+D 走同一条路——两条路径各写一遍,迟早会长歪。
func (a *app) duplicateStep(i int) {
	if i < 0 || i >= len(a.steps) {
		return
	}
	cp := a.steps[i]
	a.stepUID++
	cp.opUID = a.stepUID
	a.steps = append(a.steps, cp)
	a.stepSel = len(a.steps) - 1
	a.stepScrollTo = a.stepSel
	a.revealLeft = 8
}

// removeFile 把第 i 个文件从清单里拿掉。这是"清空列表"的单文件版本:
// 拖进 200 个文件、只有 3 个不想改时,以前唯一的办法是整表清空重来。
//
// a.files 与 a.ops 是按下标对齐的两个切片(ops[i] 永远是 files[i] 的预览
// 结果),所以两边必须一起删,否则预览会整体错位一行——那比不删更糟。
func (a *app) removeFile(i int) {
	if i < 0 || i >= len(a.files) {
		return
	}
	a.files = append(a.files[:i], a.files[i+1:]...)
	if i < len(a.ops) {
		a.ops = append(a.ops[:i], a.ops[i+1:]...)
	}
	// 选中位置跟着收缩,别指到不存在的行上
	if n := len(buildRows(a.files)); a.fileSel >= n {
		a.fileSel = n - 1
	}
	a.applied, a.appliedAt, a.appliedCount = false, time.Time{}, 0
	if len(a.files) == 0 {
		a.ops = nil
		a.status = ""
		return
	}
	// 删完重算:新名字可能依赖序号、分组计数这类"上下文",
	// 只删一行也可能让后面每一行的预览都变。
	a.preview()
}

// removeChosenFiles 移除当前选中的文件(Delete 键与右键菜单都走这里)。
func (a *app) removeChosenFiles() {
	if a.fileSel < 0 {
		return
	}
	a.removeFile(a.rowFile(a.fileSel))
}

// rowFile 把列表行下标换成 a.files 下标。列表里除了文件卡还有分组头,
// 两者混在同一个 rows 序列里,所以行号不能直接当文件号用。
func (a *app) rowFile(ri int) int {
	rows := buildRows(a.files)
	if ri < 0 || ri >= len(rows) || rows[ri].header {
		return -1
	}
	return rows[ri].file
}

// newNameFor 返回第 i 个文件的预览新名字;没有预览结果时返回空串
// (右键菜单用它判断"复制新名字"要不要可点)。
func newNameFor(a *app, i int) string {
	if i < 0 || i >= len(a.ops) {
		return ""
	}
	return a.ops[i].NewName
}

// revealInFileManager 在系统文件管理器里定位这个文件。
// mygo 没有跨平台的"显示文件"接口(只有 OpenURL),所以各平台各叫各的。
func revealInFileManager(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// explorer 的 /select 后面直接跟路径,中间不能有空格
		cmd = exec.Command("explorer", "/select,"+path)
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	}
	_ = cmd.Start() // 打不开就算了:这不是改名流程的一环
}

// ---- right pane: file table with drop zone ----

// paneHeader 是三栏标题栏的唯一写法:一个 34 DIP 的固定槽,标题在槽里垂直居中。
//
// 以前三处各写一遍,写法还不一样:左栏 MinHeight(34) + Center,中栏多一个
// Grow + SingleLine + Ellipsis,右栏 Height(34) 套 MinHeight(34) + Start。
// 三种写法里,中栏那份会按多行量文本高度、右栏那份按单行量,而 Pango(Linux)
// 对两种量法给出的高度不同——CI 的 ubuntu 上中栏标题因此比左栏低 1 DIP,
// 落不到同一条基线上;Windows 和 macOS 的度量恰好一致,所以只有 ubuntu 红。
//
// 现在高度由槽决定,文本只负责在槽里居中:不管哪种字体、哪种量法,槽都是
// 34,居中结果就都是同一个 y。
func paneHeader(c *ui.Context, title string, extra func()) {
	ui.Row(c).Gap(8).Height(34).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).Bold().FontSize(14).Grow(1).SingleLine().Ellipsis("…")
		if extra != nil {
			extra()
		}
	})
}

// paneTitle 是不带右侧控件的标题,和 paneHeader 共用同一个 34 DIP 槽。
func paneTitle(c *ui.Context, title string) {
	ui.Row(c).Shrink(0).Height(34).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).Bold().FontSize(14).SingleLine().Shrink(0)
	})
}

// resultColW 是右缘"结果"那一列的固定宽度:表头与每行卡片共用同一个
// 值,两处各写一个 64 的话,改一处忘另一处,表头就不再压在结果上方。
const resultColW float32 = 64

// dropZoneDash 是虚线拖放区的边框色。t.Border 直接压在卡片底上只有 2.4:1,
// 低于 WCAG 对非文本界面元素要求的 3:1——而这个框是空态里唯一的落点提示,
// 淡到看不见就等于没画。往文字色方向混一点,让它一眼能看出来,又不抢标题的黑。
func dropZoneDash(t *ui.Theme) ui.Color {
	return t.Border.Mix(t.Text, 0.35)
}

// dropZone 是右栏的空态:一个落点提示 + 一个真正的"添加文件…"大按钮。
//
// 以前这里是两行居中灰字,写着"把文件拖进窗口"。两个问题:
//  1. 整窗都能接文件(Window.OnFileDrop),于是"拖到哪儿"没有视觉答案——
//     用户拖到半路不知道松手会发生什么,也不会为了试而真去松手。
//  2. 空态里最该点的"添加文件…"只是工具栏上一个 14px 的次要按钮,而正文
//     区那两行字反而更像装饰。
//
// 现在把落点画出来,但只在真的拖着文件时才画:平时就摆一个虚线框,等于把
// "可以往这儿拖"说成"你正在往这儿拖"——虚线框在桌面软件里的含义是"松手
// 落在这里",一个常驻的虚线框没有落点可言,只是装饰。
//
// over 有两个来源:框自己的 FileDragOver(指针就在框上),和窗口根节点的
// (指针在窗口的任何地方)。后者见 view() 里根节点的注释:mygo 没有窗口级
// 的"文件悬停"事件,只能靠一个覆盖整窗的元素每帧被问一次。
func (a *app) dropZone(c *ui.Context, availW float32) {
	t := c.Theme()
	pt := a.currentTheme().panes(a.isDark())
	// 右栏能拖到只剩一百多 DIP:那时两行说明会把框撑得比内容还高,大号
	// 按钮也放不下并排的第二个。窄了就只留标题和主按钮,其余收进 Tooltip。
	narrow := availW < 240
	dash := dropZoneDash(t)
	// 顺序有讲究:DroppedFiles 只在"收到文件的那一帧"返回路径,所以必须
	// 每帧无条件调用(它同时把这一帧的落点登记成接文件的元素)。
	zone := ui.Column(c).Grow(1).FillWidth().Center().Gap(10).Padding(20).Radius(14)
	if paths := zone.DroppedFiles(); len(paths) > 0 {
		// 拖放是在构建这一帧的途中发现的,当场 append 会让正在逐行构建的
		// 列表长度对不上,所以排到下一帧开头再加。
		a.pendingDrop = append(a.pendingDrop, paths...)
	}
	over := zone.FileDragOver() || a.dragOver
	// 平时留一圈透明边框:尺寸按 border box 算,边框一直占着那 1px,
	// 拖拽时换成虚线就不会让框里的内容跳一下。
	if over {
		zone = zone.Background(t.Accent.Alpha(0.08)).Border(1, dash).BorderStyle(ui.BorderDashed)
	} else {
		zone = zone.Background(pt.card).Border(1, ui.Color{})
	}
	zone.Children(func() {
		// 空态给一张插图:纯文字的空白区读起来像"这里什么都没有",
		// 而一张说明"能放什么"的图在同一个位置回答"我该干什么"。
		// 图标用 currentColor 描边,跟着弱化文字色走,不抢标题。
		if !narrow {
			ui.Icon(c, dropIllus()).FontSize(48).TextColor(t.TextMuted.Alpha(0.75))
		}
		title := "把文件或文件夹拖到这里"
		if over {
			title = "松开就加进来"
		}
		ui.Text(c, title).FontSize(18).Bold().TextColor(t.Text).SingleLine()
		if !narrow {
			// 标题和两个按钮之间只留一个"或":按钮本身已经写清是什么,
			// 再补一句"也可以点下面的按钮挑"等于替用户读按钮。
			ui.Text(c, "或").FontSize(12).TextColor(t.TextMuted)
		}
		ui.Row(c).Gap(8).Wrap().Justify(ui.Center).Padding(4, 0).Children(func() {
			// 大号:空态里这是唯一的主动作,不该和工具栏里那些次要按钮同尺寸。
			//
			// 实心主色而不是 14% 淡底:空态是"还没开始"的屏幕,唯一的主
			// 动作必须是这一屏最重的东西。淡底按钮和工具栏里的次要按钮
			// 长得几乎一样,用户扫一眼分不出该点哪个。字色用 AccentText
			// (随主色深浅自动取黑或白),不用 accentInk——后者是给淡底用的。
			add := ui.ButtonBase(c).Radius(10).Padding(10, 20).TextColor(t.AccentText).
				Background(t.Accent)
			if add.Hovered() {
				add = add.Background(t.AccentHover)
			}
			add.Children(func() { ui.Text(c, "添加文件…").FontSize(14).SingleLine() })
			if add.Clicked() {
				go a.addFilesDialog()
			}
			if !narrow {
				// 拖文件夹能成,挑文件夹却只能挑文件,那条路等于没走通。
				folder := frameButton(c, "添加文件夹…", true)
				if folder.Clicked() {
					go a.addFolderDialog()
				}
			}
		})
		if !narrow {
			ui.Text(c, "以文件夹拖入的会自动分为一组").
				FontSize(12).TextColor(t.TextMuted)
		}
	})
}

func (a *app) filesView(c *ui.Context) {
	t := c.Theme()
	// 右栏自己有多宽:分隔缝能一路拖到几乎关闭,那时元数据行的三颗
	// 胶囊并排必然折行。外层 splitPanes 减两侧 8 的内边距与 10 的缝,
	// 内层 splitPanesInset 再减中栏与另一条 10 的缝。
	winW, _ := c.Size()
	rightW := winW - 2*8 - 10 - a.splitL - 10 - a.splitM
	innerW := rightW - 20 // 卡片左右各 10 的内边距
	// 结果列要 64 DIP 加 8 的间隔,再留给名字组至少一百:低于这条线
	// 就把结果词换成 Tooltip,只留状态点(实测 194 DIP 时"将更名"被
	// 挤到视口外,只剩 9.9 DIP 宽的一截)。
	showResultText := innerW >= 200
	// 再窄下去连「名字 + . + 扩展名」都排不下:文件名胶囊的 MinWidth(70)
	// 加上扩展名就超了可用宽度,扩展名会被压成 9px 宽的一列(实测 114
	// DIP 时「flac」竖排)。那时只留文件名,扩展名收进 Tooltip。
	showExt := innerW >= 124
	// 只剩状态点时那一列不需要 64:还占 64 的话,极窄的右栏里名字组
	// 只剩几个像素,连「…」都画不出来。收窄到状态点 + 间隔。
	resultW := resultColW
	if !showResultText {
		resultW = 14
	}
	// 结果列在右缘,离名字有几百像素:扫"这个名字对不对"的时候,眼睛
	// 不会往那儿走。宽到一定程度就在名字后面补一颗同色的点——它是结果
	// 列的色彩回声,不是第二套状态,颜色永远和右边那一颗一样。
	//
	// 窄的时候不要:右栏一窄,结果列本来就贴在名字右边十几像素,再补
	// 一颗就成了同一个圆点画两遍。
	showEcho := innerW >= 320
	// 右边栏:结果空间,浮在基底之上的一张浅卡片(比中栏低一阶),也是圆角。
	pt := a.currentTheme().panes(a.isDark())
	ui.Column(c).Fill().Gap(8).Padding(10).Radius(14).Background(pt.right).Clip().
		Shadow(0, 2, 10, 0, pt.shadow).Disabled(a.busy).DimDisabled(false).Children(func() {
		// ① 工具栏(吸顶,固定在右栏顶部):这里的动作都是"对文件列表本身"的操作——
		// 添加、导出、清空。用次要的 Tonal / 危险样式,不跟右下角的主操作抢。
		// 外层只负责"至少 34 高、垂直居中",内层负责换行。
		// 两个都挂在同一个 Row 上是不行的:Wrap 一旦生效,MinHeight 就不再
		// 撑高度(实测右栏标题 y=63.00,另两栏 63.67),三栏标题就差半像素
		// 落不到同一条基线上。
		// Gap(8):标题的 Row 是 Shrink(0) 的,工具栏紧跟着它,不写 Gap
		// 就是零间距——实测「文件」右缘 686.0 与排序按钮左缘 686.0 正好
		// 贴上(报告量到的 686 vs 685.9 就是这个,不是重叠,是贴死)。
		ui.Row(c).Gap(8).MinHeight(34).AlignItems(ui.Start).Children(func() {
			paneTitle(c, "文件")
			ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				// 排序菜单紧挨着"文件":它改的是这一整列的次序,和"添加/导出"
				// 那种"对清单做一件事"不同,所以放在标题与动作之间。
				// 拖动重排会打乱排序结果,所以两条路都留着:排序是"按某个键
				// 重新排一遍",拖动是"就这一次,按我的手排"。
				//
				// PopoverMenuButton 而不是 MenuButton:后者弹的是系统菜单,在
				// 自绘的圆角卡片旁边是另一套字体、另一套阴影、另一套高亮色,
				// 看起来像从别的程序里借来的。这个菜单画在窗口里,用同一套主题。
				ui.PopoverMenuButton(c, sortLabel(a.sortBy, a.sortDesc), func(m *ui.Menu) {
					for _, opt := range []struct{ by, label string }{
						{"", "添加顺序"},
						{"name", "按名称"},
						{"date", "按修改时间"},
						{"size", "按大小"},
					} {
						if m.Item(opt.label).Radio(a.sortBy == opt.by).Chosen() {
							a.sortBy = opt.by
							a.sortDesc = false
							a.pendingSort = true
						}
					}
					m.Separator()
					if m.Item("倒序").Checked(a.sortDesc).Disabled(a.sortBy == "").Chosen() {
						a.sortDesc = !a.sortDesc
						a.pendingSort = true
					}
				})
				addBtn := tonalButton(c, "添加文件…", true).Tooltip(shortcutTip("Ctrl+O"))
				if addBtn.Clicked() {
					go a.addFilesDialog()
				}
				// 按钮只说"清单",不说"CSV":用户不知道 CSV 里存了什么,
				// 更不知道它为什么能救回被改乱的名字。解释放进 Tooltip,
				// 那里有位置把"四列 + sha256 指纹"说完整。
				expBtn := tonalButton(c, "导出清单…", len(a.files) > 0).Tooltip(
					"把这一批文件导出成一张表：每行有文件指纹（sha256）、所在文件夹、现在的名字、想改成的新名字。\n" +
						"在 Excel 里改 new_name 列，再点「导入清单」——因为带指纹，文件就算已经被改过名，也能对回自己那一行。")
				if expBtn.Clicked() {
					go a.exportCSV()
				}
				// 导入CSV 原来在左栏"高级"组里,可它作用于文件清单——和导出
				// 是同一件事的两头,被拆到两个栏里谁也找不到另一半。
				impBtn := tonalButton(c, "导入清单…", !a.busy).Tooltip(
					"读回改好的清单，按每行的 sha256 找文件：文件被改过名也找得到；内容被改过会标成「有问题」，不会硬改。")
				if impBtn.Clicked() {
					go a.importCSV()
				}
				if a.confirmClear && a.confirmStale() {
					a.dropConfirm() // 挂太久:当用户改主意,收回确认态
				}
				if a.confirmClear {
					// 二次确认态:换成红色实心,明确告诉用户"再点就真删了"
					confirm := ui.ButtonBase(c).Radius(6).Padding(8, 12).Background(t.Danger).TextColor(ui.RGB(255, 255, 255))
					if confirm.Hovered() {
						confirm = confirm.Background(t.Danger.Alpha(0.85))
					}
					confirm.Children(func() { ui.Text(c, "确认清空?").SingleLine() })
					if confirm.Clicked() {
						// 清空之前先把整份清单存起来:31 个文件、当时的
						// 添加顺序、当时的排序,重新拖一遍都找不回来。
						a.clearedFiles = a.files
						a.clearedOps = a.ops
						a.clearedOrder = a.nextOrder
						a.clearedSort, a.clearedDesc = a.sortBy, a.sortDesc
						a.clearedAt = time.Now()
						a.files, a.ops, a.applied = nil, nil, false
						a.appliedAt, a.appliedCount = time.Time{}, 0
						// 序号从 #1 重新开始,排序回到"添加顺序":清空之后
						// 还留着"按大小"的话,下一批文件一进来就莫名其妙
						// 不是用户添加的顺序了。
						a.nextOrder = 0
						a.sortBy, a.sortDesc = "", false
						a.dropConfirm()
						a.status = fmt.Sprintf("已清空 %d 个文件，可以撤回", len(a.clearedFiles))
						a.scheduleClearExpiry()
					}
				} else {
					clrBtn := dangerButton(c, "清空列表", len(a.files) > 0)
					if clrBtn.Clicked() {
						a.armConfirm("clear") // 防错:两次点击才清空
					}
				}
			})
		})
		// ② AI 预览:也是"文件列表"上的操作(对清单里的文件联网起名),留在顶部
		if a.hasAIStep() {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				previewLabel := "预览（AI 联网命名）"
				if a.busyAI {
					previewLabel = "联网命名中…"
				}
				// 停止只在底栏放一个:预览按钮在忙时是灰的(禁用元素不弹
				// Tooltip,也按不动),而底栏那一颗在所有情况下都看得见。
				if primaryButton(c, previewLabel, !a.busy && len(a.files) > 0).Clicked() {
					// 不在这里开 goroutine:previewAsync 会先把 busyAI 等状态立起来
					// 再自己把真正耗时的部分丢到后台。否则点击返回到断言之间还
					// 没进 busy 态,mac 上调度偏慢时测试就会偶发看到"点完还是空闲"。
					a.previewAsync()
				}
			})
		}
		// 刚清空列表的撤回条:清空之后清单是空的,所以这条必须画在空态
		// 之上——否则"已清空 31 个文件"和退路一起消失,用户只剩重拖一遍。
		//
		// 和删步骤的撤回条同一个时限、同一个位置逻辑:动作发生在这一栏,
		// 退路也在这一栏。
		if len(a.files) == 0 && len(a.clearedFiles) > 0 && !a.clearUndoExpired() {
			ui.Row(c).FillWidth().Gap(8).Padding(8, 10).Radius(6).
				Background(t.SurfaceHover).AlignItems(ui.Center).Children(func() {
				ui.Text(c, fmt.Sprintf("已清空 %d 个文件", len(a.clearedFiles))).
					FontSize(12).TextColor(t.Text).Grow(1).SingleLine().Ellipsis("…")
				if softButton(c, "撤回", true).Radius(6).Clicked() {
					a.undoClear()
				}
			})
		} else if len(a.clearedFiles) > 0 && a.clearUndoExpired() {
			// 过期就丢掉快照:留着它只会在下次清空时被覆盖,而中间这段时间
			// 它一直占着内存(31 个文件的 fileInfo)。
			a.dropClearUndo()
		}
		if len(a.files) == 0 {
			a.dropZone(c, innerW)
		} else {
			dt := a.currentTheme()
			// 分组:文件夹拖入的文件按文件夹分组,用 List 内置的吸顶分组头
			// (ListState.Header):滚动时当前分组的文件夹名固定在顶部,滚到下一个
			// 分组时新的文件夹名把它顶上去——文件再多也知道"现在是哪个文件夹"。
			//
			// 关键:分组头必须**独立成行**,不能兼任该组第一个文件那一行。
			// mygo 的吸顶不是裁剪内容,而是把整行元素的 y 直接设成列表顶部
			// (ui/list.go pinHeader);头和卡同行时,钉住这一行 = 钉住第一张卡,
			// 它会永远浮在顶部压住后面滚上来的卡。所以行数 = 文件数 + 分组数。
			rows := buildRows(a.files)
			a.table.Key = func(ri int) any {
				if ri < 0 || ri >= len(rows) {
					return ri
				}
				if rows[ri].header {
					return "h:" + rows[ri].group
				}
				return "f:" + a.files[rows[ri].file].path
			}
			a.table.Header = func(ri int) bool {
				return ri >= 0 && ri < len(rows) && rows[ri].header
			}
			// 选中一行文件:以前文件列表完全不可点,想删掉一个文件只能
			// 清空整表重来。Selected 让点击/方向键选中,Delete 与右键菜单
			// 都作用在选中的那一行上。
			a.table.Selected = &a.fileSel
			// 拖动重排:rows/to 都是**列表行下标**(含分组头),要先换回
			// a.files 的下标。而且只在同一分组内允许移动——分组头是"组名
			// 与上一行不同"时才出现的,跨组拖会把两个文件夹的文件交错,
			// 于是凭空长出一堆组头,每组只剩一个文件。
			a.table.Reorder = func(moved []int, to int) {
				if from, dest, ok := reorderTarget(a.files, rows, moved, to); ok {
					a.pendingMove = &[2]int{from, dest}
				}
			}
			// 执行完之后,右栏顶部要说清"刚才发生了什么、还能不能退":
			// 以前只有左下角一行 12px 灰字,而改名是当天最重要的事。
			if a.applied && a.appliedCount > 0 && !a.appliedAt.IsZero() {
				ui.Row(c).FillWidth().Gap(8).Padding(4, 10).Radius(6).
					Background(t.Success.Alpha(0.12)).AlignItems(ui.Center).Children(func() {
					ui.Text(c, fmt.Sprintf("已执行 · %s · %d 个文件",
						a.appliedAt.Format("01-02 15:04"), a.appliedCount)).
						FontSize(12).TextColor(t.Text)
					ui.Spacer(c)
					// 和底栏的「撤销上次」同一套防错:撤销会动磁盘,不能
					// 因为按钮画在横幅里就变成一点就退——同一个动作有两套
					// 强度,用户迟早会在弱的那一侧误触。
					if a.confirmUndo && a.confirmStale() {
						a.dropConfirm()
					}
					if a.confirmUndo {
						label := fmt.Sprintf("确认撤销 %d 个?", len(a.journalInfoNames()))
						confirm := ui.ButtonBase(c).Radius(6).Padding(8, 12).
							Background(t.Danger).TextColor(ui.RGB(255, 255, 255))
						if confirm.Hovered() {
							confirm = confirm.Background(t.Danger.Alpha(0.85))
						}
						confirm.Children(func() { ui.Text(c, label).SingleLine() })
						if confirm.Clicked() {
							a.dropConfirm()
							go a.undo()
						}
					} else if outlinedButton(c, "撤销上次", !a.busy && a.journalExists()).Clicked() {
						a.armConfirm("undo")
					}
				})
			}
			// 整轮预览算不出来:这是"一个都算不出来",和下面那条"某几个
			// 文件改不成"是两回事,所以单独一条、颜色更重。以前这条分支
			// 只清空 a.ops,右栏于是只剩光秃秃的文件名——用户看不出
			// 是"没配步骤"还是"步骤写错了",更不知道错在哪。
			if a.previewErr != "" {
				ui.Row(c).FillWidth().Gap(6).Padding(4, 8).Radius(6).
					Background(t.Danger.Alpha(0.14)).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "预览算不出来").FontSize(12).TextColor(t.Danger).Shrink(0)
					ui.Spacer(c)
					ui.Text(c, a.previewErr).FontSize(12).TextColor(t.Danger).MaxWidth(280).
						SingleLine().Ellipsis("…").Tooltip(a.previewErr)
					// 知道是哪一步写坏了,就直接把人送到那一步的编辑器:
					// 光说"第 3 步有问题",用户还得回列表里数到第三行。
					if a.previewErrStep >= 0 && a.previewErrStep < len(a.steps) {
						if softButton(c, "去修这一步", true).Radius(6).
							Tooltip("选中出问题的那一步，右边就是它的参数").Clicked() {
							a.focusStep(a.previewErrStep)
						}
					}
				})
			}
			// 失败的聚合条:60 个文件里 5 个失败时,逐行扫描找红字
			// 是不可能完成的任务。顶部说一句"有几个没法改",比让用户
			// 自己数快得多。
			if nFail := countErrs(a.ops); nFail > 0 {
				ui.Row(c).FillWidth().Gap(6).Padding(4, 8).Radius(6).
					Background(t.Danger.Alpha(0.10)).AlignItems(ui.Center).Children(func() {
					ui.Text(c, fmt.Sprintf("%d 个文件无法更名", nFail)).FontSize(12).TextColor(t.Danger)
					ui.Spacer(c)
					msg := firstErrText(a.ops)
					if msg != "" {
						ui.Text(c, msg).FontSize(12).TextColor(t.Danger).MaxWidth(240).
							SingleLine().Ellipsis("…").Tooltip(msg)
					}
				})
			}
			// 序号列要够宽才画:三位数序号 + 右侧 8 的呼吸位。
			const seqW float32 = 30
			showSeq := innerW >= 260
			// 列头:结果列离名字有几百像素,没有表头的话右边那列只是
			// "一个小圆点加两个字",用户得自己猜那是结果。表头只在有文件时
			// 出现——空列表时它只是一行噪音。
			// Wrap + SingleLine:右栏能被拖到只剩一百多 DIP,那时"原名 → 新名"
			// 会被压成一列竖排单字("原"/"名"/"→"...),比截断更难看。
			// 宁可把"结果"表头换到第二行。
			ui.Row(c).FillWidth().Gap(8).PaddingStart(10).Wrap().Children(func() {
				// 序号列在行里占 seqW + gap,列头必须让出同样的宽度,
				// 否则"原名 → 新名"从 x=598 起、名字胶囊从 x=636 起,
				// 表头说的那一列和它下面那一列差了 38 DIP,看着像没对齐。
				if showSeq {
					ui.Box(c).Width(seqW).Height(1).Shrink(0)
				}
				ui.Text(c, "原名 → 新名").FontSize(12).TextColor(t.TextMuted).SingleLine()
				ui.Spacer(c)
				// 与行内同一套降级:结果词收起来时表头也跟着收,否则
				// 表头说"结果"而下面只剩一个圆点,反而更费解。
				if showResultText {
					ui.Text(c, "结果").FontSize(12).TextColor(t.TextMuted).Width(resultW).TextAlign(ui.End).SingleLine()
				}
			})
			// 名字胶囊的最小宽度:110 是"能认出这是哪个文件"的下限
			// (放得下 "01-…-01.mkv" 这种最常见的形态),但右栏能拖到
			// 只剩一百多 DIP,那时硬撑 110 会把整组顶出窗口。
			// 按可用宽度分档,而不是写死:保住"名字还在、还能认"这件事,
			// 比保住那个具体数字重要。
			// 名字这一行实际能用的宽度:卡片内边距(左右各 10)、结果列、
			// 两者之间的 8、以及行尾那颗 18 宽的 × 按钮都要先扣掉。
			// 110 是"能认出这是哪个文件"的下限(放得下 "01-…-01.mkv"),
			// 但右栏能拖到只剩一百多 DIP——那时硬撑 110 会把结果列顶出
			// 窗口(实测 splitL=240 时"将更名"跑到 909 > 900)。
			// 所以按实际可用宽度夹紧:宁可名字缩成 "…",也不能让
			// 扩展名或结果词掉出去。
			nameAvail := innerW - 20 - resultW - 8 - 18
			nameMinW := float32(110)
			if nameAvail < 150 {
				nameMinW = 30
			} else if nameAvail < 200 {
				nameMinW = 70
			}
			// 元数据行的两档降级:先丢日期(信息量最小),再丢大小。
			showDate, showSize := innerW >= 200, innerW >= 120
			ui.List(c, &a.table, len(rows), func(ri int) {
				r := rows[ri]
				// 分组头独立一行:吸顶的文件夹名 + 组内文件数
				if r.header {
					// 吸顶头:实底(和右栏同色)盖住从下面滚过的文件卡,
					// 读起来像"钉住"的标签条。
					ui.Row(c).Fill().Gap(8).Padding(8).
						Background(dt.panes(a.isDark()).right).Children(func() {
						ui.Text(c, r.group).FontSize(14).Bold().TextColor(t.Text).SingleLine().Ellipsis("…")
						// 组头必须带数量:只有一个分组时,标题栏右侧那句
						// 「N 个文件」说的是整个清单,而组头说的是"这个
						// 文件夹里有多少"——两者不是同一句话。何况折叠、
						// 排序之后,用户最常问的正是"这一组里到底几个"。
						ui.Text(c, fmt.Sprintf("%d 个", r.count)).FontSize(12).
							TextColor(t.TextMuted).SingleLine().Shrink(0)
						ui.Spacer(c)
					})
					return
				}
				i := r.file
				f := a.files[i]
				// 属于分组时缩进一点,视觉上从属于上面的文件夹头
				var indent float32 = 0
				if f.group != "" {
					indent = 14
				}
				// 行底先铺一层不透明的栏底色。mygo 的列表会给"选中行"刷一层
				// 主色底(它不知道我们用卡片浮在行上的结构),那层主色只会在
				// 卡片与结果列之间的 8px 缝里露出来,像一块贴错位置的绿条。
				// 铺掉它,选中态改由卡片自己表达——卡片是这一行真正的内容。
				selected := a.fileSel == ri
				row := ui.Row(c).Fill().Gap(8).PaddingStart(indent).Background(pt.right)
				// 右键菜单:mygo 的 ContextMenu 会在菜单打开时跑一次 build,
				// 再在"某一项被选中"的那一帧跑第二次,用 Chosen() 判断。
				// 所以每个 Item 都要先建、再问 Chosen——顺序不能反。
				row.ContextMenu(func(m *ui.Menu) {
					del := m.Item("从列表移除").Shortcut(0, ui.KeyDelete)
					if del.Chosen() {
						a.pendingRemove = i + 1
					}
					m.Separator()
					reveal := m.Item("在文件管理器中显示")
					if reveal.Chosen() {
						revealInFileManager(f.path)
					}
					copyNew := m.Item("复制新名字").Disabled(newNameFor(a, i) == "")
					if copyNew.Chosen() {
						c.WriteClipboard(newNameFor(a, i))
					}
					m.Separator()
					copyOld := m.Item("复制原文件名")
					if copyOld.Chosen() {
						c.WriteClipboard(filepath.Base(f.path))
					}
				})
				row.Children(func() {
					// 序号列:这一行的 #N 就是模板里 <N> 的值,也就是
					// 「追加序号」这类步骤发号的顺序。以前发号顺序只存在
					// 于 a.files 的下标里,用户看不见也改不了——排序或拖动
					// 之后想知道"第 3 个文件会变成 03 还是 04",只能执行一次
					// 再回滚。窄栏(<260)时藏起来:那时连名字都只剩省略号,
					// 再挤一列数字只会把名字压得更短。
					if showSeq {
						ui.Text(c, "#"+strconv.Itoa(i+1)).FontSize(12).TextColor(t.TextMuted).
							Width(seqW).TextAlign(ui.End).Shrink(0).FontFeatures("tnum").
							Tooltip("这一行是第 " + strconv.Itoa(i+1) + " 个，模板里的 <N> 取这个值")
					}
					// 卡内上下 8、卡间 12:内边距必须小于间距,分组才成立
					// (PUI p.18 的"内 < 外")。两个值都在 4px 网格上。
					card := ui.Column(c).Fill().Gap(4).Padding(8, 10).Radius(10).
						Background(dt.panes(a.isDark()).card)
					// 选中态:键盘上下键选中的那一行必须看得见。以前 a.fileSel
					// 只喂给列表做"当前行"记账,渲染里一次都没读,于是按方向键
					// 选中文件后界面毫无变化,再按 Delete 就把一个看不见的
					// 选择删掉了。底色 + 主色描边两重编码:底色回答"选了哪行",
					// 描边回答"这一行还是可操作目标"。
					if selected {
						card.Background(t.Selection).Border(1, t.Accent.Alpha(0.65))
					}
					// 失败的行要能一眼从"将更名"里挑出来:整卡降饱和太狠会
					// 连文件名都读不清,所以只加一道危险色描边,文字保持原样。
					if i < len(a.ops) && a.ops[i].Err != nil {
						card.Border(1, t.Danger.Alpha(0.55))
					}
					// 刚改名的那几行亮一下再淡出:执行成功是当天最重要的事,
					// 以前只有左下角一行 12px 灰字。
					if i < len(a.ops) && a.ops[i].Applied {
						if k := a.flashAlpha(); k > 0 {
							card.Border(1, t.Success.Alpha(0.55*k)).Background(t.Success.Alpha(0.16 * k))
						}
					}
					card.Children(func() {
						oldName := filepath.Base(f.path)
						newName := oldName
						if i < len(a.ops) && a.ops[i].NewName != "" {
							newName = a.ops[i].NewName
						}
						changed := newName != oldName
						oStem, oExt := filepathSplitName(oldName)
						nStem, nExt := filepathSplitName(newName)
						// 类型色系:原名组按原扩展名,预览组按新扩展名——
						// 改扩展名时颜色随类别变化,本身就是信息
						oCol := a.fileColor(dt, oExt)
						nCol := a.fileColor(dt, nExt)
						// 名字组 = [胶囊文件名] . [胶囊扩展名],点是两胶囊之间
						// 的独立文本;原名中性胶囊组,预览类型色胶囊组。
						// 整组包在一个不换行的 Row 里:否则外层 Wrap 会把
						// 中间的「.」留在上一行、扩展名胶囊掉到下一行,
						// 读起来像文件名后面多了个孤零零的句点。
						// 压缩由文件名胶囊独自承担:它有 MinWidth(70) + Ellipsis,
						// 压到极限也只是省略号。扩展名胶囊 Shrink(0),四个字母
						// 本来就不该被挤——实测被压时会缩成 9px 宽的一列。
						nameChips := func(stem, ext string, stemBg, extBg, extFg ui.Color) {
							// Wrap:这一组自己也允许换行。外层那个 Wrap 只能把
							// "整组"挪到下一行,组内溢出时它管不了——实测右栏
							// 内宽 124~154 这一档,文件名胶囊的 MinWidth(70) 加上
							// 扩展名胶囊的 Shrink(0)(webm 就有 57.7 宽)必然超过
							// 可用宽度,于是「webm」被顶到窗口外 936.8 > 900。
							// 组内换行让扩展名掉到第二行,虽然多占一行,但至少
							// 还在窗口里。
							// 不 Wrap:扩展名必须和名字同一行。允许换行时,
							// 组内放不下就把「. flac」单独甩到下一行,那一行
							// 只剩一个句点和三颗字母,读起来像渲染坏了。
							// 宽度不够就让名字收缩(它有 TruncateMiddle),
							// 名字的最小宽度由 nameMinW 按可用宽度分档。
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								// TruncateMiddle:超长文件名从中间省略,保留开头和结尾。
								// 尾部省略("01 - Awich - THE UNION 很长…")把真正
								// 用来认文件的尾巴和扩展名一起切掉了,而这一列的
								// 任务就是"让人认出这是哪个文件"——开头是序号/艺人,
								// 结尾是集数/版本/扩展名,两头都比中间有信息量。
								//
								// MinWidth(110):以前是 70,那一档下"第2话"这种短名
								// 也会被压到只剩两三个字,而 110 正好放得下
								// "01-…-01.mkv" 这类最常见的形态。
								ui.Text(c, stem).FontSize(14).Padding(2, 10).Radius(6).
									Background(stemBg).MinWidth(nameMinW).MaxWidth(190).
									Shrink(1).SingleLine().TruncateMiddle()
								if ext != "" {
									ui.Text(c, ".").FontSize(14).TextColor(t.TextMuted)
									// Shrink(0):扩展名是最短的一颗胶囊,允许它收缩时
									// 最先塌掉(实测「flac」被挤成 9px 宽)。
									ui.Text(c, strings.TrimPrefix(ext, ".")).FontSize(14).Padding(2, 10).Radius(6).
										Background(extBg).TextColor(extFg).SingleLine().Shrink(0)
								}
							})
						}
						// Wrap:窗口窄到放不下「原名 → 新名」时换行,而不是把
						// 文件名压成「…」。名字是这个软件唯一要看的东西,
						// 宁可卡片长高一点,也不能让名字读不出来。
						ui.Row(c).Gap(4).Wrap().Children(func() {
							// 原文件名:胶囊文件名 . 胶囊扩展名(原类型色,淡)
							if showExt {
								nameChips(oStem, oExt, oCol.Alpha(0.10), oCol.Alpha(0.30), t.Text)
							} else {
								nameChips(oStem, "", oCol.Alpha(0.10), oCol.Alpha(0.30), t.Text)
							}
							if changed {
								// 「→ 新名」包成一组:Wrap 换行时箭头要跟着新名字
								// 一起换到下一行,否则窄窗下箭头孤零零挂在行尾,
								// 读起来像"改成了空"。
								ui.Row(c).Gap(4).Children(func() {
									ui.Text(c, "→").TextColor(t.TextMuted)
									// 预览:胶囊文件名 . 胶囊扩展名(新类型色,实)。
									// 以前这里用饱和色底 + 白字,实测只有 3.1–3.6,
									// 和上面那排类别胶囊是同一个毛病;改成同色相浅底深字,
									// 比原名胶囊重一档即可——要的是"能读出来",不是"很显眼"。
									nExtBg, nExtFg := chipTonesEmph(nCol, a.isDark())
									if showExt {
										nameChips(nStem, nExt, nCol.Alpha(0.18), nExtBg, nExtFg)
									} else {
										nameChips(nStem, "", nCol.Alpha(0.18), nExtBg, nExtFg)
									}
								})
							} else if a.previewErr != "" {
								// 整轮算不出来:这里以前什么都不画,而右边原本该
								// 出现新名字的位置留空——用户看到的是"这个名字
								// 没有要改的地方",其实是一个名字都算不出来。
								ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
									ui.Text(c, "→").TextColor(t.TextMuted)
									ui.Text(c, "等步骤修好").FontSize(14).Padding(2, 10).Radius(6).
										Background(t.SurfaceHover).TextColor(t.TextMuted).SingleLine()
								})
							}
							// 结果的颜色回声:名字旁边一颗同色点。
							if showEcho {
								if i < len(a.ops) {
									res, echo := a.resultChip(a.ops[i])
									ui.Box(c).Size(6, 6).Radius(6 / 2).
										Background(echo).Shrink(0).Tooltip(res)
								} else if a.previewErr != "" {
									ui.Box(c).Size(6, 6).Radius(6 / 2).
										Background(neutralStroke(t)).Shrink(0).
										Tooltip("预览算不出来：" + a.previewErr)
								}
							}
							ui.Spacer(c)
							// 悬停出 ×:200 个文件里只有 3 个不想改时,不必
							// 为这 3 个清空整表。画在卡片名字行右端而不是
							// 卡片外面——飘在卡外的红点看着像错误标记。
							// 宽度占位始终留着,否则一悬停整行就抖一下。
							ui.Row(c).Width(18).AlignItems(ui.Center).Children(func() {
								if !row.Hovered() {
									return
								}
								x := ui.ButtonBase(c).Size(18, 18).Radius(18 / 2).Padding(0).
									Background(t.Danger.Alpha(0.12)).Tooltip("从列表移除（Delete）")
								x.Children(func() {
									ui.Text(c, "×").FontSize(12).TextColor(t.Danger)
								})
								if x.Clicked() {
									a.pendingRemove = i + 1
								}
							})
						})
						// 下行:原始数据胶囊。
						// 一个文件行里说同一件事两次是噪音:扩展名胶囊已经写着
						// 「mp3」,再挂一个「MP3 音频」只是把同样的信息换个说法;
						// 文件夹名已经在上面的分组头里,每行再挂一遍路径胶囊
						// 等于把「Awich - THE UNION」重复 4 次。所以:
						// 路径只在没有分组头时出现,类型只在扩展名缺失时出现,
						// 内容嗅探只在真不符时出现(相符是默认,不用报)。
						//
						// Wrap 与 SingleLine 都是必需的:窗口拉到 900 宽时右栏
						// 只剩约 350,这一行原来既不换行也不禁断行,于是每个
						// 胶囊被压成"日/期"竖排两行、"1 B"也竖着排。现在整颗
						// 胶囊换到下一行,胶囊内部永不断行。
						ui.Row(c).Gap(6).Wrap().Children(func() {
							if f.group == "" {
								path := ui.Text(c, pathTail3(filepath.Dir(f.path))).FontSize(12).Padding(1, 8).Radius(6).
									Background(dt.panes(a.isDark()).chip).TextColor(t.TextMuted).MaxWidth(170).SingleLine().Ellipsis("…")
								path.Tooltip(filepath.Dir(f.path))
							}
							// 真实类型:与扩展名相符时不必说;只有真不符才标红。
							// EPUB/DOCX 的容器本来就是 ZIP,m4a/mov 本来就是 MP4,
							// 按字符串判等会把整屏正常文件标红,假警报淹掉真警报。
							//
							// 但"嗅探不出来"不等于"没问题":以前 real=="" 直接
							// 静默放过,于是一个内容被写坏的 .docx(根本不是 ZIP)
							// 在界面上看起来和正常文件一模一样。现在分三档:
							// 真不符标红;声称是二进制格式却读不出魔数,给一条
							// 中性提示(不红,因为可能是加密/自定义封装);其余不画。
							extNoDot := strings.ToLower(strings.TrimPrefix(oExt, "."))
							switch contentCheck(extNoDot, f.realType, f.size) {
							case contentMismatch:
								ui.Text(c, "内容为 "+contentLabel(f.realType)).FontSize(12).Padding(1, 8).Radius(6).
									Background(t.Danger.Alpha(0.15)).TextColor(t.Danger).SingleLine().
									Tooltip("扩展名是 ." + extNoDot + "，但文件内容像 " + contentLabel(f.realType))
							case contentUnrecognized:
								ui.Text(c, "内容未识别").FontSize(12).Padding(1, 8).Radius(6).
									Background(dt.panes(a.isDark()).chip).TextColor(t.TextMuted).SingleLine().
									Tooltip("扩展名是 ." + extNoDot + "，但文件开头读不出已知的格式标记")
							default:
								if oExt == "" && f.typ != "" && f.typ != "无扩展名" {
									ui.Text(c, f.typ).FontSize(12).Padding(1, 8).Radius(6).
										Background(dt.panes(a.isDark()).chip).TextColor(t.TextMuted).SingleLine()
								}
							}
							// 日期与大小是补充信息,按宽度优先级让位:名字和
							// 结果列才是这个软件唯一要看的东西。阈值来自实测
							// 胶囊宽度(日期 73 DIP、大小最长「12.0 MB」55 DIP,
							// 每颗前后各 6 的间隔),再留出名字胶囊的余地。
							// FontFeatures("tnum") 让数字等宽:日期、大小、
							// 序号这些数在行与行之间竖着对齐,扫一列比扫
							// 一堆宽度不一的字形快得多。中文标签不受影响。
							if showDate {
								ui.Text(c, f.modTime.Format("2006-01-02")).FontSize(12).Padding(1, 8).Radius(6).
									Background(dt.panes(a.isDark()).chip).TextColor(t.TextMuted).
									FontFeatures("tnum").SingleLine()
							}
							if showSize {
								ui.Text(c, humanSize(f.size)).FontSize(12).Padding(1, 8).Radius(6).
									Background(dt.panes(a.isDark()).chip).TextColor(t.TextMuted).
									FontFeatures("tnum").SingleLine()
							}
							ui.Spacer(c)
						})
						// 失败原因直接写在卡片里:只写"失败"等于不说。
						// 允许折两行——单行 + 省略号会把最有信息量的
						// 后半句吃掉,而失败原因往往后半句才是关键。
						if i < len(a.ops) && a.ops[i].Err != nil {
							ui.Text(c, a.ops[i].Err.Error()).FontSize(12).TextColor(t.Danger).MaxLines(2)
						}
					})
					// 右缘结果:成功/失败/将更名/不变。
					// 整轮算不出来时也要占住这一列:留空的话右缘整片空白,
					// 加上新名列一起消失,读起来就是"预览没了"——而真相是
					// "预览暂时算不出来",两句话对用户的意义完全不同。
					if i >= len(a.ops) && a.previewErr != "" {
						ui.Row(c).Width(resultW).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
							ui.Spacer(c)
							dot := ui.Box(c).Size(7, 7).Radius(7 / 2).Background(neutralStroke(t))
							dot.Tooltip("预览算不出来：" + a.previewErr)
							if showResultText {
								ui.Text(c, "算不出").FontSize(12).TextColor(t.TextMuted).SingleLine().
									Tooltip("预览算不出来：" + a.previewErr)
							}
						})
					} else if i < len(a.ops) {
						// 结果是"状态",不是"类别":用状态点 + 纯文字,
						// 不用胶囊形状,免得和类别胶囊混成一种语言
						res, col := a.resultChip(a.ops[i])
						// 极窄的右栏放不下"状态点 + 三个字":整列会被挤到
						// 视口外,只剩半个字被裁掉。那时只留状态点,词收进
						// Tooltip。
						//
						// 但"只留颜色"等于把状态编码进色觉:红绿不分的人
						// 看到的是五个一样的小圆点。所以收掉文字时,点本身
						// 带上形状——实心圆=已更名、叉=失败、短横=跳过、
						// 空心圆=不变、箭头=将更名。形状在灰度下也读得出,
						// 颜色只是第二层线索。
						ui.Row(c).Width(resultW).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
							ui.Spacer(c)
							if showResultText {
								ui.Box(c).Size(7, 7).Radius(7 / 2).Background(col)
								txt := ui.Text(c, res).FontSize(12).TextColor(col).SingleLine()
								if a.ops[i].Err != nil {
									txt.Tooltip(a.ops[i].Err.Error())
								}
								return
							}
							mark, shapeTip := resultMark(a.rowOutcome(a.ops[i]))
							glyph := ui.Text(c, mark).FontSize(12).TextColor(col).SingleLine()
							tip := shapeTip + "：" + res
							if a.ops[i].Err != nil {
								tip = shapeTip + "：" + res + "：" + a.ops[i].Err.Error()
							}
							glyph.Tooltip(tip)
						})
					}
				})
			}).Gap(12).Grow(1)
		}
	})
}

// startFlash 让刚改名的行亮一下再淡出。界面只在有事件时重画,所以
// 这里自己排几次重画——不排的话高亮会停在第一帧上永远不淡出。
func (a *app) startFlash() {
	a.flashUntil = time.Now().Add(flashDuration)
	go func() {
		for time.Now().Before(a.flashUntil) {
			time.Sleep(flashStep)
			a.update()
		}
		a.update()
	}()
}

// flashAlpha 是这一帧高亮的强度:从 1 线性淡到 0,过了时间就是 0。
func (a *app) flashAlpha() float32 {
	if a.flashUntil.IsZero() {
		return 0
	}
	left := time.Until(a.flashUntil)
	if left <= 0 {
		return 0
	}
	return float32(left) / float32(flashDuration)
}

// rowState 是一行文件卡片现在处于哪种状态。文案、颜色、状态栏计数
// 全部由它派生,所以行里写什么、底栏算什么不可能各说一套。
type rowState int

const (
	rowPending   rowState = iota // 还没执行,等着改名
	rowApplied                   // 这一轮真的改了
	rowFailed                    // 这一轮失败了,原因在 op.Err
	rowSkipped                   // 执行过了,但这一行没动
	rowUnchanged                 // 名字本来就没变,不需要处理
	rowCanceled                  // 用户按了「停止」,这个名字还没算出来
)

// rowOutcome 是"这一行现在是什么状态"的唯一判定。
//
// 以前这条 switch 存在三份:resultChip 的文案与颜色、apply 之后统计成功
// 失败的那一段、还有一个没人调用的 resultText。三份里前两份的顺序正好
// 相反——resultChip 先判 Err、apply 先判 Applied——于是同一个文件会在
// 行里写"失败"、在状态栏被算进"成功"。顺序只能有一份。
func (a *app) rowOutcome(op core.RenameOp) rowState {
	switch {
	// 用户自己按的停止不是失败。混进 rowFailed 会让整列变红、底栏写
	// "12 个有问题",而网络一直好好的——人会被赶去查一个没坏的东西。
	case op.Err != nil && core.IsAICanceled(op.Err):
		return rowCanceled
	case op.Err != nil:
		return rowFailed
	case op.Applied:
		return rowApplied
	case a.applied:
		// 执行过这一轮,但这一个没动(名字不变、或被跳过)。执行之后再
		// 说"不变"没有意义,用户要问的是"这次为什么没改它"。
		return rowSkipped
	case op.OldName == op.NewName:
		return rowUnchanged
	}
	return rowPending
}

// applyOutcome 数这一次执行真正改了几个、几个失败。它和行里的结果共用
// rowOutcome,所以底栏说的数字和右栏每行写的字永远一致。
//
// 只看 rowApplied / rowFailed:名字没变化、本来就不需要处理的那些不算
// 失败,否则一次干净的批量看起来像出了很多错。
func (a *app) applyOutcome() (ok, failed int) {
	for _, op := range a.ops {
		switch a.rowOutcome(op) {
		case rowApplied:
			ok++
		case rowFailed:
			failed++
		}
	}
	return ok, failed
}

// resultChip gives the outcome text and its color.
func (a *app) resultChip(op core.RenameOp) (string, ui.Color) {
	t := a.currentTheme().light
	if a.isDark() {
		t = a.currentTheme().dark
	}
	switch a.rowOutcome(op) {
	case rowFailed:
		return "失败", t.Danger
	case rowApplied:
		// 执行之后这一行的名字就是磁盘上的名字,"成功"是结果而不是状态;
		// 说"已更名"才能和上面那些还没执行的"将更名"分开。
		return "已更名", t.Success
	case rowSkipped:
		return "跳过", t.TextMuted
	case rowUnchanged:
		return "不变", t.TextMuted
	case rowCanceled:
		return "没算", t.TextMuted
	}
	return "将更名", t.Accent
}

// accentInk 是"压在 12% 主色底上的主色文字"该用的颜色。
//
// 不能直接用 Accent:Accent 要够亮才承得住白字,拿它压自己的 12% 底
// 实测只有 3.50:1(默认绿)到 2.35:1(橙),低于 WCAG 4.5。亮色下改用
// 同色相、L 0.44 的墨色;暗色下 Accent 本身就有 5.4 以上,继续用它,
// 免得暗色按钮反而变闷。
func accentInk(t *ui.Theme) ui.Color {
	// 同色相的墨色/亮色,chroma 沿用 Accent 的(已被 seed 收敛过)。
	//
	// 暗色不再直接用 Accent:浅填充底是"Accent 20% 叠在栏底上",三栏统一
	// 到 0.265 之后那层底比原来亮,与 Accent(L≈0.72)的对比掉到 4.15。
	// 往上抬到 0.80 才重新过 4.5(实测最差 5.11)。
	_, ac, ah := colorToOklch(t.Accent)
	if t.Dark {
		return oklchColor(0.80, ac, ah)
	}
	return oklchColor(0.44, ac, ah)
}

// tonalButton 是"次要动作"按钮(添加文件/导出清单/导入清单)。
//
// 它就是全软件默认的那副面孔:主题色淡底、无边框、悬停加深。名字留着是
// 因为调用点说的是"这件事有多重",而"淡底"只是这个重量对应的画法。
func tonalButton(c *ui.Context, label string, enabled bool) ui.Element {
	return softButton(c, label, enabled)
}

// primaryButton 是"主操作"按钮(执行更名)。启用时是实心主色块,字色由
// 主题的 AccentText 决定——它按"白字与深墨取对比度高者"算,浅色相上
// 不会糊成一片(见 material.go 的 inkOn)。
//
// 禁用时和别的按不动的按钮长得一样:一块灰底(inertFill)、次要文字、
// 真 Disabled。这里曾经给过一块"褪色主色"的底,想让"主操作"的身份在
// 禁用态也留着;但那块底还是彩的,和"能点"的淡底分不开。灰底才是死的
// 记号,主次关系在能按的时候靠实心与淡底区分就够了。
func primaryButton(c *ui.Context, label string, enabled bool) ui.Element {
	if enabled {
		return ui.PrimaryButton(c, label).Radius(6)
	}
	return softButton(c, label, false)
}

// plainButton 是"最普通"的按钮:无框、淡底。
//
// 它和 outlinedButton 现在是同一颗——都交给 softButton。留两个名字是因为
// 调用点写的是"这件事有多重",不是"画成什么样"。
func plainButton(c *ui.Context, label string, enabled bool) ui.Element {
	return softButton(c, label, enabled)
}

// outlinedButton 是"中性次要动作"按钮(撤销上次)。
//
// 它原来是一颗镂空描边按钮,现在也归到默认面孔:描边不再表示"能点",
// 留着只会让底栏出现一颗和"按不动"同款的框。
func outlinedButton(c *ui.Context, label string, enabled bool) ui.Element {
	return softButton(c, label, enabled)
}

// dangerButton 是 outlinedButton 的悬停变体,不是第四种按钮样式。
//
// 它原来自己一档:错误色描边 + 错误色文字,平时就红着。问题是"清空列表"
// 和"删除"坐在工具栏里,一进场就是两个红框——一个还没发生的动作喊得比
// 当前要做的事还响。危险该在"手要落下去"的那一刻才显现,不是常驻。
//
// 所以平时和 outlined 一模一样(同样的字色、同样的边框),只在悬停时换成
// 错误色的底与字;真正的确认仍然走实心红(armConfirm 之后那颗),那里才是
// "这一下真的会删"。
//
// 它平时和别的按钮一样是主色淡底,只是多了那圈框——删除/清除/添加
// 文件夹是仅有的三个带框动作。
func dangerButton(c *ui.Context, label string, enabled bool) ui.Element {
	t := c.Theme()
	b := frameButton(c, label, enabled)
	if enabled && b.Hovered() {
		b = b.TextColor(t.Danger).Background(t.Danger.Alpha(0.10)).Border(1, t.Danger.Alpha(0.45))
	}
	return b
}

// shortcutTip 把快捷键写成界面上看得见的样子。
// 快捷键不写在界面上,用户找不到就等于不存在;而 Mac 的修饰键是 ⌘ 不是
// Ctrl,写死 "Ctrl+O" 会让 Mac 用户去按一个不存在的组合。
func shortcutTip(combo string) string {
	if runtime.GOOS == "darwin" {
		return strings.ReplaceAll(strings.ReplaceAll(combo, "Ctrl+", "⌘"), "Alt+", "⌥")
	}
	return combo
}

// filepathSplitName splits a bare file name into stem and ".ext".
func filepathSplitName(name string) (stem, ext string) {
	i := strings.LastIndexByte(name, '.')
	if i <= 0 {
		return name, ""
	}
	return name[:i], name[i:]
}

// update re-renders the window after a change made off the view pass.
func (a *app) update() {
	if a.win != nil {
		a.win.Update(func() {})
	}
}

func (a *app) addPaths(paths []string) {
	seen := map[string]bool{}
	for i := range a.files {
		seen[a.files[i].path] = true
	}
	// Each entry remembers which folder group it belongs to. A dropped
	// folder contributes all its (loose) files under that folder's name; a
	// dropped file is a group of its own (empty → no header).
	type entry struct{ path, group string }
	var expanded []entry
	// 跳过的东西要数出来。以前子文件夹、读不到的目录、stat 失败的路径
	// 都是 continue 掉,状态行只说"已添加，共 N 个文件"——拖进去一个装
	// 着 300 个文件的文件夹,只加进来 2 个,界面却像什么都没发生。
	subDirs, unreadable := 0, 0
	for _, p := range paths {
		p = filepath.Clean(p)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			// a dropped folder adds the files directly inside it (one level)
			ents, err := os.ReadDir(p)
			if err != nil {
				unreadable++
				continue
			}
			group := filepath.Base(p)
			for _, e := range ents {
				if !e.IsDir() {
					expanded = append(expanded, entry{filepath.Join(p, e.Name()), group})
				} else {
					subDirs++
				}
			}
			continue
		}
		expanded = append(expanded, entry{p, ""})
	}
	missing := 0
	for _, en := range expanded {
		p := en.path
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			missing++
			continue
		}
		if !seen[p] {
			seen[p] = true
			fi := statFile(p)
			fi.group = en.group
			a.nextOrder++
			fi.order = a.nextOrder
			a.files = append(a.files, fi)
		}
	}
	a.assignExtColors(a.currentTheme())
	note := addPathsNote(subDirs, unreadable, missing)
	a.confirmClear = false
	// 清单变了,旧的"刚清空"快照就不是上一个状态了:这时点撤回会把用户
	// 刚加进来的文件整批盖掉。
	if len(a.files) > 0 {
		a.dropClearUndo()
	}
	a.applied, a.appliedAt, a.appliedCount = false, time.Time{}, 0
	// AI 步骤不能在这里自动预览:preview() 是同步的,一旦含 <AI:…> 就会在
	// 主线程里联网阻塞几十秒,加入文件的瞬间 UI 卡死。AI 只走显式"预览"按钮。
	// 这与 view() 里的实时预览判断保持一致;否则清空列表后再加文件会立刻
	// 重新触发一轮查询并卡死。
	if !a.hasAIStep() {
		a.preview()
	}
	// 提示必须写在 preview() 之后:preview() 会重设 a.status(「N 个将更名」
	// 或「还没有步骤」),先写就被它整句覆盖——用户拖进一个带子文件夹的
	// 目录,看到的仍是那句常规状态,永远不知道少加了几个。
	a.status = fmt.Sprintf("已添加，共 %d 个文件", len(a.files)) + note
	a.update() // addPaths runs off the view pass (dialogs, drops)
}

// addPathsNote 把"没加进来的那些"补在状态行后面。
//
// 单独一个函数是为了能测:状态行是拼出来的字符串,而"哪些该说、哪些
// 不值得占地方"是这里的判断。子文件夹是设计选择(只加一层),必须说,
// 否则用户会以为坏了;读不到的目录和找不到的路径是真问题,更要报。
func addPathsNote(subDirs, unreadable, missing int) string {
	var parts []string
	if subDirs > 0 {
		parts = append(parts, fmt.Sprintf("跳过 %d 个子文件夹（只加文件夹里的文件）", subDirs))
	}
	if unreadable > 0 {
		parts = append(parts, fmt.Sprintf("%d 个文件夹读不到", unreadable))
	}
	if missing > 0 {
		parts = append(parts, fmt.Sprintf("%d 个路径不存在", missing))
	}
	if len(parts) == 0 {
		return ""
	}
	return "；" + strings.Join(parts, "，")
}

// dialogs are injectable so tests drive the whole flow without a window:
// nil means the real mygo dialogs.
type dialogHooks struct {
	open func(mygo.OpenDialogOptions) ([]string, error)
	save func(mygo.SaveDialogOptions) (string, error)
}

func (a *app) openDialog(opts mygo.OpenDialogOptions) ([]string, error) {
	if a.dialogs.open != nil {
		return a.dialogs.open(opts)
	}
	return mygo.Dialog.Open(opts)
}

func (a *app) saveDialog(opts mygo.SaveDialogOptions) (string, error) {
	if a.dialogs.save != nil {
		return a.dialogs.save(opts)
	}
	return mygo.Dialog.Save(opts)
}

func (a *app) addFilesDialog() {
	a.pickAndAdd(mygo.OpenDialogOptions{
		Parent:   a.win,
		Title:    "选择要改名的文件",
		Multiple: true,
	})
}

// addFolderDialog 挑一个文件夹,把它直接里面的文件都加进来(按文件夹分组)。
// 拖文件夹早就支持,可"挑"这条路以前只能挑文件——同一个能力两种入口,
// 一种有、一种没有,用户只会觉得后者坏了。
func (a *app) addFolderDialog() {
	a.pickAndAdd(mygo.OpenDialogOptions{
		Parent:    a.win,
		Title:     "选择文件夹（加它直接里面的文件）",
		Directory: true,
	})
}

// pickAndAdd 是上面两个对话框的共同后半段:开对话框、加文件、收尾。
func (a *app) pickAndAdd(opts mygo.OpenDialogOptions) {
	a.busy = true
	a.update()
	paths, err := a.openDialog(opts)
	a.busy = false
	if err != nil || len(paths) == 0 {
		a.status = ""
		a.update()
		return
	}
	a.addPaths(paths)
}

// exportCSV writes the sha256/dir/name/new_name list for external editing.
func (a *app) exportCSV() {
	a.busy = true
	a.update()
	out, err := a.saveDialog(mygo.SaveDialogOptions{
		Parent:      a.win,
		Title:       "导出清单（CSV）",
		DefaultPath: "ncreamer-list.csv",
	})
	a.busy = false
	if err != nil || out == "" {
		a.status = ""
		a.update()
		return
	}
	a.exportTo(out)
}

// exportTo writes the list CSV to path (the dialog's part stays above).
func (a *app) exportTo(path string) error {
	f, err := os.Create(path)
	if err != nil {
		a.status = "导出失败：" + err.Error()
		a.update()
		return err
	}
	defer f.Close()
	paths := make([]string, len(a.files))
	for i := range a.files {
		paths[i] = a.files[i].path
	}
	if err := core.ExportCSV(f, paths); err != nil {
		a.status = "导出失败：" + err.Error()
		a.update()
		return err
	}
	a.status = fmt.Sprintf("已导出 %d 行清单到 %s；改 new_name 列，再点「导入清单」", len(a.files), path)
	a.update()
	return nil
}

// importCSV reads an edited list back: files are located by sha256, and
// the table becomes the imported rename plan.
func (a *app) importCSV() {
	a.busy = true
	a.update()
	paths, err := a.openDialog(mygo.OpenDialogOptions{
		Parent: a.win,
		Title:  "导入清单（CSV）",
	})
	a.busy = false
	if err != nil || len(paths) == 0 {
		a.status = ""
		a.update()
		return
	}
	a.importFrom(paths[0])
}

// importFrom loads a list CSV as the current batch (dialog's part above).
func (a *app) importFrom(path string) (plan []core.RenameOp, errs []error) {
	plan, errs = core.ImportCSV(path)
	a.assignExtColors(a.currentTheme())
	a.ops = plan
	a.files = a.files[:0]
	// Imported rows lose their original folder grouping: group by directory
	// so a multi-directory list still reads as a tree.
	for _, op := range plan {
		fi := statFile(filepath.Join(op.Dir, op.OldName))
		fi.group = filepath.Base(op.Dir)
		a.nextOrder++
		fi.order = a.nextOrder
		a.files = append(a.files, fi)
	}
	// 导入的计划不是按步骤算出来的,而是表格里写好的:它就是当前计划,
	// 不该被判成"过期"——否则含 AI 步骤时导入完就点不了执行。
	// 必须写在重建 a.files 之后:planFiles 记的是"这份计划覆盖几个文件",
	// 拿导入前的数量记,一进来就被判成过期。
	a.planSpec, a.planFiles = a.specKey(), len(a.files)
	a.applied, a.appliedAt, a.appliedCount = false, time.Time{}, 0
	// 导入换掉的是整份清单:此时"撤回清空"会把用户刚导入的东西整批盖掉。
	a.dropClearUndo()
	a.status = fmt.Sprintf("已导入 %d 行，%d 个将更名", len(plan), countChanges(plan))
	if len(errs) > 0 {
		a.status += fmt.Sprintf("，%d 个有问题", len(errs))
	}
	a.update()
	return plan, errs
}

func statFile(p string) fileInfo {
	fi := fileInfo{path: p}
	if st, err := os.Stat(p); err == nil {
		fi.size, fi.modTime = st.Size(), st.ModTime()
	}
	fi.typ = detectType(p)
	fi.realType = sniffMagic(p)
	return fi
}

// listRow is one row of the file list. A group header is its own row, so a
// group of n files takes n+1 rows; this is what keeps the sticky header from
// covering the first file of its group.
type listRow struct {
	header bool
	file   int    // index into a.files, valid when !header
	group  string // group name, valid when header
	count  int    // files in this group, valid when header
}

// buildRows expands a.files into header rows plus file rows.
func buildRows(files []fileInfo) []listRow {
	rows := make([]listRow, 0, len(files)+4)
	for i := range files {
		g := files[i].group
		if g != "" && (i == 0 || files[i-1].group != g) {
			n := 0
			for j := range files {
				if files[j].group == g {
					n++
				}
			}
			rows = append(rows, listRow{header: true, group: g, count: n})
		}
		rows = append(rows, listRow{file: i})
	}
	return rows
}

// sortLabel is what the toolbar's sort button says. The order the files were
// added in has no key to name, so it reads as "添加顺序" rather than "不排序":
// "不排序" suggests the list has no order, when in fact it has the one the
// user built by adding files.
func sortLabel(by string, desc bool) string {
	var s string
	switch by {
	case "name":
		s = "按名称"
	case "date":
		s = "按修改时间"
	case "size":
		s = "按大小"
	default:
		return "添加顺序"
	}
	if desc {
		s += " ↓"
	}
	return s
}

// rowFileIndex turns a list row index into an index into a.files. Header rows
// have no file, so they report false — a drag that starts on a header is not a
// drag of anything.
func rowFileIndex(rows []listRow, ri int) (int, bool) {
	if ri < 0 || ri >= len(rows) || rows[ri].header {
		return 0, false
	}
	return rows[ri].file, true
}

// reorderTarget decides whether a drag means anything, and what it means:
// from is the a.files index being moved, dest the index it goes before.
//
// The list hands over row indexes (headers included) and a drop point that may
// sit on a header; neither is a file index, and both can name a place the move
// must not happen. Rather than compute a maybe-correct move in the callback,
// the whole decision lives here so it can be tested without a pointer: refuse
// multi-row drags (the number is one file's identity), refuse drops onto a
// header (that is another folder's territory), and refuse moves that leave the
// file's own group — the group header only appears where the group changes, so
// crossing groups interleaves two folders and grows a header per file.
func reorderTarget(files []fileInfo, rows []listRow, moved []int, to int) (from, dest int, ok bool) {
	if len(moved) != 1 {
		return 0, 0, false
	}
	from, ok = rowFileIndex(rows, moved[0])
	if !ok {
		return 0, 0, false
	}
	dest = len(files)
	if to >= 0 && to < len(rows) {
		if rows[to].header {
			return 0, 0, false
		}
		dest = rows[to].file
	}
	lo, hi := groupSpan(files, from)
	if dest < lo || dest > hi {
		return 0, 0, false
	}
	if dest == from || dest == from+1 {
		return 0, 0, false
	}
	return from, dest, true
}

// groupSpan is the range of a.files that holds one contiguous run of the same
// group, the run that index i belongs to. Files added on their own (no group)
// are each their own run: dragging one loose file past another would otherwise
// silently reshuffle a block the user never grouped.
func groupSpan(files []fileInfo, i int) (lo, hi int) {
	if i < 0 || i >= len(files) {
		return 0, 0
	}
	g := files[i].group
	if g == "" {
		return i, i + 1
	}
	lo = i
	for lo > 0 && files[lo-1].group == g {
		lo--
	}
	hi = i + 1
	for hi < len(files) && files[hi].group == g {
		hi++
	}
	return lo, hi
}

// naturalLess compares two names the way a person reads them: "第2话" before
// "第10话". A byte comparison puts "10" before "2", which on a list of numbered
// episodes looks like the sort did nothing.
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		ad, bd := isDigit(a[0]), isDigit(b[0])
		if ad && bd {
			// Compare the whole digit runs as numbers: strip leading zeros,
			// then the longer run is the larger number.
			an, bn := a, b
			for len(an) > 0 && isDigit(an[0]) {
				an = an[1:]
			}
			for len(bn) > 0 && isDigit(bn[0]) {
				bn = bn[1:]
			}
			ar, br := a[:len(a)-len(an)], b[:len(b)-len(bn)]
			at, bt := strings.TrimLeft(ar, "0"), strings.TrimLeft(br, "0")
			if len(at) != len(bt) {
				return len(at) < len(bt)
			}
			if at != bt {
				return at < bt
			}
			a, b = an, bn
			continue
		}
		// Case-insensitive at the byte level is wrong for non-ASCII, so only
		// fold ASCII letters; everything else compares as-is.
		ac, bc := a[0], b[0]
		if 'A' <= ac && ac <= 'Z' {
			ac += 'a' - 'A'
		}
		if 'A' <= bc && bc <= 'Z' {
			bc += 'a' - 'A'
		}
		if ac != bc {
			return ac < bc
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

// sortFiles orders the list by by ("name"/"date"/"size"), or restores the
// order the files were added in when by is "".
//
// Sorting happens *within* each run of the same group, not across the whole
// list: the group headers only appear where the group changes, so a global
// sort would interleave two folders and grow a header for every file. What the
// user asked for is "the files in this folder, by name", and that is what this
// does.
func sortFiles(files []fileInfo, by string, desc bool) {
	// Every branch falls back to order, so equal keys keep a stable, meaningful
	// sequence instead of whatever the sort happened to do with them.
	less := func(x, y fileInfo) bool {
		switch by {
		case "name":
			nx, ny := filepath.Base(x.path), filepath.Base(y.path)
			if nx == ny {
				return x.order < y.order
			}
			if desc {
				return naturalLess(ny, nx)
			}
			return naturalLess(nx, ny)
		case "date":
			if x.modTime.Equal(y.modTime) {
				return x.order < y.order
			}
			if desc {
				return y.modTime.Before(x.modTime)
			}
			return x.modTime.Before(y.modTime)
		case "size":
			if x.size == y.size {
				return x.order < y.order
			}
			if desc {
				return y.size < x.size
			}
			return x.size < y.size
		}
		return x.order < y.order
	}
	// Walk the contiguous runs of one group and sort each on its own.
	for start := 0; start < len(files); {
		g := files[start].group
		end := start + 1
		for end < len(files) && files[end].group == g {
			end++
		}
		block := files[start:end]
		sort.SliceStable(block, func(i, j int) bool { return less(block[i], block[j]) })
		start = end
	}
}

// moveFile moves the file at index from so it sits just before index before
// (len(files) = to the end), keeping the moved file's neighbours in order.
func moveFile(files []fileInfo, from, before int) []fileInfo {
	if from < 0 || from >= len(files) || before < 0 || before > len(files) || from == before || from+1 == before {
		return files
	}
	f := files[from]
	rest := make([]fileInfo, 0, len(files))
	rest = append(rest, files[:from]...)
	rest = append(rest, files[from+1:]...)
	if before > from {
		before--
	}
	rest = append(rest, fileInfo{})
	copy(rest[before+1:], rest[before:])
	rest[before] = f
	return rest
}

// statFileKeepGroup 与 statFile 相同,但保留原分组。
// 执行/回滚后重建 fileInfo 时如果丢掉 group,列表会进入"有的文件有组名、
// 有的没有"的混合状态,而分组头判定是"组名非空且与上一行不同"——
// 于是空组名与组名的交界处凭空多出一个组头,每个组头还会吞掉一行。
func statFileKeepGroup(p, group string) fileInfo {
	fi := statFile(p)
	fi.group = group
	return fi
}

// stepSpecs renders the task steps to CLI rule specs (with any filters).
func (a *app) stepSpecs() []string {
	var out []string
	for i := range a.steps {
		if s := a.steps[i].fullSpec(); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// specKey 是"当前这套步骤"的指纹,用来判断计划是否过期。
//
// 用 \x00 拼接而不是直接连:两步各自写成 "A|B" 与 "A" + "|B" 时,连起来
// 是同一个字符串,而它们是两条不同的管道。NUL 不会出现在规则文本里。
func (a *app) specKey() string { return strings.Join(a.stepSpecs(), "\x00") }

// planStale 报"现在这份 a.ops 是不是按过期的步骤/文件清单算出来的"。
//
// 只在有 AI 步骤时可能为真:没有 AI 的改动会立刻触发 preview(),a.ops 与
// spec 天然同步;含 <AI:> 的那一路为了不联网被跳过,计划就停在旧 spec 上。
// 此时"执行更名"必须置灰——否则按下去执行的是改之前的计划,行里显示的
// 新名字也不是当前步骤的结果。
//
// 文件数也要比:含 AI 时往清单里加文件同样不重算(addPaths 只在没有 AI 步骤
// 时才 preview),计划就只覆盖老的那几个,而列表已经多出一行没有新名字的
// 文件。只比 spec 的话,这种情况会被当成"计划没问题"。
func (a *app) planStale() bool {
	return a.planStaleWhy() != ""
}

// planStaleWhy 说清"计划为什么过期",空串表示没过期。
//
// 分开报是为了底栏那句话能对着真正变了的东西说:改了模板却说"加了文件",
// 用户会去翻文件清单,而那里什么都没变。
func (a *app) planStaleWhy() string {
	if !a.hasAIStep() {
		return ""
	}
	// planSpec 为空 = 还没按任何步骤算过(刚加进 AI 步骤、还没点过预览)。
	// 那不是"计划过期",是"还没有计划":底栏已经写着"含 AI 步骤,点预览"。
	// 把它算成过期的话,刚排好步骤就冒出一句"计划还是旧的",而根本没有旧计划。
	if a.planSpec == "" {
		return ""
	}
	if a.planSpec != a.specKey() {
		return "步骤改了，计划还是旧的"
	}
	if a.planFiles != len(a.files) {
		return "文件变了，计划还是旧的"
	}
	return ""
}

// previewAsync runs the pipeline off the UI thread, because AI steps block on
// the network for as long as the model takes (often tens of seconds). Doing
// this synchronously in the click handler froze the window with no feedback;
// now the UI shows a spinner while we wait. apply/undo follow the same shape.
func (a *app) previewAsync() {
	// 状态先在 UI 线程上立起来,再开后台。点击一返回就是忙碌态,不会出现
	// "刚点完还显示空闲"的窗口(测试与真实用户都看得见这个窗口)。
	a.busy = true
	a.busyAI = true
	a.busyStart = time.Now()
	a.status = "正在联网命名，模型思考中…"
	a.setAIProgress(0, 0)
	a.aiStopped = false
	// 取消句柄要在起后台之前就备好:用户可能在第一个请求还没发出去时
	// 就点停止,那时候 aiCancel 必须是有效的。
	runCtx, cancel := context.WithCancel(context.Background())
	a.aiCancel = cancel
	a.update()
	// Tick the UI once a second so the wait shows live elapsed seconds instead of
	// a frozen spinner. The user can tell it's working, not hung.
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				a.update()
			}
		}
	}()
	go func() {
		defer func() {
			close(stop)
			cancel()
			a.aiCancel = nil
			a.busy = false
			a.busyAI = false
			a.setAIProgress(0, 0)
			a.update()
		}()
		a.previewRun(runCtx, true)
	}()
}

// aiPrefetchHook turns the prefetch progress into status-bar updates. The
// callback runs on a worker goroutine, so it must not touch the UI tree: it
// records the numbers and asks for a repaint, which is all Window.Update does
// (it posts to the main thread).
func (a *app) aiPrefetchHook(enabled bool) func(done, total int) {
	if !enabled {
		return nil
	}
	return func(done, total int) {
		a.setAIProgress(done, total)
		a.update()
	}
}

// setAIProgress records how far the AI prefetch has got. Called from whichever
// worker goroutine finished a request, so it takes the lock; the status bar
// reads it on the UI thread through aiProgress.
func (a *app) setAIProgress(done, total int) {
	a.aiMu.Lock()
	a.aiDone, a.aiTotal = done, total
	a.aiMu.Unlock()
}

// aiProgress is the prefetch progress as the status bar should show it.
func (a *app) aiProgress() (done, total int) {
	a.aiMu.Lock()
	defer a.aiMu.Unlock()
	return a.aiDone, a.aiTotal
}

func (a *app) preview() { a.previewSync(false) }

// stopAI 是「停止」按钮:放开还没发出去的请求,并把底栏从"模型思考中"
// 换成人话。已经发出去的那几个会以取消错误返回,由 previewRun 收尾。
func (a *app) stopAI() {
	if a.aiCancel == nil {
		return
	}
	a.aiStopped = true
	a.aiCancel()
	a.status = "已停止联网命名"
	a.update()
}

// stepForSpec maps a 1-based rule number from core.ParseRules back to the
// step it came from.
//
// 两个序号不是一回事:stepSpecs() 会跳过 spec 为空白的步骤(比如一个刚
// 加进来、还没选类型的),所以"规则 3"可能是列表里的第 5 步。core 报的
// 是规则号,用户在列表里数的是步骤号——直接把 core 的原文摆出来,他会
// 对着第 3 步发呆,而真正出问题的是第 5 步。
func (a *app) stepForSpec(n int) (int, bool) {
	if n <= 0 {
		return 0, false
	}
	seen := 0
	for i := range a.steps {
		if a.steps[i].fullSpec() == "" {
			continue
		}
		seen++
		if seen == n {
			return i, true
		}
	}
	return 0, false
}

// aiCallCount 是这次执行会发出的 AI 请求数。规则解析失败时返回 0:
// 那种情况下执行按钮本来就是灰的,不必再报一个数字。
func (a *app) aiCallCount() int {
	rules, err := core.ParseRules(a.stepSpecs())
	if err != nil {
		return 0
	}
	paths := make([]string, len(a.files))
	for i, f := range a.files {
		paths[i] = f.path
	}
	return core.AICallCount(rules, paths)
}

// specErrText 把 core 的「规则 N：…」换成「第 N 步「步骤名」：…」。
//
// 认不出规则号时原样返回——宁可少一层翻译,也不能因为解析失败把真正的
// 错误信息吞掉。
func (a *app) specErrText(msg string) string {
	i, tail, ok := a.specErrAt(msg)
	if !ok {
		return msg
	}
	label := a.steps[i].opLabel
	if label == "" {
		return fmt.Sprintf("第 %d 步：%s", i+1, tail)
	}
	// core 在错误前面还挂了一个内部规则名（如「（replace）：」）。既然
	// 下一句就要显示用户看得懂的那一步叫什么,这截内部名字只会让人
	// 去猜"replace 和「正则替换」是不是两回事"。
	if strings.HasPrefix(tail, "（") {
		if _, rest, ok := strings.Cut(tail, "）："); ok {
			tail = rest
		}
	}
	return fmt.Sprintf("第 %d 步「%s」：%s", i+1, label, tail)
}

// specErrAt 从 core 的「规则 N：…」里拆出步骤下标和错误正文。
//
// 拆出来是为了让界面能挂一个「去修这一步」:只说"第 3 步有问题",用户
// 还得自己在那条列表里数到第三行。ok=false 表示认不出规则号——那时
// 调用方原样显示,别硬编一个"第 1 步"。
func (a *app) specErrAt(msg string) (int, string, bool) {
	const prefix = "规则 "
	if !strings.HasPrefix(msg, prefix) {
		return 0, msg, false
	}
	rest := msg[len(prefix):]
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return 0, msg, false
	}
	n, err := strconv.Atoi(rest[:digits])
	if err != nil {
		return 0, msg, false
	}
	// 规则号后面紧跟的是 "：" 或 "（"。两者都从冒号之后开始接原文。
	tail := rest[digits:]
	tail = strings.TrimPrefix(tail, "：")
	tail = strings.TrimPrefix(tail, ":")
	i, ok := a.stepForSpec(n)
	if !ok {
		return 0, msg, false
	}
	return i, tail, true
}

// previewSync runs the pipeline. withProgress=true wires the AI prefetch's
// progress into the status bar; the live (in-view-pass) preview leaves it off
// because it never runs AI steps — addPaths skips the auto-preview for those.
func (a *app) previewSync(withProgress bool) {
	// 同步预览不联网(AI 步骤不走这条路),所以它永远不是"被停掉的"。
	// 不清这个标记的话,停过一次之后每一次实时预览都会说"已停止"。
	a.aiStopped = false
	a.previewRun(context.Background(), withProgress)
}

// previewRun 是 previewSync 的带 context 版本:runCtx 被取消时,联网命名
// 那一段会尽快收手,而不是把剩下的请求一个个等完。
func (a *app) previewRun(runCtx context.Context, withProgress bool) {
	rules, err := core.ParseRules(a.stepSpecs())
	if err != nil {
		// 整轮算不出来:记住原因,a.ops 置空(没有可信的新名字可用),
		// 但界面不能因此装作"没有预览这回事"——见 previewErr 的注释。
		//
		// 原因先翻译成"第几步":core 只知道规则号,而列表里可能因为
		// 空 spec 的步骤而错位(见 stepForSpec)。
		msg := a.specErrText(err.Error())
		a.ops = nil
		a.previewErr = msg
		if i, _, ok := a.specErrAt(err.Error()); ok {
			a.previewErrStep = i
		} else {
			a.previewErrStep = -1
		}
		a.status = msg
		// 算不出来也算"按当前步骤算过一遍":planSpec 跟上,免得界面同时
		// 喊"算不出来"和"计划已过期"两句话。
		a.planSpec, a.planFiles = a.specKey(), len(a.files)
		return
	}
	a.previewErr = ""
	a.previewErrStep = -1
	paths := make([]string, len(a.files))
	for i := range a.files {
		paths[i] = a.files[i].path
	}
	ops, _ := core.RunPipelineProgressCtx(runCtx, rules, paths, a.aiPrefetchHook(withProgress))
	a.ops = ops
	a.planSpec, a.planFiles = a.specKey(), len(a.files)
	if a.aiStopped {
		// 停止之后那些行都带着"已取消"的错误,但它们不是失败:用户自己
		// 按的停止。写成"12 个有问题"会把人赶去查网络,而网络一直好好的。
		// 已经算出来的新名字仍然有效,所以说清"还能接着用"。
		if n := countChanges(ops); n > 0 {
			a.status = fmt.Sprintf("已停止联网命名；已算出的 %d 个仍可执行", n)
		} else {
			// 一个都没算出来时别写"已算出的 0 个仍可执行":那像是在
			// 邀功,而实际情况是什么都没拿到。
			a.status = "已停止联网命名；还没算出任何名字"
		}
		return
	}
	// 底栏那句"0 个将更名"在三种完全不同的处境里一模一样:没文件、
	// 没步骤、有步骤但没产生变更。三者下一步该做的事完全不同,却
	// 共用一句话,读者只能一个个去猜。分开说,每句都指向下一步。
	nChanges := countChanges(ops)
	switch {
	case len(a.files) == 0 && len(a.steps) == 0:
		a.status = "还没有文件和步骤"
	case len(a.files) == 0:
		a.status = fmt.Sprintf("%d 步已排好，还没有文件", len(a.steps))
	case len(a.steps) == 0:
		a.status = fmt.Sprintf("%d 个文件已就绪，还没有步骤", len(a.files))
	case nChanges == 0:
		a.status = fmt.Sprintf("0 个将更名：当前 %d 步没有产生变更", len(a.steps))
	default:
		a.status = fmt.Sprintf("%d 个将更名", nChanges)
	}
	if n := countErrs(ops); n > 0 {
		a.status += fmt.Sprintf("，%d 个有问题", n)
		// 把第一个原因也说出来。只说"有问题"时用户要自己一行行找红字,
		// 而模板写错这类问题是所有文件同一个原因,说一次就够了。
		if msg := firstErrText(ops); msg != "" {
			a.status += ":" + msg
		}
	}
	if a.hasAIStep() && strings.Contains(strings.Join(a.stepSpecs(), ""), "<AI:") {
		a.status += " ·含 AI 步骤，点“预览”联网获取命名"
	}
	// no update() here: preview usually runs inside the view pass (live
	// preview), and updating mid-pass rebuilds the tree and loses scroll
	// positions; async callers update on their own
}

// handleShortcuts 是窗口级快捷键:不管焦点落在哪都响应,除非焦点所在的
// 输入框先把它吃掉了(比如在参数框里按 Ctrl+Z 是撤销输入,不是回滚文件)。
//
// 为什么必须每帧都问一遍:mygo 的快捷键注册表每帧交换一次
// (runtime.go 的 `rt.regs, rt.nextRegs = rt.nextRegs, rt.regs`),
// 只在某一帧问过的键,下一帧就不认了。所以这里在 view() 开头无条件调用。
//
// 用 ui.Cmd 而不是 ui.Ctrl:macOS 上 Cmd 就是 Super,其它平台是 Ctrl,
// 硬写 Ctrl 会让 Mac 用户按不出来。
func (a *app) handleShortcuts(c *ui.Context) {
	// 对话框/面板开着时不接管:它们有自己的 Esc 与默认按钮,
	// 底下这层要是还响应 Ctrl+O,就成了"看不见的窗口在后台动作"。
	if a.settingsOpen || a.pickOpen || a.pickMode || a.importOpen || a.flowOpen {
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) && !a.busy {
		go a.addFilesDialog()
	}
	if c.Shortcut(0, ui.KeyF1) {
		a.openHelp()
	}
	if c.Shortcut(ui.Cmd, ui.KeyComma) {
		a.openSettings()
	}
	if c.Shortcut(ui.Cmd, ui.KeyEnter) && !a.busy && !a.planStale() && a.hasChanges() {
		// 与点按钮同一套语言:第一次按只挂确认态,再按一次才执行。
		if a.confirmApply && !a.confirmStale() {
			a.dropConfirm()
			go a.apply()
		} else {
			a.armConfirm("apply")
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyZ) && !a.busy && a.journalExists() {
		// 与点按钮同一套语言:先挂确认态,再按一次才真回滚
		if a.confirmUndo && !a.confirmStale() {
			a.dropConfirm()
			go a.undo()
		} else {
			a.armConfirm("undo")
		}
	}
	// 文件列表拿着焦点时,Delete 删的是文件而不是步骤——同一个键在两处
	// 各管各的,靠焦点区分,不靠用户记"现在删的是什么"。
	// fileFocus 是上一帧结束时记下的"文件列表拿着焦点"。
	// 不能用 a.table.FocusWithin(c):焦点在这一帧的构建过程中才被设上,
	// 而 handleShortcuts 跑在构建之前,当场问永远是 false。
	if a.fileFocus {
		if c.Shortcut(0, ui.KeyDelete) {
			a.removeChosenFiles()
		}
		return
	}
	if a.stepSel < 0 || a.stepSel >= len(a.steps) {
		return
	}
	if c.Shortcut(0, ui.KeyDelete) {
		// 与鼠标点「删除」走同一条路:直接删,删完可以撤回。
		// 键盘不该比鼠标更容易,也不该更难——两条路径必须一样。
		a.deleteStep(a.stepSel)
	}
	if c.Shortcut(ui.Cmd, ui.KeyD) {
		a.duplicateStep(a.stepSel)
	}
	if c.Shortcut(ui.Alt, ui.KeyUp) {
		a.swapSteps(a.stepSel-1, a.stepSel)
	}
	if c.Shortcut(ui.Alt, ui.KeyDown) {
		a.swapSteps(a.stepSel, a.stepSel+1)
	}
}

func (a *app) hasChanges() bool { return countChanges(a.ops) > 0 }

// batchSummary 只回答一个问题:这批一共有多少文件。
// 计数以前说三遍(右栏工具栏「N 个文件」+ 每个分组头「N 个文件」+
// 底栏「N 个将更名」),而"几个会改名"在底栏又说了一遍。现在分工是:
// 标题栏右侧=总数,分组头=每组数,底栏=将更名/有问题——各说一次。
func (a *app) batchSummary() string {
	if len(a.files) == 0 {
		return ""
	}
	return fmt.Sprintf("%d 个文件", len(a.files))
}

// confirmStale reports whether a pending second-click confirmation has
// outlived its window. 防错提示挂着不走,等于把"再点一下就删"的按钮
// 永远留在屏幕上——三秒内没点就当用户改主意了。
func (a *app) confirmStale() bool {
	return !a.confirmAt.IsZero() && time.Since(a.confirmAt) > 3*time.Second
}

// armConfirm 记下确认窗口的起点,并收回另一个待确认的动作——
// 同时挂着两个"再点一下就删"是没道理的状态。
func (a *app) armConfirm(which string) {
	a.confirmClear = which == "clear"
	a.confirmChosen = which == "chosen"
	a.confirmUndo = which == "undo"
	a.confirmApply = which == "apply"
	a.confirmAt = time.Now()
	// 界面只在有事件时重画。用户点完第一下就撒手,没有事件,
	// confirmStale 永远没机会跑,确认态就一直挂着。所以自己排一次重画。
	go func(at time.Time) {
		time.Sleep(3*time.Second + 100*time.Millisecond)
		if a.confirmAt.Equal(at) {
			a.update()
		}
	}(a.confirmAt)
}

// dropConfirm 收回所有二次确认态。
func (a *app) dropConfirm() {
	a.confirmClear = false
	a.confirmChosen = false
	a.confirmUndo = false
	a.confirmApply = false
	a.confirmFlow = false
	a.confirmAt = time.Time{}
}

// firstErrText returns the first real error message, skipping sidecar files
// (cover.jpg next to an mp3 is not the user's problem).
func firstErrText(ops []core.RenameOp) string {
	for _, op := range ops {
		if op.Err != nil && !core.IsAICanceled(op.Err) && !core.IsSidecarFile(op.OldName) {
			return op.Err.Error()
		}
	}
	return ""
}

// hasAIStep reports whether any step needs the network (<AI:...>).
func (a *app) hasAIStep() bool {
	for _, s := range a.stepSpecs() {
		if strings.Contains(s, "<AI:") {
			return true
		}
	}
	return false
}

// journalInfoNames 是"还能回滚的那批"的名字,只用来数个数。
func (a *app) journalInfoNames() []string {
	sum, ok := a.journalInfo()
	if !ok {
		return nil
	}
	return sum.Names
}

// journalExists: 回滚只在真有日志可回滚时可用.
// 只看文件在不在不够——上一次执行如果全失败,日志里是空列表,
// 按钮却亮着,点下去一个文件也不动还报"已撤销"。
func (a *app) journalExists() bool {
	_, ok := a.journalInfo()
	return ok
}

// journalInfo 读出日志里还能回滚的那一批:时间、几个文件、都是谁。
// 界面上"撤销上次"要能说出这些,而不是三个字的哑按钮。
func (a *app) journalInfo() (core.JournalSummary, bool) {
	if a.settings.Journal == "" {
		return core.JournalSummary{}, false
	}
	return core.ReadJournalSummary(a.settings.Journal)
}

// undoTip 是"撤销上次"悬停时的说明:什么时候改的、前三个文件名。
// 点之前就该知道要点掉什么。
func (a *app) undoTip(sum core.JournalSummary) string {
	tip := shortcutTip("Ctrl+Z")
	if len(sum.Names) == 0 {
		return tip + " · 没有可撤销的记录"
	}
	if !sum.Time.IsZero() {
		tip = sum.Time.Format("01-02 15:04") + " 的更名 · " + tip
	}
	n := len(sum.Names)
	shown := sum.Names
	if len(shown) > 3 {
		shown = shown[:3]
	}
	tip += "\n" + strings.Join(shown, "\n")
	if n > len(shown) {
		tip += fmt.Sprintf("\n…等 %d 个文件", n)
	}
	return tip
}

// statusColor colors feedback: failures stand out, success reads calm.
func (a *app) statusColor(t *ui.Theme) ui.Color {
	switch {
	case a.busy:
		return t.TextMuted
	case strings.Contains(a.status, "失败") || strings.Contains(a.status, "有问题"):
		return t.Danger
	case strings.Contains(a.status, "已撤销") || strings.Contains(a.status, "成功"):
		return t.Success
	}
	return t.TextMuted
}

func countChanges(ops []core.RenameOp) (n int) {
	for _, op := range ops {
		// 已经改过的不再算"将更名":否则执行完主按钮还亮着,
		// 再点一次全部失败,用户会以为程序坏了。
		if op.Err == nil && op.OldName != op.NewName && !op.Applied {
			n++
		}
	}
	return
}

func countErrs(ops []core.RenameOp) (n int) {
	for _, op := range ops {
		// 取消不算错:它没有坏掉,只是没算。
		if op.Err != nil && !core.IsAICanceled(op.Err) && !core.IsSidecarFile(op.OldName) {
			n++
		}
	}
	return
}

func (a *app) apply() {
	a.busy = true
	a.busyStart = time.Now()
	a.update()
	a.busyDone, a.busyTotal = 0, len(a.ops)
	rep, err := core.ApplyPlanReport(a.ops, a.settings.Journal, func(done, total int) {
		a.busyDone, a.busyTotal = done, total
		a.update()
	})
	a.busy = false
	a.busyStart = time.Time{}
	a.busyDone, a.busyTotal = 0, 0
	a.applied = true
	if err != nil {
		a.status = "执行出错：" + err.Error()
	} else {
		// 数字直接来自执行者:它在 os.Rename 成功的那一刻就记了一笔。
		// 以前是回头遍历 plan 看 op.Applied 反推,那是可变状态——绕过
		// 主按钮的 Disabled 重入一次,就能得到"底栏绿字成功、满屏行里
		// 写着失败"。现在计数和磁盘上发生的事是同一次观测。
		ok, failed := rep.Renamed, rep.Failed
		// 只报告这一次真的发生了什么:"失败"不该把"名字没变化、无需处理"
		// 的文件也算进去,那会让一次干净的批量看起来像出了很多错。
		if ok == 0 {
			a.status = fmt.Sprintf("没有文件被更名，失败 %d", failed)
			if failed == 0 {
				a.status = "没有文件需要更名"
			}
		} else if failed == 0 {
			// 一次干净的成功不该带"失败 0"这条尾巴:它读起来像"出了 0 个
			// 错",而用户扫底栏时只认"失败"两个字——见到就要去找是哪几个
			// 文件出了问题。没有失败就一个字都不提。
			a.status = fmt.Sprintf("成功 %d；可用“撤销上次”退回", ok)
		} else {
			a.status = fmt.Sprintf("成功 %d，失败 %d；可用“撤销上次”退回", ok, failed)
		}
		for i := range a.files {
			if i < len(a.ops) && a.ops[i].Applied {
				fi := statFileKeepGroup(filepath.Join(a.ops[i].Dir, a.ops[i].NewName), a.files[i].group)
				fi.order = a.files[i].order
				a.files[i] = fi
			}
		}
		if ok > 0 {
			a.appliedAt = time.Now()
			a.appliedCount = ok
			a.startFlash()
		}
		a.assignExtColors(a.currentTheme())
	}
	a.update()
}

func (a *app) undo() {
	a.busy = true
	a.busyStart = time.Now()
	a.update()
	rep, err := core.UndoJournalReport(a.settings.Journal)
	a.busy = false
	a.busyStart = time.Time{}
	// 只按"确实退回去了"的那些更新界面。以前是遍历所有 Applied 的行
	// 一起改成旧名——部分失败时,没退成的文件也会被画成旧名,而它其实
	// 还叫新名字,界面和磁盘当场对不上。
	restored := make(map[string]bool, len(rep.Restored))
	for _, name := range rep.Restored {
		restored[name] = true
	}
	for i := range a.ops {
		if !a.ops[i].Applied || !restored[a.ops[i].NewName] {
			continue
		}
		fi := statFileKeepGroup(filepath.Join(a.ops[i].Dir, a.ops[i].OldName), a.files[i].group)
		fi.order = a.files[i].order
		a.files[i] = fi
		a.ops[i].Applied = false
	}
	a.assignExtColors(a.currentTheme())
	// 横幅说的是"刚才执行了哪一批",回滚之后那句话已经不成立——哪怕
	// 只退了一部分,剩下的也由底栏的「撤销上次（N 个）」接着说,那里
	// 的数字直接来自日志,比横幅可靠。
	a.applied, a.appliedAt, a.appliedCount = false, time.Time{}, 0
	switch {
	case len(rep.Restored) > 0 && len(rep.Failed) > 0:
		// 尽力而为的回滚可能只退了一部分,状态里必须说清退了多少,
		// 而不是一句"回滚失败"让用户以为文件没动过。
		a.status = fmt.Sprintf("撤销部分完成：%d 个已退回，%d 个没退成（%s）",
			len(rep.Restored), len(rep.Failed), strings.Join(rep.Failed, "; "))
	case len(rep.Restored) > 0 && err != nil:
		// 文件退回去了,但日志没写成功:下次再点回滚会对着已经改回旧名的
		// 文件再 rename 一次而失败。这件事必须说出来。
		a.status = fmt.Sprintf("已撤销 %d 个，但日志未能更新：%v", len(rep.Restored), err)
	case len(rep.Restored) > 0:
		a.status = fmt.Sprintf("已撤销 %d 个", len(rep.Restored))
	case err != nil:
		a.status = "撤销失败：" + err.Error()
	default:
		a.status = "没有可撤销的更名"
	}
	a.update()
}

// detectType gives the 识别类型 column: the format by extension, plus a
// note when we can read metadata out of it.
//
// 以前这里自带一份 labels map,和 sniffMagic 的大写英文、extCategory 的
// 中文类别各说各话:同一个 .m4a 在标签里叫「MP4 音频」、在嗅探里叫
// 「MP4」,加一种格式要改三处。现在标签全部来自 formatTable,
// 这里只负责"扩展名 → 标签 + 元数据后缀"。
func detectType(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if ext == "" {
		return "无扩展名"
	}
	label := formatLabel(ext)
	if m := core.ExtractMeta(path); m != nil {
		if _, has := m["title"]; has {
			label += " ·含元数据"
		}
	}
	return label
}

func humanSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	f := float64(n)
	for _, u := range units {
		f /= 1024
		if f < 1024 {
			return fmt.Sprintf("%.1f %s", math.Round(f*10)/10, u)
		}
	}
	return fmt.Sprintf("%.1f PB", f/1024)
}

func main() {
	// 单实例:两个 ncreamer 同时跑会互相覆盖 journal、各自执行更名
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	a := &app{stepSel: -1, fileSel: -1, settings: LoadSettings(), stepScrollTo: -1, previewErrStep: -1}
	// 钉选从设置读进内存一份:目录每帧重建,逐帧读 settings 会在
	// 设置对话框改动后出现一帧延迟(拖动时看得见条目跳一下)。
	a.pinned = append([]string(nil), a.settings.Pinned...)
	// 折叠状态同样从设置带回来,并留一份在内存里(目录每帧重建)。
	if len(a.settings.Folded) > 0 {
		a.folded = make(map[string]bool, len(a.settings.Folded))
		for k, v := range a.settings.Folded {
			a.folded[k] = v
		}
	}
	a.restoreSteps()
	a.syncSeedUI()
	a.ApplySettings()
	// 第二个实例不再静默退出:用户再点一次图标、或者从"打开方式"里选了
	// 本程序,期望的是"窗口到前面来",而不是双击之后什么也没发生。
	// 那个实例自己的命令行参数(可能带着文件)由 mygo 经 socket 转给这里,
	// 走的是下面的 OnOpenFile,所以这里只管把窗口捞出来。
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		a.bringToFront()
	})
	// 外部文件入口:资源管理器里双击、"打开方式"、把文件拖到图标上,
	// 以及第二个实例带过来的文件,都从这里进来。
	mygo.App.OnOpenFile(func(path string) {
		a.openExternal(path)
	})
	// 退出前把中栏排着的步骤写回设置文件。
	//
	// 放在 OnBeforeQuit 而不是每帧:步骤每帧都可能被改(打字、拖参数),
	// 逐帧写盘会把磁盘打满。这个钩子在窗口关闭之前跑,而且系统注销/关机
	// 时也会被调用(此时必须同步完成,不能开 goroutine)。
	mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
		a.persistSteps()
	})
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:          "New Century Renamer",
			Width:          1280,
			Height:         720,
			MinWidth:       900,
			MinHeight:      480,
			StateKey:       "main",
			TitleBarStyle:  mygo.TitleBarHidden,
			TitleBarHeight: 38,
			Content:        ui.View(a.view),
		})
		// 关掉系统画的那圈窗口边框:我们自绘了包括标题栏在内的整个界面,
		// 只剩这一圈不是我们画的。系统主题是亮色而软件选了暗色时,它会被
		// 画成一道亮灰线,看上去像渲染 bug。DWM 这个属性是 Win11 才有的,
		// 老系统上失败也无所谓(那时本来就没这圈线)。
		hideSystemBorder(a.win.NativeHandle())
		// the whole window accepts file drops, not just one element
		a.win.OnFileDrop(func(e *mygo.FileDropEvent) {
			a.addPaths(e.Paths)
		})
		// follow the desktop's light/dark switches live
		mygo.Theme.OnUpdated(func() { a.update() })
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// drainPendingDrop 把排队的外部路径(拖放、外部入口、第二个实例)加进列表。
// 必须排在帧首:这些路径都是在构建这一帧的途中发现的,当场 append 会让
// 正在逐行构建的列表长度对不上。
func (a *app) drainPendingDrop() {
	if len(a.pendingDrop) == 0 {
		return
	}
	paths := a.pendingDrop
	a.pendingDrop = nil
	a.addPaths(paths)
}

// bringToFront 把窗口从最小化/后台捞回前台。第二个实例被拒绝后调用它:
// 用户再点一次图标,期望的是"窗口到前面来",而不是双击之后什么也没发生。
func (a *app) bringToFront() {
	if a.win == nil {
		return
	}
	// 最小化的窗口只 Focus 是不够的:它在任务栏里闪一下又沉下去,
	// 看起来像"点了没反应"。先 Restore 再 Show 再 Focus。
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Show()
	a.win.Focus()
}

// openExternal 收下一个从程序外面送进来的文件路径:资源管理器里双击、
// "打开方式"、把文件拖到程序图标上,以及第二个实例带过来的文件。
//
// 排到 pendingDrop 而不是直接 addPaths:和拖放区走同一条路,帧首统一
// 追加,避免在构建到一半的列表上改长度。窗口还没建好时 update() 是
// 空操作,路径就留在队列里等第一帧。
func (a *app) openExternal(path string) {
	if path == "" {
		return
	}
	a.pendingDrop = append(a.pendingDrop, path)
	a.update()
}

// syncSeedUI mirrors the settings' seed into the dialog widgets.
func (a *app) syncSeedUI() {
	s := a.liveSettings()
	a.seedFollow = s.Theme.Seed == ""
	if !a.seedFollow {
		a.seedPick = ui.Hex(s.Theme.Seed)
	} else if c, ok := systemAccent(); ok {
		a.seedPick = c
	} else {
		a.seedPick = ui.Hex(fallbackSeed)
	}
	// 等待秒数:0 表示用默认(300),输入框留空即显示默认提示
	if s.AI.Timeout > 0 {
		a.aiTimeoutText = strconv.Itoa(s.AI.Timeout)
	} else {
		a.aiTimeoutText = ""
	}
	// 并发路数:0 表示用默认(4 路),留空显示默认
	if s.AI.Concurrency > 0 {
		a.aiConcurrencyText = strconv.Itoa(s.AI.Concurrency)
	} else {
		a.aiConcurrencyText = ""
	}
	// 默认提示词:原样载入,留空表示内置默认
	a.aiPromptText = s.AI.Prompt
}

// commitAITimeout parses the "等待秒数" text buffer back into the settings.
// 空串 / 非数字 / <=0 一律视为"用默认",即清成 0。
func (a *app) commitAITimeout() {
	s := a.liveSettings()
	buf := strings.TrimSpace(a.aiTimeoutText)
	if n, err := strconv.Atoi(buf); err == nil && n > 0 {
		s.AI.Timeout = n
	} else {
		s.AI.Timeout = 0
	}
}

// commitAIConcurrency parses the "并发路数" buffer. 空串 / 非数字 / <=0 视为默认。
// 云端 API 限流时用户会填 1(单路);本地模型想多路也能填更大值。
func (a *app) commitAIConcurrency() {
	dst := a.liveSettings()
	buf := strings.TrimSpace(a.aiConcurrencyText)
	if n, err := strconv.Atoi(buf); err == nil && n > 0 {
		dst.AI.Concurrency = n
	} else {
		dst.AI.Concurrency = 0
	}
}

// commitAIPrompt saves the "默认提示词" buffer, trimmed. 空串即恢复内置默认。
func (a *app) commitAIPrompt() {
	a.liveSettings().AI.Prompt = strings.TrimSpace(a.aiPromptText)
}

const (
	// undoDeleteWindow 是"删掉的步骤还能撤回"的窗口。
	// 比二次确认的 3 秒长一些:撤回是给"我点错了"留的余地,而人意识
	// 到自己点错往往要看完那一步消失之后的列表才反应过来。
	undoDeleteWindow = 8 * time.Second

	// flashDuration 是一行"刚改完"高亮的总时长。报告建议 120–160ms,
	// 那是单行的节奏;这里整批行同时亮,需要更长一点才看得出
	// "这些一起变了",所以取 320ms。
	flashDuration = 320 * time.Millisecond
	flashStep     = 40 * time.Millisecond
)

// resultMark 是"没有文字时"结果列用的形状标记。
//
// 极窄的右栏收掉了状态词,只剩一个 7×7 的圆点。只靠颜色编码状态对
// 色觉障碍用户是读不出来的(红/绿是最常见的混淆对),而且灰度打印、
// 截图传阅这两种场景也会把颜色丢掉。形状在灰度下仍然可分,颜色退回
// 成第二层线索。
func resultMark(s rowState) (mark, label string) {
	switch s {
	case rowApplied:
		return "●", "已更名"
	case rowFailed:
		return "✕", "失败"
	case rowSkipped:
		return "–", "跳过"
	case rowUnchanged:
		return "○", "不变"
	case rowCanceled:
		// 空方块:和"不变"的圆、失败的叉都不同,灰度下也分得开。
		return "□", "没算"
	}
	return "→", "将更名"
}
