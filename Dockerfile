# Final image assembled from CI-prebuilt artifacts:
#   - `frontend-dist`             -> ./dist    (prebuilt frontend static assets)
#   - `mujian-amd64`/`mujian-arm64` -> ./bin/mujian (Go binary built with CGO)
#
# The `build` job compiles everything; this Dockerfile only assembles the
# runtime image. The binary is built on the ubuntu:24.04 runner but runs on the
# debian-13 base image here — that is safe because the only dynamic dependencies
# are glibc's libc/libm (verified via ldd) and glibc is backward compatible.
#
# 基镜 ghcr.io/felix2yu/base:debian-13 提供 ca-certificates、tzdata、gosu 与
# PUID/PGID 降权入口（本仓库是唯一需要 glibc 的 CGO 项目，故走 debian 版基镜）。
# 原先用 `su -s /bin/sh ubuntu` 降权，改为基镜的 gosu + 数字 uid/gid。
FROM ghcr.io/felix2yu/base:debian-13

# curl 供 HEALTHCHECK 使用（基镜 debian 版不含 curl / wget）。
# 安全补丁的新鲜度改由基镜保证：base-image.yml 每周重建基镜，其 digest 变化会让
# 这一层缓存失效并重新解析，因此不再需要额外的 CACHEBUST 层。
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app/data/uploads

ENV TZ=Asia/Shanghai \
    PUID=1000 \
    PGID=1000 \
    ALLOW_LOCAL_STORAGE=true \
    APP_DATA_DIR=/app/data
WORKDIR /app

COPY --chown=1000:1000 --chmod=755 bin/mujian ./mujian
COPY --chown=1000:1000 dist ./dist

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/healthz || exit 1

CMD ["./mujian"]
