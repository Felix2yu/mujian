# 幕间前端 UI 控件与交互模式盘点

> 审计对象：`web/src`（15 个路由页、13 个业务组件、8 个图表组件、`lib/app.css` 615 行）
> 代码规模：24,120 行 `.svelte`，其中 scoped `<style>` 8,201 行（34%）
> 统计方式：对模板与样式块做模式匹配 + 逐文件人工核对，所有结论均附文件与行号

---

## 0. 一句话结论

**基础层不缺，缺的是「行为层」和「结构层」。**

`app.css` 已经把色彩、圆角、阴影、按钮、输入框、卡片、徽章、空态、横幅、骨架都做成了全局类，因此不存在「每个页面自己画按钮」的问题；真正重复的是三类东西：

1. **行为契约**（弹窗的 Esc/焦点陷阱/滚动锁定/portal）——6 个弹窗各写一遍，口径从「完整实现」到「完全缺失」；
2. **组合结构**（分区卡片 + 标题 + 网格 + 工具栏）——`.card.sec` / `.card.section` / `.card` 三种写法，卡片内 `h3` 字号 4 档；
3. **同一交互的多次实现**（分段控制 3 份、组合下拉 5+1 份、灯箱 4 份、portal 3 份、原生 `confirm()` 12 处）。

优先级最高的不是新增组件，而是**先把已经事实上存在的契约固化成唯一实现**（弹窗、确认、反馈），再谈组合控件。

---

## 1. 现有底座盘点

`web/src/lib/app.css` 已提供（可直接复用，无需再造）：

| 类别 | 现有类/元素 | 覆盖情况 |
|---|---|---|
| 卡片 | `.card` `.card-hover` | 全局，卡片网格共用 |
| 按钮 | `.btn` + `.primary/.ghost/.danger/.sm/.lg` | 全局，128 处引用 |
| 输入 | `.input`（含 `select.input`/`textarea.input`、暗色适配、iOS 16px 防缩放） | 全局，约 110 处 |
| 标签 | `.pill`（强调）/ `.tag`（中性） | 全局 |
| 消息 | `.banner.error/.success/.info` | 全局，49 处引用 |
| 空态 | `.empty` + `.ico/.t/.h` | 全局，页面级 11 处 |
| 加载 | `.skeleton` + `@keyframes shimmer` | 全局，15 处引用 |
| 文本 | `.muted` `.tiny` `.err` `.ok` | 全局 |
| 页头 | `.page-head` + `.sub` | 全局，10 页使用 |
| 其它 | `.row`（两列 + 移动端塌缩）、`.divider`、`.fade-up`/`.stagger`、`:focus-visible`、`prefers-reduced-motion` | 全局 |

**缺口清单**（这些才是重复的源头）：`.hint`（局部副本 6 份）、`.card.sec` 内边距与标题字号（8 份）、`.grid` 卡片网格（7 份）、分段控制（3 份）、弹窗外壳（6 份）、图标按钮（约 15 处散落）、组合下拉（5+1 份）。

---

## 2. 控件与模式盘点

### 2.1 按钮

| 项 | 数据 |
|---|---|
| `<button>` 总数 | 186 |
| 使用 `.btn` 系列 | 128（69%），分布于 21 个文件 |
| 未用 `.btn` 的原生按钮 | 58：图标按钮、chip、日历格、分段按钮、星级、下拉项等 |

`.btn` 修饰符组合的**写法**不一致（同一个语义出现 4 种顺序）：

| 写法 | 次数 | 说明 |
|---|---|---|
| `btn sm` | 15 | — |
| `btn` | 13 | — |
| `btn primary` | 12 | — |
| `btn ghost sm` | 8 | 顺序 A |
| `btn danger sm` | 6 | 顺序 A |
| `btn sm ghost` | 5 | 顺序 B（`CoverPicker:249`、`CostCompletion:414,551,613,743`） |
| `btn sm danger` | 2 | 顺序 B（`data/+page.svelte:349`、`:362`） |
| `btn primary danger` | 1 | `VenuePanel.svelte:218`，两个修饰符互斥，`primary` 被 `danger` 覆盖，属冗余 |

危险操作视觉语义缺失：`.btn.danger` 是「透明底 + 红字」（`app.css:235`），没有实底红色变体，于是**确认删除的主按钮**（`ConfirmModal.svelte:38`、`categories/+page.svelte:137`）看起来比 `.btn.primary` 弱一档，与「危险操作应更醒目」相反。

图标按钮（无 `.btn`）尺寸与描边各不相同，无统一实现：

`del`（3 处，artists/dramas/categories 列表 ✕，**CSS 三份逐字复制，均 24×24 / 12px**）、`x`（`CoverPicker:228`、`CategoryTags:176`）、`close`（`BatchEditModal:155`）、`search-clear`（`+page.svelte:737`）、`view-x`（`:827`）、`nav-btn`（`calendar:60/95`）、`menu-btn`（`+layout.svelte:576`）、`op`（`dramas/[id]:376`）、`mask-bg`（`calendar:585`）。

另有 **7 处 `<a class="btn">` 伪按钮**（首页 `772`、records/[id] `458`、settings `691`、data `310/312`、analytics/[year] `107/119`）：链接承担按钮语义，因此拿不到 `.btn:active` 的缩放反馈、也无法 `disabled`，且 `data` 页两处需 `eslint-disable svelte/no-navigation-without-resolve` 才能通过 lint。

### 2.2 表单字段

| 项 | 数据 |
|---|---|
| `<label>` | 104（settings 34、BatchEditModal 22、RecordForm 20、首页筛选 17） |
| `.input` | 约 110 |
| `type="checkbox"` | 45（BatchEditModal 22、settings 11、首页 3） |

- `<label>` 的视觉已由全局元素选择器统一（`app.css:331`），因此**不需要 `Field` 组件**；
- 真正重复的是 label 内的三种附加物：必填标记 `.req`（仅 `RecordForm:2730` 一处定义）、说明 `.hint`（6 份副本）、行内错误（各页手写 `<div class="err">` 或 `.banner.error`）；
- `accent-color: var(--accent)` 在 6 个文件声明了 10 次，**唯独 `BatchEditModal.svelte` 漏了**——该组件有 22 个复选框，是全站最密集处，实际渲染为浏览器默认蓝色；
- 复选框**尺寸四档**：18px（`BackupPanel:429`、`CategoryTags:286`、`RecordForm:3197`、`settings:1548`）、16px（`CoverMaintenance:405`、首页 `.select-all:1690`）、15px（首页 `.missing-opt:1525`）、14px（`calendar:1318`）；
- `RecordCard:104-108` 用 `<span class="record-check">` 伪装复选框（批量选择），键盘与读屏语义与原生不一致。

### 2.3 下拉补全（重复最严重的一处）

`RecordForm.svelte` 手写了 6 套结构相同的组合框（城市、场馆、同行、剧团、渠道、剧目树），每套持有 3 个状态变量 + 约 30 行键盘处理：

| 字段 | 状态变量组 | 键盘处理 |
|---|---|---|
| 城市 | `showCityList` / `cityActive` / `cityBlurTimer`（`:729-734`） | `onCityKeydown` |
| 场馆 | `showAddrList` / `addrActive` / `addrBlurTimer`（`:730-734`） | `onAddrKeydown` |
| 同行 | `showFriendsList` / `friendsActive` / `friendsBlurTimer`（`:809-`） | `onFriendsKeydown` |
| 剧团 | `showCompanyList` / `companyActive` / `companyBlurTimer`（`:624-629`） | `onCompanyKeydown:666` |
| 渠道 | `showChannelList` / `channelActive` / `channelBlurTimer`（`:625-629`） | `onChannelKeydown` |
| 剧目 | `showDramaList`（`:234`） | 独立处理 |

`comboNav(e, ctx)`（`:857`）已经把「↑↓/Esc/Enter」抽成一个函数，但**打开/关闭/失焦延时/选中后是否保持打开**仍是 5 份拷贝。估值：约 250 行逻辑 + 约 120 行模板可收敛为 1 个组件 + 5 处配置。

反例：`routes/+page.svelte:1109/1122/1135` 的筛选面板仍在用原生 `<datalist>`，而 `RecordForm:623/728/807` 的注释明确写了「Safari / 移动端原生 datalist 基本不弹建议」——**同一份数据、同一类交互，表单用自定义下拉、筛选用原生 datalist**，这是当前最明确的移动端体验缺陷。

### 2.4 分段控制（同一控件三种实现）

| 位置 | 类名 | 激活修饰符 |
|---|---|---|
| 首页筛选「任一为空 / 全部为空」`+page.svelte:1011` | `.seg` / `.seg-btn`（定义 `:1482-1504`） | `.seg-btn.on` |
| 地图底图切换 `map/+page.svelte:277` | `.seg` / `.seg-btn`（定义 `:342-365`） | `.seg-btn.active` |
| 费用补全视图切换 `CostCompletion.svelte:426` | `.cost-seg-btn`（定义 `:854-872`） | `.cost-seg-btn.on` |
| 设置主题切换 `settings/+page.svelte:518` | `.theme-opt`（图标 + 文字的竖排变体） | `.theme-opt.on` |

三份 CSS 几乎逐字相同（`border-radius` + `background: var(--surface-2)` + 选中反色），却各带一个作用域前缀和一个不同的激活类名。

### 2.5 弹窗与浮层（契约差异最大）

6 个实现，类名体系分两派（`modal-mask/modal-head/modal-foot` vs `modal-mask/modal-title/modal-actions`）：

| 实现 | 位置 | 类名 | Esc | 焦点陷阱 | 初始焦点 | 恢复焦点 | 滚动锁定 | aria-modal | portal |
|---|---|---|---|---|---|---|---|---|---|
| 全局确认框 | `ConfirmModal.svelte:10` | `cm-*` | ✅ | ❌ | ❌ | ❌ | ❌ | ✅ | 布局层 |
| 批量编辑 | `BatchEditModal.svelte:151` | `modal-mask/modal` | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| 日历当日 | `calendar/+page.svelte:588` | `mask/modal` | ✅ | ❌ | ❌ | ✅ | ❌ | ✅ | ❌ |
| 剧种删除 | `categories/+page.svelte:123` | `modal-mask/modal` | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ |
| 封面选择 | `CoverPicker.svelte:217` | `overlay/panel` | ✅ | ❌ | ✅ | ❌ | ❌ | ✅ | ✅ |
| 首页筛选面板 | `+page.svelte:916` | `filter-mask/filter-panel` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ |

即：契约最完整的（筛选面板）是最不像「弹窗」的那个；最像弹窗的（批量编辑，唯一带 `<h2>` 标题栏和 `.modal-foot` 的）**一项都没做**，连 `role="dialog"` 都没有。

> 补充：`+page.svelte:678-717` 的焦点陷阱实现证明这套能力已经存在，只是没有被复用。`fade-up` 残留 `transform` 会被 `position:fixed` 当作包含块——这个坑在 `CoverPicker:196`、`RecordForm:1573`、`ChartTip:15` 各注释了一遍，`use:portal` 也被复制了 3 份。

### 2.6 二次确认（两套方言）

| 方式 | 调用点 |
|---|---|
| `askConfirm()`（自定义弹窗，Promise） | 8 处 / 5 文件：`+page.svelte:217,431`；`data:51,71`；`artists/[id]`；`dramas/[id]:2`；`VenuePanel:1` |
| 原生 `confirm()` | **12 处 / 5 文件**：`BackupPanel.svelte:124,139,149,170,175`；`CoverMaintenance.svelte:70,112,128`；`records/[id]:164,267`；`artists/+page.svelte:50`；`dramas/+page.svelte:92` |
| 页面自建删除弹窗 | `categories/+page.svelte:122-141`（带「影响 N 条演出」的正文，本可用 `askConfirm` 的 `impact`） |

同一产品里，「删一条演出」用自定义弹窗，「删一个演员档案」用浏览器原生弹窗，中间还夹着一个手写的第三种。

### 2.7 提示与反馈

| 模式 | 用量 | 说明 |
|---|---|---|
| `.banner error/success/info` | 49 处 / 23 文件（settings 17、CostCompletion 4） | 唯一的「消息」原语，但只是 CSS 类，前缀符号靠手写：`⚠`（多数）、`✓`（`data:263`）、`💡`（`settings:639`）、`⏳`（`settings:661`） |
| 瞬时行内成功提示 | `settings:1387`（`已保存 ✓`，2400ms 自清）、`records/[id]:435,453`（`✓ 已保存`）、`settings:706`（复制，2000ms）、`records/[id]:294`（1600ms） | 各自一个布尔 + `setTimeout`，**自动消失时长 1600 / 2000 / 2400 三档**，无统一 |
| Toast / Snackbar | **不存在** | 全库无 toast/snackbar 实现 |
| 「操作成功但无反馈」 | `artists/+page.svelte:31`（新建后 `location.href` 跳转）、`dramas:add`、`categories:add`、首页批量删除后静默 `load()` | 与上两类形成第三种口径 |

结论：**不要引入 toast**（现有信息架构是「就地反馈」，与紧凑布局一致），应做的是把 `.banner` 变成带 `tone` 的组件并统一「短暂成功提示」的一条路径。

### 2.8 空态（`.empty` 一名三用）

| 用法 | 位置 | 问题 |
|---|---|---|
| 页面级空态（`ico/t/h` 结构） | `+page:850,857`、`artists:92`、`dramas:192,219`、`categories:96`、`map:294`、`artists/[id]:231`、`dramas/[id]:384,422`、`CoverPicker:301` —— 共 11 处 | 图标来源 4 种：`OperaIcon`（首页/演员）、内联 SVG（`map:296`）、文字（`categories:97` 的 `❏`、`dramas:193` 的 `戏`）、无图标 |
| 图表内「暂无数据」 | `RankList:24`、`ScatterChart:48`、`Donut:78` | `RankList` 的 `.empty` **依赖**全局 `.empty` 的 `display:flex` 才能居中，属隐式耦合 |
| 日历空白格占位 | `calendar:483` `class="cell empty"` | 与全局空态同名，仅靠 `.cell{padding:0}` 的特异性侥幸不出错 |
| 纯文本空态 | `calendar:567` `.month-empty`、`data:327`「回收站是空的。」、`analytics:235` | 无结构、无引导动作 |

`small` 修饰符（`artists/[id]:231`、`dramas/[id]:384`）在全局**未定义**，是空类名。

### 2.9 加载态

| 模式 | 位置 |
|---|---|
| 页面级骨架 | 12 个页面，15 处 `<div class="skeleton" style="height: Npx">` |
| 组合骨架（封面+标题+副文） | `+page:842`、`records/[id]:352`（`.skel-cover/.skel-line` 在各自文件里重复定义） |
| 日历专用骨架 | `calendar:126`（`blank-skel`，行数对齐真实周数，防切月抖动——**不可替换**） |
| 文本加载 | `+layout:409`「加载中…」、`settings:500` 单个 220px 骨架 |
| 按钮内忙碌 | 词形统一为「动词 + 中…」：`导入中…`/`检查中…`/`创建中…`/`添加中…`/`删除中…`/`转换中…`/`验证中…`（**这一项已经统一，无需改动**） |

内联 `style="height:…"` 全库 85 处，其中骨架占 15 处，是高频的样式外泄点。

### 2.10 错误态

| 模式 | 位置 |
|---|---|
| `banner error` + 重试 | 仅 `+page:849-855`（空态卡 + 重试按钮）、`settings:502-510`（整页降级 + 重试） |
| 仅 `banner error` | 其余 15 个页面/组件（`map:287`、`calendar:94`、`artists:85`、`data:221`…） |
| 追加分页失败 | `+page:903`（`.sentinel-error` 带重试，仅在滚动加载时出现） |
| 行内字段错误 | `RecordForm`（顶部红条 + 字段红字 + 红边，`checkDateLocal`） |

即：首屏失败可恢复的只有首页与设置页，其它页面一旦首屏失败，用户只能刷新。

### 2.11 列表 / 卡片网格

7 处 `.grid` 定义 + 6 个其它网格容器，`minmax` 与 `gap` 参数不统一：

| 容器 | 位置 | 轨道 | gap |
|---|---|---|---|
| `.grid`（记录卡） | `+page:1783` | `minmax(172px,1fr)` | 14 |
| `.grid`（演员卡） | `artists:126` | `minmax(160px,1fr)` | 12 |
| `.grid`（剧目卡） | `dramas:300` | `minmax(200px,1fr)` | 10 |
| `.grid`（剧种卡） | `categories:150` | `minmax(200px,1fr)` | 10 |
| `.grid`（详情页记录卡） | `artists/[id]:362`、`dramas/[id]:668` | `minmax(172px,1fr)`（与首页逐字相同） | 14 |
| `.grid`（封面选择器） | `CoverPicker:424` | `minmax(min(120px,100%),1fr)` | 10 |
| `.refs-grid`（说明卡并排） | `data:548` | `minmax(min(320px,100%),1fr)` | 16 |
| `.grid-2` / `.grid-3` | `analytics:718`、`[year]:500` | `1fr 1fr` / `1fr 1fr 1fr` | 12 |
| `.cards-row` | `records/[id]:1242` | 两列（详情页信息卡） | — |
| `.settings-grid` | `settings:1396` | 2 列 + `CARD_COLS` DP 分列 | — |
| `.kpis` / `.hl-grid` | `analytics:678`、`[year]:470,516` | auto-fit 140/180px | — |

其中 3 处（首页、演员详情、剧目详情）的**轨道与间距逐字相同**（详情页仅多一行 `margin-top: 4px`），是可直接合并的一组。

另一个结构层重复：分区卡片的包裹写法两种——`.card section`（RecordForm、record detail、artist/drama detail）与 `.card sec`（settings、data、analytics、CostCompletion、BackupPanel），两者 `padding: 18px 20px` 完全一致；卡片内 `h3` 字号却有 4 档：

| 字号 | 位置 |
|---|---|
| 14.5px | `CostCompletion:776` |
| 15px | `analytics:715`、`[year]:497`、`VenuePanel:342` |
| 15.5px | `settings:1423`、`data:484`、`BackupPanel:378`、`CoverMaintenance:363`、`RecordForm:2727` |
| 16px | `records/[id]:1136`、`artists/[id]:348`、`dramas/[id]:523`、`CoverPicker:379` |

### 2.12 分页

- 唯一有分页的是首页：无限滚动，`PAGE_SIZE=30`（`+page:22`），哨兵 + `IntersectionObserver`（`:600-621`）、请求序号防乱序（`:444`）、追加失败提示（`:903`）、跨页全选（`:199`）。
- `CoverPicker` 另有一套**按钮分页**（`page`/`totalPages`，`CoverPicker:15-32`，带越界回退 `:108-110`），与首页的无限滚动是两种范式。
- 演员/剧目/剧种列表全量返回，无分页。
- 详情页关联推荐用 `limit:50` 后 `slice(0,8)`（`records/[id]:236,246`），天然不需要分页。

**结论：目前不存在「分页组件」的复用需求**，不要为一致性引入分页控件。

### 2.13 徽章 / 标签 / chip（同名不同义）

| 类名 | 语义 | 位置 |
|---|---|---|
| `.pill` | 强调标签（可点击跳筛选） | 全局；`records/[id]:393-399` |
| `.tag` | 中性标签（演员/剧目/场馆） | 全局；`records/[id]:474`、`RecordCard:145` |
| `.chip`（4 种） | ① 激活的筛选条件（可✕移除，实底浅红）`+page:1629` ② 只读数据条（中性描边）`[year]:595` ③ 可勾选切换（`chip:has(input:checked)`）`BatchEditModal:763` ④ 剧种编辑胶囊 `CategoryTags:222` | 4 个文件 |
| `.capsule` / `.cap-text` | 表单里的多值胶囊 | `RecordForm:3125` |
| `.d-cat` | 剧目卡剧种角标 | `dramas:248` |
| `.troupe-tag` | 卡片剧团标签（含 `+N`） | `RecordCard:140` |
| `.ztag` | 折子链接 | `records/[id]:521` |
| `.pstat` | 导入预览统计块 | `data:227` |

`.chip` 一名四义是最需要处理的命名污染：既可能是按钮、也可能是纯展示、还可能是 `<label>` 包隐藏 checkbox。

状态色值硬编码两份（`:926a18`/`:5a5a60`/`:5d3e6e`）：

- `RecordCard.svelte:237-247`（`rgba(...,0.82)` 叠半透明）
- `records/[id]:1102-1114`（同色系 solid hex）

### 2.14 图标

| 方式 | 用量 |
|---|---|
| 内联 `<svg>` | 25 处 / 12 文件（calendar 6、首页 4、settings 3、layout 3；其中 5 处是图表组件的画布） |
| `OperaIcon` 组件 | 首页/演员/剧目占位图 |
| 文字符号 | `✕`（13 处）、`×`（1 处，`CategoryTags:176`）、`⌕`（`:726`）、`⇪`（`data:184`、`artists/[id]:172`）、`❏`（`categories:97`）、`戏`（`dramas:193`） |
| Emoji | KPI 图标 `🎫💰📍🎭📚`（`analytics:170-193`）、区块标题 `📈⚡📄`、提示 `💡⏳⚠✓` |

四套体系并存。其中「删除/关闭」用 `✕` 与 `×` 两种字符（仅 CategoryTags 用 `×`）。

### 2.15 灯箱

4 处实现，其中全局灯箱是「事实标准」而未被遵守：

| 实现 | 位置 | 触发方式 |
|---|---|---|
| 全局委托 | `+layout.svelte:250-273`，任意 `.coverable` 元素 | 类名委托（推荐） |
| 记录详情 | `records/[id]:368-381`（`role=button` + `onclick`） | 手写，未加 `.coverable` |
| 演出表单 | `RecordForm:1577,2669`（`use:portal` + 自己的 Esc） | 手写 |
| 封面维护 | `CoverMaintenance:163`（自己的 Esc） | 手写 |

后果：每新增一处手写灯箱，就要重复实现一次 Esc 与滚动锁定；`calendar:620` 的注释甚至记录了「`.coverable` 会与全局委托打架（全局灯箱在捕获阶段 `preventDefault`，会把『点海报进详情』改写成看大图），所以故意不挂该类」。

### 2.16 事件与属性语法

- 事件属性基本完成 Svelte 5 迁移（`onclick`/`onkeydown`），仅 `CoverPicker:214` 残留 `<svelte:window on:keydown>`。
- 激活态修饰符 `on` 与 `active` 各半：`class:on` 14 处（settings 4、CostCompletion 5、RecordForm 1、VenuePanel 1、records/[id] 1、+page 2）vs `class:active` 12 处（layout 3、RecordForm 6、CategoryTags 1、records/[id] 1、+page 1）。`RecordForm` 同时使用两种。

---

## 3. 三档分类结论

判据（四项，满足前三项才考虑统一）：① 交互语义是否同构；② 视觉口径是否已经事实一致；③ 数据结构是否同构；④ 是否存在不可参数化的业务规则。

### A 档：可直接统一（不改交互，只收敛实现）

| # | 对象 | 现状 | 动作 | 依据 |
|---|---|---|---|---|
| A1 | `.hint` 说明文字 | 6 份副本（BatchEditModal/BackupPanel/CostCompletion/RecordForm/settings/calendar），字号 12/12.5、色 `text-3`，且全局已有 `.tiny`（色 `text-muted`） | 提到 `app.css` 为 `.hint`，与 `.tiny` 明确分工（`.hint`=表单说明、`.tiny`=元信息） | ②③ 完全一致，纯命名分裂 |
| A2 | `.card.sec` 内边距 + 卡片标题字号 | `.section`/`.sec` 各 `padding:18px 20px`；`h3` 4 档字号 12 处声明 | 全局 `.card.sec{padding:18px 20px}` + `.card.sec h3{font-size:15.5px}`（取多数） | ② 视觉已一致，只是没抽 |
| A3 | `input[type=checkbox/radio]` 主题色 | 10 处 `accent-color: var(--accent)`，BatchEditModal 的 22 个复选框遗漏 | 提到 `app.css` 全局 | ② 已有明确口径，只是漏应用 |
| A4 | `use:portal` | 3 份逐字相同（`CoverPicker:200`、`RecordForm:1577`、`ChartTip:15`） | 抽 `lib/actions/portal.js` | ③ 完全同构 |
| A5 | 二次确认 | 8 处 `askConfirm` vs 12 处原生 `confirm()` vs 1 处手写弹窗 | 全部改走 `askConfirm`，删掉 categories 的自建弹窗 | ① 语义同为「破坏性操作确认」；项目已有唯一实现。注意 `categories:40` 的注释写明「原生 confirm 在预览浏览器中被屏蔽，故用页内弹窗」——**该处动机正确、实现重复**，换 `askConfirm` 即可同时满足动机与一致性 |
| A6 | `.btn` 修饰符顺序 | `sm/ghost`、`ghost/sm`、`sm/danger`、`primary danger` 等 4 种写法 | 约定「语义修饰 → 尺寸修饰」（`btn primary sm`）并修正 7 处 | ② 渲染结果相同，纯书写口径 |
| A7 | `✕` / `×` | 13 : 1 | 统一 `✕` | ② |
| A8 | 激活态修饰符 | `on` 14 : `active` 12 | 统一 `.active`（导航、状态 chip 已在用） | ② |

A 档合计：约 30 处声明 → 4 处；无 UI 变更，可逐页回归。

### B 档：参数化统一（同一契约，参数化内容与尺寸）

| # | 目标组件 | 收敛对象 | 必需 props | 判断依据 |
|---|---|---|---|---|
| B1 | `Modal`（弹窗外壳） | 6 个实现（见 2.5） | `open/onclose/width/labelledby/closeOnMask/scroll` + `head`/`foot` snippet | ① 交互契约完全相同（Esc、陷阱、滚动锁、focus 返回）；差异只在**内容布局与宽度** |
| B2 | `Segmented` | `.seg`×2、`.cost-seg-btn`、`.theme-opt` | `items:[{value,label,icon?}]`、`bind:value`、`variant:'text'\|'icon'`、`size` | ① 同为单选组；差异是排版方向与是否带图标 |
| B3 | `EmptyState` | 11 处页面空态 + 3 处图表内空态 + 2 处纯文本 | `title/hint/size` + `icon` snippet + `action` snippet | ① 语义同（无数据引导）；差异在**图标来源**（`OperaIcon`/内联 SVG/文字/无）与是否带动作 |
| B4 | `Banner` | 49 处 `.banner` | `tone:'error'\|'success'\|'info'`、内容 children | ② 视觉已统一；差异只是手写前缀符号（`⚠/✓/💡/⏳`），由组件内置即可消除漂移 |
| B5 | `IconButton` | 约 15 处散落（`del`（3 份逐字 CSS）/`x`/`close`/`search-clear`/`view-x`/`nav-btn`/`op`） | `label`（必填 aria-label）、`tone`、`size`、`children` | ① 同为「无文字按钮」；差异在尺寸 24/28/30/38、描边有无与危险色。`.del` 是同一份 CSS 复制三次，收敛收益最直接 |
| B6 | `Skeleton` | 15 处内联 `style="height:Npx"` | `h/w/radius/count` | ② 只是内联尺寸；组合骨架（首页/详情页）仍由页面拼 |
| B7 | 卡片网格 | `.grid` 7 处定义（3 种 `minmax`）+ `.cards-row` | 全局 `.grid-cards{--grid-min:172px;--grid-gap:14px}` | ② 参数只有轨道宽度与 gap；**用 CSS 变量而非组件**，避免为布局引入包装元素 |
| B8 | `StatusBadge` | `RecordCard:237`（overlay 变体）与 `records/[id]:1102`（chip 变体） | `status:0\|1\|2\|3`、`variant:'overlay'\|'chip'` | ① 语义同（演出状态）；色值重复硬编码，且缺语义 token |
| B9 | `ComboInput` / `TagInput` | RecordForm 6 套 + 首页 3 个原生 `datalist` | `value(s)`、`suggestions`、`separator`、`oncreate`、`multiline` | ① 交互同构（焦点即开、失焦延时关、↑↓/Enter/Esc）；差异是单值/多值与是否允许新建。已有 `comboNav` 作为内核 |
| B10 | 灯箱 | 全局委托 1 + 手写 3（见 2.15） | `use:lightbox` action 或 `.coverable` + `data-no-lightbox` | ① 能力同（全屏看图 + Esc/遮罩关闭）；但全局委托与「卡片可点击」冲突（`calendar:620`），须先给委托加显式退出标记再统一。**顺序上放在 P3 之后**，不要先动 |

### C 档：应保持独立（不要为一致性而统一）

| # | 对象 | 位置 | 保留理由 |
|---|---|---|---|
| C1 | 日历网格与日期格 | `calendar:200-560` | 业务专属：`grid/row/gridcell` 三层结构 + roving tabindex + 海报层网格 + 节假日角标 + 骨架行数对齐防抖动。任何抽象都会损失这些约束 |
| C2 | 图表组件族 | `components/analytics/*` | 数据形状本就不同（`data[{label,value}]` / `labels+series` / `points`）；已正确拆成 6 个组件，`height/unit/labelEvery` 参数口径统一。**不应合并成统一 `Chart`** |
| C3 | `.settings-grid` + `CARD_COLS` DP | `settings:1396`、CARD_COLS | 分列依赖逐卡实测高度（权重表写死在源码），属该页专属布局算法；抽象需注入权重，收益为负 |
| C4 | `RecordForm` 字段宽度档位 | `.row.two-flex/.four-equal/.fee-row` | 与字段语义绑定（城市↔场馆并排、时间/状态/评分/时长四等分），且带 `!important` 的移动端塌缩覆盖；抽象会制造新的特异性战争 |
| C5 | 首页无限滚动 + 筛选面板 | `+page:600-621`、`:916-1200` | 唯一消费点；筛选面板还耦合 URL 同步、缺失字段治理、保存视图。**若未来出现第二个长列表，再抽 `use:infiniteScroll` action**，现在不抽 |
| C6 | `BatchEditModal` 三态字段控件 | `BatchEditModal:150-590` | 「保持不变 / 设为值 / 清空」是批量编辑独有的语义，与普通表单字段不同构；只应替换其外层弹窗壳（B1） |
| C7 | 剧团标签按 canvas 测宽收敛 | `RecordCard:37-73` | 性能敏感（避免 DOM 反复测量）、仅卡片一处需要；抽出去只会增加调用成本 |
| C8 | 日历「只看订阅范围」 | `calendar:41-54` | ICS 导出规则（不含已取消/未赴约）是业务约定，不是 UI 模式 |
| C9 | KPI 卡 emoji 图标 | `KpiCard` | 分析页独立视觉语言（emoji + 大数字），全站唯一使用场景；强行换成 SVG 会削弱该页识别度 |
| C10 | 分析页「链接即按钮」 | `analytics/+page.svelte`、`analytics/[year]/+page.svelte`（两页 0 个 `<button>`） | 这是**阅读型页面**：交互只有「跳转 / 切换年份」，用 `<a>` 语义正确、可右键新标签打开。不要为了「全站按钮统一」把它们改成 `<button>` |

---

## 4. 需要修正的具体不一致（清单）

### 4.1 命名

| 问题 | 证据 | 建议 |
|---|---|---|
| `.chip` 一名四义 | `+page:1629` / `[year]:595` / `BatchEditModal:763` / `CategoryTags:222` | 拆为 `.chip-filter`、`.chip-meta`、`.chip-toggle`；`CategoryTags` 保留 `.chip` |
| `.empty` 一名三义 | 页面空态 / 图表「暂无数据」/ 日历空白格 | 日历改 `.cell.is-blank`；图表内改 `.empty-inline` |
| `.section` vs `.sec` | 8 文件 | 统一 `.card.sec`（A2） |
| `on` vs `active` | 14 : 12 | 统一 `.active`（A8） |
| `.hint` vs `.tiny` | 色值不同（`text-3` vs `text-muted`） | 明确分工（A1） |
| `small` 空类名 | `artists/[id]:231`、`dramas/[id]:384` | 要么定义 `.empty.sm`，要么删掉 |
| `✕` vs `×` | `CategoryTags:176` | 统一 `✕`（A7） |

### 4.2 属性 / props

| 组件 | 问题 |
|---|---|
| `KpiCard` | `icon` 收 emoji 字符串，而全站其它图标是 SVG/组件；`value` 收 `number \| string`（`:172` 传数字、`:179` 传已格式化字符串），格式化职责边界模糊 |
| 图表族 | `VBarChart` 用 `maxValue`，`LineChart` 用 `yMax`，`ScatterChart` 用 `yMax` —— 同一概念两个名 |
| `ConfirmModal` | `danger` 默认 `true`（`confirm.js:12`），导致「删除视图」这类非破坏操作要显式传 `danger:false`（`+page:435`）；默认值与使用频率相反 |
| `BackLink` | `fallback` 默认 `'/'`，但详情页多数传 `/artists`、`/dramas`；`label` 默认 `'← 返回'` 与传入的 `'← 演员'` 并存（两种语序） |
| 表单字段 | 无统一 `error` 属性，字段级错误由各页自行渲染（`RecordForm` 红边 + 红字、settings 顶部 banner） |

### 4.3 交互行为

| 场景 | 不一致 |
|---|---|
| 弹窗关闭 | 批量编辑/剧种删除无法用 Esc 关闭；封面选择器可；筛选面板可 |
| 弹窗焦点 | 仅筛选面板与封面选择器设初始焦点；批量编辑打开后焦点仍在背景按钮上 |
| 删除确认 | 原生 `confirm()`（12 处，无法样式化、移动端文案被截断）vs 自定义弹窗（8 处） |
| 保存反馈 | 瞬时行内文字（settings/rating/status）vs 常驻 banner（data 导入）vs 无反馈（列表页增删） |
| 下拉补全 | 表单自定义下拉 vs 筛选面板原生 `datalist`（Safari/移动端不弹） |
| 首屏失败 | 首页/设置页可重试，其余 15 处只能刷新 |
| 新增跳转 | `artists:42` 用 `location.href` 整页刷新（其它处用 `goto`/`resolve`） |

### 4.4 样式口径

| 项 | 值 |
|---|---|
| 卡片内 `h3` 字号 | 14.5 / 15 / 15.5 / 16 px（12 处） |
| 卡片内边距 | 统一 `18px 20px`（无需改） |
| 网格轨道 / 间距 | 轨道 120 / 160 / 172 / 200 / 320(min) / 1fr-fr；gap 10 / 12 / 14 / 16 |
| 复选框主题色 | 6 文件声明 10 次，`BatchEditModal` 缺失 |
| 状态色 | 两处硬编码同一色系，一处半透明叠色、一处 solid |
| 内联样式 | 85 处 `style="…"` |

### 4.5 死代码 / 空类名（顺带清理）

| 位置 | 问题 |
|---|---|
| `app.css:344-363` | 整段 `@media` 兜底 `.row > .f-xs/.f-sm/.f-md/.f-lg/.f-dt`，但全库模板中已无任何 `f-*` 类（grep 零命中），`RecordForm` 也不再定义它们 |
| `artists/[id]:231`、`dramas/[id]:384` | `class="empty small card"` 中的 `small` 在全局与局部均未定义 |
| `BackLink.svelte` | `.back` 的局部样式与调用方页面内的 `.back` 副本并存（多处页面自行复制） |
| `categories:30` | `api.createCategory({ name, activeIds: [], recordCount: 0 })` 夹带两个空字段，后端并不接收 |

---

## 5. 抽象层次建议

分三层，配套目录约定（`#lib` 别名已配好）：

```
web/src/lib/
├── actions/portal.js          # A4：use:portal
├── ui/                        # 基础控件（无业务语义，只认 props）
│   ├── Modal.svelte           # B1
│   ├── Segmented.svelte       # B2
│   ├── EmptyState.svelte      # B3
│   ├── Banner.svelte          # B4
│   ├── IconButton.svelte      # B5
│   ├── Skeleton.svelte        # B6
│   └── StatusBadge.svelte     # B8
├── components/                # 业务组件（认领域对象）
│   ├── ComboInput.svelte      # B9 单值
│   ├── TagInput.svelte        # B9 多值（并可承接 CategoryTags 的通用部分）
│   └── …（现有 RecordCard / CoverPicker / …）
└── app.css                    # 结构层：.card.sec / .hint / .grid-cards / status tokens
```

**布局容器不建组件，走全局类 + CSS 变量。** 理由：`.grid` 的场景只需要「轨道下限 + 间距」两个参数，用 `<CardGrid>` 会给 11 个列表页各加一层 DOM，与项目「紧凑、高信息密度、拒绝无意义包裹」的既有取向冲突。

```css
/* app.css 追加 */
.card.sec { padding: 18px 20px; }
.card.sec h3 { margin: 0 0 14px; font-size: 15.5px; }
.hint { font-size: 12px; color: var(--text-3); display: block; margin-top: 4px; }
input[type='checkbox'], input[type='radio'] { accent-color: var(--accent); }
.grid-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(var(--grid-min, 172px), 1fr));
  gap: var(--grid-gap, 14px);
}
:root { --status-ok: var(--accent); --status-wish: #926a18; --status-cancel: #5a5a60; --status-miss: #5d3e6e; }
```

**Modal 的契约（必须内建，不可由调用方选择）**：`use:portal` → `body.overflow=hidden` → Esc 关闭 → Tab 循环陷阱 → 初始焦点到 `[autofocus]` 或首个可聚焦项 → 关闭时焦点回到触发元素。这是把 `+page.svelte:678-717` 与 `calendar:130-146` 已有的正确做法固化为唯一实现。

---

## 6. 可执行收敛路线

按「改动半径 / 回归成本」排序，每批都可独立发布。

### P0 — 纯样式与工具，零交互变更（1 个工作单元）

| 步骤 | 内容 | 涉及文件 | 风险 |
|---|---|---|---|
| 0.1 | 全局补 `.hint`、`.card.sec` + `h3`、`accent-color`、`.grid-cards`、状态 tokens | `lib/app.css` | 极低。`h3` 统一到 15.5px 会让 analytics(15→15.5)、详情页(16→15.5)、CostCompletion(14.5→15.5) 有 0.5~1.5px 视觉变化，**需截图确认** |
| 0.2 | 删 6 份 `.hint` 副本、12 处 `h3` 声明、10 处 `accent-color` | settings / data / RecordForm / BatchEditModal / BackupPanel / CostCompletion / CostMaintenance / calendar / analytics×2 / records[id] / artists[id] / dramas[id] | 低。**必须核对 Svelte scoped 特异性**：组件内若还有 `padding` 局部覆盖，删全局前先确认 |
| 0.3 | `use:portal` → `lib/actions/portal.js` | CoverPicker / RecordForm / ChartTip | 低 |
| 0.4 | 修 `.btn` 修饰符顺序 7 处、删 `VenuePanel:218` 的冗余 `primary`、`✕` 统一、`class:on`→`class:active` | 见 4.1/4.2 | 低，但 `on→active` 要同步改 CSS 选择器，漏改会导致选中态消失 |

> 注意：`settings/+page.svelte` 的 `CARD_COLS` 权重是**实测高度**，任何卡片内边距/字号变化都会改变列高分配。P0 改完必须重测权重（遍历 `.settings-grid .col > .card` 取 `getBoundingClientRect().height`，按 h3 文本映射 key 更新 CARD_COLS）。

### P1 — 弹窗契约归一（最高收益）

| 步骤 | 内容 | 涉及文件 | 风险 |
|---|---|---|---|
| 1.1 | 新建 `ui/Modal.svelte`，内建 portal + 滚动锁 + Esc + 焦点陷阱 + 焦点恢复 | 新文件 | 中。焦点陷阱与 `fade-up` 的 `transform` 包含块冲突是既有坑，必须走 portal 并等过渡结束再送焦点（参考 `+page:707` 的 220ms 延时） |
| 1.2 | 迁移 `BatchEditModal`（当前 0 项契约） | `BatchEditModal.svelte:151-158,588` | 中。该组件 837 行、内部有下拉/多选，迁壳时**只换外层**，不动内容 |
| 1.3 | 迁移 `categories` 删除弹窗 → 直接改用 `askConfirm`（`impact`/`impactLabel` 已支持「影响 N 条」） | `categories:122-141` | 低，可删除 40 行模板 + 30 行 CSS |
| 1.4 | 迁移 `calendar` 当日弹窗（保留其焦点回格逻辑，`:130-146`） | `calendar:580-680` | 中。日历弹窗有 `scale` 过渡与 gridcell 焦点回位，需保留 |
| 1.5 | 迁移 `CoverPicker`（保留左右翻页键、`aria-busy`） | `CoverPicker:217` | 中。浮层内有搜索框与网格滚动，`min-height:0` 链条不能断 |
| 1.6 | 首页筛选面板改挂 `Modal` | `+page:916-930` | 低。它已是契约最完整的实现，本质是「把已有代码搬进组件」 |
| 1.7 | 12 处原生 `confirm()` → `askConfirm`，并把 `danger` 默认值改为 `false` | BackupPanel / CoverMaintenance / records[id] / artists / dramas / confirm.js | 低。注意 `askConfirm` 是异步的，调用处已在 `async` 函数内；`danger` 默认值变更需同时检查 8 个既有调用点（当前多数显式传了 `danger`） |

### P2 — 组合控件（按使用密度排序）

| 步骤 | 内容 | 涉及页面 | 风险 |
|---|---|---|---|
| 2.1 | `Segmented`（4 处） | 首页筛选、地图、费用补全、设置主题 | 低—中。设置主题是 `variant="icon"` 竖排，需确认窄屏不塌 |
| 2.2 | `EmptyState`（11 + 3 + 2） | 首页、演员、剧目、剧种、地图、演员详情、剧目详情、封面选择器、日历、数据分析 | 低。图标用 snippet 注入，**不做图标体系重构** |
| 2.3 | `Banner`（49 处） | 23 个文件 | 低，但 settings 有 17 处、其中若干是「多元素进度条」，需支持 children |
| 2.4 | `IconButton`（约 15 处） | 演员/剧目/剧种列表、封面选择器、批量编辑、日历工具栏、顶栏汉堡 | 中。`aria-label` 必填后要逐一补文案（现有 `title` 有、`aria-label` 缺失的地方会被暴露） |
| 2.5 | `Skeleton`（15 处内联） | 12 个页面 | 低 |
| 2.6 | `.grid-cards` 替换 3 处逐字相同的 `.grid`（首页 172 / 演员详情 172 / 剧目详情 172），其余保留各自 minmax | 3 页 | 低。**不要顺手把 160/200 也改掉**，那是列表密度差异，属有意为之 |

### P3 — 业务组件（收益最大、风险最高）

| 步骤 | 内容 | 涉及页面 | 风险 |
|---|---|---|---|
| 3.1 | `ComboInput`（单值）+ `TagInput`（多值），以 `comboNav` 为内核，统一「焦点开 / 失焦 120ms 关 / IME 安全 / 选中后是否保持打开」 | `RecordForm.svelte` 5 个字段 | **高**。这里是全站最复杂的交互之一：中文输入法、Backspace 删尾项、拖拽排序、失焦提交、表单层 Enter 拦截（`:670` 的 `stopPropagation` 是必要条件）。必须先写组件再逐字段替换，一次只换一个字段并手测 Safari/中文输入 |
| 3.2 | 用 `ComboInput` 替换首页筛选面板的 3 个原生 `<datalist>` | `+page:1102-1138` | 中。修复移动端补全失效；需注意筛选面板的 `onchange={load}` 语义要改为 `onpick` |
| 3.3 | `StatusBadge` 两处 + 状态 token | RecordCard、records/[id] | 低。可以先建组件再替换，色值从 token 取 |
| 3.4 | `CategoryTags` 与 `TagInput` 合并评估 | RecordForm、dramas 列表 | 暂缓。`CategoryTags` 带拖拽排序 + 「已知/未建档」双色语义，先抽 `TagInput` 到能覆盖 80% 再考虑 |

### 验证清单（每批必做）

1. `pnpm run build`（**dev 不报重复声明，必须 build**）；build 前 `rm -rf dist .svelte-kit node_modules/.vite node_modules/.vite-temp` 避免陈旧 chunk。
2. 窄屏 `documentElement.scrollWidth > clientWidth` 检测（改网格/弹窗必查）。
3. 指定页面截图对比（P0 涉及字号 0.5px 变化，需 1:1 对照）。
4. 设置页改样式后重测 `CARD_COLS` 权重。
5. 弹窗类改动额外测：Esc、Tab 循环、关闭后焦点回到触发元素、背景不可滚动、Safari 中文输入法。

---

## 7. 明确不做的统一

1. **不引入 Toast 系统**。现有反馈是「就地 + 常驻」两条路径，符合紧凑布局；引入浮层会在移动端遮挡底部操作。应统一的是「瞬时成功提示」的一条实现，不是引入新范式。
2. **不合并图表组件**。数据形状不同，`height/unit/labelEvery` 已统一，合并只会有更多的 `if`。
3. **不抽分页组件**。只有首页有分页，且形态是无限滚动。
4. **不把 `.grid` 全部改成一个值**。160px（演员头像卡）/172px（记录海报卡）/200px（剧目/剧种文字卡）/120px（封面缩略图）分别对应不同内容密度，是设计意图而非漂移。
5. **不为「图标统一」重构 Emoji**。KPI 与区块标题的 emoji 是分析页的视觉语言；需要统一的是「同一交互的图标」（删除/关闭），不是全局图标体系。
6. **不动 `CARD_COLS` 与 `RecordForm` 字段档位**。前者的输入是实测像素，后者的输入是字段语义。

---

## 8. 风险登记

| 风险 | 说明 | 缓解 |
|---|---|---|
| Svelte scoped 特异性 | 删除组件内 `.hint`/`.card.sec h3` 后，若全局规则与组件内其它规则优先级不同，样式可能不生效或过度生效 | 每次只删一个文件的副本，dev 里用计算样式核对 |
| `fade-up` + `position:fixed` | 入场动画 `fill-mode:both` 残留 `transform`，使 `.fade-up` 成为 fixed 的包含块 | 所有浮层必须 portal 到 `body`；`data` 页四个工具组件已挂在 `.fade-up` 之外，迁移时勿改动其父级 |
| `$state(new Set())` 不响应 | 批量选择、封面维护的选中集合 | 任何「统一选择逻辑」的改动都要保持 `x = new Set(x)` 的整体赋值写法 |
| 弹窗焦点陷阱与过渡动画 | 陷阱在 `transform` 动画期间计算 `offsetParent` 会出错 | 等过渡结束（约 200ms）再设初始焦点，参考 `+page:708` |
| 中文输入法 | 组合下拉、CategoryTags、搜索框都依赖 `isComposing` / `compositionend` | 抽 `ComboInput` 时必须把 IME 判断放进组件，不能留给调用方 |
| settings 列高 | 任何卡片内容/间距变化都会破坏 `CARD_COLS` 的平衡 | P0/P2 改完重测权重 |
| 移动端 16px 输入字号 | `app.css:321` 用高特异性 `html body input` 覆盖，组件内若写 `font-size` 会与之打架 | 抽输入类组件时不要自带 `font-size` |

---

## 附：改动收益估算

| 批次 | 预计净减代码 | 收益 |
|---|---|---|
| P0 | 约 180 行（副本删除） | 统一口径，消除 4 档 `h3` 与 2 套 `.hint` |
| P1 | 约 120 行（弹窗外壳与确认逻辑） | **修复 2 个弹窗完全不可用键盘关闭**；13 处确认框（12 原生 + 1 自建）收敛到同一实现 |
| P2 | 约 200 行 | 消除 49 处手写符号、15 处内联样式、4 处重复网格 |
| P3 | 约 250 行（RecordForm 内部），新增 2 组件 | 修复移动端筛选补全失效；表单维护成本大幅下降 |

---

## 附录：计数口径

本文所有数字均以 `web/src` 当前工作树为准，逐项 grep 复核：

| 指标 | 值 | 口径 |
|---|---|---|
| `<button>` | 186 | 含所有章节，不含 `<a>` 伪按钮 |
| `<a class="btn*">` | 7 | 链接语义承担按钮外观 |
| `.btn` 引用 | 128 | 占 69% |
| `<label>` | 104 | — |
| `.input` | 110 | — |
| `.banner` | 49 | 23 个文件 |
| `.skeleton` | 21（其中 15 处内联高度） | — |
| `class="empty…"` | 15（页面级 11 / 图表内 3 / 日历占位 1） | — |
| 原生 `confirm(` | 12 | 排除 `askConfirm(` |
| `askConfirm(` | 8 | 5 个文件 |
| `class:on` / `class:active` | 14 / 12 | — |
| `style="…"` 内联 | 85 | — |
| scoped CSS | 8,201 行 / 34% | 按 `<style>` 起始行到文件末 |

> 同日早间曾有一份初稿落在 `outputs/ui-convergence-audit-2026-10-22.md`。两份的定性结论一致，本文件是复核后的版本：初稿中若干计数为目测估算（如原生 `confirm()` 记为 13 处），本文已按上表口径逐条重数，并补充了 `<a class="btn">`、复选框四档尺寸、`CoverPicker` 按钮分页、`app.css` 死代码四项发现。

---

# 实施记录（P1 → P2 → P3 已落地）

按本文推荐顺序完成 P1/P2/P3；P0（纯样式副本清理）未动。以下为实际落地清单与验证结论。

## 新增文件

| 文件 | 作用 |
|---|---|
| `web/src/lib/actions/portal.js` | 挂载到 `document.body` 的 action，替代 CoverPicker / RecordForm / ChartTip 三份逐字相同的私有副本 |
| `web/src/lib/ui/Modal.svelte` | 全站唯一弹窗外壳：`role="dialog"` + `aria-modal` + `aria-labelledby/aria-label`、滚动锁、Esc、Tab 焦点陷阱、延迟 220ms 初始焦点、关闭后焦点回位；`variant='dialog'｜'sheet'` |
| `web/src/lib/ui/Segmented.svelte` | 分段控制，`variant`：`pill`（首页缺失模式）/`soft`（地图底图）/`line`（费用补全）/`card`（设置主题） |
| `web/src/lib/ui/EmptyState.svelte` | 空态（`icon` / `title` / `hint` / `small` / `bare` / `action`） |
| `web/src/lib/ui/Banner.svelte` | 提示条，`tone` 内置默认前缀符号，`icon={false}` 可去掉 |
| `web/src/lib/ui/IconButton.svelte` | 图标按钮，`label` 必填（aria-label），`size` sm/md/lg |
| `web/src/lib/ui/Skeleton.svelte` | 骨架块，消灭 15 处内联 `style="height:…"` |
| `web/src/lib/ui/StatusBadge.svelte` | 状态徽标，`variant="badge"`（海报角标，`floating`）/`"chip"`（详情页胶囊）；色值走 `--status-*` token |
| `web/src/lib/components/ComboInput.svelte` | 补全输入框，`multi` 支持多值（末段作查询词、选中替换末段），中文输入法安全 |

## 迁移与删除

- **弹窗外壳 6 → 1**：`BatchEditModal`、`calendar` 当日弹窗 + 年份选择、`CoverPicker`、首页筛选面板（sheet）、`categories` 删除弹窗（改为 `askConfirm`）。删除各自的 `.modal-mask/.modal/.modal-head/.close/.modal-foot` 等样式与手写 Esc / 焦点陷阱 / 滚动锁代码。`BatchEditModal` 原先**完全没有** `role="dialog"`、Esc 与焦点陷阱，随迁移一并修复。
- **确认框 13 → 1**：12 处原生 `confirm()` 全部改 `askConfirm`（BackupPanel ×5、CoverMaintenance ×3、`records/[id]` ×2、`artists`、`dramas`；`categories` 的自建弹窗亦收敛）。`confirm.js` 中 `danger` 默认值 `true → false`，只有破坏性操作显式传 `true`。
- **提示条 49 → `Banner`**：机械迁移 32 处手写 `⚠`/`✓` 前缀。
- **图标按钮 8 处**：3 份 `.del`、2 份 `.x`、`search-clear`、`view-x`、2 份 `.op` 收敛到 `IconButton`。
- **网格 3 页**：首页、演员详情、剧目详情的手写 `grid-template-columns: repeat(auto-fill, minmax(17x px,1fr))` 收敛为全局 `.grid-cards`。
- **RecordForm 4 字段**：城市 / 场馆 / 渠道 / 同行 改用 `ComboInput`，删除约 180 行 state 与 handler；首页 3 处原生 `datalist` 同步替换（顺带修复 Safari / 移动端不弹建议）。
- **状态色**：`app.css` 新增 `--status-ok/-wish/-cancel/-miss`；`RecordCard` 角标与详情页胶囊改用 `StatusBadge`，删掉两处各自硬编码的色值。
- **设置页主题选择**：三张卡片改用 `Segmented variant="card"`，SVG 图标以 snippet 传入。

## 有意保持独立

依本文「不强行统一」原则，以下未并入：日历日期格（WAI-ARIA roving tabindex）、图表族及其单行空态、设置页 `CARD_COLS` 最优划分 DP、RecordForm 字段宽度档位、首页无限滚动、剧团 tagbox（拖拽排序）、canvas 测宽、KPI emoji、分析页链接即按钮、日历当日的 soft 状态 chip。P3.4（`CategoryTags` 与 `TagInput` 合并）按原结论暂缓。

## 验证

- `vite build` 通过；`prettier --check` 全绿；`eslint` **0 error / 16 warning**（改造前基线 17 warning，未新增）。
- Playwright（真 Chromium，1280×900 + 390×844）**33/33 通过**：弹窗 6 项契约（打开 / 标题 / 滚动锁 / 初始焦点 / Tab 陷阱 / Esc 关闭 + 焦点回位 + 滚动恢复）、9 页窄屏零横向溢出、日历当日弹窗、4 个 `ComboInput` 聚焦出建议 + 键入过滤 + 键盘选中 + 多值点选、详情页状态胶囊、删除确认走 `askConfirm`、无结果空态。
- 验证期间修掉一个**既有**缺陷：`RankList` 的键是 `(it.id ?? i)`，而 `/api/analytics` 的 `top_venues` 后端返回 `'' AS id`（`''` 不是 nullish，`??` 不兜底）→ 10 条同为 `''` → 控制台 `each_key_duplicate`。改为 `(it.id || i)`，与同组件里 `{#if hrefFn && it.id}` 的口径对齐。

## 部署链路提醒

前端产物要进二进制，落点是 **`internal/web/dist`**（`internal/web/embed.go` 的 `//go:embed all:dist`），不是旧路径 `backend/dist`；且必须先 `rm -rf internal/web/dist` 再 `cp -r`，否则会嵌套成 `dist/dist`、二进制仍带旧外壳。`Makefile` 的 `build` 目标已对此有注释警告。

