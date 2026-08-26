# Final image assembled from CI-prebuilt artifacts:
#   - `frontend-dist`             -> ./dist    (prebuilt frontend static assets)
#   - `mujian-amd64`/`mujian-arm64` -> ./bin/mujian (Go binary built with CGO)
#
# The `build` job compiles everything; this Dockerfile only assembles the
# runtime image. The binary is built on the ubuntu:24.04 runner but runs on
# debian:13-slim here — that is safe because the only dynamic dependencies are
# glibc's libc/libm (verified via ldd) and glibc is backward compatible.
#
# No system image-codec packages are needed: avif-go and chai2010/webp bundle
# their own headers and static libraries (AVIF/WebP codecs are linked into the
# binary), and SQLite is pure Go (modernc.org/sqlite).
#
# debian:13-slim ships no pre-created uid-1000 account, so one is created
# below — named `ubuntu` to keep the PUID/PGID=1000 semantics of previous
# releases.

FROM debian:13-slim

# Cache-buster for the package-refresh layer below: CI passes a unique value
# per run so `apt-get upgrade` re-resolves against the live security pocket
# instead of replaying a stale cached layer (base-image snapshots lag security
# fixes — e.g. an openssl CVE published days after this layer was cached).
# Only this layer and those after it rebuild; earlier layers stay cached.
ARG CACHEBUST=0

# Upgrade base-image packages before installing ours: the published image
# digest can lag behind the security pocket, and scanners flag the stale
# snapshot even when fixes already exist upstream.
RUN apt-get update \
    && apt-get upgrade -y \
    && apt-get install -y --no-install-recommends \
      ca-certificates tzdata curl \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 1000 --user-group --create-home --shell /bin/sh ubuntu \
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
