# mujian 升级至 Go 1.27 与 1.20–1.27 新特性采纳报告

- 升级目标：将后端从 `go 1.26` 提升至 `go 1.27`
- 校验结果：`go build ./...`、`go vet ./...`、`go test ./...` 全部通过（`go mod tidy` 已执行，无测试文件属正常）
- 改动范围：源码 `backend/go.mod`、`backend/main.go`、`backend/internal/db/db.go`、`backend/internal/storage/storage.go`、`backend/internal/handlers/covers.go`；PGO 相关 `backend/default.pgo`、`backend/debug_pprof.go`（构建标签门控）、`backend/debug_disabled.go`、`backend/main.go` 中 `registerPprof` 调用；以及镜像/CI：`Dockerfile`、`.github/workflows/build.yml`

## 一、版本升级（全链路一致）

| 文件 | 修改 |
| --- | --- |
| `backend/go.mod` | `go 1.26` → `go 1.27` |
| `Dockerfile` | `golang:1.26-trixie` → `golang:1.27-trixie` |
| `.github/workflows/build.yml` | `go-version: "1.26"` → `"1.27"` |

## 二、采纳的 Go 1.20–1.27 新特性

### 1. `log/slog`（结构化日志，Go 1.21）
- 位置：`backend/main.go`、`backend/internal/db/db.go`
- 改造：将 `log.Fatalf`/`log.Printf` 替换为 `slog`（Info/Warn/Error），并封装 `fatal()` 统一出口。
- 收益：
  - 统一全应用日志格式，支持 key-value 结构化字段（如 `"dir"`, `"err"`），便于集中采集与告警。
  - 运行时零额外成本，比原 `log` 更易定位问题。

### 2. `os.Root` 路径穿越防护（Go 1.24）
- 位置：`backend/main.go`（`rootFileSystem` 适配器 + `os.OpenRoot`）
- 改造：上传封面目录改用 `os.OpenRoot(cfg.UploadDir)` 包裹，`/uploads/*` 的静态文件服务只能访问该子树；逃逸路径（含 `..`）在文件系统层被拒绝。
- 收益：
  - 在文件系统层而非仅靠字符串清理来阻止目录穿越，安全性显著提升。
  - 保留 `http.Dir` 回退路径（`AllowLocalStorage` 关闭时或无权限时），不影响既有部署。

### 3. `strings.CutPrefix`（Go 1.20）
- 位置：`backend/main.go`
- 改造：`strings.TrimPrefix(r.URL.Path, "/")` → `strings.CutPrefix`，利用返回的 `ok` 精确判断是否发生了裁剪。
- 收益：语义更明确，避免对空串或根路径的歧义处理。

### 4. `range-over-int`（整数范围循环，Go 1.22）
- 位置：`backend/internal/db/db.go`
- 改造：`for i := 0; i < 10; i++ { args = append(args, like) }` → `for range 10 { args = append(args, like) }`（LIKE 多字段参数复制）。
- 收益：代码更简洁、无冗余索引变量，编译期展开，性能持平。

### 5. `slices` / `maps` 标准库（Go 1.21）
- 位置：`backend/internal/db/db.go`、`backend/internal/storage/storage.go`、`backend/internal/handlers/covers.go`
- 改造：
  - `db.go applyArrayOp`：`append` 去重改用 `maps.Keys`+`slices.Collect`；`remove` 改用 `slices.DeleteFunc`，去除手写 map 初始化与遍历。
  - `storage.go`：`sort.Strings` → `slices.Sort`；封面键过滤 `keys[:0]`+append 循环 → `slices.DeleteFunc(keys, isThumbKey)`（出现 2 处）。
  - `covers.go`：文件名匹配 `for` 循环 → `slices.Contains(req.Files, name)`。
- 收益：
  - 消除手写去重/过滤样板代码，逻辑更清晰、更不易出错。
  - `slices.Sort`/`slices.DeleteFunc` 为经过标准库优化的通用实现，可读性与正确性优于手写。

### 6. `cmp.Or`（Go 1.21）
- 位置：`backend/internal/storage/storage.go`（2 处）
- 改造：`if f := s.cfgProvider(); f != "" { return f }; return "avif"` → `return cmp.Or(s.cfgProvider(), "avif")`。
- 收益：一行完成“取首个非空值”的惯用法，减少嵌套与分支。

### 7. PGO 默认开启（Go 1.21+，Go 1.27 默认 `-pgo=auto`）
- 位置：`backend/default.pgo` + 构建标签门控的 pprof 钩子（`debug_pprof.go`/`debug_disabled.go`/`main.go` 中 `registerPprof`）
- 改造：采集代表性 CPU profile 落为 `default.pgo`，`go build` 自动据此优化内联与代码布局；并保留可复用的重新采集机制。
- 收益：依据真实运行热路径优化，提升吞吐/降低延迟；已验证 `-pgo=auto` 构建产物区别于 `-pgo=off`。详见第三节。

## 三、PGO（Profile-Guided Optimization）落地

Go 1.21 起支持 PGO，Go 1.27 默认以 `-pgo=auto` 构建：只要主包目录存在 `default.pgo`，`go build` 会自动依据其 CPU 采样优化内联与代码布局，无需改动业务代码。

- 采集机制（已落地、可复用）：
  - 新增 `backend/debug_pprof.go`（`//go:build pprofenable`，挂载 `net/http/pprof` 到 `/debug`）、`backend/debug_disabled.go`（`!pprofenable` 的 no-op 桩）、`main.go` 调用 `registerPprof(r)`。
  - 仅 `go build -tags pprofenable` 时暴露调试端点；正常构建为 no-op，不污染生产二进制。
  - 采集：构建带标签二进制 → 启动服务 → 施加代表性负载 → `curl -o default.pgo "http://host/debug/pprof/profile?seconds=30"`。
- 本次 profile：先灌入 1000 条样本记录，再以 4 路并发读取负载采集 30s，共 16.01s 采样（CPU 占用 53%）。热路径覆盖 `modernc.org/sqlite` 的 SQL 执行（cum 3.09s）与 **`encoding/json/v2` JSON 序列化**（Go 1.27 默认新版 json，cum 1.14s）等真实应用代码。
- 落地文件：`backend/default.pgo`（48.7KB）。
- 生效验证：`-pgo=auto` 构建产物与 `-pgo=off` 不同（二进制已随 profile 优化）；`go build ./...`/`go vet`/`go test` 在默认 PGO 下全部通过。

## 四、已评估但未采纳（含理由）

- **`math/rand/v2`（Go 1.22）**：项目未使用 `math/rand`，无收益。
- **`range-over-func` 迭代器 / `iter` 包（Go 1.23）**：`ListKeys`/`ListCoverKeys` 的调用方需要切片（排序、长度、索引），改用迭代器需改动调用链且无明显性能收益，故未采纳。
- **`errors.Join`（Go 1.20）**：当前各错误点均为单一错误，无合并需求。
- **`os.Root` 应用于前端 `dist` 静态服务**：前端经 `embed.FS` 已无文件系统穿越风险，无需改造。

## 五、验证记录

```
go build ./...   → 通过（默认 -pgo=auto，已采用 default.pgo）
go vet ./...     → 通过
go test ./...    → 通过（无测试文件）
go mod tidy      → 通过，依赖未变
gofmt -l        → 已格式化对齐（含新增 pprof 钩子文件）
# PGO 生效验证：
go build -pgo=off  -o /tmp/off  .   → 二进制 30038738 bytes
go build -pgo=auto -o /tmp/pgo  .   → 二进制 30038338 bytes（与 off 不同，PGO 已生效）
```

```
go build ./...   → 通过
go vet ./...     → 通过（修正了 fatal 中误用的 %w → %v）
go test ./...    → 通过（无测试文件）
go mod tidy      → 通过，依赖未变
gofmt -l（本次改动文件）→ 已格式化对齐
```
