# 日历标题显示剧种 Implementation Plan

## Repository Research

### 当前状态

- **后端** `GetCalendarEvents` (`backend/internal/db/db.go:3019`) 查询了 `category_name` 但未查 `category_names`（JSON 数组字段）。`CalendarEvent` 模型 (`models.go:212`) 已有 `CategoryNames []string` 字段，但从未被填充。

- **前端** 日历页面 (`frontend/src/routes/calendar/+page.svelte`) 在弹窗列表第 333 行用 `{e.name}` 显示演出名，海报 `title` 属性（第 266 行）和 fallback 文字（第 271 行）也是原始名字。

- **数据结构**: `Record.category_names` 是 JSON 数组，如 `["昆剧"]`；拼盘演出也是单元素数组 `["拼盘"]`；多剧种拼盘可能有多元素。

- **现有剧种类别**: 京剧、越剧、豫剧、黄梅戏、昆剧、评弹、秦腔、粤剧、苏剧、沪剧、淮剧、川剧、蒲剧、婺剧、锡剧、潮剧、瓯剧、高甲戏、梨园戏、楚剧、湘剧、滑稽戏、台州乱弹、目连戏、扬剧、舞剧、音乐会、音乐剧、拼盘。

### 用户需求拆解

1. **加剧种前缀**: 单剧种、剧名不含剧种关键词 → 显示如 `豫剧《白蛇传》`
2. **不加的情况**:

   - 多剧种（`category_names` 数组长度 > 1）或分类为"拼盘" → 不加

   - 名字里已包含剧种关键词（如 `昆剧折子戏专场`、`京剧经典名段演唱会`）→ 不加

## Files and Modules

- **`backend/internal/db/db.go`** `GetCalendarEvents`: SQL 增加查询 `category_names` 列，Scan 增加解析 JSON 到 `CalendarEvent.CategoryNames`

- **`frontend/src/routes/calendar/+page.svelte`**: 添加 `formatTitle()` 函数；弹窗列表 `.di-name`、海报 `title` 和 `p-ph` fallback 改用格式化后的标题

## Implementation Steps

### Step 1: 后端补全 category\_names 字段

`GetCalendarEvents` 当前 SQL:

```sql
SELECT id, name, date, city, address, cover_file, cover_thumb, rating, active_status, category_name
```

改为加上 `category_names`:

```sql
SELECT id, name, date, city, address, cover_file, cover_thumb, rating, active_status, category_name, category_names
```

Scan 时把 `category_names`（JSON 文本）解析为 `[]string` 填入 `e.CategoryNames`。复用项目已有的 JSON 反序列化模式（项目中其他地方用 `json.Unmarshal([]byte(raw), &slice)` 解析类似字段）。

### Step 2: 前端 formatTitle 函数

在 Svelte 组件的 `<script>` 块中添加：

```js
// 不会作为"剧种前缀"加在剧名前的分类（本身就是汇总/类型而非具体剧种）
const NON_PREPEND_CATEGORIES = new Set(['拼盘', '音乐会', '音乐剧', '舞剧']);

function formatEventTitle(name, categoryName, categoryNames) {
  if (!categoryName || !name) return name;

  // 1. 多剧种（数组长度 > 1），不加剧种
  if (categoryNames && categoryNames.length > 1) return name;

  // 2. 剧种属于"非具体剧种"类别（拼盘、音乐会等），不加
  if (NON_PREPEND_CATEGORIES.has(categoryName)) return name;

  // 3. 剧名里已包含剧种关键词，不加（避免重复如"昆剧昆剧折子戏专场"）
  if (name.includes(categoryName)) return name;

  // 4. 单剧种 + 剧名不含剧种 → 加剧种前缀
  return `${categoryName}《${name}》`;
}
```

**关于书名号格式**: 用户示例用了 `豫剧《白蛇传》`，采用中文书名号包裹剧名。如果名字本身已经有书名号（如 `《扈家庄》《空城计》`），会变成 `京剧《《扈家庄》《空城计》》`...

所以需要额外处理：如果名字首尾已有书名号则不再包裹：

```js
const alreadyBracketed = /^《.*》$/.test(name);
return alreadyBracketed ? `${categoryName} ${name}` : `${categoryName}《${name}》`;
```

### Step 3: 前端替换显示位置

- 第 333 行 `.di-name`: `{e.name}` → `{formatEventTitle(e.name, e.category_name, e.category_names)}`

- 第 266 行海报 `title={e.name}` → `title={formatEventTitle(e.name, e.category_name, e.category_names)}`

- 第 271 行 fallback 文字 `{e.name?.[0] ?? '?'}` → 保持取首字不变（海报格子太窄，只显示首字）

## Dependencies and Considerations

- **JSON 解析模式**: 项目中 `records` 表的 JSON 数组字段（`artist_names`, `play`, `category_names`）在 Go 代码里统一用 `json.Unmarshal` 从 `[]byte` 解析。`category_names` 为空时存的是 `'[]'`，Scan 时要用 `sql.NullString` 或直接 string + 判断是否为空/`[]`。

- **数据库迁移**: 不涉及 schema 变更，只是读已有的 `category_names` 列，零迁移风险。

- **向后兼容**: ICS 日历导出 (`backend/internal/ics/ics.go`) 是另一套实现，不在本次改动范围；如果用户反馈 ICS 也需要加剧种，可以后续跟进。

- **"非具体剧种"名单**: 目前 hardcode 了 `['拼盘', '音乐会', '音乐剧', '舞剧']`。未来如果用户新增了其他汇总类分类，需要手动更新这个列表。可以考虑从 categories 表读取所有分类，标记哪些是"非具体剧种"，但这超出了本次范围。

## Validation

1. **后端**: `go test ./...` 通过，已有 `TestGetCalendarEvents` 覆盖基本路径；可补充一个 case 验证返回的 `CategoryNames` 被正确填充。
2. **前端**: 启动 dev server，打开日历页面，检查：

   - 单剧种且剧名不含剧种 → 显示 `剧种《剧名》`

   - 拼盘 → 只显示原名

   - 多剧种（如果有） → 只显示原名

   - 剧名含剧种关键词 → 只显示原名

   - 原名已有书名号 → 只加前缀不重复包裹

## Risks

- **剧名已有书名号**: 已在 Step 2 处理，用正则检测 `^《.*》$`。如果部分剧名只加了前或后一个书名号（边缘情况），会被正常包裹导致不完美但不影响阅读。

- **category\_names 存储格式不一致**: 某些旧记录可能 `category_names` 存的是 `null` 或空字符串而非 `[]`。后端 Scan 时需要优雅处理这些边界值。

- **前端 function 位置**: `formatEventTitle` 在 `$derived` 和模板里都要用到，必须放在函数调用之前（Svelte `<script>` 中顺序有要求吗？模板可以引用 script 里任何位置的函数，OK）。

