# 幕间 (MuJian)

![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white) ![Svelte](https://img.shields.io/badge/Svelte-5-FF3E00?logo=svelte&logoColor=white) [![build](https://img.shields.io/github/actions/workflow/status/Felix2yu/mujian/build.yml?branch=main)](https://github.com/Felix2yu/mujian/actions) [![codecov](https://codecov.io/gh/Felix2yu/mujian/branch/main/graph/badge.svg)](https://codecov.io/gh/Felix2yu/mujian) [![last-commit](https://img.shields.io/github/last-commit/Felix2yu/mujian)](https://github.com/Felix2yu/mujian) [![license](https://img.shields.io/github/license/Felix2yu/mujian)](https://github.com/Felix2yu/mujian/blob/main/LICENSE)

现场演出记录管理应用。记录每一场演出，管理剧目与折子档案，支持日历、地图、数据分析、封面管理与 PWA 离线使用。

## 功能特性

- **演出记录** — 添加/编辑/删除演出（删除进回收站，保留 30 天可恢复），批量更新与删除，搜索与多条件筛选；关键词搜索支持空格分隔多词（AND 组合，如「牡丹亭 上海」）
- **复制新建** — 新建演出时可搜索既往演出，勾选需要的字段一键复制（内容类字段默认勾选，时间/封面/状态/评分/座位/同行默认不勾）
- **剧目与折子** — 剧目（剧种）档案、折子别名、手动排序；演出自动关联剧目
- **日历视图** — 按月查看演出，支持海报显示，导出 ICS 订阅
- **数据分析** — 统计总览、月度趋势、分类/城市分布、消费统计、高分推荐
- **地图** — Leaflet 地图按城市/场馆查看演出，同场馆坐标自动对齐
- **封面管理** — 内容哈希去重合并、未引用封面清理与回收站、统一缩略图、批量格式转换（AVIF / WebP / JPEG）
- **自动备份** — 设置页可配格式（数据库快照 / data.json / zip 含封面）、间隔与保留份数，定时快照到 `data/backups/`，支持手动备份、下载、在线恢复与推送 S3
- **票根多图** — 每场演出可附加多张照片（票根/现场照），详情页相册浏览与管理
- **存储热切换** — 本地 ↔ S3 保存后立即生效，无需重启
- **导入导出** — 兼容「记录现场」导出的 `data.json` 与 `JI_LU_XIAN_CHANG.android.zip`（含 base64 封面），支持 JSON / ZIP 备份恢复
- **AI 助手（MCP）** — 内置 Model Context Protocol 服务器，支持 AI 批量查询/修改/分析演出数据：按演员统一剧团、合并近似场馆写法、从互联网补充剧目常演折子等
- **PWA 支持** — 可添加到主屏幕，离线缓存，推送通知提醒
- **暗色模式** — 跟随系统或手动切换

## 技术栈

| 层级 | 技术 |
|------|------|
| 前端 | SvelteKit, Svelte 5, pnpm, Vite, Leaflet |
| 后端 | Go 1.27, Chi Router, log/slog, PGO |
| 数据库 | SQLite（modernc.org/sqlite，纯 Go） |
| 图片 | AVIF（默认）/ WebP / JPEG（avif-go、chai2010/webp 自带静态库，需 CGO） |
| 存储 | 本地磁盘或 S3（AWS SDK v2），封面按内容哈希（`covers/<sha256>.<ext>`）寻址 |
| 部署 | Docker（ghcr.io/felix2yu/mujian，amd64/arm64），docker-compose |
| CI/CD | GitHub Actions + Codecov（覆盖率 ≥85% 门禁） |

## 快速开始

### 环境要求

- Node.js 20+ 与 pnpm
- Go 1.27
- CGO 编译器（AVIF/WebP 编码需要；两个图像库自带静态库，无需系统 libavif）：
  - macOS：Xcode Command Line Tools
  - Debian/Ubuntu：`apt install gcc`

### 开发模式

```bash
# 前端（端口 5173，API 自动代理到 :8080）
cd frontend && pnpm install
pnpm run dev

# 后端（端口 8080，dev.sh 监听 .go 变化自动重建重启）
cd .. && make dev-backend
```

访问 http://localhost:5173 。

### 生产构建

```bash
make build
```

产物为 `backend/mujian`（前端 dist 已内嵌），运行：

```bash
cd backend && ./mujian
```

配置通过环境变量，见下文「环境变量」。

### Docker

```bash
make docker   # docker compose up -d
```

镜像构建于 GitHub Actions（`ghcr.io/felix2yu/mujian`），`docker-compose.yml` 挂载 `mujian-data` 卷，默认端口 8080。

### 测试与覆盖率

```bash
cd backend && CGO_ENABLED=1 go test ./... -coverprofile=coverage.out
```

CI 中每次推送/PR 都会运行测试并上传 Codecov；`codecov.yml` 设定 project 与 patch 覆盖率目标 **85%**（`main.go` 与 pprof 调试桩不计入门禁）。

## AI 助手（MCP 服务）

后端内置 MCP（Model Context Protocol）服务器，随 HTTP 服务启动并通过 `/mcp` 端点以 Streamable HTTP 暴露，直接读写数据库，供 opencode 等 AI 编程助手批量查找、修改、分析演出数据。

```bash
cd backend && ./mujian        # MCP 随服务自动启动；opencode 用户由项目根 opencode.json 接管（remote 指向 /mcp）
```

工具分三类（共 15 个）：

| 类别 | 工具 |
|------|------|
| 查询/分析 | `search_records`、`get_record`、`list_artists`、`get_artist_detail`、`list_dramas`、`get_drama_detail`、`list_venues`、`value_counts`、`get_stats` |
| 批量修改 | `batch_update_company_by_artist`、`batch_merge_venues`、`batch_update_records` |
| 折子管理 | `batch_create_zhezis`、`update_zhezi`、`delete_zhezi` |

典型场景：

1. **按演员统一剧团** — 预览某演员全部演出的剧团写法，一次统一为标准名。
2. **合并近似场馆** — 如「xx剧院」与「xx剧院（某某店）」实为同址，合并地址并同步坐标。
3. **补充剧目常演折子** — AI 联网查证后批量写入剧目折子档案，供演出记录选用。

所有批量修改均支持 `dry_run` 预览。配置 `MJ_AUTH_TOKEN` 后，MCP 客户端需在请求头携带 `Authorization: Bearer <令牌>`。详细说明见 [AGENTS.md](AGENTS.md) 与 [docs/mcp.md](docs/mcp.md)。

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `8080` | HTTP 监听端口 |
| `DB_PATH` | `./data/mujian.db` | SQLite 数据库路径（WAL 模式 + `synchronous=NORMAL`，导入走单事务批量写入） |
| `UPLOAD_DIR` | `./data/uploads` | 封面/缩略图存储目录 |
| `TZ` | `Asia/Shanghai` | 时区 |
| `IMAGE_FORMAT` | `avif` | 新上传封面编码格式（`avif`/`webp`/`jpeg`），可在设置页改，立即生效 |
| `STORAGE_TYPE` | `local` | `local` 或 `s3`（设置页可切换） |
| `ALLOW_LOCAL_STORAGE` | `true` | 设为 `false` 时禁用本地存储选项 |
| `THEME` | `auto` | 默认主题（`auto`/`light`/`dark`） |
| `S3_ENDPOINT` / `S3_BUCKET` / `S3_REGION` | — | S3 对象存储配置（使用 S3 时必填） |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | — | S3 凭证 |
| `S3_PUBLIC_URL` | — | S3 封面公开访问地址前缀 |
| `MJ_AUTH_TOKEN` | — | 设置后所有 API/MCP 请求需携带该令牌（Bearer 头或 `?token=` 参数） |
| `BACKUP_INTERVAL_HOURS` | `0` | 自动备份间隔（0 = 关闭），可在设置页改 |
| `BACKUP_KEEP` | `10` | 备份快照保留份数 |
| `BACKUP_FORMAT` | `db` | 备份格式（`db`/`json`/`zip`） |
| `BACKUP_REMOTE` | `false` | 备份成功后推送到 S3 的 `backups/` 目录 |

## 项目结构

```
mujian/
├── frontend/                  # SvelteKit 前端
│   └── src/
│       ├── lib/               # 组件 (RecordForm, RecordCard, BatchEditModal...)、api.js、stores.js
│       └── routes/            # 页面路由
│           ├── +page.svelte           # 首页（演出列表）
│           ├── records/               # 演出详情 / 新增 / 编辑
│           ├── dramas/                # 剧目与折子
│           ├── categories/            # 分类管理
│           ├── covers/                # 封面管理（去重/清理/缩略图/格式转换）
│           ├── map/                   # 地图
│           ├── import/                # 导入
│           ├── analytics/             # 数据分析
│           └── settings/              # 设置
│   └── static/sw.js           # Service Worker（PWA 缓存与推送）
├── backend/                   # Go 后端（前端 dist 通过 go:embed 内嵌）
│   ├── main.go                # 入口、路由挂载（/api、/mcp）、uploads 静态服务（os.Root 防护）
│   ├── default.pgo            # PGO profile
│   └── internal/
│       ├── config/            # 配置加载与设置持久化
│       ├── db/                # SQLite 数据层
│       ├── handlers/          # HTTP 处理器（含流式进度接口）
│       ├── ics/               # iCalendar 导出
│       ├── mcp/               # MCP 服务器（AI 批量查询/修改/分析工具）
│       ├── models/            # 数据模型
│       └── storage/           # 封面存储（本地/S3）、图片编解码、格式嗅探
├── codecov.yml                # Codecov 覆盖率门禁
├── .github/workflows/build.yml # CI：构建 + 测试 + 覆盖率上传 + Docker 镜像
├── docker-compose.yml
├── Dockerfile
└── Makefile
```

## API 接口

所有接口前缀 `/api`：

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | `/records` | 列表 / 创建演出 |
| GET/PUT/DELETE | `/records/{id}` | 详情 / 更新 / 删除 |
| GET | `/records/all` `/records/search` | 全量列表 / 搜索 |
| POST | `/records/batch` `/records/batch/delete` | 批量更新 / 批量删除 |
| POST | `/records/align-venues` | 同场馆坐标对齐 |
| GET | `/records/deleted` | 回收站列表 |
| POST | `/records/{id}/restore`、`/records/trash/purge` | 从回收站恢复 / 清空回收站 |
| DELETE | `/records/{id}/purge` | 彻底删除（不可恢复） |
| GET/POST/DELETE | `/records/{id}/photos` | 票根照片列表 / 关联 / 移除（`{pid}` 删除、`/reorder` 排序） |
| POST | `/records/import` | 导入（JSON / 记录现场 ZIP） |
| GET/POST | `/categories`；PUT/DELETE `/categories/{id}` | 分类管理 |
| POST | `/categories/reorder` | 分类手动排序（`{"ids":[...]}`） |
| GET/POST | `/dramas` | 剧目列表 / 创建 |
| GET/PUT/DELETE | `/dramas/{id}` | 剧目详情（含折子与关联演出）/ 更新 / 删除 |
| POST | `/dramas/reorder` | 剧目手动排序（`{"ids":[...]}`，首个为最前） |
| POST | `/dramas/{id}/zhezis` | 新建折子 |
| PUT/DELETE | `/zhezis/{id}` | 更新 / 删除折子 |
| POST | `/dramas/{id}/zhezis/reorder` | 折子排序 |
| GET | `/dramas/tree` | 剧目+折子结构（选择器用） |
| GET | `/calendar` `/calendar.ics` | 月历事件 / ICS 导出 |
| GET | `/stats` `/dashboard` | 统计 / 仪表盘 |
| GET | `/autocomplete/{field}` `/field/{field}/{value}` | 字段补全 / 字段筛选 |
| GET/PUT | `/settings` | 读取 / 更新设置 |
| POST | `/upload` | 上传封面（≤8MB，按当前编码格式存储） |
| GET | `/export?format=zip` | 导出 JSON / ZIP 备份 |
| POST | `/backup/run`、`/backup/restore-from` | 立即备份 / 从已有快照恢复（json/zip） |
| GET | `/backup/list`、`/backup/download?file=` | 备份列表 / 下载快照 |
| DELETE | `/backup?file=` | 删除一份备份 |
| GET | `/covers` | 封面复用选择器 |
| GET/POST | `/covers/duplicates` `/covers/merge` | 查重 / 合并重复封面 |
| GET/POST | `/covers/orphans` `/covers/cleanup` | 未引用封面扫描 / 清理 |
| POST | `/covers/trash/purge` | 清空回收站 |
| POST | `/covers/thumbs` | 重新生成缩略图 |
| POST | `/covers/convert` | 单张封面格式转换 |
| POST | `/covers/convert-batch` | 批量格式转换（流式进度） |

> 批量转换与缩略图重生成接口以 NDJSON 流式返回进度（每行一个 JSON，`done:true` 结尾），前端展示实时进度；HTTP 层面无固定超时。

## 演出状态

`active_status` 整型字段：

| 值 | 显示 |
|----|------|
| 0 | 正常 |
| 1 | 想看 |
| 2 | 已取消 |
| 3 | 未赴约 |

## 封面与图片

- 封面按内容哈希存储，相同内容自动去重（`covers/<sha256>.<ext>`）；缩略图为独立文件（`covers/<hash>.thumb.<ext>`）。
- 批量格式转换按**真实编码格式（魔数嗅探）**跳过已是目标格式的文件，不会因扩展名误判而重复压缩。
- 「重新生成缩略图」按当前编码设置为所有封面重建缩略图并清理旧格式。

## 数据备份

- **导出**：设置页或 `GET /api/export`（JSON），`?format=zip` 同时打包封面文件。
- **导入**：设置页「恢复备份」，或演出导入页上传 `data.json` / 「记录现场」导出的 `JI_LU_XIAN_CHANG.android.zip`（自动解码 base64 封面）。
- **自动备份**：设置页可配置格式、间隔与保留份数。格式三选一：数据库快照（`.db`，SQLite `VACUUM INTO`，停机换回文件即可恢复）、纯数据（`data.json`）、数据 + 封面（`.zip`，即完整导出包）；后两种可从「数据」页导入恢复。间隔：每天 / 每 3 天 / 每周 / 每 2 周 / 每月 / 每季度，默认关闭。快照类备份只含数据库，封面文件在 `data/uploads/covers/`（S3 模式在桶里）。

## 测试覆盖率

CI 每次推送/PR 自动运行测试并上传 Codecov（目标 ≥85%）。实时覆盖率见下方图表：

[![codecov](https://codecov.io/gh/Felix2yu/mujian/branch/main/graph/badge.svg)](https://codecov.io/gh/Felix2yu/mujian)

<img src="https://codecov.io/gh/Felix2yu/mujian/graphs/icicle.svg" alt="Codecov 覆盖率冰柱图" width="100%">

> 另提供 [sunburst](https://codecov.io/gh/Felix2yu/mujian/graphs/sunburst.svg) 与 [tree](https://codecov.io/gh/Felix2yu/mujian/graphs/tree.svg) 两种视图。
