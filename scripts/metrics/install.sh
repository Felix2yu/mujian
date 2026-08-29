#!/bin/sh
# install.sh — 安装幕间（MuJian）指标采集定时任务
#
# 本脚本只做三件事：
#   1. 把 collect.sh / report.sh 放到 $PREFIX/lib/mujian-metrics/
#   2. 创建指标输出目录 $DIR
#   3. 注册定时触发（systemd timer 优先，否则 cron.d）
#
# 它不会修改 nginx 配置、不会改动 mujian 容器、不会安装任何常驻服务。
#
# 用法：
#   sudo sh install.sh                          默认：每分钟常规采集 + 每小时扩展采集
#   sudo sh install.sh --interval 5             改为每 5 分钟一次
#   sudo sh install.sh --dir /srv/metrics       自定义指标目录
#   sudo sh install.sh --container my-mujian    自定义容器名
#   sudo sh install.sh --nginx-log /path/log    自定义 nginx 访问日志路径
#   sudo sh install.sh --uninstall              卸载（保留已采集数据）

set -u

PREFIX="${PREFIX:-/usr/local}"
DIR="${DIR:-/var/lib/mujian-metrics}"
INTERVAL="${INTERVAL:-1}"
CONTAINER_NAME="${CONTAINER_NAME:-mujian}"
NGINX_LOG="${NGINX_LOG:-/var/log/nginx/mujian.access.log}"
DATA_VOLUME="${DATA_VOLUME:-/var/lib/docker/volumes/mujian-data/_data}"
UNINSTALL=0

while [ $# -gt 0 ]; do
    case "$1" in
        --dir)        DIR="$2"; shift 2 ;;
        --prefix)     PREFIX="$2"; shift 2 ;;
        --interval)   INTERVAL="$2"; shift 2 ;;
        --container)  CONTAINER_NAME="$2"; shift 2 ;;
        --nginx-log)  NGINX_LOG="$2"; shift 2 ;;
        --data-volume) DATA_VOLUME="$2"; shift 2 ;;
        --uninstall)  UNINSTALL=1; shift ;;
        -h|--help)    sed -n '2,22p' "$0" | sed 's/^#\{1,2\} \{0,1\}//'; exit 0 ;;
        *) echo "未知参数: $1" >&2; exit 2 ;;
    esac
done

SRC_DIR=$(cd "$(dirname "$0")" && pwd)
LIB_DIR="$PREFIX/lib/mujian-metrics"
ENV_FILE="$LIB_DIR/mujian-metrics.env"

# ------------------------------------------------------------ UNINSTALL ----
if [ "$UNINSTALL" -eq 1 ]; then
    echo "卸载幕间指标采集…"
    if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
        systemctl disable --now mujian-metrics.timer 2>/dev/null
        systemctl disable --now mujian-metrics-extended.timer 2>/dev/null
        rm -f /etc/systemd/system/mujian-metrics.service \
              /etc/systemd/system/mujian-metrics.timer \
              /etc/systemd/system/mujian-metrics-extended.service \
              /etc/systemd/system/mujian-metrics-extended.timer
        systemctl daemon-reload 2>/dev/null
        echo "  已移除 systemd timer"
    fi
    rm -f /etc/cron.d/mujian-metrics
    rm -f "$PREFIX/bin/mujian-metrics-report"
    echo "  已移除定时任务与命令链接"
    echo
    echo "已采集数据保留在: $DIR（确认不再需要后请手动删除）"
    exit 0
fi

# ------------------------------------------------------------ PRECHECK -----
if [ "$(id -u)" -ne 0 ]; then
    echo "请以 root 运行（需要写入 $LIB_DIR、$DIR 与定时任务配置）。" >&2
    echo "  sudo sh $0" >&2
    exit 1
fi

echo "检查依赖…"
_missing=""
for c in awk sed sort df curl mktemp; do
    if command -v "$c" >/dev/null 2>&1; then
        echo "  [ok]   $c"
    else
        echo "  [缺失] $c"
        _missing="$_missing $c"
    fi
done
if command -v docker >/dev/null 2>&1; then echo "  [ok]   docker（扩展指标需要）"
else echo "  [提示] docker 不可用 —— 扩展模式下的重启次数/网络 IO 将不可用，不影响常规采集"; fi
[ -n "$_missing" ] && { echo "缺少必需命令:$_missing，请先安装后重试。" >&2; exit 1; }

# ------------------------------------------------------------- INSTALL -----
echo
echo "安装脚本到 $LIB_DIR …"
mkdir -p "$LIB_DIR"
cp "$SRC_DIR/collect.sh" "$LIB_DIR/collect.sh"
cp "$SRC_DIR/report.sh"  "$LIB_DIR/report.sh"
cp "$SRC_DIR/nginx-mujian.conf" "$LIB_DIR/nginx-mujian.conf" 2>/dev/null
chmod 0755 "$LIB_DIR/collect.sh" "$LIB_DIR/report.sh"

mkdir -p "$DIR/state"
chmod 0755 "$DIR"

# 采集器不读环境文件（cron 环境极简），配置直接内联到定时任务的 ExecStart /
# crontab 行里，避免「手动跑正常、定时任务跑不出数据」这类问题。
ENV_EXPORT="MUJIAN_METRICS_DIR=$DIR MUJIAN_CONTAINER=$CONTAINER_NAME MUJIAN_NGINX_LOG=$NGINX_LOG MUJIAN_DATA_VOLUME=$DATA_VOLUME"

if [ "$INTERVAL" -gt 1 ]; then
    CRON_SCHED="*/$INTERVAL * * * *"
else
    CRON_SCHED="* * * * *"
fi

echo
echo "注册定时触发…"
if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    cat > /etc/systemd/system/mujian-metrics.service <<EOF
[Unit]
Description=幕间（MuJian）指标采集
Documentation=file://$LIB_DIR/nginx-mujian.conf

[Service]
Type=oneshot
Environment=$ENV_EXPORT
ExecStart=/bin/sh $LIB_DIR/collect.sh
# 采集本身只需数十毫秒；留足余量，超时则放弃本轮，不堆积进程
TimeoutStartSec=30
# 低性能设备上避免与业务争抢 CPU
Nice=10
IOSchedulingClass=best-effort
IOSchedulingPriority=7
EOF

    cat > /etc/systemd/system/mujian-metrics.timer <<EOF
[Unit]
Description=每 ${INTERVAL} 分钟采集一次幕间指标

[Timer]
OnBootSec=2min
OnUnitActiveSec=${INTERVAL}min
Persistent=false
# 分散触发时刻，避免与整点的其他任务同时启动
RandomizedDelaySec=10

[Install]
WantedBy=timers.target
EOF

    cat > /etc/systemd/system/mujian-metrics-extended.service <<EOF
[Unit]
Description=幕间（MuJian）扩展指标采集（低频重指标）

[Service]
Type=oneshot
Environment=$ENV_EXPORT
ExecStart=/bin/sh $LIB_DIR/collect.sh --extended
TimeoutStartSec=120
Nice=10
IOSchedulingClass=best-effort
IOSchedulingPriority=7
EOF

    cat > /etc/systemd/system/mujian-metrics-extended.timer <<EOF
[Unit]
Description=每小时采集一次幕间扩展指标

[Timer]
OnBootSec=5min
OnUnitActiveSec=60min
Persistent=false
RandomizedDelaySec=60

[Install]
WantedBy=timers.target
EOF

    systemctl daemon-reload
    systemctl enable --now mujian-metrics.timer >/dev/null 2>&1
    systemctl enable --now mujian-metrics-extended.timer >/dev/null 2>&1
    echo "  已启用 systemd timer（常规 ${INTERVAL} 分钟 / 扩展 60 分钟）"
    systemctl list-timers mujian-metrics*.timer --no-pager 2>/dev/null | sed 's/^/  /'
else
    mkdir -p /etc/cron.d
    cat > /etc/cron.d/mujian-metrics <<EOF
# 幕间（MuJian）指标采集
# 常规采集：每 ${INTERVAL} 分钟；扩展采集：每小时第 17 分
SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin
$CRON_SCHED root $ENV_EXPORT /bin/sh $LIB_DIR/collect.sh
17 * * * * root $ENV_EXPORT /bin/sh $LIB_DIR/collect.sh --extended
EOF
    chmod 0644 /etc/cron.d/mujian-metrics
    echo "  已安装 /etc/cron.d/mujian-metrics"
fi

ln -sf "$LIB_DIR/report.sh" "$PREFIX/bin/mujian-metrics-report"
echo "  命令链接: $PREFIX/bin/mujian-metrics-report"

# ------------------------------------------------------------- VERIFY ------
echo
echo "试运行一次…"
if env $ENV_EXPORT sh "$LIB_DIR/collect.sh" >/dev/null 2>&1; then
    echo "  采集成功。最新快照："
    sed -n '2,3p' "$DIR/metrics.prom" | sed 's/^/    /'
    echo "  指标条目数: $(tail -1 "$DIR/metrics.jsonl" | tr ',' '\n' | grep -c ':')"
else
    echo "  [警告] 采集未成功。请检查上面的路径配置。" >&2
fi

cat <<EOF

==============================================================================
 安装完成
==============================================================================
 指标目录   $DIR
            ├── metrics.jsonl        时序历史（按日轮转，默认保留 30 天）
            ├── metrics.prom         最新快照（Prometheus 文本格式）
            └── state/               采集状态（日志偏移、CPU 基准等）

 查看报告   mujian-metrics-report
            mujian-metrics-report --hours 168
            mujian-metrics-report --prom

------------------------------------------------------------------------------
 还需你手动完成一步：配置 Nginx 日志格式
------------------------------------------------------------------------------
 采集器需要 access log 带 request_time / upstream_response_time 才能算出
 响应时间。请把下面两段加入 nginx 配置（详见
 $LIB_DIR/nginx-mujian.conf）：

   http {
       log_format mujian escape=default
           '\$remote_addr [\$time_local] p=\$uri '
           'urt=\$upstream_response_time s=\$status b=\$body_bytes_sent rt=\$request_time';
   }
   server {
       access_log /var/log/nginx/mujian.access.log mujian buffer=64k flush=5s;
   }

 然后： nginx -t && systemctl reload nginx

 未配置前采集器不会报错，但 nginx_* 延迟类指标为空，且会输出
 nginx_log_format_ok=0 提示你。可用下面命令确认是否已生效：
   mujian-metrics-report --last | grep nginx_log_format_ok
==============================================================================
EOF
