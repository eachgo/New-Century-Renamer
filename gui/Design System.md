````markdown
# Mono Design System —— LLM 可读设计规范

> **版本** v1.0 · **适用范围** 金融/记账类移动端 App（iOS 优先，375×812pt 基准）
> **文档目的** 本文档把一套**视觉设计稿**翻译成纯文本规格，供**没有任何图像输入能力的 LLM** 直接读取并生成像素级还原的 HTML/CSS/React/Tailwind 代码。
> **语言约定** 令牌名、类名、CSS 变量使用英文；所有说明、文案示例、术语对照使用中文。生成界面时**默认输出中文文案**。

---

## 0. 如何使用本文档

1. 将本文档整体作为 system prompt 或上下文喂给 LLM。
2. 附上需求，例如：`请用本文档的 Design Tokens 生成「首页」页面，纯 HTML + CSS，无外部图片，图标用内联 SVG。`
3. 要求 LLM 在生成代码前，先输出「实现清单」（用到哪些令牌、哪些组件），再输出代码。
4. 生成后用 **§11 生成约束清单** 逐条自检。

---

## 1. 视觉 DNA（一句话还原）

> **奶油暖白底 + 深墨绿主色 + 橙色点睛**的「温柔金融」风格：大圆角、几乎无投影、以色块分层而非以阴影分层；数字大而醒目；图标圆润；插画用 3D 卡片；整体克制、干净、亲和，不冷硬。

关键词：`warm cream` `deep forest green` `soft orange` `rounded` `flat-with-tint` `big numerals` `airy spacing`。

**反面清单（禁止出现）**：纯黑 #000、冷灰 #F5F5F5、锐利直角、霓虹渐变、玻璃拟态、粗描边、紫色科技风、重投影。

---

## 2. 设计令牌 Design Tokens

### 2.1 色彩 Color

> 主色是**深墨绿**，不是蓝色；强调色是**暖橙**。背景是**暖白/奶油色**，绝非纯白。

| Token | HEX | 用途 |
|---|---|---|
| `--bg-canvas` | `#EFEDE6` | 设备外的展示底板（多机排版时） |
| `--bg-app` | `#FBFAF6` | App 页面底色（暖白） |
| `--surface` | `#FFFFFF` | 卡片、列表容器 |
| `--surface-muted` | `#F1F0EA` | 次级底：图标槽、输入框、未选中 chip、灰卡 |
| `--brand-950` | `#0C2417` | 最深绿（余额卡渐变暗部、深色气泡） |
| `--brand-900` | `#10301F` | **主色**：深色卡片、用户气泡、选中 chip、发送按钮 |
| `--brand-700` | `#1C4B31` | 深色卡上的次级块、按下态 |
| `--brand-500` | `#2F7A52` | 正向金额、链接、辅助图标 |
| `--brand-100` | `#DDE8DF` | 浅绿图标槽（Transfer、收入） |
| `--accent-500` | `#F2703C` | **强调橙**：主按钮、FAB、Top Up 图标、Lifestyle 扇区 |
| `--accent-600` | `#E0622E` | 橙色按下态 |
| `--accent-100` | `#FBE2D3` | 浅橙图标槽、洞察卡底、支出扇区底 |
| `--accent-050` | `#FDF0E8` | 推广 Banner 底（极浅橙米色） |
| `--gray-100` | `#ECEBE5` | 灰图标槽（Pay Bills / Card）、图表 Others 扇区 |
| `--text-primary` | `#171A17` | 主文本（近黑带一点绿味） |
| `--text-secondary` | `#8C8C85` | 副文本、时间、元信息、占位符 |
| `--text-tertiary` | `#B4B3AC` | 最弱文本、禁用态 |
| `--text-inverse` | `#FFFFFF` | 深色底上的文本 |
| `--positive` | `#2E8B5B` | 入账 `+` 金额 |
| `--negative` | `#171A17` | 出账 `-` 金额（不用红色，保持克制） |
| `--divider` | `#EAE8E1` | 1px 分割线 |
| `--dot-unread` | `#F2703C` | 未读红点/橙点 |

**色彩比例**：暖白 70% / 深绿 15% / 橙 8% / 浅绿浅橙 7%。橙色永远是**点睛**，不得大面积铺满。

**渐变**（仅用于深色卡片，其余一律纯色）：
```css
background:
  radial-gradient(120% 120% at 15% 0%, #1B4A2E 0%, transparent 55%),
  linear-gradient(160deg, #123420 0%, #0C2417 100%);
```

### 2.2 字体 Typography

| 用途 | 字体栈 |
|---|---|
| 拉丁/数字 | `"Plus Jakarta Sans", "Gilroy", "Poppins", sans-serif` |
| 中文 | `"PingFang SC", "HarmonyOS Sans SC", "Noto Sans SC", "Source Han Sans SC"` |

**推荐加载**（HTML 头部必须带 `<link>`，否则回退系统字体）：
```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&family=Noto+Sans+SC:wght@400;500;700&display=swap" rel="stylesheet">
```

字体整体气质：**几何无衬线 + 轻微圆润 + 大数字紧凑**。禁止 Inter / Roboto / Arial / system-ui 作为主字体。
所有金额、编号使用等宽数字：`font-variant-numeric: tabular-nums;`

### 2.3 字号 / 行高 / 字重 Scale

| Token | 字号 | 行高 | 字重 | 用途 |
|---|---|---|---|---|
| `--fs-display-xl` | 34px | 1.1 | 700 | 余额大数字 `$15,123.45` |
| `--fs-display-l` | 28px | 1.15 | 700 | 图表中心总额 `$3,800` |
| `--fs-h1` | 20px | 1.3 | 700 | 屏幕标题（Payments / Money assistant） |
| `--fs-h2` | 18px | 1.35 | 700 | 分区标题（Recent Transactions） |
| `--fs-h3` | 15px | 1.4 | 700 | 列表项标题、卡片标题 |
| `--fs-body-l` | 14px | 1.5 | 600 | 按钮文字、强调正文 |
| `--fs-body` | 14px | 1.7 | 400 | 正文段落（中文行高放宽） |
| `--fs-caption` | 12px | 1.5 | 500 | 元信息、日期、副标题 |
| `--fs-micro` | 11px | 1.35 | 600 | 底部导航标签、时间戳、百分比 |

> **中文行高比英文大 0.15–0.25**；中文正文最小 13px，建议 14px。字间距：中文 `letter-spacing: 0`；全大写英文标签 `0.08em`。

### 2.4 间距 Spacing（4px 基准栅格）

| Token | 值 | 用途 |
|---|---|---|
| `--sp-1` | 4px | 图标与文字微间距 |
| `--sp-2` | 8px | 元素内间距、行内 gap |
| `--sp-3` | 12px | 列表项内边距、小卡片 gap |
| `--sp-4` | 16px | **默认卡片内边距**、列表行间距 |
| `--sp-5` | 20px | **页面左右安全边距**、大卡片内边距 |
| `--sp-6` | 24px | 分区之间间距 |
| `--sp-8` | 32px | 大区块间距 |
| `--sp-10` | 40px | 顶部留白 / 段落分组 |

- 页面左右统一 `20px`，卡片之间 `16px`，分区之间 `24px`。
- 列表项高度 `64px`（含 40px 图标 + 12px 上下）。
- 触控目标最小 `44×44px`。

### 2.5 圆角 Radius

| Token | 值 | 用途 |
|---|---|---|
| `--r-xs` | 8px | 小标签、进度条端点 |
| `--r-sm` | 12px | 图标槽内部、小 chip |
| `--r-md` | 16px | 图标方块、列表行容器 |
| `--r-lg` | 20px | 通用卡片、气泡 |
| `--r-xl` | 24px | **主卡片**（余额卡、账单卡） |
| `--r-2xl` | 32px | 弹窗、大面板 |
| `--r-pill` | 999px | 按钮、chips、输入框、头像 |
| `--r-device` | 44px | 手机外框 |

> 规则：**容器越大圆角越大**。禁止 0/2/4px 圆角出现在卡片上。

### 2.6 阴影与层次 Elevation

设计主要靠**色块分层**，阴影极轻：

| Token | 值 | 用途 |
|---|---|---|
| `--shadow-0` | `none` | 默认（大多数卡片） |
| `--shadow-1` | `0 1px 2px rgba(16,48,31,.04), 0 6px 16px rgba(16,48,31,.05)` | 浮起的白卡 |
| `--shadow-2` | `0 12px 32px rgba(16,48,31,.10)` | 弹层 |
| `--shadow-fab` | `0 10px 22px rgba(242,112,60,.35)` | 橙色悬浮按钮 |
| `--shadow-device` | `0 30px 70px rgba(40,38,30,.14)` | 展示用手机外框 |

### 2.7 动效 Motion

| Token | 值 |
|---|---|
| `--ease-out` | `cubic-bezier(.22, 1, .36, 1)` |
| `--ease-spring` | `cubic-bezier(.34, 1.56, .64, 1)` |
| `--dur-fast` | 150ms（按压、hover） |
| `--dur-base` | 250ms（切换、展开） |
| `--dur-slow` | 400ms（页面转场、卡片入场） |
| `--stagger` | 50ms（列表逐条入场） |

约定：卡片入场 `opacity 0→1 + translateY(12px→0)`；数字滚动 600ms；按压缩放 `scale(.96)`；FAB 呼吸光晕 2s 循环（可选）。**禁止**弹跳过大、旋转入场、视差滚动。

### 2.8 分割线 / 描边

- 分割线：`1px solid var(--divider)`，只用横向。
- 图标描边：`1.8px`，`stroke-linecap: round`，`stroke-linejoin: round`。
- 深色块上的描边：`rgba(255,255,255,.12)`。
- **禁止** 2px 以上外描边、禁止虚线边框。

---

## 3. 中文排版与本地化规则

1. **行高放大**：正文 `1.7`，标题 `1.3`。
2. **标点**：使用全角标点（，。：；？），中英文之间不加空格（可选加 `0.12em` 间距）。
3. **数字/单位**：金额使用半角数字 + 千分位逗号 + 两位小数，前置货币符号：`¥15,123.45`。中文场景金额用 `¥`，不用 `$`。
4. **正负号**：入账 `+¥2,450.00`（绿色）；出账 `-¥5.60`（主文本色，不用红色）。
5. **日期**：`10月22日` / `10月22日 · 银行卡支付`；今年内不写年份。
6. **标题不换行优先**：分区标题单行，超出用省略号；卡片标题最多 2 行。
7. **字重映射**：英文 `600` ≈ 中文 `500`；英文 `700` ≈ 中文 `700`（思源黑体 Medium/Bold）。
8. **按钮文字**：动词短语，2–4 字最佳（转账 / 充值 / 缴费 / 收款）。
9. **禁止**：感叹号堆砌、营销腔、 emoji 出现在正文（图表/图标除外）。

---

## 4. 组件库 Components

### 4.1 设备外框与状态栏（仅展示用）

- 屏幕：`375×812px`，圆角 `44px`，底色 `--bg-app`。
- 状态栏：高 `44px`；左 `9:41`（`--fs-caption` 700）；右依次信号/WiFi/电池（内联 SVG，`--text-primary`）。
- 多机排版：底板 `--bg-canvas`，机与机间距 `32px`，水平居中，可轻微上下错位。

### 4.2 顶部导航栏 Top App Bar

结构：`[Logo + 品牌名]  ← 灵活空间 →  [铃铛 + 橙点] [头像]` 或 `[返回] [居中标题] [更多]`。

- 高度 `56px`，左右内边距 `20px`。
- Logo：两个交叠的圆角斜块（深绿），尺寸 `24px`；品牌名 `--fs-h2` 700。
- 铃铛 `22px` 线性图标，右上角 `7px` 橙色圆点（`--dot-unread`），白描边 `1.5px`。
- 头像：`36px` 圆形图片，可带 `2px` 白色描边。
- 详情页：左侧返回箭头 `24px`，标题居中 `--fs-h1` 700，右侧 `···` 图标。

### 4.3 资产卡片 Balance Card（视觉主角）

尺寸：宽 `335px`（= 375 - 20×2），高 `196px`，圆角 `24px`，深绿渐变底 + **同心弧线纹理**（白色 `6%` 透明度的细密放射曲线，可用 SVG 或 `repeating-radial-gradient`）。

内容（内边距 `20px`）：
1. 顶部：`总余额` 标签（`--fs-caption`，`rgba(255,255,255,.7)`）。
2. 主数字 `¥15,123.45`（`--fs-display-xl` 700，白）+ 右侧眼睛图标 `20px`（`rgba(255,255,255,.6)`），二者间距 `8px`，垂直居中。
3. 趋势行：上箭头（`--accent-500`）+ `12%`（`--accent-500` 700）+ `较上月`（`rgba(255,255,255,.6)` `--fs-caption`）。
4. 右下角：双圆卡片组织标（红 `#EB001B` 70% + 黄 `#F79E1B` 70% 重叠 `12px`，总宽 `44px`）。
5. 底部按钮行（间距 `10px`）：
   - 主按钮：`发送图标 + 转账`，橙底 `--accent-500`，圆角 `999px`，高 `48px`，宽约 55%。
   - 次按钮：`下载图标 + 收款`，`rgba(255,255,255,.14)` 底，白字，同高，圆角 `999px`。

状态：加载时数字用 `#2A2E2A` 色块骨架；隐藏余额时显示 `••••••`。

### 4.4 快捷操作 Quick Action Tile

一行 4 个，均分，间距 `12px`。

- 图标槽：`56×56px`，圆角 `16px`，图标 `24px`。
- 标签：`--fs-micro` 600，`--text-primary`，居中，与图标间距 `8px`。

| 项 | 图标 | 槽底色 | 图标色 |
|---|---|---|---|
| 转账 | 双向箭头 | `--brand-100` | `--brand-900` |
| 充值 | 加号圆 | `--accent-100` | `--accent-500` |
| 缴费 | 票据 | `--surface-muted` | `--text-primary` |
| 银行卡 | 卡片 | `--surface-muted` | `--text-primary` |

交互：按下 `scale(.96)` + 底色加深 4%。

### 4.5 按钮 Buttons

| 类型 | 底色 | 文字 | 高 | 圆角 | 用途 |
|---|---|---|---|---|---|
| `btn-primary` | `--accent-500` | 白 700 `--fs-body-l` | 48px | 999px | 转账、立即支付 |
| `btn-secondary` | `rgba(255,255,255,.14)`（深底上）/ `--surface-muted`（浅底上） | 白 / `--text-primary` | 48px | 999px | 收款、次要操作 |
| `btn-ghost` | transparent | `--brand-500` 600 | 40px | 999px | 查看全部 |
| `btn-icon` | 依场景 | — | 44px | 999px | 发送、关闭 |
| `btn-block` | 同 primary | — | 52px | 999px | 底部固定主 CTA |

图标与文字间距 `8px`，图标 `18–20px`。禁用态：底色 `--gray-100`，文字 `--text-tertiary`。

### 4.6 推广 Banner

- 高 `112px`，圆角 `20px`，底色 `--accent-050`（或 `linear-gradient(100deg,#FDF0E8,#F7E7DA)`）。
- 左侧文字区 `~62%`：标题两行（`--fs-h3` 700，`--text-primary`）+ 副标题一行（`--fs-caption`，`--text-secondary`）。
- 右侧：**3D 卡片插画**（两张深绿银行卡斜放，带高光与阴影），可出血到卡片右缘。
- 右下角：`32px` 白色圆形按钮 + 右箭头（`--text-primary`），阴影 `--shadow-1`。

### 4.7 列表行 List Row（交易 / 账单 / 付款）

结构：`[40px 圆形品牌图标] [标题 + 元信息]  ←→  [金额] [可选 ›]`

- 容器：白底，圆角 `16px`（独立卡）或无底（列表内），内边距 `12px 16px`，最小高 `64px`。
- 图标：`40px` 圆形，`object-fit: cover`；无品牌图时用 `--surface-muted` 底 + 线性图标。
- 标题：`--fs-h3` 600；元信息：`--fs-caption` `--text-secondary`（如 `银行卡支付 · 10月22日`）。
- 金额：`--fs-h3` 700，右对齐，`tabular-nums`；入账 `--positive` 且带 `+`。
- 箭头 `›`：`16px`，`--text-tertiary`。
- 分隔：相邻行之间 `1px solid var(--divider)` 或留 `8px` 间隙（两选一，全页统一）。
- 按下态：整行底色 `--surface-muted`。

**常用条目数据（可直接复用）**：
`星巴克 -¥35.00` / `亚马逊 -¥499.00` / `工资 +¥24,500.00` / `视频会员 -¥139.00` / `超市 -¥624.00` / `餐厅 -¥382.00` / `网约车 -¥165.00`。

### 4.8 分段控件 Chips / Segmented

- 高 `36px`，圆角 `999px`，内边距 `0 18px`，`--fs-body-l` 600。
- 选中：底 `--brand-900`，文字白。未选中：底 `--surface-muted`，文字 `--text-primary`。
- 三个一组（`账单 / 付款 / 定期`），间距 `8px`，左对齐。
- 小型筛选 chip（如 `本月 ▾`）：高 `28px`，`--fs-caption`，白底 + `1px var(--divider)` 描边，带下箭头。

### 4.9 分区标题 Section Header

`[标题]  ←→  [查看全部]`，上下留白 `16px`。
标题 `--fs-h2` 700 `--text-primary`；`查看全部` `--fs-caption` 600 `--brand-500`，右侧带 `›`（可选）。

### 4.10 底部标签栏 Bottom Nav + FAB

- 栏高 `80px`（含 22px 安全区），白底，顶部 `1px var(--divider)`。
- 5 槽位：`首页 / 分析 / [中央 FAB] / 账单 / 我的`。
- 标签 `--fs-micro` 600；激活态 `--brand-900` + 实心图标；未激活 `--text-tertiary` + 线性图标。
- **中央 FAB**：`56px` 橙色圆形，向上浮出栏顶 `18px`，图标 `24px`（扫码/相机，白色），`--shadow-fab`，外圈 `6px` 白色描边（`ring`）。
- 保留 `env(safe-area-inset-bottom)`。

### 4.11 智能助手（对话式）Assistant

**用户气泡**
- 底 `--brand-900`，白字 `--fs-body`，圆角 `20px`（右下角 `6px`，示意「发出」方向），内边距 `14px 18px`，最大宽 `78%`，右对齐。
- 气泡下方时间戳：`--fs-micro` `--text-secondary`，右对齐，间距 `6px`。

**AI 回复**
- **无气泡**，直接是暖白底段落，左侧 `18px` 橙色四角星图标（`--accent-500`），文字 `--fs-body` `--text-primary`，行高 `1.7`。
- 段落与段落间距 `16px`。

**数据卡片（AI 内嵌统计）**
- 白底圆角 `16px`，内边距 `14px 16px`，两行，行间距 `12px`，中间 `1px var(--divider)`。
- 每行：`[36px 圆形图标槽] [标签 + 副标签]  ←→  [金额]`
  - 收入行：槽 `--brand-100`，上箭头 `--brand-900`，金额 `--brand-900` 700。
  - 支出行：槽 `--accent-100`，下箭头 `--brand-900`，金额 `--text-primary` 700。

**洞察卡 Insight**
- 底 `--accent-100` 或 `--accent-050`，圆角 `16px`，内边距 `16px`。
- 左侧 `20px` 灯泡图标 `--accent-500`；右侧正文 `--fs-body`，行高 `1.7`，可 2–4 行。

**建议动作 Suggested Actions**
- 标题 `--fs-h3` 700，间距 `12px`。
- 行：`[24px 图标] [动作名]  ←→  [›]`，行高 `56px`，行间 `1px var(--divider)`。
- 例：`查看支出明细`、`设置每月预算`。

**输入栏**
- 容器：白底，圆角 `999px`，高 `52px`，`1px var(--divider)` 描边，左侧占位文字 `--fs-body` `--text-tertiary`：`问问你的钱……`。
- 右侧内嵌 `40px` 深绿圆形发送按钮（纸飞机白色图标）。
- 固定在页面底部（栏上方），左右 `20px`，与底部导航间距 `12px`。

### 4.12 账单卡片 Bill Card

- 白底，圆角 `16px`，内边距 `14px 16px`，最小高 `68px`，卡片间距 `12px`，带 `--shadow-1`。
- 左 `40px` 圆角 `12px` 图标槽（品牌图或 `--surface-muted` + 线性图标）。
- 中：名称 `--fs-h3` 700 / 副行 `--fs-caption` `--text-secondary`（`10月25日到期`）。
- 右：金额 `--fs-h3` 700 + `›`。
- 逾期/紧急：副行文字用 `--accent-500`；未开启自动扣款：右侧加小标签。

### 4.13 环形图 Donut + 图例

- 尺寸 `140×140px`，环宽 `22px`，段间隙 `3px`，端点 `stroke-linecap: butt`（或微圆角 `4px`）。
- 起始角 `-90°`（12 点方向），顺时针排列，按占比从大到小。
- 配色顺序：`--brand-900` → `--accent-500` → `--gray-100`（后续用 `--brand-500`、`--accent-200`）。
- 中心：总额 `--fs-display-l` 700（第一行）+ 标签 `总计` `--fs-caption` `--text-secondary`（第二行）。
- 图例（右侧，垂直排列，行间距 `14px`）：
  `● [类别名]`（`--fs-caption` 600）/ `金额`（`--fs-h3` 700）/ `百分比`（`--fs-micro` `--text-secondary`，右对齐）。
- 类别命名：`必要支出 / 生活方式 / 其他`。

---

## 5. 页面模板 Page Templates

### 5.1 首页 Home
`状态栏 → App Bar(品牌+铃铛+头像) → 资产卡片 → 快捷操作×4 → 推广 Banner → 分区「最近交易」+查看全部 → 交易列表×4~6 → 底部导航(含 FAB)`
滚动：内容区滚动，App Bar 与底部导航固定；余额卡片随滚动轻微上移 `scale(.98)`（可选）。

### 5.2 智能助手 Assistant
`状态栏 → 导航(返回+标题+更多) → 对话流(用户气泡 → AI 文本 → 数据卡片×2 → 洞察卡) → 「建议动作」区 → 输入栏 → 底部导航`

### 5.3 账单与付款 Payments
`状态栏 → 导航 → 分段控件(账单/付款/定期) → 分区「待缴账单」+查看全部 → 账单卡×3 → 分区「支出概览」+筛选chip「本月」 → 环形图+图例 → 分区「最近付款」+查看全部 → 付款行×3 → 底部导航`

---

## 6. 图标规范 Iconography

- 风格：**线性 + 圆头圆角**，统一 `1.8px` 描边，`24×24px` 网格绘制，视觉重量一致。
- 尺寸档位：`16 / 18 / 20 / 24 / 28`，UI 中默认 `24`。
- 颜色继承文字色（`currentColor`），禁止多色图标（品牌 logo 除外）。
- 常用图标清单：首页、图表、扫码、账单卡片、用户、铃铛、眼睛、发送（纸飞机）、下载、双向箭头、加号圆、票据、银行卡、返回、更多（···）、右箭头（›）、下箭头、上箭头、灯泡、齿轮、时钟/饼图、手机、Wi-Fi、购物车、餐具。
- 图标必须以内联 SVG 输出，不得依赖图标字体或外部 CDN。

---

## 7. 插画与图片 Imagery

| 类型 | 规格 | 说明 |
|---|---|---|
| 3D 卡片插画 | `200×140px`，PNG/WebP，透明底 | 两张深墨绿银行卡斜置，柔和高光，无硬边；仅用于 Banner |
| 品牌 logo | `40×40px` 圆形裁切 | 用真实/通用商户标识；缺失时用首字母圆牌 |
| 用户头像 | `36–40px` 圆形 | 真实人像照片，自然光，浅背景 |
| 纹理 | 深色卡片上的同心弧线 | 白色 `4–8%` 透明度，SVG pattern 或渐变模拟 |

图片一律 `object-fit: cover`；圆角与所在容器一致。**生成代码时**若无真实图片，使用 `https://placehold.co/40x40/ECEBE5/171A17?text=商` 之类占位，或纯 CSS 色块 + 文字首字。

---

## 8. 数据可视化规范

- **仅使用环形图与迷你条形**，禁止 3D 图表、禁止多彩彩虹配色。
- 数字必须 `tabular-nums`，右对齐。
- 百分比永远与绝对值同时出现（`¥19,200.00` + `50%`）。
- 颜色顺序固定：深绿 → 橙 → 浅灰。
- 中心留白充足，图表与图例间距 `24px`。
- 空状态：环形图显示一圈 `--gray-100`，中心文字 `暂无数据`。

---

## 9. 文案与术语对照表 Glossary

| English | 中文 | 备注 |
|---|---|---|
| Mono | Mono（莫诺） | 品牌名，不译或保留拉丁 |
| Total Balance | 总余额 | |
| Send Money | 转账 | 主 CTA |
| Request | 收款 | |
| Transfer | 划转 | 同行内调拨用「划转」，跨账户用「转账」 |
| Top Up | 充值 | |
| Pay Bills | 缴费 | |
| Card | 银行卡 | |
| Spend smarter this month | 本月，更聪明地花钱 | Banner 标题 |
| Set a budget and stay on track | 设定预算，稳步达成 | Banner 副标 |
| Recent Transactions | 最近交易 | |
| See all | 查看全部 | |
| Money assistant | 智能财务助手 | 屏幕标题可简为「智能助手」 |
| Your balance decreased by… | 你的余额本周减少了…… | |
| Total income / Total spending | 总收入 / 总支出 | |
| 1 deposit / 12 transactions | 1 笔入账 / 12 笔交易 | |
| Suggested actions | 建议操作 | |
| View spending details | 查看支出明细 | |
| Set a monthly budget | 设置每月预算 | |
| Ask anything about your money… | 问问你的钱…… | 输入框占位 |
| Payments | 账单与付款 | 屏幕标题 |
| Bills / Payments / Scheduled | 账单 / 付款 / 定期 | 分段控件 |
| Upcoming bills | 待缴账单 | |
| Due Oct 25 | 10月25日到期 | |
| Spending summary | 支出概览 | |
| This month | 本月 | 筛选 chip |
| Needs / Lifestyle / Others | 必要支出 / 生活方式 / 其他 | |
| Total | 总计 | 图表中心 |
| Recent payments | 最近付款 | |
| Home / Insights / Payments / Profile | 首页 / 分析 / 账单 / 我的 | 底部导航 |

**语气**：专业、简洁、温和、以用户利益为中心。用「你」不用「您」（年轻向）；陈述事实 + 给出建议，不评判消费行为。

---

## 10. 无障碍 Accessibility

- 正文对比度 ≥ `4.5:1`，大字 ≥ `3:1`。
  - `--text-primary` on `--bg-app` ≈ 15:1 ✔
  - `--text-secondary` on `--bg-app` ≈ 3.6:1 → **仅用于 ≥14px 的次要信息**
  - 白字 on `--accent-500` ≈ 3.1:1 → 按钮文字使用 **700 字重 + 15px 以上**
- 状态不只靠颜色：正负金额必须带 `+ / -` 符号；图表必须带图例与数值。
- 触控目标 ≥ `44×44px`，相邻目标间距 ≥ `8px`。
- 所有图标按钮提供 `aria-label`（中文）。
- 支持 `prefers-reduced-motion: reduce` 时关闭入场动画。
- 可选「隐藏余额」功能（眼睛图标），隐藏后显示 `••••••`。

---

## 11. 生成约束清单（LLM 硬性自检）

生成任何界面后，逐条确认：

1. 页面底色是 `#FBFAF6`，不是 `#FFFFFF` 或 `#F5F5F5`。
2. 主色是深墨绿 `#10301F`，强调色是橙 `#F2703C`，**没有蓝色/紫色**。
3. 卡片圆角 ≥ `16px`，主卡片 `24px`，按钮 `999px`。
4. 阴影极轻或没有；层次靠色块，不靠投影。
5. 字体栈包含 `Plus Jakarta Sans` + `PingFang SC`，并有 Google Fonts `<link>`。
6. 金额使用 `tabular-nums`，带 `¥` 与千分位。
7. 中文文案、全角标点、日期为 `X月X日`。
8. 图标全部为内联 SVG，`1.8px` 描边，圆头圆角。
9. 页面左右边距 `20px`，卡片间距 `16px`，分区间距 `24px`。
10. 底部导航 5 槽 + 中央橙色 FAB（仅首页/主 tab 页）。
11. 不引入外部 JS 框架依赖（除非用户指定）；不使用占位灰条（除非明确骨架屏）。
12. 代码完整可运行：一个 HTML 文件包含全部 CSS，无死链、无 TODO。

---

## 12. 快速启动片段 Copy-paste Tokens

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mono</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@400;500;600;700;800&family=Noto+Sans+SC:wght@400;500;700&display=swap" rel="stylesheet">
<style>
:root{
  /* color */
  --bg-canvas:#EFEDE6; --bg-app:#FBFAF6; --surface:#FFFFFF; --surface-muted:#F1F0EA;
  --brand-950:#0C2417; --brand-900:#10301F; --brand-700:#1C4B31; --brand-500:#2F7A52; --brand-100:#DDE8DF;
  --accent-600:#E0622E; --accent-500:#F2703C; --accent-100:#FBE2D3; --accent-050:#FDF0E8;
  --gray-100:#ECEBE5; --divider:#EAE8E1;
  --text-primary:#171A17; --text-secondary:#8C8C85; --text-tertiary:#B4B3AC; --text-inverse:#FFFFFF;
  --positive:#2E8B5B;
  /* type */
  --font:"Plus Jakarta Sans","PingFang SC","HarmonyOS Sans SC","Noto Sans SC",sans-serif;
  --fs-display-xl:34px; --fs-display-l:28px; --fs-h1:20px; --fs-h2:18px; --fs-h3:15px;
  --fs-body-l:14px; --fs-body:14px; --fs-caption:12px; --fs-micro:11px;
  /* space */
  --sp-1:4px; --sp-2:8px; --sp-3:12px; --sp-4:16px; --sp-5:20px; --sp-6:24px; --sp-8:32px;
  /* radius */
  --r-xs:8px; --r-sm:12px; --r-md:16px; --r-lg:20px; --r-xl:24px; --r-2xl:32px; --r-pill:999px;
  /* elevation */
  --shadow-1:0 1px 2px rgba(16,48,31,.04),0 6px 16px rgba(16,48,31,.05);
  --shadow-fab:0 10px 22px rgba(242,112,60,.35);
  /* motion */
  --ease-out:cubic-bezier(.22,1,.36,1); --dur-fast:150ms; --dur-base:250ms; --dur-slow:400ms;
}
*{box-sizing:border-box;margin:0;padding:0}
body{
  font-family:var(--font); color:var(--text-primary);
  background:var(--bg-canvas); -webkit-font-smoothing:antialiased;
}
.num{font-variant-numeric:tabular-nums}
</style>
</head>
<body>
  <!-- 界面内容 -->
</body>
</html>
```

**余额卡片骨架**

```html
<section class="balance">
  <p class="label">总余额</p>
  <div class="row">
    <h1 class="num">¥15,123.45</h1>
    <svg class="eye" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><path d="M2 12s3.6-6 10-6 10 6 10 6-3.6 6-10 6-10-6-10-6Z"/><circle cx="12" cy="12" r="2.6"/></svg>
  </div>
  <p class="trend"><span class="up">↑ 12%</span> 较上月</p>
  <div class="actions">
    <button class="btn-primary">转账</button>
    <button class="btn-secondary">收款</button>
  </div>
</section>

<style>
.balance{
  position:relative; overflow:hidden;
  padding:var(--sp-5); border-radius:var(--r-xl); color:#fff;
  background:
    radial-gradient(120% 120% at 15% 0%, #1B4A2E 0%, transparent 55%),
    linear-gradient(160deg,#123420 0%,#0C2417 100%);
}
.balance .label{font-size:var(--fs-caption);font-weight:600;color:rgba(255,255,255,.7)}
.balance .row{display:flex;align-items:center;gap:var(--sp-2);margin-top:var(--sp-2)}
.balance h1{font-size:var(--fs-display-xl);font-weight:700;line-height:1.1;letter-spacing:-.02em}
.balance .eye{width:20px;height:20px;color:rgba(255,255,255,.6)}
.balance .trend{margin-top:var(--sp-2);font-size:var(--fs-caption);color:rgba(255,255,255,.6)}
.balance .trend .up{color:var(--accent-500);font-weight:700;margin-right:var(--sp-1)}
.balance .actions{display:flex;gap:10px;margin-top:var(--sp-5)}
.balance button{flex:1;height:48px;border:0;border-radius:var(--r-pill);
  font-family:var(--font);font-size:var(--fs-body-l);font-weight:700;cursor:pointer;
  transition:transform var(--dur-fast) var(--ease-out)}
.balance button:active{transform:scale(.96)}
.btn-primary{background:var(--accent-500);color:#fff}
.btn-secondary{background:rgba(255,255,255,.14);color:#fff}
</style>
```

**列表行骨架**

```html
<div class="row-item">
  <span class="avatar">星</span>
  <div class="info"><p class="title">星巴克</p><p class="meta">银行卡支付 · 10月22日</p></div>
  <p class="amount num">-¥35.00</p>
</div>

<style>
.row-item{display:flex;align-items:center;gap:var(--sp-3);
  padding:12px var(--sp-4);min-height:64px;background:var(--surface);border-radius:var(--r-md)}
.row-item .avatar{width:40px;height:40px;border-radius:var(--r-pill);
  background:var(--surface-muted);display:grid;place-items:center;font-size:var(--fs-caption);font-weight:700}
.row-item .info{flex:1;min-width:0}
.row-item .title{font-size:var(--fs-h3);font-weight:600}
.row-item .meta{font-size:var(--fs-caption);color:var(--text-secondary);margin-top:2px}
.row-item .amount{font-size:var(--fs-h3);font-weight:700}
.row-item .amount.in{color:var(--positive)}
</style>
```

---
