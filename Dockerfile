# Final image assembled from CI-prebuilt artifacts:
#   - `frontend-dist`             -> ./dist    (prebuilt frontend static assets)
#   - `mujian-amd64`/`mujian-arm64` -> ./bin/mujian (Go binary built with CGO + libavif)
#
# The `build` job compiles everything; this Dockerfile only assembles the
# runtime image. The binary is built on the ubuntu:24.04 runner but runs on
# ubuntu:26.04 here — that is safe because the only dynamic dependencies are
# glibc's libc/libm (verified via ldd; libavif is linked statically into the
# binary), and glibc is backward compatible. libavif16 is installed as a
# defensive runtime dep for the dlopen path.
#
# The ubuntu base image already ships an `ubuntu` user at uid 1000, so we run
# the app as that user instead of creating a colliding uid-1000 account.

FROM ubuntu:26.04

# Upgrade base-image packages before installing ours: the published image
# digest can lag behind the security pocket, and scanners flag the stale
# snapshot even when fixes already exist upstream. Pebble (bundled with the
# 26.04 base image) is a Go daemon manager this image never runs — and its
# bundled stdlib carried fixable HIGH CVEs — so it is purged.
RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install -y --no-install-recommends \
      ca-certificates tzdata libavif16 curl \
    && apt-get purge -y pebble \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /app/data/uploads \
    && chown -R ubuntu:ubuntu /app

ENV TZ=Asia/Shanghai PUID=1000 PGID=1000 ALLOW_LOCAL_STORAGE=true
WORKDIR /app

COPY bin/mujian ./mujian
COPY dist ./dist
RUN chmod +x ./mujian \
    && chown -R ubuntu:ubuntu /app

EXPOSE 8080
CMD ["sh", "-c", "exec su -s /bin/sh ubuntu -c './mujian'"]
