# 幕间前端问题修复报告

基于前次 6 维度审查的 12 项问题，已全部落地修复。前后端均通过构建与测试（`go test ./internal/db/` 全绿；`pnpm run build` 成功）。

## ① 返回按钮 / 筛选保留（原最高严重度）

**问题**：首页筛选是纯前端 `$state`、从不写入 URL；点开记录再「← 返回」走 `history.back()` 落到的是 `/`（无 query）→ 筛选被清空，误入未筛选页。且 `BackLink` 只要存在站内历史就一律 `history.back()`，「← 剧目 / ← 演员」在从某条演出点进来时会返回到那条演出详情，与标签承诺不符。

**修复**：
- `frontend/src/routes/+page.svelte`：新增 `urlReady` + `buildFilterQuery()` + `$effect`，筛选变化后用 `history.replaceState` 把状态写回地址栏（不新增历史记录）；`onMount` 仍从 URL 回填筛选。这样从筛选后的首页进入记录、再返回，回到的是带筛选的首页。
- `frontend/src/lib/components/BackLink.svelte`：传了 `href`（规范化上级页，如 `/dramas`、`/artists`）时直接 `goto(href)`，标签与目的地永远一致；未传 `href`（演出详情）时才 `history.back()`，保留首页筛选上下文。

## ② 剧种污染（聚合修复 —— 你的专项要求）

**问题**：剧目 `categoryNames` 由后端按「关联该剧目」的所有演出 `category_names` 聚合；一场多剧种/拼盘演出会把所有剧种灌进每个关联剧目，污染其剧种显示，被迫「手动修正」。

**修复（聚合操作仅取单独演出 + 过滤拼盘噪声）**：
- `backend/internal/db/db.go` 的 `dramaCategoriesAll()` 与 `dramaCategoriesFor()` 增加：
  ```sql
  WITH solo AS (
    SELECT record_id FROM record_dramas GROUP BY record_id HAVING COUNT(*) = 1
  )
  ... JOIN solo s ON s.record_id = rd.record_id ...
  ```
  仅聚合「该剧目被单独演出（record_dramas 仅含 1 条）」的演出 `category_names`，拼盘类演出的噪声数据被完全过滤，结果只反映剧目独立演出时的真实剧种信息。
- 新增回归测试 `TestMultiCategory`（牡丹亭独演=昆曲、拼盘同演长生殿=京昆）：断言牡丹亭聚合结果不含「京昆」、长生殿因仅出现在拼盘中而聚合为空。
- 手动覆盖仍保留为**显式逃生舱**：仅出现在拼盘、从无任何单独演出的剧目，可在剧目详情页手动标注剧种（这是有意为之，区别于之前「在演出编辑页里静默改写全局剧种」的 footgun）。
- **中性化 footgun**：`frontend/src/lib/components/RecordForm.svelte` 的剧目内联编辑移除「剧种」字段（只留改名），`saveDramaEdit` 不再全局改写剧目剧种；剧目详情页提示文案同步为「按该剧目单独演出时的剧种自动统计（拼盘演出已自动排除）」。

## ③ 批量编辑币种永久禁用

**问题**：`BatchEditModal` 的 `priceCurrency / payPriceCurrency / otherCostCurrency` 三个 `<select>` 没有对应 `enabled` 勾选框，`enabled` 永远 `false` → 币种无法编辑。

**修复**：`frontend/src/lib/components/BatchEditModal.svelte` 为每个币种 `<select>` 补齐 `enabled` 勾选框（`toggleField('priceCurrency')` 等），与 `save()` 内既有的 `if (fields.xxxCurrency.enabled)` 逻辑接通。

## ④ 编辑页无法直接加折子

**问题**：编辑演出时折子只能勾选、无新增入口，必须跳到剧目详情页添加再返回刷新。

**修复**：`frontend/src/lib/components/RecordForm.svelte` 在每个已选剧目的折子区新增「＋ 添加折子」内联输入，调用 `api.createZhezi` → `loadDramaTree()` 刷新 → 自动选中新折子，无需离开编辑页。

## ⑤ 多子项布局 / 卡片被拉大

**问题**：`RecordCard` 的 `.troupes`（剧团标签）无截断，多剧团把卡片撑高，Grid 同行被 stretch 拉高。

**修复**：
- `frontend/src/lib/components/RecordCard.svelte`：剧团标签最多显示 3 个，超出以 `+N` 收起（`troupeTags / shownTroupes / extraTroupes` 派生），并加 `.troupe-tag.more` 样式。
- 演员无关联档案（`artist_ids` 缺 id）时由 `<a>`（死链）改为 `<span>`，不再有无效跳转。

## ⑥ 其他交互 / 展示

- **记录详情「分类」只显首个剧种**：`frontend/src/routes/records/[id]/+page.svelte` 的「分类」卡片改为遍历 `categoryNames` 数组（与数组语义一致）。
- **编辑页币种自由文本易输错**：`RecordForm` 三个币种 `<input>` 改为 `<select>`，含 `CNY/USD/HKD/TWD/JPY/EUR/GBP/KRW/AUD/SGD` 常见币种，并保留当前值（`currencyOptions()` 派生），降低大小写/拼写不一致。
- 剧目详情页剧种编辑提示文案更新，反映新的单独演出聚合语义。

## 改动文件清单
- `backend/internal/db/db.go` — 剧种聚合（solo-only）+ 排序
- `backend/internal/db/db_test.go` — 拼盘噪声回归测试
- `frontend/src/lib/components/BackLink.svelte`
- `frontend/src/routes/+page.svelte`
- `frontend/src/lib/components/RecordForm.svelte`
- `frontend/src/lib/components/RecordCard.svelte`
- `frontend/src/lib/components/BatchEditModal.svelte`
- `frontend/src/routes/records/[id]/+page.svelte`
- `frontend/src/routes/dramas/[id]/+page.svelte`

## 验证
- 后端：`cd backend && CGO_ENABLED=1 go test ./internal/db/ -count=1` → `ok`；`go build ./...` 通过。
- 前端：`cd frontend && pnpm run build` → `✓ built` / `✔ done`。
- 浏览器端交互（返回保留筛选、内联加折子、批量币种）建议本地 `./dev.sh` 起服务后手动走查一遍。
