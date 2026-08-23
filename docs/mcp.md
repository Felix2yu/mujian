# MCP 服务（mujian）

幕间后端内置 MCP（Model Context Protocol）服务器，让 AI 编程助手（opencode、Claude Code 等）能够直接、安全地批量查找、修改、分析演出数据，而无需通过 HTTP API 或裸 SQL。

## 架构

```
opencode ──Streamable HTTP (JSON-RPC)──▶ https://<服务地址>/mcp ──▶ backend/data/mujian.db
```

- 传输：Streamable HTTP（Stateless + JSON 响应），MCP 服务随主 HTTP 服务一起启动，挂载在 `/mcp` 端点。
- 数据库：与 HTTP API 共用同一个 SQLite（WAL）、同一个 `*db.DB` 实例。
- 入口：`main.go` 的 `/mcp` 路由；实现在 `backend/internal/mcp/`。
- 无鉴权：暴露面与 `/api` 一致，由反向代理（nginx）或内网边界保护。

## 启动方式

```bash
# MCP 随 HTTP 服务自动启动，无需单独运行
cd backend && ./mujian   # 之后 AI 客户端连接 http://<服务地址>/mcp

# opencode：项目根 opencode.json 已注册 remote MCP（url 指向 /mcp），
# 改成你的实际域名后重启 opencode 即可使用全部 mujian 工具。
```

## 工具清单（15 个）

### 查询 / 分析

| 工具 | 说明 |
|------|------|
| `search_records` | 多条件筛选演出：关键词（名称/城市/场馆/剧团/备注/演员）、`artist_name`/`artist_id`、`drama_name`/`drama_id`、`zhezi_id`、城市、分类、年月或起止日期；默认返回 50 条，可用 `limit` 调整 |
| `get_record` | 按 ID 取单条完整详情（含关联剧目/折子/演员） |
| `list_artists` | 全部演员档案（含别名、演出次数） |
| `get_artist_detail` | 演员详情 + 关联演出；支持 `id` 或姓名/别名 |
| `list_dramas` | 全部剧目档案（剧种、折子数、演出次数） |
| `get_drama_detail` | 剧目详情 + 折子列表 + 关联演出；支持 `id` 或名称 |
| `list_venues` | 场馆按地址分组统计（次数/城市/坐标状态）；`query` 子串过滤，用于发现同址异名 |
| `value_counts` | `company`/`city`/`channel`/`category_name` 取值频次，发现相似写法 |
| `get_stats` | 总览统计（场次、消费、均分、城市数） |

### 批量修改

| 工具 | 说明 |
|------|------|
| `batch_update_company_by_artist` | 把某演员参与的所有演出的 `company` 统一为指定值；`dry_run=true` 只预览 |
| `batch_merge_venues` | 将 `source_address` 的所有记录地址改写为 `target_address`；`sync_coordinates=true` 时把目标场馆已有坐标同步给合并后的记录；支持 `dry_run` |
| `batch_update_records` | 按 ID 列表通用更新：标量字段直接赋值（company/city/address/rating 等）；数组字段（`drama_ids`/`zhezi_ids`/`artist_names`/`play`/`guest`）支持 `set`/`append`/`remove` 三种操作 |

### 折子管理

| 工具 | 说明 |
|------|------|
| `batch_create_zhezis` | 给剧目批量写入折子清单，已存在的同名（或同别名）自动跳过；`drama_id` 与 `drama_name` 二选一 |
| `update_zhezi` | 部分更新折子的名称/别名/备注 |
| `delete_zhezi` | 删除折子并解除所有演出关联 |

## 典型工作流

### 1. 按演员统一剧团

```
batch_update_company_by_artist(artist_name="张三", company="上海昆剧团", dry_run=true)
#   → 检查 matched / will_change / records 预览
batch_update_company_by_artist(artist_name="张三", company="上海昆剧团")
```

### 2. 合并近似场馆写法

```
list_venues(query="大剧院")          # 找出「上海大剧院」「上海大剧院（西店）」等候选
# 与用户确认对应关系后：
batch_merge_venues(source_address="上海大剧院（西店）",
                   target_address="上海大剧院", sync_coordinates=true, dry_run=true)
# 确认无误再去掉 dry_run 执行
```

### 3. 从互联网补充剧目常演折子

1. `get_drama_detail(name="牡丹亭")` 查看已有折子；
2. 用 websearch/webfetch 查证该剧目常演折子（维基百科、剧团官网、戏迷资料等，注意甄别可靠性）；
3. `batch_create_zhezis(drama_id=…, names=["游园","惊梦","拾画","画祭"], remark="来源：xxx")` 一次写入，重名自动跳过；
4. 之后在演出记录编辑页即可选用这些折子。

## 设计要点

- **dry_run 优先**：两个批量修改工具都带 `dry_run` 参数（默认 false）。约定流程是先预览影响范围、经用户确认再执行。
- **模糊解析有兜底**：演员/剧目支持精确名 → 别名 → 不区分大小写的部分匹配；部分匹配唯一时直接采用，多个候选时返回候选列表要求指定 ID，绝不静默猜测。
- **数据模型现实**：场馆没有独立实体表（以 `records.address` 为隐式标识），剧团是文本字段；因此「合并」就是批量改写字段值，坐标同步复用既有 `SyncVenueCoordinates`。
- **错误走工具级**：可恢复错误（如未找到记录）以 `isError=true` 的结果返回给模型自行纠正，不中断协议会话。
