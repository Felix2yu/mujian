# 幕间（MuJian）指标采集

面向**内网单台赛扬 J4105 + Docker + Nginx 反代、单人使用**场景的轻量指标方案。

核心约束与对应决策：

| 约束 | 决策 |
|------|------|
| 低性能设备、要求开销极低 | **无常驻进程**，由 cron / systemd timer 触发，单次约 60 ms；不引入 Prometheus / node_exporter / Grafana |
| 不改动现有业务逻辑 | 采集全部发生在**宿主机旁路**：只读 `/proc`、`/sys/fs/cgroup`、nginx 日志与现有 `/api/stats`，**不改动一行 Go 代码、不改容器启动命令、不读写业务数据库** |
| 通用易读格式 | 同时输出 **JSONL 结构化时序日志**与 **Prometheus 文本暴露格式**快照 |
| 不影响服务稳定性 | 采集器与业务服务零耦合；任何采集失败都只影响指标，不影响业务（见「稳定性」章） |

---

## 1. 架构

```
┌─────────────── 宿主机（Debian + Nginx）───────────────┐
│                                                        │
│  cron / systemd timer  (每 1 分钟)                     │
│        │                                               │
│        ▼                                               │
│  collect.sh  ──► /proc/stat /proc/meminfo /proc/loadavg  │  宿主机 CPU/内存/负载
│                ──► df                                    │  磁盘
│                ──► /sys/fs/cgroup/.../docker-<id>.scope  │  容器 CPU/内存/PID
│                ──► nginx access log（增量解析）           │  请求量/延迟/错误
│                ──► curl /api/stats                       │  业务数据规模
│        │                                               │
│        ▼                                               │
│  /var/lib/mujian-metrics/                              │
│        ├── metrics.jsonl      时序历史                  │
│        ├── metrics.prom       最新快照（Prometheus）     │
│        └── state/             日志偏移、CPU 基准等      │
│                                                        │
│  ┌────────────── Docker ──────────────┐                │
│  │  mujian 容器（未做任何改动）         │                │
│  └────────────────────────────────────┘                │
└────────────────────────────────────────────────────────┘
```

**为什么不用 `docker stats`**：实测该命令单次耗时约 **2 秒**（`--no-stream` 只是取流式接口的第一个数据帧，仍需等待一次采样周期）。按 60 秒间隔即 3.3% 单核占空比，与「极低开销」目标冲突。改为直接读 cgroup 伪文件后，单次采集从 **2009 ms 降到 57 ms（35×）**。

---

## 2. 部署

### 2.1 前置条件

- 宿主机可写 `/etc/cron.d` 或可用 systemd（脚本自动检测，二选一）
- 已安装 `awk sed sort df curl mktemp`（Debian 默认齐全）
- `docker` 命令可用（仅扩展指标需要，缺失不影响常规采集）

### 2.2 安装

```bash
cd scripts/metrics
sudo sh install.sh \
  --container mujian \                                   # docker 容器名
  --nginx-log /var/log/nginx/mujian.access.log \         # nginx 访问日志路径
  --data-volume /var/lib/docker/volumes/mujian-data/_data \
  --dir /var/lib/mujian-metrics \                        # 指标输出目录
  --interval 1                                           # 常规采集间隔（分钟）
```

脚本会：安装脚本到 `/usr/local/lib/mujian-metrics/`、创建指标目录、注册定时触发
（systemd timer 优先，否则 `/etc/cron.d/mujian-metrics`）、创建
`/usr/local/bin/mujian-metrics-report` 命令链接，并试运行一次验证。

若宿主机 CPU 余量紧张，把 `--interval` 调到 5，开销与存储都降为 1/5。

### 2.3 配置 Nginx（**必须手动完成**）

采集器需要 access log 带 `request_time` / `upstream_response_time` 才能算出响应时间。
完整说明见 `scripts/metrics/nginx-mujian.conf`，核心两段：

```nginx
# http { } 块内
log_format mujian escape=default
    '$remote_addr [$time_local] p=$uri '
    'urt=$upstream_response_time s=$status b=$body_bytes_sent rt=$request_time';

# 站点 server { } 块内
access_log /var/log/nginx/mujian.access.log mujian buffer=64k flush=5s;
```

```bash
nginx -t && systemctl reload nginx
```

格式设计要点：

- 用 `k=v` 标签而非默认 combined 格式 —— combined 里的 `$http_user_agent` 含空格，
  按空格切分后字段位置不固定，无法稳定定位状态码
- `rt` / `b` / `s` 三个单 token 字段固定在**行尾**，从行尾定位永远可靠
- `urt` 放在 `s`/`b` **之前** —— 多上游时它会输出 `0.100, 0.200`（含空格），
  放在状态码之前可确保行尾三字段位置不受影响
- 省略 `$http_referer` / `$http_user_agent` —— 指标用不到，去掉可省约 40% 日志体积

**未配置前采集器不会报错**，但 `nginx_*` 延迟类指标为空，并输出
`nginx_log_format_ok=0` 作为自检提示。

### 2.4 验证

```bash
mujian-metrics-report                      # 最近 24 小时报告
mujian-metrics-report --hours 168          # 最近 7 天
mujian-metrics-report --prom               # Prometheus 快照
mujian-metrics-report --last               # 最近一次采集的全部原始字段

sh /usr/local/lib/mujian-metrics/collect.sh --self-test   # 解析逻辑自检
```

### 2.5 卸载

```bash
sudo sh install.sh --uninstall     # 移除定时任务与命令链接，保留已采集数据
```

---

## 3. 指标项清单

### 3.1 宿主机（前缀 `host_`）— 每轮采集

| 指标 | 含义 | 单位 | 来源 |
|------|------|------|------|
| `host_cpu_util_pct` | CPU 总使用率（含 user/system/iowait 等，排除 idle） | % | `/proc/stat` 相邻两次采样差值 |
| `host_load1` / `host_load5` / `host_load15` | 系统平均负载（1/5/15 分钟） | 个 | `/proc/loadavg` |
| `host_mem_total_mb` | 物理内存总量 | MB | `/proc/meminfo` |
| `host_mem_available_mb` | 可用内存（含可回收的 page cache，比 MemFree 更能反映真实余量） | MB | `MemAvailable` |
| `host_mem_used_mb` | 已用内存 = 总量 − 可用 | MB | 计算 |
| `host_mem_used_pct` | 内存使用率 | % | 计算 |
| `host_swap_used_mb` | 已用交换分区（无 swap 时不输出） | MB | `SwapTotal − SwapFree` |
| `host_disk_used_mb{mount="…"}` | 分区已用空间 | MB | `df -Pk` |
| `host_disk_avail_mb{mount="…"}` | 分区可用空间 | MB | `df -Pk` |
| `host_disk_used_pct{mount="…"}` | 分区使用率 | % | `df -Pk` |
| `host_uptime_s` | 宿主机连续运行时长 | 秒 | `/proc/uptime` |

> 监控路径默认 `/` 与 `/var`，可用 `MUJIAN_DISK_PATHS` 覆盖。同一文件系统上的多个
> 路径会自动去重，避免产生重复样本。

### 3.2 Docker 容器（前缀 `container_`）— 每轮采集

| 指标 | 含义 | 单位 | 来源 |
|------|------|------|------|
| `container_up` | 容器是否可观测到（1=正常，0=容器不存在或 cgroup 不可读） | 0/1 | cgroup 目录探测 |
| `container_cpu_pct` | 容器 CPU 使用率，**100% = 打满 1 个逻辑核**（4 核满载为 400%） | % | cgroup `cpu.stat` 的 `usage_usec` 差值 ÷ 采样间隔 |
| `container_mem_used_mib` | 容器当前内存占用 | MiB | cgroup `memory.current` |
| `container_mem_limit_mib` | 容器内存上限（未设限时不输出） | MiB | cgroup `memory.max` |
| `container_mem_pct` | 内存占上限百分比（未设限时不输出） | % | 计算 |
| `container_pids` | 容器内进程/线程数 | 个 | cgroup `pids.current` |
| `container_status` | 运行状态（`running` / `exited` 等） | 字符串 | **低频**，`docker inspect` |
| `container_restart_total` | 累计重启次数（判断容器反复崩溃的关键信号） | 次 | **低频**，`docker inspect` |
| `container_net_rx_mib` / `container_net_tx_mib` | 容器网络累计收/发流量 | MiB | **低频**，`docker stats` |
| `container_blk_read_mib` / `container_blk_write_mib` | 容器块设备累计读/写量 | MiB | **低频**，`docker stats` |

同时支持 **cgroup v2**（Debian 12/13 默认，`/sys/fs/cgroup/system.slice/docker-<id>.scope/`）
与 **cgroup v1**（`/sys/fs/cgroup/{cpuacct,memory,pids}/docker/<id>/`）自动识别。
若 cgroup 挂在非标准位置，用 `MUJIAN_CGROUP_ROOT` 覆盖。

> 容器 ID 缓存在 `state/cid`，只有缓存失效（容器重建 → cgroup 目录消失）时才调用
> 一次 `docker inspect`（约 25 ms）。**稳态下每轮采集零 docker 调用。**
>
> 网络/块设备 IO 单次约 2 秒，严格限定在低频路径；设 `MUJIAN_COLLECT_IO=0` 可完全关闭。

### 3.3 Nginx 代理层（前缀 `nginx_`）— 每轮采集

每个计数型指标同时给出两种形式：

- `*_interval` — **本采集区间内的增量**（看「这一分钟发生了什么」）
- `*_total` — **自部署起的累计值**（单调递增，符合 Prometheus counter 语义）

| 指标 | 含义 | 单位 |
|------|------|------|
| `nginx_requests_*` | 请求总量 | 次 |
| `nginx_responses_2xx_*` / `3xx` / `4xx` / `5xx` | 按状态码分类的响应数（错误数看 4xx/5xx） | 次 |
| `nginx_bytes_sent_*` | 发送给客户端的字节数 | B |
| `nginx_slow_requests_*` | 慢请求数（响应 > 1 s，阈值可用 `MUJIAN_SLOW_MS` 调） | 次 |
| `nginx_request_duration_avg_ms` | 区间内平均响应时间 | ms |
| `nginx_request_duration_p50_ms` / `p95` / `p99` | 响应时间分位数 | ms |
| `nginx_request_duration_max_ms` | 区间内最大响应时间 | ms |
| `nginx_upstream_duration_avg_ms` | 上游（mujian）平均处理时间，扣除了 Nginx 自身开销 | ms |
| `nginx_upstream_duration_samples` | 参与上游时间统计的样本数 | 个 |
| `nginx_log_readable` | access log 是否可读（0 = 路径配错） | 0/1 |
| `nginx_log_rotated` | 本轮是否检测到日志轮转 | 0/1 |
| `nginx_log_partial_bytes` | 被跳过的未写完半行字节数（下次补齐后会计入） | B |
| `nginx_log_malformed_*` | 无法解析的行数 | 行 |
| `nginx_log_format_ok` | **配置自检**：日志格式是否符合要求（0 = 需检查 log_format） | 0/1 |

### 3.4 业务数据规模（前缀 `mujian_`）

**每轮采集**（全部来自 `/api/stats`，响应体约 100 B，是唯一高频轮询的业务接口）：

| 指标 | 含义 | 单位 |
|------|------|------|
| `mujian_records_total` | 演出记录总数 —— **核心规模指标** | 条 |
| `mujian_records_delta` | 相比上一次采样的净增量 | 条 |
| `mujian_cities_total` | 去重的城市数 | 个 |
| `mujian_total_cost` | 累计花费 | 元 |
| `mujian_avg_rating` | 平均评分 | 分 |
| `mujian_api_up` | `/api/stats` 是否返回 200 | 0/1 |
| `mujian_stats_fetch_ms` | `/api/stats` 端到端耗时（含 HTTP 栈，是真实可用性的直接体现） | ms |
| `mujian_healthz_http_code` | `/healthz` 状态码（区分「服务挂了」与「接口异常」） | 码 |

**低频采集（每小时）** —— 这两个接口响应较大（约 160 KB / 45 KB），故不放高频路径：

| 指标 | 含义 | 单位 |
|------|------|------|
| `mujian_artists_total` | 演员档案总数 | 个 |
| `mujian_dramas_total` | 剧目档案总数 | 个 |
| `mujian_data_volume_mb` | 数据卷总占用 | MB |
| `mujian_db_size_mb` | SQLite 主库大小（WAL 另计） | MB |
| `mujian_uploads_size_mb` | 封面图总占用 | MB |
| `mujian_cover_files_total` | 封面文件数（含缩略图） | 个 |

> **增长趋势**由 `report.sh` 基于 `mujian_records_total` 的时间序列差分计算，
> 输出「窗口内净增」与「折算增速（条/天）」。

### 3.5 采集器自身

| 指标 | 含义 |
|------|------|
| `collector_missing_fields` | 本轮采集失败的字段数。**非零即说明有指标悄悄采不到**，是排查采集问题的第一入口 |
| `interval_s` | 距上次采样的实际间隔（秒）。显著大于配置值说明系统负载高或定时任务被延迟 |

---

## 4. 输出格式

### 4.1 `metrics.jsonl`（结构化日志）

每行一个 JSON 对象，扁平结构，便于 `jq` / Python / 任何日志系统消费：

```json
{"ts":1787944760,"ts_iso":"2026-08-28T19:19:20Z","host":"mujian-host","interval_s":60,
 "host_cpu_util_pct":3.42,"host_mem_used_pct":5.46,"container_cpu_pct":0.35,
 "nginx_requests_interval":7,"nginx_request_duration_p95_ms":64.0,
 "mujian_records_total":298,"mujian_api_up":1,"collector_missing_fields":0}
```

按日轮转为 `metrics-YYYY-MM-DD.jsonl`，默认保留 30 天（`MUJIAN_METRICS_KEEP_DAYS`）。
单条约 1.6 KB，60 秒间隔下约 **2.2 MB/天、67 MB/30 天**。

### 4.2 `metrics.prom`（Prometheus 文本暴露格式）

```prometheus
# 幕间（MuJian）指标快照 — Prometheus 文本暴露格式
# 生成时间: 2026-08-28T19:19:20Z   采集间隔: 60s
# TYPE host_cpu_util_pct gauge
host_cpu_util_pct 3.42
# TYPE host_disk_used_pct gauge
host_disk_used_pct{mount="/"} 57
# TYPE nginx_requests_total counter
nginx_requests_total 209
```

符合 Prometheus 暴露格式规范（同一指标名的 `# TYPE` 只声明一次，`_total` 为 counter、
其余为 gauge）。现在可直接查看；将来若要接入 Prometheus，套一个
`node_exporter` 的 textfile collector 或任一个静态文件 server 即可抓取，无需改动采集器。

---

## 5. 开销实测

在 Debian 13 容器中实测（J4105 上预期慢 1.5–2×）：

| 项目 | 实测值 |
|------|--------|
| 单次常规采集（含宿主机 + 容器 + Nginx + 业务四组） | **57 ms** |
| 单次扩展采集（追加实体计数、数据卷 `du`） | 132 ms |
| 报告生成（1440 个样本 / 24 h） | 66 ms |
| 常驻内存 | **0**（无常驻进程） |
| 磁盘（60 s 间隔） | 2.2 MB/天，30 天 67 MB |
| CPU 占空比（60 s 间隔，按 J4105 慢 2× 估算约 115 ms） | **约 0.19% 单核** |
| CPU 占空比（300 s 间隔） | 约 0.04% 单核 |

对比：若用 `docker stats` 做容器采集，单次 **2009 ms**，60 s 间隔下占空比 3.35% ——
高出 17 倍。这正是本方案改用 cgroup 直读的原因。

定时任务已配置 `Nice=10` 与 `IOSchedulingClass=best-effort`（systemd），
确保采集不与业务争抢资源。

---

## 6. 稳定性保证

1. **零代码改动**：不修改 Go 服务、不改 Dockerfile、不改容器启动命令、
   不读写业务数据库（只调现有 HTTP 接口）。
2. **进程隔离**：采集器是独立的短生命周期进程，崩溃或超时都只影响指标。
3. **失败降级**：每一组采集独立执行，任一组失败只让该组指标缺失，
   其余照常产出，并通过 `collector_missing_fields` 暴露。已验证的降级路径：
   - Docker 不可用 → `container_up=0`，其余指标正常
   - Nginx 日志不存在/格式不符 → `nginx_log_readable=0` 或 `nginx_log_format_ok=0`
   - 业务服务不可达 → `mujian_api_up=0`、`mujian_healthz_http_code=0`
4. **状态原子写**：所有状态文件用 `.new` + `mv` 原子替换，进程被中断不会留下半截状态。
5. **无写入放大**：对 nginx 日志只读（按字节偏移增量读取，含轮转检测与半行丢弃），
   不修改不截断；对容器只做文件读取。
6. **可控的进程数**：定时任务 `TimeoutStartSec=30`（扩展 120 s），超时即放弃本轮，
   不会堆积进程。

---

## 7. 故障排查

| 现象 | 排查 |
|------|------|
| `mujian-metrics-report` 提示无数据 | 确认定时任务已运行：`systemctl list-timers mujian-metrics*` 或 `ls -la /var/lib/mujian-metrics/` |
| `nginx_log_format_ok=0` | log_format 未按要求配置，见 2.3 节 |
| `nginx_log_readable=0` | `MUJIAN_NGINX_LOG` 路径不对，或运行 cron 的用户无读权限 |
| `nginx_requests_interval` 恒为 0 | 确认 nginx 已 reload 且新请求写入了该日志文件 |
| `container_up=0` | 容器名不对，或 cgroup 路径非标准 → 试 `MUJIAN_CGROUP_ROOT=/sys/fs/cgroup/unified` |
| `container_cpu_pct` 缺失 | 首样本无差值基准属正常；若持续缺失，检查 `interval_s` 是否 > 0 |
| `collector_missing_fields > 0` | 用 `--last` 看具体缺哪些字段，通常指向上面某一条 |
| 手动跑正常、定时任务没数据 | 环境变量未注入。安装脚本已把配置内联到 crontab / ExecStart；若你手动改过配置，需同步修改 |

手动验证采集器本身：

```bash
# 不写盘，只打印一次采集结果
MUJIAN_METRICS_DIR=/tmp/t MUJIAN_NGINX_LOG=/var/log/nginx/mujian.access.log \
  sh /usr/local/lib/mujian-metrics/collect.sh --dry-run

# 解析逻辑自检（内置样例，不依赖外部文件）
sh /usr/local/lib/mujian-metrics/collect.sh --self-test
```

---

## 8. 文件说明

| 文件 | 用途 |
|------|------|
| `collect.sh` | 采集器主脚本（POSIX sh，零依赖） |
| `report.sh` | 报告查看器（终端报告 + ASCII 趋势图） |
| `install.sh` | 安装 / 卸载定时任务 |
| `nginx-mujian.conf` | Nginx log_format 配置片段与说明 |

所有配置均支持环境变量覆盖，完整列表见 `collect.sh` 顶部的 CONFIG 段。
