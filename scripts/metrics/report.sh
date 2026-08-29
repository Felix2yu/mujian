#!/bin/sh
# report.sh — 幕间（MuJian）指标报告查看器
#
# 读取 collect.sh 产出的 metrics.jsonl（含按日轮转的 metrics-*.jsonl），
# 输出人类可读的终端报告与迷你趋势图。单次运行约 0.1–0.5 s，无常驻进程。
#
# 用法：
#   report.sh                 最近 24 小时（默认）
#   report.sh --hours 168     最近 7 天
#   report.sh --all           全部保留期数据
#   report.sh --prom          直接打印 Prometheus 快照
#   report.sh --last          打印最近一条完整 JSON（原始字段全量）
#
# 环境变量：
#   MUJIAN_METRICS_DIR   指标目录，默认 /var/lib/mujian-metrics

OUT_DIR="${MUJIAN_METRICS_DIR:-/var/lib/mujian-metrics}"
HOURS=24
MODE="report"

while [ $# -gt 0 ]; do
    case "$1" in
        --hours) HOURS="$2"; shift 2 ;;
        --all)   HOURS=0; shift ;;
        --prom)  MODE="prom"; shift ;;
        --last)  MODE="last"; shift ;;
        -h|--help) sed -n '2,18p' "$0" | sed 's/^#\{1,2\} \{0,1\}//'; exit 0 ;;
        *) echo "未知参数: $1" >&2; exit 2 ;;
    esac
done

[ -d "$OUT_DIR" ] || { echo "指标目录不存在: $OUT_DIR" >&2; exit 1; }

# ---- 直通模式 ----
if [ "$MODE" = "prom" ]; then
    [ -f "$OUT_DIR/metrics.prom" ] && cat "$OUT_DIR/metrics.prom" || \
        { echo "尚无快照: $OUT_DIR/metrics.prom" >&2; exit 1; }
    exit 0
fi
if [ "$MODE" = "last" ]; then
    _f="$OUT_DIR/metrics.jsonl"
    [ -s "$_f" ] || { echo "尚无数据" >&2; exit 1; }
    tail -n 1 "$_f" | sed 's/,/,\n  /g; s/^{/{\n  /; s/}$/\n}/'
    exit 0
fi

# ---- 汇总数据源：把历史文件拼成一个 TSV（单次 awk 扫描，后续都读 TSV）----
TMPD=$(mktemp -d)
trap 'rm -rf "$TMPD"' EXIT INT TERM
TSV="$TMPD/data.tsv"

NOW=$(date +%s)
CUTOFF=0
[ "$HOURS" -gt 0 ] && CUTOFF=$((NOW - HOURS * 3600))

_files=""
[ -f "$OUT_DIR/metrics.jsonl" ] && _files="$OUT_DIR/metrics.jsonl"
for f in "$OUT_DIR"/metrics-*.jsonl; do
    [ -f "$f" ] && _files="$_files $f"
done
[ -n "$_files" ] || { echo "尚无数据：$OUT_DIR 下没有 metrics*.jsonl" >&2; exit 1; }

# 从每个 JSON 行里抽取固定列；字段缺失时输出空。
# 只做一次全量扫描，之后所有展示逻辑都基于 TSV，避免重复解析。
# shellcheck disable=SC2086
cat $_files | awk -v cut="$CUTOFF" '
function grab(s, k,   n, a, i, p, v) {
    n = split(s, a, ",")
    sub(/^[ \t]*\{/, "", a[1])
    p = "\"" k "\":"
    for (i = 1; i <= n; i++) {
        if (substr(a[i], 1, length(p)) == p) {
            v = substr(a[i], length(p) + 1)
            gsub(/[}\]]/, "", v)
            gsub(/^[ \t]*"|"[ \t]*$/, "", v)
            return v
        }
    }
    return ""
}
{
    # JSON 里带标签的键（如 "disk{mount=\"/\"}"）其内层引号被转义为 \"，
    # 先还原成普通引号，才能与下面的查找键对齐。本脚本只取数值型指标，
    # 因此不会影响任何取值。
    line = $0
    gsub(/\\"/, "\"", line)

    # 区间计数优先取 *_interval；若数据来自旧版本（只有 *_total）
    # 则回退到 *_total，保证报告对两种数据都可用。
    req  = grab(line, "nginx_requests_interval");       if (req  == "") req  = grab(line, "nginx_requests_total")
    e4xx = grab(line, "nginx_responses_4xx_interval");  if (e4xx == "") e4xx = grab(line, "nginx_responses_4xx_total")
    e5xx = grab(line, "nginx_responses_5xx_interval");  if (e5xx == "") e5xx = grab(line, "nginx_responses_5xx_total")

    ts = grab(line, "ts") + 0
    if (cut > 0 && ts < cut) next
    printf "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
        ts,
        grab(line,"host_cpu_util_pct"),
        grab(line,"host_load1"),
        grab(line,"host_mem_used_pct"),
        grab(line,"host_mem_used_mb"),
        grab(line,"host_mem_total_mb"),
        grab(line,"host_disk_used_pct{mount=\"/\"}"),
        grab(line,"container_cpu_pct"),
        grab(line,"container_mem_used_mib"),
        grab(line,"container_pids"),
        req, e4xx, e5xx,
        grab(line,"nginx_request_duration_p95_ms"),
        grab(line,"mujian_records_total"),
        grab(line,"mujian_api_up")
}' | sort -n -k1,1 > "$TSV"

N=$(wc -l < "$TSV" | tr -d ' ')
if [ "$N" -eq 0 ]; then
    echo "指定时间窗口内没有样本（窗口 ${HOURS}h）。可加大 --hours 或用 --all。" >&2
    exit 1
fi

FIRST_TS=$(head -n1 "$TSV" | cut -f1)
LAST_TS=$(tail -n1 "$TSV" | cut -f1)
SPAN=$((LAST_TS - FIRST_TS))

# col <n>          取最新值
col() { tail -n1 "$TSV" | cut -f"$1"; }
# or_dash <v>      空值显示为 -（首样本时 CPU 差值类指标尚无基准值）
or_dash() { [ -n "$1" ] && printf '%s' "$1" || printf -- '-'; }
# series <n>       取整列数值序列
series() { cut -f"$1" "$TSV" | grep -E '^-?[0-9.]+$'; }
# series_sum <n>   区间求和（用于请求量等区间计数型指标）
series_sum() { series "$1" | awk '{s+=$1} END{printf "%.0f", s+0}'; }
# series_avg <n>
series_avg() { series "$1" | awk '{s+=$1; n++} END{if(n>0) printf "%.2f", s/n; else printf ""}'; }
# series_max <n>
series_max() { series "$1" | awk 'BEGIN{m=""} {if(m==""||$1+0>m+0) m=$1+0} END{printf "%.2f", m+0}'; }
# delta <n>        末值 - 首值（用于累计型业务指标）
delta() { series "$1" | awk 'NR==1{f=$1} END{if(NR>0) printf "%d", $1-f; else printf ""}'; }

fmt_dur() {
    _s=$1
    if [ "$_s" -lt 3600 ]; then printf '%dm' $((_s / 60))
    elif [ "$_s" -lt 86400 ]; then printf '%dh%dm' $((_s / 3600)) $((_s % 3600 / 60))
    else printf '%dd%dh' $((_s / 86400)) $((_s % 86400 / 3600)); fi
}

# 迷你趋势图：把序列降采样到 40 个点后映射到 ▁▂▃▄▅▆▇█
sparkline() {
    awk '{
        if ($1 ~ /^-?[0-9.]+$/) { v[++n] = $1 + 0 }
    }
    END {
        if (n == 0) { printf "(数据积累中)"; exit }
        W = 40
        m = (n < W) ? n : W
        for (i = 1; i <= m; i++) {
            if (n < W) { x = v[i] }
            else {
                lo = int((i-1)*n/W) + 1; hi = int(i*n/W); if (hi < lo) hi = lo
                s = 0; c = 0
                for (j = lo; j <= hi; j++) { s += v[j]; c++ }
                x = (c > 0) ? s/c : 0
            }
            b[i] = x
            if (i == 1 || x < mn) mn = x
            if (i == 1 || x > mx) mx = x
        }
        c0="▁"; c1="▂"; c2="▃"; c3="▄"; c4="▅"; c5="▆"; c6="▇"; c7="█"
        rng = mx - mn
        out = ""
        for (i = 1; i <= m; i++) {
            if (rng <= 0) k = 0; else k = int((b[i]-mn)/rng * 7.999)
            if (k > 7) k = 7
            if (k < 0) k = 0
            out = out ((k==0)?c0:(k==1)?c1:(k==2)?c2:(k==3)?c3:(k==4)?c4:(k==5)?c5:(k==6)?c6:c7)
        }
        printf "%s  [%s … %s]", out, mn, mx
    }'
}

# ------------------------------------------------------------- RENDER ------
printf '\n'
printf '==========================================================================\n'
printf ' 幕间（MuJian）运行状态报告\n'
printf ' 生成时间  %s\n' "$(date '+%Y-%m-%d %H:%M:%S %z')"
printf ' 数据窗口  %s（%s 前 ~ 现在）   样本数 %s   实际跨度 %s\n' \
    "$([ "$HOURS" -gt 0 ] && echo "${HOURS}h" || echo "全部")" \
    "$(fmt_dur "$SPAN")" "$N" "$(fmt_dur "$SPAN")"
printf '==========================================================================\n'

printf '\n【宿主机】\n'
_lbl() { printf '  %-16s' "$1"; }
_lbl "CPU 使用率"; printf '%6s%%\n' "$(or_dash "$(col 2)")"
printf '      %s\n' "$(series 2 | sparkline)"
_lbl "负载 1m"; printf '%6s      ' "$(or_dash "$(col 3)")"
printf '内存 %6s / %s MB (%s%%)\n' "$(or_dash "$(col 5)")" "$(or_dash "$(col 6)")" "$(or_dash "$(col 4)")"
printf '      内存趋势 %s\n' "$(series 4 | sparkline)"
_lbl "根分区使用率"; printf '%6s%%\n' "$(or_dash "$(col 7)")"
printf '      %s\n' "$(series 7 | sparkline)"

printf '\n【Docker 容器】\n'
_lbl "CPU"; printf '%6s%%\n' "$(or_dash "$(col 8)")"
printf '      %s\n' "$(series 8 | sparkline)"
_lbl "内存"; printf '%6s MiB\n' "$(or_dash "$(col 9)")"
printf '      %s\n' "$(series 9 | sparkline)"
_lbl "进程数"; printf '%6s\n' "$(or_dash "$(col 10)")"
if [ -f "$OUT_DIR/metrics.prom" ]; then
    _st=$(sed -n 's/^container_up{.*} //p;s/^container_up //p' "$OUT_DIR/metrics.prom" | tail -n1)
    _rs=$(sed -n 's/^container_restart_total //p' "$OUT_DIR/metrics.prom" | tail -n1)
    _lbl "运行状态"; printf 'up=%s  重启次数=%s\n' "${_st:-未知}" "${_rs:-未知}"
fi

printf '\n【Nginx 代理层】（区间累计 / 最近区间）\n'
_lbl "请求量"; printf '%6s 次   平均 %.1f 次/分钟\n' \
    "$(series_sum 11)" "$(series_avg 11 | awk '{printf "%.2f", $1}')"
printf '      %s\n' "$(series 11 | sparkline)"
_lbl "4xx 错误"; printf '%6s 次\n' "$(series_sum 12)"
_lbl "5xx 错误"; printf '%6s 次\n' "$(series_sum 13)"
_lbl "响应 p95"; printf '%6s ms   峰值 %s ms\n' "$(or_dash "$(col 14)")" "$(or_dash "$(series_max 14)")"
printf '      %s\n' "$(series 14 | sparkline)"

printf '\n【业务数据】\n'
_lbl "演出记录总数"; printf '%6s 条\n' "$(or_dash "$(col 15)")"
_d=$(delta 15)
printf '      窗口内净增 %s 条   趋势 %s\n' "$_d" "$(series 15 | sparkline)"
if [ -n "$_d" ] && [ "$SPAN" -gt 0 ]; then
    _rate=$(awk -v d="$_d" -v s="$SPAN" 'BEGIN{printf "%.2f", d/(s/86400)}')
    printf '      折算增速  %s 条/天\n' "$_rate"
fi
_lbl "服务可用性"; printf 'api_up=%s\n' "$(col 16)"
_dn=$(series 16 | awk '{n++; if($1+0<1) d++} END{if(n>0) printf "%.2f", 100*(n-d+0)/n; else printf ""}')
[ -n "$_dn" ] && printf '      窗口内可用率 %s%%\n' "$_dn"

printf '\n--------------------------------------------------------------------------\n'
printf ' 提示：report.sh --hours 168 看 7 天趋势 | --prom 看 Prometheus 快照\n'
printf '       --last 看最近一次采集的全部原始字段\n'
printf '==========================================================================\n\n'
