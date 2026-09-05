# 幕间（MuJian）项目

现场演出记录管理应用：Go + SQLite 后端，SvelteKit 前端。

## MCP 服务（mujian）

本项目通过 `opencode.json` 注册了 remote MCP 服务 `mujian`，指向后端 HTTP 服务的 `/mcp` 端点（Streamable HTTP，随服务启动，无单独进程），直接读写 `backend/data/mujian.db`。涉及演出数据查询、批量修改、分析时优先使用这些 MCP 工具，而不是直接用 sqlite 命令行改库。

### 工具速查

| 类别 | 工具 | 用途 |
|------|------|------|
| 查询 | `search_records` | 按关键词/演员/剧目/折子/城市/日期/渠道/剧团/评分票价/状态筛选演出，支持 `missing` 查空字段与 `compact` 精简投影 |
| 查询 | `search_records_by_location` | 按坐标中心点和半径搜索附近演出 |
| 查询 | `get_record` / `get_artist_detail` / `get_drama_detail` | 单条详情 |
| 查询 | `list_artists` / `list_dramas` / `list_venues` | 实体清单 |
| 查询 | `value_counts` / `get_stats` / `get_analytics` / `get_dashboard` | 取值频次 / 总览统计 / 深度分析 / 看板统计 |
| 记录 CRUD | `create_record` / `update_record` / `delete_record` / `batch_delete_records` | 演出记录增删改 |
| 批量 | `batch_update_company_by_artist` | 统一某演员所有演出的剧团名（支持 dry_run） |
| 批量 | `batch_merge_venues` | 合并同一场馆的不同写法（支持 dry_run、坐标同步） |
| 批量 | `batch_update_records` | 按 ID 通用批量更新（标量赋值 + 数组 set/append/remove，含 artist_ids） |
| 剧目 | `create_drama` / `update_drama` / `delete_drama` | 剧目档案增删改 |
| 折子 | `batch_create_zhezis` / `update_zhezi` / `delete_zhezi` | 折子批量创建与维护 |
| 演员 | `create_artist` / `update_artist` / `delete_artist` | 演员档案增删改 |
| 合并 | `merge_artists` / `merge_dramas` | 合并重复演员/剧目档案（关联改挂 + 别名并入 + 删除 source） |
| 照片 | `list_record_photos` | 列出演出记录附加的照片/票根文件名 |
| 分类 | `list_categories` / `create_category` / `update_category` / `delete_category` | 剧种分类管理 |
| 回收站 | `list_deleted_records` / `restore_record` / `purge_record` / `purge_deleted_records` | 已删除记录恢复与永久删除 |
| 排序 | `reorder_categories` / `reorder_dramas` / `reorder_zhezis` / `reorder_artists` | 实体排序调整 |
| 封面 | `list_covers` / `cover_duplicates` / `merge_covers` / `cover_orphans` / `cleanup_covers` | 封面去重与清理 |
| 导入导出 | `export_data` / `import_data` | 数据导出为 JSON / 从 JSON 导入 |
| 备份 | `run_backup` / `list_backups` / `delete_backup` / `restore_from_backup` | 备份管理与恢复（run_backup 直接执行，无 dry_run） |
| 地图 | `list_map_points` | 获取有坐标的演出记录（地图展示） |

共 55 个工具，详细说明见 [docs/mcp.md](docs/mcp.md)。

### 典型工作流

1. **按演员统一剧团**：`search_records(artist_name=…)` 或 `batch_update_company_by_artist(dry_run=true)` 预览 → 确认后 `dry_run=false` 执行。
2. **合并近似场馆名**（如「xx剧院」与「xx剧院（某某店）」）：`list_venues(query=xx)` 找出候选 → 与用户确认对应关系 → `batch_merge_venues(source, target, sync_coordinates=true)`。拿不准时先 dry_run。
3. **从互联网补充剧目常演折子**：先 `get_drama_detail(name=剧目名)` 看已有折子 → 用 websearch/webfetch 查证该剧目常演折子（维基百科、剧团官网、戏迷资料等，注意甄别可靠性） → `batch_create_zhezis(drama_id=…, names=[…])` 一次写入，重名自动跳过 → 之后用户在演出记录里即可选用这些折子。
4. **发现并清理重复封面**：`cover_duplicates()` 查看重复分组 → `merge_covers(sources, target, dry_run=true)` 预览 → `merge_covers(sources, target)` 执行合并 → `cover_orphans()` 检查孤立文件 → `cleanup_covers(dry_run=true)` 预览 → `cleanup_covers()` 执行清理。

### 注意事项

- 所有变更类工具（增删改、批量更新、合并、清理等）都带 `dry_run` 参数，**默认 true（仅预览、不落库）**。调用方需显式传 `dry_run:false` 才会真正执行。即：不传 dry_run 先预览影响范围，确认无误后再以 `dry_run:false` 执行。
- 场馆没有独立实体表，以 `records.address` 为隐式标识；剧团是 `records.company` 文本字段。
- 演员、剧目、折子是一等实体（artists/dramas/zhezis 表 + 关联表）。
- 数据库为 SQLite（WAL），MCP 进程与 HTTP 服务可同时运行。

## 开发

- 后端：`cd backend && go build .`；测试 `go test ./...`
- 一键开发环境：`./dev.sh`
