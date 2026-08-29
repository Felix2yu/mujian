#!/bin/sh
# collect.sh — 幕间（MuJian）轻量指标采集器
#
# 设计约束：
#   1. 零侵入：只读 /proc、docker CLI、nginx access log 与现有 /api/stats 接口。
#      不修改 mujian 服务任何代码、不修改容器启动命令、不写入业务数据库。
#   2. 极低开销：单次运行约 0.15–0.3 s，默认 60 s 一次，CPU 占空比 < 0.5%，
#      无常驻进程（由 cron / systemd timer 触发）。
#   3. 依赖仅 POSIX 工具：sh / awk / sed / sort / df / curl / docker，不引入
#      Prometheus / node_exporter / Grafana 等常驻组件。
#
# 输出：
#   $OUT_DIR/metrics.jsonl        时序历史，每行一个 JSON 采样点（结构化日志）
#   $OUT_DIR/metrics.prom         最新快照，Prometheus 文本暴露格式
#
# 用法：
#   collect.sh                 常规采集（建议每分钟）
#   collect.sh --extended      追加低频重指标（建议每小时）：实体计数、数据卷体积
#   collect.sh --dry-run       仅打印 JSON 到 stdout，不写盘
#   collect.sh --self-test     用内置样例校验 nginx 日志解析与输出格式
#
# 所有配置均可用环境变量覆盖，详见下方 CONFIG 段。

# ---------------------------------------------------------------- CONFIG ----
CONTAINER="${MUJIAN_CONTAINER:-mujian}"
STATS_URL="${MUJIAN_STATS_URL:-http://127.0.0.1:8080/api/stats}"
HEALTH_URL="${MUJIAN_HEALTH_URL:-http://127.0.0.1:8080/healthz}"
ARTISTS_URL="${MUJIAN_ARTISTS_URL:-http://127.0.0.1:8080/api/artists}"
DRAMAS_URL="${MUJIAN_DRAMAS_URL:-http://127.0.0.1:8080/api/dramas}"
NGINX_LOG="${MUJIAN_NGINX_LOG:-/var/log/nginx/mujian.access.log}"
OUT_DIR="${MUJIAN_METRICS_DIR:-/var/lib/mujian-metrics}"
DATA_VOLUME="${MUJIAN_DATA_VOLUME:-/var/lib/docker/volumes/mujian-data/_data}"
DISK_PATHS="${MUJIAN_DISK_PATHS:-/ /var}"
KEEP_DAYS="${MUJIAN_METRICS_KEEP_DAYS:-30}"
CURL_MAX_TIME="${MUJIAN_CURL_MAX_TIME:-5}"
SLOW_MS="${MUJIAN_SLOW_MS:-1000}"

# --------------------------------------------------------------- GLOBALS ----
MODE=""
TS=""
TS_ISO=""
MISSING=0
M=""      # 数值/字符串指标：name<TAB>value<TAB>type
TMPD=""
TAB=$(printf '\t')

cleanup() { [ -n "$TMPD" ] && rm -rf "$TMPD"; }
trap cleanup EXIT INT TERM

# put <name> <json-value> <n|s>   n=数值（同时写入 prom），s=字符串（仅 json）
put() {
    [ -n "$2" ] || { MISSING=$((MISSING + 1)); return 0; }
    printf '%s\t%s\t%s\n' "$1" "$2" "$3" >> "$M"
}
putn() { put "$1" "$2" n; }
puts() { put "$1" "\"$2\"" s; }

# ------------------------------------------------------------- UTILITIES ----
# to_mib "45.2MiB" -> 45.2 ; "3.84GiB" -> 3932.16 ; "1.2MB" -> 1.14
# 统一折算为 MiB，便于跨指标比较。docker 对 NetIO/BlockIO 用 1000 进制
# （MB/GB），对 MemUsage 用 1024 进制（KiB/MiB/GiB），此处分别换算。
to_mib() {
    # 先去掉空白：`docker stats` 的 "A / B" 形式经 cut 切分后会带前导/
    # 尾随空格，不清理会导致下面的单位分支全部匹配失败而静默返回原值。
    _s=$(printf '%s' "$1" | tr -d ' \t')
    _v=$(printf '%s' "$_s" | sed 's/[^0-9.]//g')
    _u=$(printf '%s' "$_s" | sed 's/[0-9.]//g')
    [ -n "$_v" ] || return 0
    case "$_u" in
        B|'')   awk -v v="$_v" 'BEGIN{printf "%.2f", v/1048576}' ;;
        kB|KB)  awk -v v="$_v" 'BEGIN{printf "%.2f", v*1000/1048576}' ;;
        KiB)    awk -v v="$_v" 'BEGIN{printf "%.2f", v/1024}' ;;
        MB)     awk -v v="$_v" 'BEGIN{printf "%.2f", v*1000000/1048576}' ;;
        MiB)    awk -v v="$_v" 'BEGIN{printf "%.2f", v}' ;;
        GB)     awk -v v="$_v" 'BEGIN{printf "%.2f", v*1000000000/1048576}' ;;
        GiB)    awk -v v="$_v" 'BEGIN{printf "%.2f", v*1024}' ;;
        *)      printf '%s' "$_v" ;;
    esac
}

# fieldnum "12.3%" -> 12.3
strip_pct() { printf '%s' "$1" | sed 's/[% ]//g'; }

# ---------------------------------------------------------- HOST METRICS ----
# CPU 使用率按「与上次采样的 /proc/stat 差值」计算，无需在脚本内 sleep 等待，
# 也不引入常驻采样进程。首次运行无历史值时输出空（该点被跳过）。
collect_host_cpu() {
    [ -r /proc/stat ] || return 0
    _line=$(awk '/^cpu /{print; exit}' /proc/stat) || return 0
    [ -n "$_line" ] || return 0
    _cur=$(printf '%s\n' "$_line" | awk '{
        t=0; for(i=2;i<=NF;i++) t+=$i
        printf "%d %d", t, $5+$6          # total, idle+iowait
    }')
    _prev=""
    [ -f "$WORKD/cpu.prev" ] && _prev=$(cat "$WORKD/cpu.prev")
    printf '%s' "$_cur" > "$WORKD/cpu.prev"
    [ -n "$_prev" ] || return 0
    # 注意 printf 必须带换行：POSIX 的 read 在未遇到换行就到 EOF 时返回非零，
    # 缺了它下面的 while 循环体一次都不会执行，指标会静默消失。
    printf '%s\n%s\n' "$_prev" "$_cur" | awk 'NR==1{p=$1;pi=$2;next}
        { dt=$1-p; di=$2-pi; if(dt>0) printf "%.2f\n", 100*(1-di/dt) }' | \
    while read -r _u; do [ -n "$_u" ] && putn host_cpu_util_pct "$_u"; done
}

collect_host() {
    # 负载（1/5/15 分钟）
    if [ -r /proc/loadavg ]; then
        read -r L1 L5 L15 _r < /proc/loadavg
        putn host_load1  "$L1"
        putn host_load5  "$L5"
        putn host_load15 "$L15"
    fi

    # 内存（kB -> MB）。四个字段合并为一次 awk，减少进程创建开销
    #（低性能设备上进程创建是主要成本之一）。
    if [ -r /proc/meminfo ]; then
        _mm=$(awk '/^MemTotal:/{a=$2} /^MemAvailable:/{b=$2}
                   /^SwapTotal:/{c=$2} /^SwapFree:/{d=$2}
                   END{printf "%d %d %d %d", a, b, c, d}' /proc/meminfo)
        _mt=${_mm%% *}; _r=${_mm#* }
        _ma=${_r%% *};   _r=${_r#* }
        _st=${_r%% *};   _sf=${_r#* }
        if [ -n "$_mt" ] && [ "$_mt" -gt 0 ] && [ -n "$_ma" ]; then
            putn host_mem_total_mb "$(awk -v v="$_mt" 'BEGIN{printf "%.0f", v/1024}')"
            putn host_mem_available_mb "$(awk -v v="$_ma" 'BEGIN{printf "%.0f", v/1024}')"
            putn host_mem_used_mb "$(awk -v t="$_mt" -v a="$_ma" 'BEGIN{printf "%.0f", (t-a)/1024}')"
            putn host_mem_used_pct "$(awk -v t="$_mt" -v a="$_ma" 'BEGIN{printf "%.2f", 100*(t-a)/t}')"
        fi
        if [ -n "$_st" ] && [ "$_st" -gt 0 ] && [ -n "$_sf" ]; then
            putn host_swap_used_mb "$(awk -v t="$_st" -v f="$_sf" 'BEGIN{printf "%.0f", (t-f)/1024}')"
        fi
    fi

    # 磁盘（所有挂载点一次 df 调用）
    # 按挂载点去重：多个被监控路径可能落在同一文件系统上（如 / 与 /var），
    # 不去重会产生完全相同的 label 组合，Prometheus 侧视为重复样本。
    _df=$(df -Pk $DISK_PATHS 2>/dev/null | awk -F' ' 'NR>1 && !seen[$6]++ {
        print $3"\t"$4"\t"$5"\t"$6
    }')
    if [ -n "$_df" ]; then
        printf '%s\n' "$_df" | while IFS="$TAB" read -r _u _a _c _mp; do
            [ -n "$_mp" ] || continue
            _tag=$(printf '%s' "$_mp" | sed 's|^/$|/|')
            putn "host_disk_used_mb{mount=\"$_mp\"}" "$(awk -v v="$_u" 'BEGIN{printf "%.0f", v/1024}')"
            putn "host_disk_avail_mb{mount=\"$_mp\"}" "$(awk -v v="$_a" 'BEGIN{printf "%.0f", v/1024}')"
            putn "host_disk_used_pct{mount=\"$_mp\"}" "$(printf '%s' "$_c" | sed 's/%//')"
        done
    fi

    # 运行时长（秒）
    [ -r /proc/uptime ] && putn host_uptime_s "$(awk '{printf "%.0f", $1}' /proc/uptime)"
}

# ----------------------------------------------------- CONTAINER METRICS ----
#
# 重要：不要用 `docker stats --no-stream` 做高频采集。
# 实测该命令单次耗时约 2 秒（它打开的是 stats 流式接口，--no-stream 只是取
# 第一个数据帧，仍需等待一次采样周期）。按 60s 间隔即 3.3% 单核占空比，
# 与「极低开销」目标冲突。
#
# 因此稳态采集改为直接读 cgroup 伪文件（几次文件读取，约 1–3 ms）：
#   cgroup v2（Debian 12/13 + systemd 默认）：
#     /sys/fs/cgroup/system.slice/docker-<id>.scope/{cpu.stat,memory.current,memory.max,pids.current}
#   cgroup v1：
#     /sys/fs/cgroup/{cpuacct,memory,pids}/docker/<id>/...
#
# 容器 ID 缓存在状态目录里，只有缓存失效（容器重建 → 目录消失）时才调用
# 一次 docker inspect（约 25 ms）。稳态下每轮采集零 docker 调用。
CGV2_DIR=""
CGV1_ID=""
# cgroup 挂载根。默认 /sys/fs/cgroup；少数宿主机会挂到 /sys/fs/cgroup/unified，
# 此时用 MUJIAN_CGROUP_ROOT 覆盖即可。
CGROOT="${MUJIAN_CGROUP_ROOT:-/sys/fs/cgroup}"

probe_cgroup() {
    CGV2_DIR=""; CGV1_ID=""
    for _p in "$CGROOT/system.slice/docker-$1.scope" \
              "$CGROOT/docker/$1" \
              "$CGROOT/systemd/system.slice/docker-$1.scope" \
              "$CGROOT/system.slice/libpod-$1.scope"; do
        if [ -d "$_p" ] && [ -f "$_p/cpu.stat" ]; then CGV2_DIR="$_p"; return 0; fi
    done
    if [ -d "$CGROOT/memory/docker/$1" ] || [ -d "$CGROOT/cpuacct/docker/$1" ]; then
        CGV1_ID="$1"; return 0
    fi
    return 1
}

resolve_cgroup() {
    _cid=""
    [ -f "$WORKD/cid" ] && _cid=$(cat "$WORKD/cid" 2>/dev/null)
    [ -n "$_cid" ] && probe_cgroup "$_cid" && return 0
    command -v docker >/dev/null 2>&1 || return 1
    _cid=$(docker inspect -f '{{.Id}}' "$CONTAINER" 2>/dev/null)
    [ -n "$_cid" ] || return 1
    printf '%s' "$_cid" > "$WORKD/cid.new" && mv "$WORKD/cid.new" "$WORKD/cid"
    probe_cgroup "$_cid"
}

# container_cpu_pct 由 usage_usec 的两次差值算得，语义与 docker stats 一致
# （100% = 打满一个逻辑核；4 核机器满载为 400%）。
emit_container_cpu() {
    _uu="$1"
    [ -n "$_uu" ] || return 0
    _pu=""
    [ -f "$WORKD/cpu_usage" ] && _pu=$(cat "$WORKD/cpu_usage" 2>/dev/null)
    if [ -n "$_pu" ] && [ "${INTERVAL_S:-0}" -gt 0 ]; then
        putn container_cpu_pct "$(awk -v c="$_uu" -v p="$_pu" -v i="$INTERVAL_S" \
            'BEGIN{d=(c-p)/1000000; if(d<0)d=0; printf "%.2f", 100*d/i}')"
    fi
    printf '%s' "$_uu" > "$WORKD/cpu_usage.new" && mv "$WORKD/cpu_usage.new" "$WORKD/cpu_usage"
}

collect_container() {
    resolve_cgroup || { putn container_up 0; return 0; }
    putn container_up 1

    if [ -n "$CGV2_DIR" ]; then
        _uu=""
        [ -f "$CGV2_DIR/cpu.stat" ] &&
            _uu=$(awk '/^usage_usec /{print $2; exit}' "$CGV2_DIR/cpu.stat")
        emit_container_cpu "$_uu"

        if [ -f "$CGV2_DIR/memory.current" ]; then
            read -r _mc < "$CGV2_DIR/memory.current"
            putn container_mem_used_mib "$(awk -v v="$_mc" 'BEGIN{printf "%.2f", v/1048576}')"
        fi
        if [ -f "$CGV2_DIR/memory.max" ]; then
            read -r _mx < "$CGV2_DIR/memory.max"
            # 未设内存上限时该文件内容为 "max"
            if [ -n "$_mx" ] && [ "$_mx" != "max" ]; then
                putn container_mem_limit_mib "$(awk -v v="$_mx" 'BEGIN{printf "%.2f", v/1048576}')"
                if [ -n "$_mc" ] && [ "$_mx" -gt 0 ]; then
                    putn container_mem_pct "$(awk -v u="$_mc" -v m="$_mx" \
                        'BEGIN{printf "%.2f", 100*u/m}')"
                fi
            fi
        fi
        [ -f "$CGV2_DIR/pids.current" ] && { read -r _pc < "$CGV2_DIR/pids.current"; putn container_pids "$_pc"; }
        return 0
    fi

    # cgroup v1 回退路径
    if [ -n "$CGV1_ID" ]; then
        if [ -f "$CGROOT/cpuacct/docker/$CGV1_ID/cpuacct.usage" ]; then
            read -r _ns < "$CGROOT/cpuacct/docker/$CGV1_ID/cpuacct.usage"
            emit_container_cpu "$(awk -v v="$_ns" 'BEGIN{printf "%.0f", v/1000}')"
        fi
        if [ -f "$CGROOT/memory/docker/$CGV1_ID/memory.usage_in_bytes" ]; then
            read -r _mu < "$CGROOT/memory/docker/$CGV1_ID/memory.usage_in_bytes"
            putn container_mem_used_mib "$(awk -v v="$_mu" 'BEGIN{printf "%.2f", v/1048576}')"
        fi
        if [ -f "$CGROOT/memory/docker/$CGV1_ID/memory.limit_in_bytes" ]; then
            read -r _ml < "$CGROOT/memory/docker/$CGV1_ID/memory.limit_in_bytes"
            # 未设限时是一个接近 2^63 的巨大值，视为无限制
            if [ -n "$_ml" ] && [ "$_ml" -lt 1099511627776 ]; then
                putn container_mem_limit_mib "$(awk -v v="$_ml" 'BEGIN{printf "%.2f", v/1048576}')"
                [ -n "$_mu" ] && [ "$_ml" -gt 0 ] &&
                    putn container_mem_pct "$(awk -v u="$_mu" -v m="$_ml" 'BEGIN{printf "%.2f", 100*u/m}')"
            fi
        fi
        if [ -f "$CGROOT/pids/docker/$CGV1_ID/pids.current" ]; then
            read -r _pc < "$CGROOT/pids/docker/$CGV1_ID/pids.current"
            putn container_pids "$_pc"
        fi
    fi
}

# --------------------------------------------------------- NGINX METRICS ----
# 增量解析 access log：只读取上次偏移之后的新增字节，日志轮转（inode 变化或
# 文件变小）时自动从头开始。
#
# 期望的 log_format（见 nginx-mujian.conf）：
#   $remote_addr [$time_local] p=$uri urt=$upstream_response_time \
#   s=$status b=$body_bytes_sent rt=$request_time
#
# 关键设计：rt / b / s 三个字段固定在行尾且均为单 token，因此从行尾定位
# 永远可靠；$uri 与 $upstream_response_time 可能含空格（多上游时为
# "0.1, 0.2"），故采用前向扫描提取，且失败时降级为「未知」而非算错。
collect_nginx() {
    NGINX_CONF_OK=0
    [ -r "$NGINX_LOG" ] || { putn nginx_log_readable 0; return 0; }
    putn nginx_log_readable 1

    _size=$(wc -c < "$NGINX_LOG" | tr -d ' ')
    _ino=$(ls -i "$NGINX_LOG" 2>/dev/null | awk '{print $1}')
    [ -n "$_ino" ] || _ino=0

    _off=0
    _pino=0
    _first=1
    if [ -f "$WORKD/nginx.off" ]; then
        _first=0
        _off=$(cut -d: -f1 < "$WORKD/nginx.off" 2>/dev/null)
        _pino=$(cut -d: -f2 < "$WORKD/nginx.off" 2>/dev/null)
    fi
    [ -n "$_off" ] || _off=0
    [ -n "$_pino" ] || _pino=0

    # 轮转检测：inode 变化，或文件被截断（新 size < 旧 offset）。
    # 首次运行没有历史状态，不算轮转（否则会误报一次）。
    _rotated=0
    if [ "$_first" -eq 0 ] && { [ "$_pino" != "$_ino" ] || [ "$_size" -lt "$_off" ]; }; then
        _off=0
        _rotated=1
    fi
    putn nginx_log_rotated "$_rotated"

    _chunk="$TMPD/nginx.chunk"
    : > "$_chunk"
    if [ "$_size" -gt "$_off" ]; then
        tail -c "+$((_off + 1))" "$NGINX_LOG" > "$_chunk" 2>/dev/null
    fi

    # 丢弃末尾未写完的半行（避免把截断的请求当成一条完整记录）
    _partial=0
    _csize=$(wc -c < "$_chunk" | tr -d ' ')
    if [ "$_csize" -gt 0 ]; then
        if [ "$(tail -c 1 "$_chunk" | wc -l)" -eq 0 ]; then
            _partial=$(tail -n 1 "$_chunk" | wc -c | tr -d ' ')
            sed '$d' "$_chunk" > "$_chunk.t" && mv "$_chunk.t" "$_chunk"
        fi
    fi
    putn nginx_log_partial_bytes "$_partial"

    _rtf="$TMPD/nginx.rt"
    : > "$_rtf"

    awk -v rtf="$_rtf" -v slowms="$SLOW_MS" '
    BEGIN { n=0; c2=0; c3=0; c4=0; c5=0; mal=0; bytes=0; srt=0; smax=0; slow=0; surt=0; nurt=0 }
    {
        if (NF < 3) { mal++; next }
        s  = $(NF-2); b = $(NF-1); rt = $NF
        # 行尾三字段必须是 s=<3位数字> b=<数字|-> rt=<数字>，否则判为格式不符
        if (s !~ /^s=[0-9][0-9][0-9]$/) { mal++; next }
        if (rt !~ /^rt=[0-9]+\.?[0-9]*$/) { mal++; next }
        if (b !~ /^b=([0-9]+|-)$/) { mal++; next }

        code = substr(s,3)+0
        rtms = substr(rt,4) * 1000
        if (b != "b=-") bytes += substr(b,3)+0

        n++
        if      (code >= 500) c5++
        else if (code >= 400) c4++
        else if (code >= 300) c3++
        else                  c2++

        srt += rtms
        if (rtms > smax) smax = rtms
        if (rtms > slowms) slow++
        printf "%.1f\n", rtms >> rtf

        # upstream_response_time：取第一个 urt= 字段，多上游（含逗号）时跳过
        for (i = 1; i <= NF; i++) {
            if ($i ~ /^urt=/) {
                v = substr($i,5)
                if (v != "-" && v !~ /,/ && v ~ /^[0-9]+\.?[0-9]*$/) { surt += v*1000; nurt++ }
                break
            }
        }
    }
    END {
        printf "%d %d %d %d %d %d %d %.1f %.1f %d %.1f %d\n",
            n, c2, c3, c4, c5, mal, bytes,
            (n>0 ? srt/n : 0), smax, slow,
            (nurt>0 ? surt/nurt : 0), nurt
    }' "$_chunk" > "$TMPD/nginx.sum" 2>/dev/null

    read -r NN C2 C3 C4 C5 MAL BYTES RTAVG RTMAX SLOW URTAVG NURT < "$TMPD/nginx.sum"
    NN=${NN:-0}; C2=${C2:-0}; C3=${C3:-0}; C4=${C4:-0}; C5=${C5:-0}
    MAL=${MAL:-0}; BYTES=${BYTES:-0}; RTAVG=${RTAVG:-0}; RTMAX=${RTMAX:-0}
    SLOW=${SLOW:-0}; URTAVG=${URTAVG:-0}; NURT=${NURT:-0}

    # 配置自检：若全部行都无法解析，说明 log_format 未按要求配置
    if [ "$MAL" -eq 0 ] && [ "$NN" -gt 0 ]; then NGINX_CONF_OK=1; fi
    if [ "$NN" -eq 0 ] && [ "$MAL" -eq 0 ]; then NGINX_CONF_OK=1; fi
    putn nginx_log_format_ok "$NGINX_CONF_OK"

    # 两类指标同时给出：
    #   *_interval  本采集区间内的增量（gauge，便于看「这一分钟发生了什么」）
    #   *_total     自部署起的累计值（counter，符合 Prometheus 单调递增语义）
    _cum="$WORKD/nginx.cum"
    _cr=0; _c2=0; _c3=0; _c4=0; _c5=0; _cb=0; _cm=0; _cs=0
    # shellcheck disable=SC2086
    if [ -f "$_cum" ]; then
        set -- $(cat "$_cum")
        _cr=${1:-0}; _c2=${2:-0}; _c3=${3:-0}; _c4=${4:-0}
        _c5=${5:-0}; _cb=${6:-0}; _cm=${7:-0}; _cs=${8:-0}
    fi
    _cr=$((_cr + NN));   _c2=$((_c2 + C2)); _c3=$((_c3 + C3))
    _c4=$((_c4 + C4));   _c5=$((_c5 + C5)); _cb=$((_cb + BYTES))
    _cm=$((_cm + MAL));  _cs=$((_cs + SLOW))
    printf '%s %s %s %s %s %s %s %s\n' \
        "$_cr" "$_c2" "$_c3" "$_c4" "$_c5" "$_cb" "$_cm" "$_cs" > "$_cum"

    putn nginx_requests_interval           "$NN"
    putn nginx_responses_2xx_interval      "$C2"
    putn nginx_responses_3xx_interval      "$C3"
    putn nginx_responses_4xx_interval      "$C4"
    putn nginx_responses_5xx_interval      "$C5"
    putn nginx_bytes_sent_interval         "$BYTES"
    putn nginx_log_malformed_interval      "$MAL"
    putn nginx_slow_requests_interval      "$SLOW"

    putn nginx_requests_total              "$_cr"
    putn nginx_responses_2xx_total         "$_c2"
    putn nginx_responses_3xx_total         "$_c3"
    putn nginx_responses_4xx_total         "$_c4"
    putn nginx_responses_5xx_total         "$_c5"
    putn nginx_bytes_sent_total            "$_cb"
    putn nginx_log_malformed_total         "$_cm"
    putn nginx_slow_requests_total         "$_cs"

    if [ "$NN" -gt 0 ]; then
        putn nginx_request_duration_avg_ms "$RTAVG"
        putn nginx_request_duration_max_ms "$RTMAX"
        putn nginx_upstream_duration_avg_ms "$URTAVG"
        putn nginx_upstream_duration_samples "$NURT"
        _p=$(sort -n "$_rtf" | awk '{a[NR]=$1} END{
            if (NR==0) exit
            p=int(NR*0.50); q=int(NR*0.95); r=int(NR*0.99)
            if(p<1)p=1; if(q<1)q=1; if(r<1)r=1
            printf "%.1f %.1f %.1f", a[p], a[q], a[r]
        }')
        putn nginx_request_duration_p50_ms "$(printf '%s' "$_p" | cut -d' ' -f1)"
        putn nginx_request_duration_p95_ms "$(printf '%s' "$_p" | cut -d' ' -f2)"
        putn nginx_request_duration_p99_ms "$(printf '%s' "$_p" | cut -d' ' -f3)"
    fi

    # 推进偏移量（轮转后从新文件头开始）
    _newoff=$((_size - _partial))
    printf '%s:%s\n' "$_newoff" "$_ino" > "$WORKD/nginx.off"
}

# ------------------------------------------------------ BUSINESS METRICS ----
# 仅依赖现有公开接口，不读数据库文件（避免与 WAL 写入端产生任何交互）。
# /api/stats 响应体约 100 B，是唯一用于高频轮询的业务接口。
collect_business() {
    _body="$TMPD/stats.json"
    _t=$(curl -s -o "$_body" -w '%{time_total} %{http_code}' \
         --max-time "$CURL_MAX_TIME" "$STATS_URL" 2>/dev/null)
    _code=$(printf '%s' "$_t" | cut -d' ' -f2)
    _tt=$(printf '%s' "$_t" | cut -d' ' -f1)

    if [ "$_code" = "200" ] && [ -s "$_body" ]; then
        putn mujian_api_up 1
        putn mujian_stats_fetch_ms "$(awk -v v="$_tt" 'BEGIN{printf "%.1f", v*1000}')"
        # 提取顺序无关：取 key 之后到第一个非数字字符为止。
        # 注意不能用 `s/,.*//` —— 当目标字段是 JSON 的最后一个时，
        # 其后缀是 `}` 而非 `,`，会把右花括号一并带进数值里。
        _rec=$(sed 's/.*"total_records"://'  "$_body" | sed 's/[^0-9.eE+-].*//')
        _city=$(sed 's/.*"total_cities"://'  "$_body" | sed 's/[^0-9.eE+-].*//')
        _cost=$(sed 's/.*"total_cost"://'    "$_body" | sed 's/[^0-9.eE+-].*//')
        _rate=$(sed 's/.*"avg_rating"://'    "$_body" | sed 's/[^0-9.eE+-].*//')
        putn mujian_records_total "$_rec"
        putn mujian_cities_total  "$_city"
        putn mujian_total_cost    "$_cost"
        putn mujian_avg_rating    "$_rate"

        # 增量与增长速率：与上一次采样值差分
        _prev=""
        [ -f "$WORKD/records.prev" ] && _prev=$(cat "$WORKD/records.prev")
        if [ -n "$_rec" ] && [ -n "$_prev" ]; then
            putn mujian_records_delta "$(awk -v a="$_rec" -v b="$_prev" 'BEGIN{printf "%d", a-b}')"
        fi
        [ -n "$_rec" ] && printf '%s' "$_rec" > "$WORKD/records.prev"
    else
        putn mujian_api_up 0
        putn mujian_stats_http_code "${_code:-0}"
    fi

    # 健康探针（独立于业务接口，用于区分「服务挂了」与「接口异常」）
    _hc=$(curl -s -o /dev/null -w '%{http_code}' --max-time "$CURL_MAX_TIME" "$HEALTH_URL" 2>/dev/null)
    putn mujian_healthz_http_code "${_hc:-0}"
}

# 低频重指标（建议每小时）。
# 只有这里才会调用 docker CLI —— 稳态的高频采集路径零 docker 调用。
collect_extended() {
    # 运行状态与重启次数（重启次数是判断容器反复崩溃的关键信号，
    # 只能通过 docker inspect 获得，故放在低频路径）
    if command -v docker >/dev/null 2>&1; then
        _info=$(docker inspect -f '{{.State.Status}}|{{.RestartCount}}' "$CONTAINER" 2>/dev/null)
        if [ -n "$_info" ]; then
            puts container_status "$(printf '%s' "$_info" | cut -d'|' -f1)"
            putn container_restart_total "$(printf '%s' "$_info" | cut -d'|' -f2)"
        fi
        # 网络 / 块设备累计 I/O。仅 docker stats 提供，单次约 2 秒，
        # 因此严格限制在低频路径；设 MUJIAN_COLLECT_IO=0 可完全关闭。
        if [ "${MUJIAN_COLLECT_IO:-1}" != "0" ]; then
            _st=$(docker stats --no-stream \
                --format '{{.NetIO}}|{{.BlockIO}}' "$CONTAINER" 2>/dev/null)
            if [ -n "$_st" ]; then
                _net=$(printf '%s' "$_st" | cut -d'|' -f1)
                _blk=$(printf '%s' "$_st" | cut -d'|' -f2)
                putn container_net_rx_mib  "$(to_mib "$(printf '%s' "$_net" | cut -d'/' -f1)")"
                putn container_net_tx_mib  "$(to_mib "$(printf '%s' "$_net" | cut -d'/' -f2)")"
                putn container_blk_read_mib  "$(to_mib "$(printf '%s' "$_blk" | cut -d'/' -f1)")"
                putn container_blk_write_mib "$(to_mib "$(printf '%s' "$_blk" | cut -d'/' -f2)")"
            fi
        fi
    fi

    _count_objects() {
        # 统计 JSON 数组顶层对象个数（按花括号深度，不依赖 jq）
        awk '{ for(i=1;i<=length($0);i++){ c=substr($0,i,1)
            if(ins){ if(c=="\\"){i++; continue}; if(c=="\"") ins=0 }
            else if(c=="\"") ins=1
            else if(c=="{"){ d++; if(d==1) n++ }
            else if(c=="}") d-- } }
            END{ print n+0 }'
    }
    _a=$(curl -s --max-time 20 "$ARTISTS_URL" 2>/dev/null | _count_objects)
    _d=$(curl -s --max-time 20 "$DRAMAS_URL" 2>/dev/null | _count_objects)
    putn mujian_artists_total "$_a"
    putn mujian_dramas_total  "$_d"

    if [ -d "$DATA_VOLUME" ]; then
        _du=$(du -sm "$DATA_VOLUME" 2>/dev/null | awk '{print $1}')
        putn mujian_data_volume_mb "$_du"
        _db=$(du -sm "$DATA_VOLUME/mujian.db" 2>/dev/null | awk '{print $1}')
        putn mujian_db_size_mb "$_db"
        _up=$(du -sm "$DATA_VOLUME/uploads" 2>/dev/null | awk '{print $1}')
        putn mujian_uploads_size_mb "$_up"
        _cnt=$(find "$DATA_VOLUME/uploads/covers" -type f 2>/dev/null | wc -l | tr -d ' ')
        putn mujian_cover_files_total "$_cnt"
    fi
}

# ----------------------------------------------------------- SELF-TEST ------
self_test() {
    _fail=0
    _d=$(mktemp -d)
    # 构造样例日志：10 条请求，其中 2 条 4xx、1 条 5xx、1 条慢请求（1.5s）
    # 1 条多上游（urt 含逗号，应被跳过而不计入 upstream 均值）
    {
        i=0
        while [ $i -lt 6 ]; do
            echo "192.168.1.10 [29/Aug/2026:02:39:2$i +0800] p=/api/records urt=0.0$i s=200 b=70447 rt=0.01$i"
            i=$((i+1))
        done
        echo "192.168.1.10 [29/Aug/2026:02:40:01 +0800] p=/favicon.ico urt=- s=404 b=209 rt=0.002"
        echo "192.168.1.10 [29/Aug/2026:02:40:02 +0800] p=/nope urt=0.001 s=404 b=209 rt=0.003"
        echo "192.168.1.10 [29/Aug/2026:02:40:03 +0800] p=/api/x urt=1.490 s=500 b=21 rt=1.500"
        echo "192.168.1.10 [29/Aug/2026:02:40:04 +0800] p=/multi urt=0.100, 0.200 s=200 b=100 rt=0.300"
    } > "$_d/access.log"

    M="$_d/m"; : > "$M"; TMPD="$_d"; WORKD="$_d"
    NGINX_LOG="$_d/access.log"
    collect_nginx

    _chk() { # name expected actual
        if [ "$2" = "$3" ]; then printf '  PASS  %-32s = %s\n' "$1" "$3"
        else printf '  FAIL  %-32s expected=%s actual=%s\n' "$1" "$2" "$3"; _fail=$((_fail+1)); fi
    }
    # 取最后一次写入的值（同一次自检中会重复调用 collect_nginx）
    _get() { sed -n "s/^$1\t//p" "$M" | tail -n 1 | cut -f1; }

    echo "nginx 日志解析自检（样例 10 行）"
    _chk nginx_requests_interval       10 "$(_get nginx_requests_interval)"
    _chk nginx_responses_2xx_interval   7 "$(_get nginx_responses_2xx_interval)"
    _chk nginx_responses_4xx_interval   2 "$(_get nginx_responses_4xx_interval)"
    _chk nginx_responses_5xx_interval   1 "$(_get nginx_responses_5xx_interval)"
    _chk nginx_slow_requests_interval   1 "$(_get nginx_slow_requests_interval)"
    _chk nginx_log_malformed_interval   0 "$(_get nginx_log_malformed_interval)"
    _chk nginx_log_format_ok            1 "$(_get nginx_log_format_ok)"
    # 10 条中：404 那条 urt=-、多上游那条含逗号，两条不计入 upstream 均值
    _chk nginx_upstream_duration_samples 8 "$(_get nginx_upstream_duration_samples)"
    echo "  p50 = $(_get nginx_request_duration_p50_ms) ms, p95 = $(_get nginx_request_duration_p95_ms) ms, max = $(_get nginx_request_duration_max_ms) ms"

    # 累计计数器：第二轮应累加到 10（而非 20），验证偏移未重复计数
    collect_nginx
    _chk "第二轮新请求数（应为 0）"   0 "$(_get nginx_requests_interval)"
    _chk "累计请求数（应保持 10）"   10 "$(_get nginx_requests_total)"

    rm -rf "$_d"
    if [ "$_fail" -eq 0 ]; then echo "自检通过"; return 0; fi
    echo "自检失败：$_fail 项"
    return 1
}

# -------------------------------------------------------------- OUTPUT ------
emit_json() {
    printf '{"ts":%s,"ts_iso":"%s","host":"%s","interval_s":%s' \
        "$TS" "$TS_ISO" "$(uname -n)" "${INTERVAL_S:-0}"
    while IFS="$TAB" read -r _n _v _t; do
        [ -n "$_n" ] || continue
        # 指标名可能含 Prometheus 标签（如 disk{mount="/"}），其中的双引号
        # 必须转义，否则产出的 JSON 非法。
        _ne=$(printf '%s' "$_n" | sed 's/"/\\"/g')
        printf ',"%s":%s' "$_ne" "$_v"
    done < "$M"
    printf '}\n'
}

emit_prom() {
    printf '# 幕间（MuJian）指标快照 — Prometheus 文本暴露格式\n'
    printf '# 生成时间: %s   采集间隔: %ss\n' "$TS_ISO" "${INTERVAL_S:-0}"
    # 注意：Prometheus 规范要求同一指标名的 # TYPE / # HELP 只能声明一次，
    # 带不同 label 的样本共用该声明。故此处按 base name 去重。
    awk -F'\t' '
    $3 != "n" { next }
    {
        name = $1; val = $2
        base = name; sub(/\{.*/, "", base)
        lbl = ""
        if (match(name, /\{[^}]*\}/)) lbl = substr(name, RSTART + 1, RLENGTH - 2)
        if (!(base in seen)) {
            seen[base] = 1
            printf "# TYPE %s %s\n", base, (base ~ /_total$/) ? "counter" : "gauge"
        }
        if (lbl != "") printf "%s{%s} %s\n", base, lbl, val
        else           printf "%s %s\n", base, val
    }' "$M"
}

# ---------------------------------------------------------------- MAIN ------
case "${1:-}" in
    --self-test) self_test; exit $? ;;
    --extended)  MODE="extended" ;;
    --dry-run)   MODE="dry" ;;
    "")          MODE="normal" ;;
    -h|--help)   sed -n '2,30p' "$0" | sed 's/^#\{1,2\} \{0,1\}//'; exit 0 ;;
    *) echo "未知参数: $1（支持 --extended / --dry-run / --self-test）" >&2; exit 2 ;;
esac

TS=$(date +%s)
TS_ISO=$(date -u +%Y-%m-%dT%H:%M:%SZ)

TMPD=$(mktemp -d)
M="$TMPD/metrics"
: > "$M"

# WORKD 是「跨轮次持久化目录」：采集函数直接读写它，并用 .new + mv 原子
# 替换，避免进程被中断时留下半截状态。--dry-run 时指向临时目录，不落盘。
STATE="$OUT_DIR/state"
WORKD="$TMPD"
if [ "$MODE" != "dry" ]; then
    mkdir -p "$OUT_DIR" "$STATE" 2>/dev/null
    WORKD="$STATE"
    # 上次采样时间戳，用于计算 interval_s、CPU 差值与增长速率
    if [ -f "$STATE/last_ts" ]; then
        INTERVAL_S=$(awk -v a="$TS" -v b="$(cat "$STATE/last_ts")" 'BEGIN{d=a-b; if(d<0)d=0; printf "%d", d}')
    else
        INTERVAL_S=0
    fi
fi

# 采集（各段独立，单段失败不影响其余）
collect_host_cpu
collect_host
collect_container
collect_nginx
collect_business
[ "$MODE" = "extended" ] && collect_extended

# 采集器自身健康度：缺失字段数（用于发现「某项指标悄悄采不到」）
putn collector_missing_fields "$MISSING"

if [ "$MODE" = "dry" ]; then
    emit_json
    exit 0
fi

# ---- JSONL 时序（按日轮转 + 保留策略）----
JSONL="$OUT_DIR/metrics.jsonl"
TODAY=$(date -u +%Y-%m-%d)
CUR_DATE=""
[ -f "$STATE/cur_date" ] && CUR_DATE=$(cat "$STATE/cur_date")
if [ -n "$CUR_DATE" ] && [ "$CUR_DATE" != "$TODAY" ]; then
    [ -f "$JSONL" ] && mv "$JSONL" "$OUT_DIR/metrics-$CUR_DATE.jsonl"
fi
printf '%s' "$TODAY" > "$STATE/cur_date"
emit_json >> "$JSONL"

# ---- Prometheus 快照 ----
emit_prom > "$OUT_DIR/metrics.prom.tmp" && mv "$OUT_DIR/metrics.prom.tmp" "$OUT_DIR/metrics.prom"

# 状态文件（cpu.prev / nginx.off / nginx.cum / records.prev / cid / cpu_usage）
# 已在各采集函数内以 .new + mv 原子写入 WORKD，此处只需推进采样时间戳。
printf '%s' "$TS" > "$STATE/last_ts"

# ---- 清理过期文件 ----
if [ "$KEEP_DAYS" -gt 0 ] 2>/dev/null; then
    find "$OUT_DIR" -name 'metrics-*.jsonl' -type f -mtime +"$KEEP_DAYS" -delete 2>/dev/null
fi

exit 0
