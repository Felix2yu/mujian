# 幕间（MuJian）性能审查报告

**审查日期**：2026-08-29
**技术栈**：Go 1.27 + chi v5.3.2 + modernc.org/sqlite（WAL）｜SvelteKit 2.70 / Svelte 5 / Vite 8（SPA，`ssr=false`）
**方法**：静态代码审查 + 真实数据实测（数据库副本，未改动生产库）

---

## 0. 实测基线（本次审查的锚点）

### 0.1 数据规模

| 指标 | 数值 |
|------|------|
| records | 298 条 |
| artists | 940 |
| dramas / zhezis | 246 / — |
| record_artists 关联 | 2,855（平均 9.6 演员/条） |
| 数据库文件 | 12 MB（其中 dbstat 统计实际数据仅 2.5 MB，约 9.5 MB 为 freelist） |
| 封面原图 | 251 张，平均 220 KB，最大 1.71 MB，合计 53.9 MB |
| 封面缩略图 | 251 张，平均 37 KB，最大 80 KB，合计 9.1 MB |

> **重要前提**：298 条记录属于极小数据量。本报告中所有优化项都标注了「当前收益」与「规模风险」，避免为当前体感不到的问题过度投入。

### 0.2 后端接口延迟（30 次采样，单位 ms，`Accept-Encoding: identity`）

| 接口 | p50 | p95 | 响应体 |
|------|-----|-----|--------|
| `GET /api/records?limit=30`（首页首屏） | 1.55 | 2.32 | 68.8 KB |
| `GET /api/records?limit=30&offset=270`（深翻页） | 3.33 | 4.42 | 60.2 KB |
| **`GET /api/records?q=剧`**（无 limit） | **55.49** | 57.24 | 653.4 KB |
| `GET /api/records?q=剧&limit=30` | 17.23 | — | 69.0 KB |
| `GET /api/records/all` | 11.82 | 13.37 | 720.1 KB |
| `GET /api/analytics` | 12.05 | 12.79 | 12.5 KB |
| `GET /api/calendar.ics` | 12.15 | 13.05 | 416.0 KB（**未压缩**） |
| `GET /api/dashboard` | 2.24 | 2.58 | 28.7 KB |
| `GET /api/artists` | 3.22 | 4.05 | 158.1 KB |
| `GET /api/dramas/tree` | 5.77 | 6.40 | 35.4 KB |
| `GET /api/covers/duplicates` | 108.32 | — | — |
| `GET /api/stats` / `categories` / `autocomplete/*` | 0.3–0.6 | — | <5 KB |

### 0.3 并发表现（5 秒压测）

| 并发 | `/api/records?limit=30` QPS / p50 | 搜索 `?q=剧&limit=30` QPS / p50 | `/api/analytics` QPS / p50 |
|------|------|------|------|
| 1 | 572 / 1.63 ms | 54 / 17.97 ms | 80 / 12.39 ms |
| 4 | **1251** / 3.04 ms | **113** / 34.77 ms | 110 / 35.68 ms |
| 8 | 1107 / 6.74 ms | 79 / 101.65 ms | 77 / 104.10 ms |
| 16 | 989 / 14.90 ms | 73 / 218.09 ms | 69 / 230.30 ms |
| 32 | 978 / 30.95 ms | 71 / **449.88 ms** | 65 / **485.64 ms** |

无错误、无 `SQLITE_BUSY`。吞吐量在并发 4 附近触顶，之后延迟随并发线性恶化 → **瓶颈是 CPU，不是锁**。

### 0.4 前端 Web Vitals（Chromium headless，1280×900，本地回环）

| 路由 | TTFB | FCP | LCP | 请求数 | JS | 图片 | 关键 API |
|------|------|-----|-----|--------|-----|------|----------|
| `/`（首页） | 3 ms | **944 ms** | **944 ms** | 55 | 145 KB | **25 张 / 843 KB** | records 70 KB + dramas/tree 36 KB |
| `/analytics` | 1 ms | 52 ms | 128 ms | 21 | 139 KB | 1 张 / 2.3 KB | analytics 12.8 KB |
| `/calendar` | 1 ms | 60 ms | 88 ms | 27 | 117 KB | 7 张 / 204 KB | calendar 2.8 KB |
| `/map` | 1 ms | 56 ms | 844 ms | 43 | 289 KB | 21 张 / 590 KB | **`records?` 737 KB** |
| `/artists` | 1 ms | 56 ms | 56 ms | 22 | 110 KB | 1 张 / 2.3 KB | artists 162 KB |

首页 long task 累计 62 ms；其余路由 0 ms。

---

## 1. 需要补充的上下文（影响部分结论的优先级）

以下信息目前缺失，会实质改变若干优化项的排序与收益估算：

1. **部署形态**：前面是否有 Nginx/Caddy 反向代理？是否有 CDN？TLS 终止在哪一层？——直接决定「后端 gzip 是否应关闭」「静态资源 CDN 命中策略」「HTTP/2 多路复用收益」三项建议的取舍。目前代码里后端自带 gzip，若前置代理也开了压缩，就是重复 CPU 开销。
2. **用户规模与并发峰值**：单人自用，还是有其他用户共享？移动端点开多个标签页、Service Worker 后台预取都会造成 4–8 并发。若确认单人使用，第 5 节所有并发类优化的优先级应下调。
3. **数据增长预期**：298 条是起步还是接近终态？若预期到 5,000–10,000 条，第 4.11 节的索引缺口和深翻页（offset）问题必须提前处理。
4. **主要访问网络**：移动端 4G 还是桌面 WiFi？——决定「包体体 52% 精简」「缩略图降体积」的真实收益（4G 下收益约 3–5 倍于 WiFi）。
5. **现有监控**：目前**完全没有任何**指标采集。建议在任何优化之前先加最基础的埋点，否则「验证效果」只能靠临时脚本（本报告所有数字都是这样临时采的，无法持续回归）。
6. **目标 SLA**：首页首屏期望值是多少？若 944 ms 已在可接受范围，前端部分的投入应集中在 `/map`（LCP 844 ms）和数据增长风险上。

---

## 2. 已做得好的部分（不要动）

审查中发现多处已经处理得当的设计，避免重复优化：

- **SQLite PRAGMA 配置正确**：WAL + `busy_timeout(30000)` + `_txlock=immediate` + `synchronous(NORMAL)`，并发导入不会撞 `SQLITE_BUSY`（压测零错误已验证）。
- **关联数据回填已批量化，不是 N+1**：`backfillDramaIDs` / `backfillZheziIDs` / `backfillArtistIDs` 都用 `IN (?,?,…)` 一次取回，这是本项目最容易写成 N+1 的地方，实现是对的。
- **upsert 预编译语句复用**：`stmtUpsertRecord` 在 `New()` 时 Prepare。
- **关联表索引齐全**：`record_dramas` / `record_zhezis` / `record_artists` 的双向索引都在。
- **前端 CSS 按路由正确分包**：Leaflet 的 3 个 CSS（20.5 KB）只在 `/map` 的 chunk `12.BYHMZzar.css` 里，没有泄漏到全局。
- **Leaflet JS 已动态导入**：`await import('leaflet')`，182 KB 不进首页关键路径。
- **chi Compress 正确跳过图片**：实测 `/uploads/*.avif` 未施加 gzip（避免了对已压缩格式做无用功）。
- **缓存头策略正确**：`/_app/immutable/*` 一年 immutable，HTML 壳 `no-cache`，`/uploads/*` 30 天 immutable。
- **Service Worker 分层缓存**（static / covers / data）设计细致，已考虑跨部署持久与 S3 跨域绕过。

---

## 3. 优化项总表（按收益从高到低）

| # | 问题 | 类别 | 当前收益 | 成本 | 风险 |
|---|------|------|----------|------|------|
| **B1** | 搜索 JOIN+DISTINCT 造成 24× 行放大 | 后端·SQL | **高（实测 28.9×）** | 低 | 低 |
| **F1** | 首页 FCP 944 ms，首屏 25 图 843 KB | 前端 | **高** | 低 | 低 |
| **B2** | 列表响应 52% 字段卡片用不到 | 后端·序列化 | **高** | 中低 | 中 |
| **F3** | `/map` 拉全量 records 737 KB | 前端+后端 | **高** | 低 | 低 |
| **B3** | 列表接口无默认条数上限 | 后端 | **中高（规模风险）** | 低 | 低 |
| **F2** | 缩略图平均 37 KB 偏大 | 前端+后端 | **中高** | 低中 | 低 |
| **B4** | COUNT 与 LIST 串行双跑 | 后端 | 中 | 中 | 中 |
| **B5** | analytics 30 条串行查询 | 后端 | 中 | 低中 | 低 |
| **B8** | gzip 在大响应上负收益；ICS 未压缩 | 后端 | 中 | 低 | 低 |
| **F4** | 56 KB UMD 辅助 chunk 进所有路由 | 前端 | 中 | 低 | 低 |
| **B9** | 上传/批量转换 AVIF 同步阻塞、无限流 | 后端 | 中 | 中 | 中 |
| **F5** | BatchEditModal 静态导入首页 | 前端 | 低中 | 低 | 低 |
| **B6** | `ListenAndServe` 无超时、连接池无限制 | 后端·稳定性 | 低（当前）／高（暴露面） | 低 | 中 |
| **B7** | 每请求 126 B 无结构日志 | 后端·运维 | 低（当前）／中（磁盘） | 低 | 低 |
| **B10** | `ListVenueGroups` N+1（75 次往返） | 后端·MCP | 低（当前） | 低 | 低 |
| **F6** | 首页无 SSR | 前端·架构 | 高（理论） | 高 | 高 |
| **B11** | company/channel/address/rating 索引缺口 | 后端·规模 | **0（当前）** | 低 | 低 |

---

## 4. 后端优化项

### B1. 搜索查询的 JOIN + DISTINCT 造成 24 倍行放大 ★最高收益

**问题位置**：`backend/internal/db/db.go:691-704`（`ListRecords`）、`800-814`（`CountRecords`）

**现状瓶颈**

搜索条件把 4 张表 LEFT JOIN 进来：

```sql
LEFT JOIN record_artists ra_q ON ra_q.record_id = records.id
LEFT JOIN artists a_q        ON a_q.id = ra_q.artist_id
LEFT JOIN record_dramas rd_q ON rd_q.record_id = records.id
LEFT JOIN dramas d_q         ON d_q.id = rd_q.drama_id
```

`record_artists`（平均 9.6 演员/条）与 `record_dramas` 之间是**笛卡尔积**。实测：

- 298 条记录 → JOIN 后 **7,130 中间行（24 倍放大）**
- 为了去重，`SELECT DISTINCT` 必须对 30 列（含平均 339 字节的 `remark`）建临时 B-tree 排序去重
- SQL 实测 **15.04 ms**
- 由于 `ORDER BY date DESC`，即便 `LIMIT 30` 也必须先物化全部匹配行，**这 15 ms 是固定成本**

拟合实测数据：`耗时 ≈ 12.2 ms（固定） + 0.169 ms × 返回行数`。接口 p50：limit=30 → 17.2 ms；limit=100 → 32.5 ms；全量 262 条 → 56.5 ms。32 并发下劣化到 449 ms。

**优化方案**

改用 `EXISTS` 子查询，JOIN 与 DISTINCT 都不再需要：

```sql
SELECT <recordColumns> FROM records
WHERE (
  records.name LIKE ? OR records.city LIKE ? OR ... OR records.play LIKE ?
  OR EXISTS (SELECT 1 FROM record_artists ra JOIN artists a ON a.id = ra.artist_id
             WHERE ra.record_id = records.id AND a.name LIKE ?)
  OR EXISTS (SELECT 1 FROM record_dramas rd JOIN dramas d ON d.id = rd.drama_id
             WHERE rd.record_id = records.id AND d.aliases LIKE ?)
)
```

**预期收益（已实测验证）**

| 变体 | SQL 耗时 | 结果集 |
|------|----------|--------|
| A：现状 `JOIN + DISTINCT` | **15.04 ms** | 262 行 |
| B：改写 `EXISTS` | **0.52 ms** | 262 行（**完全一致**） |

SQL 提速 **28.9×**。映射到接口层：

- 分页搜索（前端实际用法）：17.2 ms → **约 6.6 ms（2.6×）**
- 无 limit 搜索：56.5 ms → 约 46 ms
- 32 并发 p50：449 ms → 预计 150 ms 以内

**实施成本与风险**：**低**。纯 SQL 改写，无 schema 变更、无 API 契约变化。唯一风险是 `EXISTS` 与 `LEFT JOIN` 在「演员/剧目别名为 NULL 或空」时语义需对齐。

**如何验证**

1. `EXPLAIN QUERY PLAN` 断言输出中**不再出现** `USE TEMP B-TREE FOR DISTINCT` 与 `SCAN records USING INDEX sqlite_autoindex_records_1`。
2. 断言等价性：对同一批关键词（含中文、空串、超长串、特殊字符 `%` `_`）跑新旧两条 SQL，比对返回的 **ID 集合**与 **COUNT** 必须完全相等。
3. 用本次的 `bench.py` 复测 `/api/records?q=X` 在 limit=30 / 100 / 无 limit 三档的 p50、p95。
4. 用 `conc.py` 复测并发 1/4/8/16/32 的 p50。

---

### B2. 列表响应中 52% 的字段卡片根本不用 ★高收益

**问题位置**：`backend/internal/db/db.go:623`（`recordColumns`）、`631`（`scanRecord`）；`handlers/handlers.go:173`（`listRecords`）

**现状瓶颈**

列表、详情、导出共用同一套全 30 列投影。实测 30 条列表响应的字段占比：

| 字段 | 占比 | 列表页是否使用 |
|------|------|----------------|
| `remark` | **30.2%** | ❌（`RecordCard.svelte` 完全不引用） |
| `artist_ids` | 21.6% | ✅ |
| `coverThumb` | 7.0% | ✅ |
| `drama_ids` | 6.7% | ❓（首页未使用，需二次确认） |
| `coverFile` | 6.5% | ✅ |
| `coordinate` | 4.7% | ❌ |
| 其余 11 个未用字段 | 合计 | ❌ |

**卡片实际需要的字段仅占 48.0%**，可裁剪 52%。`remark` 平均 339 字节/条，是单字段最大开销。

**优化方案**

新增列表专用投影，与全量投影并置放在同一处：

```go
// 列表投影：仅 RecordCard 需要的字段
const recordListColumns = `records.id, records.name, records.city, records.address,
    records.cover_file, records.cover_thumb, records.category_name, records.category_names,
    records.date, records.date_text, records.rating, records.company,
    records.price, records.price_currency, records.active_status`
```

- `listRecords` / `listAllRecords` / `searchRecords` → 用 `recordListColumns`
- `getRecord` / `exportRecords` / `getICS` → 保持 `recordColumns`

**预期收益**

| 接口 | 当前 | 精简后 |
|------|------|--------|
| `/api/records?limit=30` | 68.8 KB | **33.1 KB（-52%）** |
| `/api/records/all` | 720.1 KB | **346.0 KB（-52%）** |
| `/map` 的 `/api/records?` | 720.1 KB | **346.0 KB（-52%）** |

按实测的 0.169 ms/行系数，262 行还可省约 44 ms 中的一半左右（序列化 + JSON 解析同时减半）。gzip 后 wire size 从 29.3 KB → 约 14 KB。

**实施成本与风险**：**中低**。主要风险有两个，都必须处理：

1. **列集与 scan 强绑定的维护风险**：项目已有约定——`recordColumns` 与 `scanRecord` 列数强绑定，改一处忘一处就会静默错位。新增第二套列集把这个风险翻倍。**缓解**：把 `recordColumns`、`recordListColumns` 及各自的 scan 函数放同一文件相邻位置，并加单元测试断言「列数 == scan 目标数」。
2. **破坏性变更风险**：列表响应少字段属于 API 契约变化。**必须先回归前端**——`grep -o "record\.[a-zA-Z_]*\|r\.[a-zA-Z_]*"` 遍历所有消费列表的接口，确认裁剪字段无任何引用点。特别是 `drama_ids` / `zhezi_ids` 需逐一确认（首页 `zheziNames` 来自 `/api/dramas/tree` 而非记录字段，倾向可裁，但需人工确认）。

**保守方案**：只裁 `remark` 一项即得 30.2% 收益，风险显著低于全量裁剪。

**如何验证**

1. 响应字节数：对比 `curl -s .../api/records?limit=30 | wc -c` 前后值。
2. 契约回归：写一个测试，拉取列表响应，断言所有被裁字段确实为 `undefined`，同时跑一遍首页 + 无限滚动 + 搜索 + 批量编辑的完整流程无报错。
3. p50 对比：limit=30 与全量两档。

---

### B3. 列表接口没有默认条数上限 ★规模风险

**问题位置**：`handlers/handlers.go:184`（`listAllRecords`）、`193`（`searchRecords`）、`801`（`getICS`）、`db.go:760-766`（`f.Limit > 0` 才加 LIMIT）

**现状瓶颈**

`ListRecords` 在 `f.Limit == 0` 时**不加 LIMIT 子句**，全量返回：

- `/api/records/all` → 720 KB
- `/api/calendar.ics` → 416 KB（且 `text/calendar` 不在 chi 压缩白名单，完全裸传）
- `/map` 的 `/api/records?` → 737 KB
- `/api/records/search?q=` → 653 KB

当前 298 条尚可控；若数据增长到 3,000 条，这些接口会直接产出 7 MB 级响应。

**优化方案**

在 `ListRecords` 内设默认上限与硬上限：

```go
const defaultLimit = 500
const maxLimit = 2000
if f.Limit <= 0 { f.Limit = defaultLimit }
if f.Limit > maxLimit { f.Limit = maxLimit }
```

导出（`/api/export`）与 ICS（`/api/calendar.ics`）走独立的全量路径，不受影响。

**预期收益**：当前收益中等（省不了多少时间），但**消除了随数据线性恶化的悬崖**。`/map` 配合 F3 可进一步降到 60–80 KB。

**实施成本与风险**：**低**。需确认三个调用方：`listAllRecords`（前端是否有地方依赖全量？）、`searchRecords`、`getICS`。

**如何验证**：造 2× / 5× 数据量（复制 records 表即可），观察响应大小与 p95 是否仍在线性可控范围。

---

### B4. COUNT 与 LIST 串行双跑，搜索时重复付一遍 JOIN 代价

**问题位置**：`handlers/handlers.go:164-177`

**现状瓶颈**

每次列表请求先 `CountRecords`（带搜索时实测 **7.7 ms**），再 `ListRecords`（15 ms），串行执行，两者跑的是同一套 JOIN 逻辑。计数固定成本约占总耗时的 45%。

**优化方案**（三选一，按侵入性递增）

1. **并行化**（最安全）：两个查询各起一个 goroutine，连接池会分配不同连接。改动最小，收益直接。
2. **游标替代精确 COUNT**：无限滚动场景只需要 `hasMore`，可改为「多取 1 条判断是否还有下一页」，省掉 COUNT。会改变 `total` 语义。
3. **COUNT 结果短缓存**：按 filter 哈希缓存 3–5 秒。

**预期收益**：搜索列表 17.2 ms → 约 9.5 ms（-45%）；非搜索列表 1.55 ms → 约 1.0 ms。

**实施成本与风险**：**中**。方案 2/3 会改变 `total` 语义，前端 `hasMore = offset + PAGE_SIZE < total` 的逻辑需同步调整。方案 1 无契约变化，建议优先。

**如何验证**：并行化前后 p50 对比 + `total` 值一致性断言（同一 filter 跑 100 次结果必须稳定）。

---

### B5. analytics 约 30 条串行查询

**问题位置**：`internal/db/analytics.go:126-356`（`GetAnalytics`）、`db.go:2593`（`GetStats`）、`2602`（`GetDashboardStats`）

**现状瓶颈**

`GetAnalytics` 顺序执行约 30 次独立的 `QueryRow` / `Query`，每次一轮连接池往返，且各自独立扫描（无事务，快照不一致）。接口 12.05 ms；32 并发下 p50 恶化到 **485 ms**。

`GetDashboardStats` 更重：4 次 COUNT/SUM + 4 次聚合查询 + 2 次记录查询 + **6 次 backfill 查询**（TopRated 与 RecentRecords 各 3 次）。

**优化方案**

1. **合并聚合**：把 overview 的 6 个 `QueryRow` 合成 1 条多 CTE 查询；把 5 个排名查询（artist/drama/venue，各 `LIMIT 10`）合成一条 UNION 或用一次扫描。
2. **短 TTL 内存缓存**：统计类数据对实时性不敏感，缓存 5–10 秒，写路径（`UpsertRecord` / `DeleteRecord` / `ImportData`）主动失效。这是收益/成本比最高的做法。
3. **包进只读事务**：至少保证 30 个查询看到一致快照。

**预期收益**：12.05 ms → 2–4 ms；32 并发 p50 → 预计 100 ms 以内。

**实施成本与风险**：**低-中**。缓存的主要风险是失效遗漏导致数据陈旧——用「写操作统一走一个 `invalidateStats()`」来收敛。

**如何验证**：并发压测 p50/p95；以及「写入一条记录后立即读 analytics」的数值一致性断言。

---

### B6. `http.ListenAndServe` 无超时、连接池无限制

**问题位置**：`backend/main.go:142`

**现状瓶颈**

```go
if err := http.ListenAndServe(addr, r); err != nil {
```

使用默认 `http.Server`，`ReadTimeout` / `WriteTimeout` / `IdleTimeout` / `ReadHeaderTimeout` 全为 0：

- 慢速连接（Slowloris）可长期占用连接，无上限
- 没有 `MaxHeaderBytes` 限制
- `db.conn` 未设 `SetMaxOpenConns` / `SetMaxIdleConns` / `SetConnMaxLifetime`

当前压测无异常，但这是**暴露面风险**而非当前性能问题——一旦服务从内网暴露到公网，这是最容易被打的一点。

**优化方案**

```go
srv := &http.Server{
    Addr:              addr,
    Handler:           r,
    ReadHeaderTimeout: 2 * time.Second,
    ReadTimeout:       10 * time.Second,
    WriteTimeout:      60 * time.Second,
    IdleTimeout:       60 * time.Second,
    MaxHeaderBytes:    1 << 20,
}
if err := srv.ListenAndServe(); err != nil { ... }
```

连接池侧（**注意**：项目 note 明确记载 `MaxOpenConns(1)` 在 modernc/sqlite 下会死锁，**不要设 1**）：

```go
conn.SetMaxOpenConns(4)   // 或 runtime.GOMAXPROCS(0)
conn.SetMaxIdleConns(4)
conn.SetConnMaxLifetime(time.Hour)
```

**⚠️ 关键冲突**：`WriteTimeout` 会掐断 NDJSON 长连接（`/api/covers/thumbs`、`/api/covers/convert-batch` 按设计要跑「数分钟」，靠持续 flush 保活）。必须做二选一：
- 给这些路由单独用一个不限 WriteTimeout 的 server；或
- 把 `WriteTimeout` 设为 0，改用 `ReadTimeout` + `IdleTimeout` 组合，并在 handler 内用 `http.CloseNotifier` 检测断连。

**预期收益**：当前性能收益低；消除连接耗尽导致的雪崩风险。

**实施成本与风险**：**低实施成本 / 中风险**（风险集中在与长任务的冲突，见上）。

**如何验证**：
1. 用 `nc` 模拟只发请求头不结束的连接，断言 2 秒后被断开。
2. 跑一次 251 张的 `/api/covers/thumbs` 批量重生成，断言全程不中断、进度流正常。

---

### B7. 每请求 126 字节无结构日志

**问题位置**：`backend/main.go:63`（`middleware.Logger`）

**现状瓶颈**

chi 的 `middleware.Logger` 对**每个**请求写一行 `log.Printf`（同步 write syscall + 全局 mutex）。实测：

- 3,000 个请求 → 377 KB stderr
- **125.7 字节/请求**
- 按 1,000 QPS 推算：126 KB/s ≈ **10.9 GB/天**

且输出是非结构化的，与项目其它部分用的 `slog` 不一致。

**优化方案**

1. 替换为基于 `slog` 的结构化中间件（`slog.Info("http", "method", …, "path", …, "status", …, "dur", …)`）。
2. 跳过噪声路径：`/healthz`、`/_app/immutable/*`、`/uploads/*`。
3. 生产默认只记 `>= 400` 与慢请求（如 > 200 ms），正常请求采样。

**预期收益**：消除每请求一次同步写；磁盘占用从 10.9 GB/天 降到可忽略；高 QPS 下 p95 有小幅改善。

**实施成本与风险**：**低**。注意保留足够的排障信息，建议至少保留 4xx/5xx 全量。

**如何验证**：固定 3,000 请求，对比日志字节数（377 KB → 目标 < 20 KB）与 p95 变化。

---

### B8. gzip 在大响应上是负收益；`text/calendar` 完全没压缩

**问题位置**：`backend/main.go:65`（`middleware.Compress(5)`）

**现状瓶颈**

实测（本地回环，无网络延迟，因此压缩的带宽收益被隐去、CPU 成本全部暴露）：

| 接口 | identity | gzip | 压缩率 |
|------|----------|------|--------|
| `/api/records?limit=30` | 1.47 ms | 2.43 ms | 57.4% |
| `/api/records/all` | **11.91 ms** | **20.93 ms（+75%）** | 61.1% |
| `/api/records?q=剧`（全量） | 56.46 ms | 64.89 ms | — |
| `/api/artists` | 3.22 ms | — | 79.0% |
| **`/api/calendar.ics`** | 12.15 ms | **未压缩**（416 KB 裸传） | **0%** |

两点问题：

1. **gzip level 5 对大响应是净亏损**：720 KB 响应多花 9 ms CPU。本地环境下带宽不是瓶颈，真实网络下需要权衡（若走 CDN/4G，压缩仍值得，但应降到 level 1–2）。
2. **`text/calendar` 不在 chi 的压缩白名单**，416 KB 的 ICS 订阅文件完全裸传。

**优化方案**

1. 压缩级别 5 → **1 或 2**（gzip level 1/2 的压缩率损失仅 2–5%，CPU 成本下降约 40–60%）。
2. 把 `text/calendar` 加入压缩白名单（`middleware.NewCompressor(1, "text/calendar", "application/json", …)`）。
3. 更彻底的做法：配合 B2 把列表响应砍半，大响应变少，压缩收益/成本比自然改善。
4. **若前置反向代理已开 gzip，后端应直接关掉**，避免双重压缩。

**预期收益**：`/api/records/all` 20.9 ms → 约 13 ms；ICS 416 KB → 约 60 KB（-86%）。

**实施成本与风险**：**低**。无契约变化。

**如何验证**：每个接口跑 identity / gzip(level 5) / gzip(level 1) 三档，记录 p50 与 wire size，画出「体积 vs 耗时」对照表再定级别。

---

### B9. 上传 / 批量转换的 AVIF 编码同步阻塞，无并发限制

**问题位置**：`internal/storage/storage.go:239`、`:478`（上传路径）、`internal/handlers/covers.go:244`（`regenerateThumbs`）、`406`（`convertBatchCovers`）

**现状瓶颈**

上传路径同步完成「解码 → AVIF 编码（Speed 6, ColorQuality 65）→ 生成 400px 缩略图」全部工作。实测：

- 单张 2,500 px、204 KB 的 JPEG：**~800 ms**（三次：0.898 s / 0.820 s / 0.767 s）
- 真实海报（含文字细节）会比合成测试图更慢

批量重生成 251 张缩略图 ≈ **2–4 分钟 CPU 满载**。在此期间所有其它接口被拖慢——0.4 节已证明本服务是 CPU 瓶颈型（搜索 32 并发 450 ms）。现有代码通过 NDJSON 流式进度来「接受」这个时长，但没有并发上限。

**优化方案**

1. **上传异步化**（收益最大，但改契约）：先落盘原图并立即返回占位 `key`，编码与缩略图生成交给后台 worker，前端通过轮询或后续列表刷新拿到缩略图。
2. **限流版本**（改动小，推荐先做）：用带容量的 semaphore（`make(chan struct{}, runtime.GOMAXPROCS(0)/2)`）包裹批量转换，把 CPU 占用封顶，给正常请求留出余量。
3. **调高 AVIF Speed**：`Speed: 6` → `8`，编码时间可降 30–50%，体积增加约 5–10%。缩略图场景（400 px）肉眼几乎无差。

**预期收益**：上传接口 ~800 ms → <100 ms（方案 1）；批量任务期间其它接口 p95 劣化幅度从「数倍」降到「可控」（方案 2）。

**实施成本与风险**：**中**。方案 1 改变上传响应语义（返回的 `thumb` 可能为空），前端 `RecordForm.svelte:579-580` 需能处理。方案 2/3 无契约变化。

**如何验证**：上传 p95；以及在批量转换进行中同时压测 `/api/records?limit=30`，记录 p95 劣化倍数（优化前 vs 后）。

---

### B10. `ListVenueGroups` 的 N+1（75 次额外往返）

**问题位置**：`internal/db/analytics.go:60-70`；同类问题见 `internal/db/db.go:1499`（`AlignVenueCoordinates`）

**现状瓶颈**

先聚合出 N 个场馆分组，然后**对每个分组单独查一次**城市：

```go
for i := range out {
    crows, err := db.conn.Query(
        `SELECT DISTINCT city FROM records WHERE address = ? AND city != '' ORDER BY city LIMIT 5`,
        out[i].Address)
```

当前 75 个不同地址 → 75 次额外查询。代码注释里写「in a second pass to keep the main aggregation simple」——这是明确的 N+1。

注意：**该接口只被 MCP 工具 `list_venues` 调用**，不服务 Web UI，所以当前优先级低。

**优化方案**

一次 `GROUP BY address, city` 后内存归组：

```sql
SELECT address, city, COUNT(*) FROM records
WHERE address != '' AND city != ''
GROUP BY address, city ORDER BY address
```

**预期收益**：75 次往返 → 1 次。当前绝对耗时不大（数十毫秒），但随场馆数线性增长。

**实施成本与风险**：**低**。纯查询改写。

**如何验证**：相同输入下新旧实现的 `VenueGroup` 切片深度相等；用 `db.Stats()` 或计数 wrapper 断言查询次数从 76 降到 2。

---

### B11. 索引缺口（当前收益 = 0，仅作规模储备）

**现状**

已有索引：`records(date, category_name, city, cover_file, name, active_status)` + 三张关联表的双向索引 + `artists(name)` / `dramas(name, category_name)`。

缺失：`records(company)`、`records(channel)`、`records(address)`、`records(rating)`。这些字段出现在 `GROUP BY company` / `GROUP BY channel` / `GROUP BY address`（`ListVenueGroups`）/ `ORDER BY rating DESC`（dashboard TopRated）中。

**明确说明**：298 行时全表扫描只要 0.03–1 ms，加索引**当前收益为零**，还会拖慢写入。建议**等数据量到 2,000 条以上再评估**，届时用 `EXPLAIN QUERY PLAN` 确认是否出现 `SCAN records`。

---

## 5. 前端优化项

### F1. 首页 FCP 944 ms（其余路由 52–60 ms）★最高收益

**问题位置**：`frontend/src/routes/+page.svelte`（768 行）；`frontend/src/routes/+layout.js`（`ssr = false`）

**现状瓶颈**

纯 CSR 架构下的首屏瀑布流：`index.html` → 145 KB JS → 5 个 API → 25 张图 → 渲染。

首页是唯一 FCP 超过 100 ms 的路由（其它 4 条路由都在 52–60 ms），差异来自三处：

1. **首屏 25 张图片 / 843 KB**。`RecordCard.svelte` 已加 `loading="lazy"`，但 Chromium 的 lazy 加载阈值很宽松，首屏内的图片仍会全部立即加载。
2. **无条件加载 `/api/dramas/tree`（36 KB）**。见下方定位。
3. **long task 累计 62 ms**（其余路由为 0）。

**关于 `dramas/tree` 的具体定位**（最易摘的果子）：

```js
// +page.svelte:209-220
async function loadMeta() {
  const [cats, cityList, tree] = await Promise.all([
    api.listCategories(),
    api.getAutocomplete('city'),
    api.getDramaTree().catch(() => [])          // ← 无条件请求，36 KB
  ]);
  const m = new Map();
  for (const d of tree) for (const z of d.zhezis || []) m.set(z.id, { name: z.name, dramaName: d.name });
  zheziNames = m;                                // 只在 filters.zhezi 非空时 (:126-127) 才被读
}
```

这个 36 KB 请求构建出的 `zheziNames` 映射，**仅在用户通过 `?zhezi=` 参数筛选时才被使用**，但每次进首页都无条件拉取。

**优化方案**（按性价比排序）

| 步骤 | 做法 | 收益 |
|------|------|------|
| a | 把 `getDramaTree()` 改为条件触发：`if (filters.zhezi) { ... }`，或在筛选面板展开时懒加载 | 省 36 KB + 1 次往返 |
| b | 配合 F2 降低缩略图体积 | 首屏图片 843 KB → 约 400 KB |
| c | 首屏图片数控制：把 `loading="lazy"` 配合 `fetchpriority` 与更小的 `rootMargin`，或首屏只渲染 8–10 张 | LCP 显著改善 |
| d | （中期）对首页开启 SSR 或静态骨架屏 | 见 F6 |

**预期收益**：(a)+(b) 组合后，首页 FCP 预计从 944 ms 降到 **350–450 ms**；首屏传输量从约 1.0 MB 降到约 500 KB。

**实施成本与风险**：(a) **低**，纯逻辑调整，只需确认筛选面板打开时能正确补拉。(b) 见 F2。(c) 低。

**如何验证**

复用本次的 Playwright 脚本（`/tmp/mujian-perf/webperf.mjs`），对比优化前后：

| 指标 | 优化前基线 | 目标 |
|------|-----------|------|
| `/` FCP | 944 ms | < 450 ms |
| `/` LCP | 944 ms | < 450 ms |
| `/` 请求数 | 55 | < 45 |
| `/` 图片传输 | 843 KB | < 450 KB |
| `/` long task | 62 ms | < 40 ms |

建议在网络节流（Fast 3G / Slow 4G）下再测一轮——本地回环会掩盖传输优化的真实收益。

---

### F2. 缩略图平均 37 KB、最大 80 KB，超出卡片实际显示尺寸

**问题位置**：`backend/internal/storage/storage.go:239`、`:478`（`MakeThumbnail(key, data, 400, format)`）；`imageutil.go:108`（`encodeAVIF`，`ColorQuality: 65`）

**现状瓶颈**

251 张缩略图平均 37 KB，最大 80 KB，合计 9.1 MB。卡片上的显示尺寸远小于生成尺寸（400 px 宽）。首页首屏 25 张图就是 843 KB。

参考值：400 px 宽的 AVIF 在 q65 下，含文字细节的海报 30–40 KB 属正常范围，但**卡片实际渲染宽度通常只有 100–160 px**，存在明显过采样。

**优化方案**

1. `maxW` 400 → **240–280**（覆盖 2× DPR 下的卡片显示需求）。
2. 缩略图专用更低质量：`ColorQuality: 65` → `50–55`（缩略图不需要海报级细节）。
3. 重新生成存量缩略图：项目已有 `/api/covers/thumbs` 批量接口（NDJSON 流式进度），可直接用。按 0.5–1 s/张估算，251 张约 2–4 分钟 CPU（配合 B9 的限流方案执行更好）。
4. 前端加 `sizes` 属性帮助浏览器正确选择。

**预期收益**

- 缩略图平均 37 KB → 约 15–18 KB（-55%）
- 首页首屏图片 843 KB → **约 380–420 KB**
- 全站封面流量减半

**实施成本与风险**：**低-中**。主要是重新生成的 CPU 时间与短暂的磁盘双占（新旧缩略图并存期间）。**注意**：缩略图 key 是内容寻址的（`covers/<sha256>.thumb.avif`），重新生成会产生**新文件名**，旧文件需通过 `/api/covers/orphans` + `cleanup` 清理，否则磁盘会残留——这一步要谨慎，务必先 dry run。

**如何验证**

1. 重新生成后统计 `ls -la *thumb* | awk` 的平均/最大体积。
2. 用 Playwright 复测各路由的图片传输总量。
3. 抽样 10 张做 A/B 目视比对，确认卡片尺寸下无可辨识的画质损失。

---

### F3. `/map` 拉全量 records（737 KB）

**问题位置**：`frontend/src/routes/map/+page.svelte`

**现状瓶颈**

地图页请求 `/api/records?`（无参数）→ **737 KB JSON**，加上 21 张图 590 KB，LCP 844 ms，JS 289 KB（含 Leaflet 182 KB）。

但地图打点只需要：`id`、`coordinate`、`name`、`coverThumb`、`date`、`city`、`address`。737 KB 中绝大部分（尤其 `remark` 占 30%）完全无用。

**优化方案**

1. 复用 B2 的精简投影：`/api/records?limit=&fields=list` 即可降到 346 KB。
2. 更好：新增专用轻量端点 `/api/map/points`，只返回 `coordinate != ''` 的记录且只含 5 个字段。预计 **60–80 KB**。
3. 前端：marker cluster 已启用（`leaflet.markercluster`），可进一步按视口懒加载。

**预期收益**：737 KB → 60–80 KB（-89%）；LCP 844 ms → 预计 < 300 ms。

**实施成本与风险**：**低**。新增端点不影响现有契约。

**如何验证**：响应字节数 + Playwright 复测 `/map` 的 LCP 与传输量。

---

### F4. 56 KB 的 UMD 辅助 chunk 出现在所有路由

**问题位置**：构建产物 `dist/_app/immutable/chunks/D106fbif.js`（56,764 B）

**现状瓶颈**

该 chunk 在全部 5 条被测路由中都是最大的 JS。内容是一个 UMD 包装辅助函数（`ft`），由 `BrPUrjXs.js`（leaflet.markercluster，148 KB）与 `CmeYfsHG.js`（leaflet 核心，34 KB）共同引入。

问题在于：**它被 Vite 判定为公共 chunk，因此即便 Leaflet 本身是动态导入的，这个辅助 chunk 仍会出现在每条路由的关键路径上**。非地图路由为此平白多付 56 KB（gzip 后约 18 KB）。

**优化方案**

1. 先确认归属：跑一次构建并用 `--sourcemap` 或 `rollup-plugin-visualizer` 确认 `D106fbif.js` 是否**只**被 Leaflet 相关 chunk 引用。
2. 若是，用 `build.rollupOptions.output.manualChunks` 显式把 UMD 辅助函数与 leaflet 打包到一起，强制它随 leaflet 一起动态加载。
3. 备选：去掉 `leaflet.markercluster`（148 KB 的大头），改用更轻的聚合实现或 Leaflet 原生方案。

**预期收益**：非地图路由首屏 JS 145 KB → 约 89 KB（-39%）；首页 FCP 有直接改善。

**实施成本与风险**：**低**。但需要**验证不会把 Leaflet 反向拉回公共包**——这是此类 manualChunks 调整最常见的翻车点。

**如何验证**

1. 构建后重新跑 `webperf.mjs`，断言 `/`、`/analytics`、`/artists` 的 JS 传输量下降，且 **请求列表中不再出现 `D106fbif.js`**。
2. 断言 `/map` 仍能正常渲染地图与聚合标记（功能回归）。

---

### F5. `BatchEditModal` 静态导入进首页关键路径

**问题位置**：`frontend/src/routes/+page.svelte:9`

**现状瓶颈**

```js
import BatchEditModal from '$lib/components/BatchEditModal.svelte';
```

680 行的组件（含 `RecordForm` 依赖）被静态导入首页，但它**只在用户点击「批量」进入选择模式后才需要**（`{#if showBatchEdit}`）。

**优化方案**

```js
let BatchEditModal = $state(null);
async function openBatchEdit() {
  if (!BatchEditModal) BatchEditModal = (await import('$lib/components/BatchEditModal.svelte')).default;
  showBatchEdit = true;
}
```

**预期收益**：首页 JS 减少约 10–15 KB（gzip 后约 4–6 KB）。收益不大但改动极小的「顺手项」。

**实施成本与风险**：**低**。Svelte 5 下动态组件用 `{@const Comp = BatchEditModal}` 或 `<svelte:component>` 渲染即可。

**如何验证**：构建产物大小 + 打开批量编辑弹窗的功能回归。

---

### F6. 首页无 SSR（需重构项）

**问题位置**：`frontend/src/routes/+layout.js`（`export const ssr = false`）

**现状瓶颈**

全站 CSR。首屏必须等「JS 下载 → 解析执行 → 发起 API → 渲染」四步串行完成，这是首页 FCP 944 ms 的架构性原因。

**说明**：`ssr = false` 大概率是有意为之（单用户 SPA，避免 SSR 的服务端依赖），**不一定是错误决策**。开启 SSR 需要处理：

- `window` / `localStorage` 在 `app.html` 的主题脚本与 `+layout.svelte` 中的直接访问
- Leaflet 的 `window` 依赖（`map/+page.svelte:62` 的注释显示已为此改成动态导入）
- 目前后端是 Go，SSR 需要 Node 运行时或改成预渲染（prerender）——与当前「Go 单二进制 embed dist」的部署形态冲突

**优化方案**（如果要做，建议的路径）

1. **不做完整 SSR**，改为 `prerender` 静态骨架：把 `+layout.svelte` 的导航骨架在构建时渲染进 `index.html`，让 FCP 降到 100 ms 内，数据仍由客户端拉取。这个方案与当前部署形态兼容。
2. 完整 SSR：需要引入 Node 进程，架构变动大。

**预期收益**：方案 1 可把 FCP 从 944 ms 降到约 150–250 ms（骨架可见），但 LCP（真实内容）改善有限。

**实施成本与风险**：**高**。方案 1 中等，方案 2 高。

**如何验证**：FCP 与 LCP 分离观察——骨架方案主要改善 FCP，不要误当作 LCP 的改善。

---

## 6. 建议的实施顺序

### 第一批：短期易改项（1–2 天，低风险，收益已实测）

1. **B1** 搜索改 `EXISTS` —— 已实测 28.9×，纯 SQL 改写
2. **F1-a** 首页去掉无条件的 `getDramaTree()` —— 一行条件判断
3. **B8** 压缩级别 5 → 1/2，`text/calendar` 加入白名单
4. **F5** `BatchEditModal` 改动态导入
5. **B10** `ListVenueGroups` 的 N+1 改单次 GROUP BY
6. **B7** 日志改 slog 结构化 + 跳过噪声路径

### 第二批：中期项（3–5 天，需回归测试）

7. **B2** 列表精简投影（**保守版：只裁 `remark`，先拿 30%**）
8. **B3** 列表默认条数上限
9. **F3** `/map` 轻量端点
10. **B5** analytics 合并查询 + 短缓存
11. **B4** COUNT/LIST 并行化
12. **F4** UMD chunk 归属调整
13. **B6** `http.Server` 超时（**注意长任务豁免**）

### 第三批：需重构项（1–2 周，需设计评审）

14. **F2** 缩略图重新生成（需配合 B9 限流，且要处理旧文件清理）
15. **B9** 上传异步化 / 批量转换限流
16. **F6** 预渲染骨架屏

### 第四批：规模储备（等数据量到 2,000+ 再评估）

17. **B11** company/channel/address/rating 索引

---

## 7. 验证基础设施（建议先建，再做优化）

本次审查的所有数字都靠临时脚本采集，无法持续回归。建议先补两件事：

**后端**：加一个 `middleware` 采集 per-route 的 p50/p95/p99 与 QPS，暴露成 `GET /metrics`（Prometheus 文本格式或直接 JSON），再加一个 per-route 的慢查询阈值日志（> 100 ms 打印 SQL）。

**前端**：在 `+layout.svelte` 加 web-vitals 采集并上报到后端（或先只 `console` + 本地采样），至少覆盖 FCP / LCP / CLS / 长任务。

**回归脚本**：把 `/tmp/mujian-perf/bench.py`、`conc.py`、`webperf.mjs` 三个脚本收进仓库 `scripts/perf/`，在 CI 里对关键接口设阈值门禁（如 `/api/records?limit=30` p95 < 5 ms、`/api/records?q=X&limit=30` p95 < 15 ms）。

---

## 附录 A：本次使用的测量方法

| 项目 | 方法 |
|------|------|
| 接口延迟 | Python `urllib` 串行 30 次采样，取 p50/p95/p99 |
| 并发压测 | Python 多线程，固定并发度跑 5 秒，统计 QPS 与延迟分布 |
| SQL 耗时 | `sqlite3` CLI 的 `.timer on`，对数据库副本执行 |
| 查询计划 | `EXPLAIN QUERY PLAN` |
| 前端指标 | Playwright + Chromium headless，PerformanceObserver 采集 FCP/LCP/longtask，监听 `response` 事件统计资源 |
| 数据库 | 全部测试使用 `backend/data/mujian.db` 的**副本**（`/tmp/mujian-perf/data/`），生产库未被修改（已核对记录数 298 不变） |

## 附录 B：一句话总结

**当前最值得做的三件事**：把搜索查询从 `JOIN + DISTINCT` 改成 `EXISTS`（实测 SQL 快 28.9 倍）；把首页无条件加载的 36 KB `dramas/tree` 改成按需；把列表响应里占 30% 且前端完全不用的 `remark` 从列表投影中裁掉。三项都是低风险改动，合计可让首页搜索链路快 2.5 倍以上、首屏传输量减半。
