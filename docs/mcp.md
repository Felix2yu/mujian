# MCP 服务（mujian）

幕间后端内置 MCP（Model Context Protocol）服务器，让 AI 编程助手能够直接、安全地批量查找、修改、分析演出数据，而无需通过 HTTP API 或裸 SQL。

## 架构

```
AI 客户端 ──Streamable HTTP (JSON-RPC)──▶ https://<服务地址>/mcp ──▶ backend/data/mujian.db
```

- 传输：Streamable HTTP（Stateless + JSON 响应），MCP 服务随主 HTTP 服务一起启动，挂载在 `/mcp` 端点。

- 数据库：与 HTTP API 共用同一个 SQLite（WAL）、同一个 `*db.DB` 实例。

- 入口：`main.go` 的 `/mcp` 路由；实现在 `backend/internal/mcp/`。

- 鉴权：若设置了 `MJ_AUTH_TOKEN` 环境变量，所有 MCP 请求需要在 HTTP 头携带 `Authorization: Bearer <令牌>`。

- 无鉴权（开发环境）：暴露面与 `/api` 一致，由反向代理（nginx）或内网边界保护。

## 启动方式

```bash
# MCP 随 HTTP 服务自动启动，无需单独运行
cd backend && ./mujian   # 之后 AI 客户端连接 http://<服务地址>/mcp
```

## 客户端配置

### opencode / Trae

项目根 [`opencode.json`](../opencode.json) 已注册 remote MCP：

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "mujian": {
      "type": "remote",
      "url": "{file:.mcp-url}",
      "enabled": true,
      "timeout": 60000
    }
  }
}
```

本地新建 `.mcp-url` 文件写入你的服务地址（如 `https://mujian.example.com/mcp`，该文件已 gitignore），重启 opencode 即可使用全部工具。

### Claude Code

在项目根创建或编辑 `.claude/settings.json`：

```json
{
  "mcpServers": {
    "mujian": {
      "type": "url",
      "url": "https://mujian.example.com/mcp"
    }
  }
}
```

### Cherry Studio

在 Cherry Studio 的 MCP 配置中添加 remote MCP 服务：

```json
{
  "mcpServers": {
    "mujian": {
      "type": "url",
      "url": "https://mujian.example.com/mcp"
    }
  }
}
```

如果服务配置了 `MJ_AUTH_TOKEN`，可在 Cherry Studio 自定义请求头中添加：

```
Authorization: Bearer <你的令牌>
```

### Continue.dev

在 `~/.continue/config.json` 的 `experimental.mcpServers` 中添加：

```json
{
  "experimental": {
    "mcpServers": {
      "mujian": {
        "type": "url",
        "url": "https://mujian.example.com/mcp"
      }
    }
  }
}
```

### Cursor

在项目中创建 `.cursor/mcp.json`：

```json
{
  "mcpServers": {
    "mujian": {
      "type": "url",
      "url": "https://mujian.example.com/mcp"
    }
  }
}
```

### Windsurf

在项目中创建 `.windsurf/mcp.json`：

```json
{
  "mcpServers": {
    "mujian": {
      "type": "url",
      "url": "https://mujian.example.com/mcp"
    }
  }
}
```

### 其他 MCP 客户端

所有支持 **Streamable HTTP（URL）** 类型 MCP 的客户端均可使用上述配置模板。只需将 `url` 指向 `https://<你的服务地址>/mcp` 即可。如果服务配置了鉴权，参考客户端文档添加 `Authorization: Bearer <令牌>` 请求头。

## 工具清单

### 查询 / 分析（13 个）

| 工具                       | 说明                                                                                                                                |
| ------------------------ | --------------------------------------------------------------------------------------------------------------------------------- |
| `search_records`         | 多条件筛选演出：关键词（名称/城市/场馆/剧团/备注/演员）、`artist_name`/`artist_id`、`drama_name`/`drama_id`、`zhezi_id`、城市、分类、年月或起止日期，另支持渠道/剧团/评分与票价区间/演出状态/`missing`（查空字段，数据卫生）/`compact`（精简投影）/`limit`+`offset` 分页 |
| `get_record`             | 按 ID 取单条完整详情（含关联剧目/折子/演员）                                                                                                         |
| `list_artists`           | 全部演员档案（含别名、演出次数）；`query` 按姓名/别名子串过滤                                                                                                |
| `get_artist_detail`      | 演员详情 + 关联演出；支持 `id` 或姓名/别名                                                                                                        |
| `list_dramas`            | 全部剧目档案（剧种、折子数、演出次数）；`query` 按名称/别名子串过滤                                                                                            |
| `get_drama_detail`       | 剧目详情 + 折子列表 + 关联演出；支持 `id` 或名称                                                                                                    |
| `list_venues`            | 场馆按地址分组统计（次数/城市/坐标状态）；`query` 子串过滤，用于发现同址异名                                                                                       |
| `value_counts`           | `company`/`city`/`channel`/`category_name` 取值频次，发现相似写法                                                                            |
| `get_stats`              | 总览统计（场次、消费、均分、城市数）                                                                                                                |
| `search_records_by_location` | 按坐标中心点和半径（米）搜索附近的演出记录，返回按距离排序的列表；建议半径不超过 10000（10公里）                                                                       |
| `get_analytics`          | 深度分析数据（与网页分析页一致）：观演频率与间隔、重看统计、剧种多样性、票价分布、星期分布、新剧发现曲线等                                                                             |
| `get_dashboard`          | 看板统计（与网页首页一致）：总场次/总消费/均分、近 12 个月按月/按剧种/按城市分布、成本趋势、最高评分与最近记录                                                                        |
| `list_record_photos`     | 列出某条演出记录附加的照片/票根文件名与排序（不含图片内容）                                                                                                    |

### 演出记录 CRUD（4 个）

| 工具                     | 说明                                         |
| ---------------------- | ------------------------------------------ |
| `create_record`        | 创建演出记录（`name` 必填），支持一次性传入演员/剧目/折子          |
| `update_record`        | 更新单条演出记录的任意字段，数组支持 `set`/`append`/`remove` |
| `delete_record`        | 删除单条演出记录（移入回收站）                                   |
| `batch_delete_records` | 按 ID 列表批量删除（移入回收站）                                |

### 批量修改（3 个）

| 工具                               | 说明                                                    |
| -------------------------------- | ----------------------------------------------------- |
| `batch_update_company_by_artist` | 把某演员参与的所有演出的 `company` 统一为指定值                         |
| `batch_merge_venues`             | 将 `source_address` 的所有记录地址改写为 `target_address`；可选同步坐标 |
| `batch_update_records`           | 按 ID 列表通用更新：标量字段直接赋值，数组字段（含 `artist_ids`，直接按档案 ID 改演员关联）支持 `set`/`append`/`remove` |

### 剧目管理（3 个）

| 工具             | 说明                            |
| -------------- | ----------------------------- |
| `create_drama` | 创建新剧目档案（`name` 必填），可附带别名和剧种   |
| `update_drama` | 更新剧目名称/别名/备注/剧种；剧种为空数组时回到自动聚合 |
| `delete_drama` | 删除剧目及其所有折子                    |

### 折子管理（3 个）

| 工具                    | 说明                           |
| --------------------- | ---------------------------- |
| `batch_create_zhezis` | 给剧目批量写入折子清单，已存在的同名（或同别名）自动跳过 |
| `update_zhezi`        | 部分更新折子的名称/别名/备注              |
| `delete_zhezi`        | 删除折子并解除所有演出关联                |

### 演员管理（3 个）

| 工具              | 说明                          |
| --------------- | --------------------------- |
| `create_artist` | 创建新演员档案（`name` 必填），可附带别名和简介 |
| `update_artist` | 更新演员的名称/别名/备注/简介            |
| `delete_artist` | 删除演员档案（同时解除与演出记录的关联）        |

### 实体合并（2 个）

| 工具             | 说明                                                                                     |
| -------------- | ---------------------------------------------------------------------------------------- |
| `merge_artists` | 合并重复演员档案：演出关联改挂 target，姓名与别名并入 target 别名，bio/备注/封面仅在 target 为空时补入，删除 source；双方支持 id 或姓名定位 |
| `merge_dramas`  | 合并重复剧目档案：演出关联与折子并入 target（同名折子去重），姓名与别名并入 target 别名，删除 source；双方支持 id 或名称定位               |

### 分类管理（4 个）

| 工具                | 说明                  |
| ----------------- | ------------------- |
| `list_categories` | 列出所有分类（剧种），含演出计数和排序 |
| `create_category` | 创建新分类               |
| `update_category` | 更新分类名称              |
| `delete_category` | 删除分类                |

### 回收站管理（4 个）

| 工具                     | 说明                    |
| ---------------------- | --------------------- |
| `list_deleted_records` | 列出已删除的演出记录（回收站），支持分页 |
| `restore_record`       | 恢复已删除的演出记录到正常状态      |
| `purge_record`         | 永久删除演出记录（不可恢复）        |
| `purge_deleted_records`| 清空回收站（永久删除所有已删除记录）   |

### 排序管理（4 个）

| 工具                  | 说明                                      |
| ------------------- | --------------------------------------- |
| `reorder_categories` | 按指定顺序重新排列分类，需提供完整排序后的 ID 列表        |
| `reorder_dramas`     | 按指定顺序重新排列剧目，需提供完整排序后的 ID 列表        |
| `reorder_zhezis`     | 按指定顺序重新排列剧目下的折子，需提供 drama_id 和 ids |
| `reorder_artists`    | 按指定顺序重新排列演员，需提供完整排序后的 ID 列表        |

### 封面管理（5 个）

| 工具                 | 说明                                           |
| ------------------ | -------------------------------------------- |
| `list_covers`      | 列出封面（去重），支持按文件名查询，含引用计数                      |
| `cover_duplicates` | 查找内容哈希相同的重复封面分组                              |
| `merge_covers`     | 合并重复封面：将 sources 的引用全部指向 target，然后删除 sources |
| `cover_orphans`    | 查找没有被任何演出记录引用的孤立封面文件                         |
| `cleanup_covers`   | 清理所有孤立封面（无引用的文件）                             |

### 导入导出（2 个）

| 工具            | 说明                                     |
| ------------- | -------------------------------------- |
| `export_data` | 导出数据。默认返回统计概览（计数、分类列表）；`to_file=true` 写入备份目录 `export-*.json` 并返回路径；`include_records=true` 才在响应中内联全部记录 |
| `import_data` | 从 JSON 导入演出记录（按记录 upsert：同 ID 覆盖，不删除未包含的现有数据）；数据可经 `json_data` 内联或 `file_path`（服务器本地文件）提供 |

### 备份管理（4 个）

| 工具                   | 说明                          |
| -------------------- | --------------------------- |
| `run_backup`         | 手动触发一次备份（非破坏性，直接执行，无 dry_run） |
| `list_backups`       | 列出所有备份文件（按时间倒序）            |
| `delete_backup`      | 删除指定备份文件                    |
| `restore_from_backup`| 从指定备份文件恢复数据，支持 .json 格式 |

### 地图点位（1 个）

| 工具                | 说明                                   |
| ----------------- | ------------------------------------ |
| `list_map_points` | 获取所有有坐标的演出记录，支持按城市/分类过滤 |

**共计 55 个工具。**

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

### 4. 发现并清理重复封面

```
cover_duplicates()                    # 查看所有重复封面分组
merge_covers(sources=["abc.jpg","def.jpg"], target="ghi.jpg", dry_run=true)
# 确认无误后执行
merge_covers(sources=["abc.jpg","def.jpg"], target="ghi.jpg")
cover_orphans()                       # 清理前检查有无孤立文件
cleanup_covers(dry_run=true)          # 预览要删除的孤立文件
cleanup_covers()                      # 真正清理
```

### 5. 合并重复的演员/剧目档案

```
list_artists(query="张三")             # 发现同一演员的两个档案
merge_artists(source_name="张 三", target_id="art_abc", dry_run=true)
#   → 核对双方别名与演出次数，确认保留 target
merge_artists(source_name="张 三", target_id="art_abc")
#   → 演出关联改挂 target，"张 三" 并入别名，source 档案删除（剧目用 merge_dramas，同名折子自动去重）
```

## 设计要点

- **dry\_run 优先**：所有修改类工具都带 `dry_run` 参数（默认 `true`）。约定流程是先预览影响范围、经用户确认再执行。

- **模糊解析有兜底**：演员/剧目支持精确名 → 别名 → 不区分大小写的部分匹配；部分匹配唯一时直接采用，多个候选时返回候选列表要求指定 ID，绝不静默猜测。

- **数据模型现实**：场馆没有独立实体表（以 `records.address` 为隐式标识），剧团是文本字段；因此「合并」就是批量改写字段值，坐标同步复用既有 `SyncVenueCoordinates`。

- **错误走工具级**：可恢复错误（如未找到记录）以 `isError=true` 的结果返回给模型自行纠正，不中断协议会话。

