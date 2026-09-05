# 日历同步（ICS 订阅 / CalDAV）

幕间提供三种把演出记录放进系统日历的方式，按需选择：

| 方式 | 地址 | 实时性 | Apple 日历地图 | 适用场景 |
|------|------|--------|----------------|----------|
| **导出 .ics 导入** | 设置 → 日历 → 导出日历 | 一次性快照 | ✅ 有 | 偶尔导出、换设备迁移 |
| **ICS 订阅** | 设置 → 日历 → 订阅链接 | 定期轮询（分钟级延迟） | ❌ 无（Apple 平台限制） | 无 HTTPS 环境、Google 日历、第三方日历客户端 |
| **CalDAV 账户** | `/caldav/user/calendars/mujian/` | 自动增量同步 | ✅ 有 | **iOS / macOS 原生日历（推荐）** |

## 何时推荐 CalDAV

- **想在 Apple 日历里看到地图卡片**：ICS 订阅源的事件一律不被 Apple 渲染地图（`LOCATION` 不可点），这是 Apple 对订阅日历的平台限制，任何 ICS 字段都绕不过。CalDAV 账户同步进来的事件是本地一等事件，`LOCATION` 会被系统在本机地理编码，正常显示地图。
- **想要实时同步**：CalDAV 走 ETag 增量拉取，编辑记录后日历自动更新；ICS 订阅依赖客户端轮询，且 Apple 对订阅刷新频率不可控。
- CalDAV 是**只读投影**：日历侧的增删改会被服务器以 403 拒绝，所有编辑仍在幕间进行。

## CalDAV 配置步骤

日历集合地址：`https://<你的域名>/caldav/user/calendars/mujian/`（设置页「日历」卡片可一键复制）。

- **iOS**：设置 → 日历 → 账户 → 其他 → 添加 CalDAV 账户
- **macOS**：系统设置 → 互联网账户 → 添加其他账户 → CalDAV 账户

表单填写：

| 字段 | 值 |
|------|-----|
| 服务器 | 只填域名（如 `mujian.example.com`），**不带** `https://` 和路径；系统会经 `/.well-known/caldav` 自动发现 |
| 用户名 | 随意 |
| 密码 | 访问令牌 `auth_token`（设置页「安全」处的令牌，即 `MJ_AUTH_TOKEN`） |

未配置令牌（内网直通模式）时无需密码，用户名随意即可。CalDAV 需要 **HTTPS**——iOS 添加 CalDAV 账户基本要求 TLS，明文 http 会被拒绝。

## 事件重复

同一台设备上同时保留「幕间」ICS 订阅和幕间 CalDAV 账户，同一场演出会出现两份事件（内容相同、所属日历不同）。配好 CalDAV 后建议在日历应用里退订旧的「幕间」订阅。

## 反向代理注意事项

CalDAV 使用 `PROPFIND` / `REPORT` 等 WebDAV 方法。大多数反代（nginx 全量 `proxy_pass`、Caddy、Traefik）会原样转发任意方法，无需特殊配置；但如果你的 nginx 按路径拆分了 `location`，注意：

1. **`/caldav` 与 `/.well-known/caldav` 必须也 `proxy_pass` 到后端**。落在静态文件 `location / { root ...; try_files ...; }` 里的 PROPFIND 会被 nginx 直接 405（静态资源只接受 GET/HEAD）。
2. **保留 `Authorization` 头**。nginx 默认转发；若配置里有 `proxy_set_header Authorization "";` 之类用于隐藏上游凭据的指令，CalDAV 的 Basic 认证会被破坏。
3. **转发请求体**。`REPORT` 携带 XML body，不要对这些路径开 `proxy_pass_request_body off`。
4. 变量形式的 `proxy_pass`（如 `proxy_pass http://$upstream:$port;`）需要 `resolver` 指令（docker 环境常用 `resolver 127.0.0.11`）；直接写上游地址则不需要。

SWAG / LinuxServer.io 风格的全量转发配置（`location /` 统一 `proxy_pass`）天然兼容，无需额外处理。

自检命令（405 = 被反代拦截；401 = 已到后端、缺认证；207/200 = 正常）：

```bash
# 能力探测：应返回 204，且响应头含 Dav: 1, 3, calendar-access
curl -i -X OPTIONS https://<域名>/caldav/user/ | head -5

# 发现入口：应 302 到 /caldav/user/
curl -si -X PROPFIND https://<域名>/.well-known/caldav | head -4

# 日历集合（浏览器直接打开也可以）：登录后应返回完整 ICS 内容
curl -su ":<令牌>" -X PROPFIND -H "Depth: 0" https://<域名>/caldav/user/calendars/mujian/ | head -c 300
```

## 实现位置

- 协议栈：[`backend/internal/caldav/`](../backend/internal/caldav/)（基于 `emersion/go-webdav`，只读 `caldav.Backend`）
- 事件渲染：[`backend/internal/ics/ics.go`](../backend/internal/ics/ics.go) 的 `EventCalendar`，与 ICS 订阅共用同一套格式（`GEO`、`X-APPLE-STRUCTURED-LOCATION` 等）
- 路由与鉴权：`backend/main.go`（Basic 认证，密码即访问令牌；兼容 Bearer / X-Auth-Token / `?token=`）
