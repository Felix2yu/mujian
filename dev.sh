#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BACKEND_DIR="$ROOT/backend"
FRONTEND_DIR="$ROOT/frontend"
BE_PORT=8080
FE_PORT=5173
BE_PID_FILE="/tmp/mujian-be.pid"

# ---------- 端口冲突清理 ----------
for port in $BE_PORT $FE_PORT; do
  pid=$(lsof -ti ":$port" -sTCP:LISTEN 2>/dev/null || true)
  if [ -n "$pid" ]; then
    echo "[dev] 端口 $port 被 PID $pid 占用，先杀掉"
    kill "$pid" 2>/dev/null || true
    sleep 1
  fi
done

# ---------- 退出清理 ----------
cleanup() {
  echo ""
  echo "[dev] 清理中..."
  if [ -f "$BE_PID_FILE" ]; then
    kill "$(cat "$BE_PID_FILE")" 2>/dev/null || true
    rm -f "$BE_PID_FILE"
  fi
  pkill -f "vite.*dev" 2>/dev/null || true
  pkill -f "fswatch.*mujian" 2>/dev/null || true
  wait 2>/dev/null || true
  echo "[dev] 已退出"
  exit 0
}
trap cleanup SIGINT SIGTERM

# ---------- 编译 Go ----------
echo "[dev] 编译 Go 后端..."
( cd "$BACKEND_DIR" && CGO_ENABLED=1 go build -o mujian . ) || { echo "[dev] Go build 失败"; exit 1; }

# ---------- 启动 Go ----------
echo "[dev] 启动后端 (port=$BE_PORT)..."
( cd "$BACKEND_DIR" && ./mujian > /tmp/mujian-backend.log 2>&1 & echo $! > "$BE_PID_FILE" )
for i in $(seq 1 20); do
  curl -s -o /dev/null "http://localhost:$BE_PORT/healthz" && echo "[dev] 后端就绪 ✓" && break
  sleep 0.5
done

# ---------- 启动 SvelteKit dev server ----------
if [ ! -d "$FRONTEND_DIR/node_modules" ]; then
  echo "[dev] 前端依赖未装，pnpm install..."
  ( cd "$FRONTEND_DIR" && pnpm install --frozen-lockfile )
fi

echo "[dev] 启动前端 dev server (port=$FE_PORT, HMR)..."
( cd "$FRONTEND_DIR" && pnpm run dev > /tmp/mujian-frontend.log 2>&1 & )
for i in $(seq 1 30); do
  curl -s -o /dev/null "http://localhost:$FE_PORT" && echo "[dev] 前端就绪 ✓" && break
  sleep 0.5
done

# ---------- fswatch: .go 变动 → rebuild + restart backend ----------
if command -v fswatch &>/dev/null; then
  echo "[dev] fswatch 监听 .go 文件..."
  (
    fswatch -0 --event Updated "$BACKEND_DIR" 2>/dev/null | while IFS= read -r -d "" file; do
      case "$file" in
        *.go)
          echo "[dev] .go 变更 → 重建后端..."
          (
            cd "$BACKEND_DIR"
            CGO_ENABLED=1 go build -o mujian . 2>&1 && {
              old_pid=$(cat "$BE_PID_FILE" 2>/dev/null || true)
              [ -n "$old_pid" ] && kill "$old_pid" 2>/dev/null || true
              sleep 0.3
              ./mujian >> /tmp/mujian-backend.log 2>&1 &
              echo $! > "$BE_PID_FILE"
              echo "[dev] 后端已重启"
            } || echo "[dev] build 失败，等下次保存"
          )
          ;;
      esac
    done
  ) &
fi

echo ""
echo "══════════════════════════════════════════"
echo "  前端 : http://localhost:$FE_PORT  (HMR)"
echo "  后端 : http://localhost:$BE_PORT"
echo "  日志 : tail -f /tmp/mujian-*.log"
echo "  Ctrl+C 全部停掉"
echo "══════════════════════════════════════════"

wait
