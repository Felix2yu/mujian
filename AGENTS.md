# 幕间（MuJian）项目

现场演出记录管理应用：Go + SQLite 后端，SvelteKit 前端。

## MCP 服务（mujian）

本项目通过 `opencode.json` 注册了本地 MCP 服务 `mujian`（`backend -mcp` 子命令，stdin/stdout），直接读写 `backend/data/mujian.db`。涉及演出数据查询、批量修改、分析时优先使用这些 MCP 工具，而不是直接用 sqlite 命令行改库。

### 工具速查

| 类别 | 工具 | 用途 |
|------|------|------|
| 查询 | `search_records` | 按关键词/演员/剧目/折子/城市/日期筛选演出 |
| 查询 | `get_record` / `get_artist_detail` / `get_drama_detail` | 单条详情 |
| 查询 | `list_artists` / `list_dramas` | 实体清单 |
| 查询 | `list_venues` | 场馆按地址分组统计（次数/城市/坐标） |
| 查询 | `value_counts` | company/city/channel/category_name 取值频次 |
| 分析 | `get_stats` | 总览统计 |
| 批量 | `batch_update_company_by_artist` | 统一某演员所有演出的剧团名（支持 dry_run） |
| 批量 | `batch_merge_venues` | 合并同一场馆的不同写法（支持 dry_run、坐标同步） |
| 批量 | `batch_update_records` | 按 ID 通用批量更新（标量赋值 + 数组 set/append/remove） |
| 折子 | `batch_create_zhezis` | 给剧目批量写入折子（自动跳过重名） |
| 折子 | `update_zhezi` / `delete_zhezi` | 维护折子 |

### 典型工作流

1. **按演员统一剧团**：`search_records(artist_name=…)` 或 `batch_update_company_by_artist(dry_run=true)` 预览 → 确认后 `dry_run=false` 执行。
2. **合并近似场馆名**（如「xx剧院」与「xx剧院（某某店）」）：`list_venues(query=xx)` 找出候选 → 与用户确认对应关系 → `batch_merge_venues(source, target, sync_coordinates=true)`。拿不准时先 dry_run。
3. **从互联网补充剧目常演折子**：先 `get_drama_detail(name=剧目名)` 看已有折子 → 用 websearch/webfetch 查证该剧目常演折子（维基百科、剧团官网、戏迷资料等，注意甄别可靠性） → `batch_create_zhezis(drama_id=…, names=[…])` 一次写入，重名自动跳过 → 之后用户在演出记录里即可选用这些折子。

### 注意事项

- 所有批量修改类工具都带 `dry_run` 参数（默认 false）；数据变更前先用 dry_run 展示影响范围，经用户确认再执行。
- 场馆没有独立实体表，以 `records.address` 为隐式标识；剧团是 `records.company` 文本字段。
- 演员、剧目、折子是一等实体（artists/dramas/zhezis 表 + 关联表）。
- 数据库为 SQLite（WAL），MCP 进程与 HTTP 服务可同时运行。

## 开发

- 后端：`cd backend && go build .`；测试 `go test ./...`
- 一键开发环境：`./dev.sh`
