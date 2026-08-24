# Final image assembled from CI-prebuilt artifacts:
#   - `frontend-dist`             -> ./dist    (prebuilt frontend static assets)
#   - `mujian-amd64`/`mujian-arm64` -> ./bin/mujian (Go binary built with CGO + libavif)
#
# The `build` job compiles everything; this Dockerfile only assembles the
# runtime image. The CGO + libavif ABI matches the build runner because both
# use ubuntu:24.04, so the dynamically linked libavif loads at runtime.

FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates tzdata libavif16 curl \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -u 1000 -m -s /sbin/nologin mujian \
    && mkdir -p /app/data/uploads \
    && chown -R mujian:mujian /app

ENV TZ=Asia/Shanghai PUID=1000 PGID=1000 ALLOW_LOCAL_STORAGE=true
WORKDIR /app

COPY bin/mujian ./mujian
COPY dist ./dist
RUN chmod +x ./mujian \
    && chown -R mujian:mujian /app/dist

EXPOSE 8080
CMD ["sh", "-c", "exec su -s /bin/sh mujian -c './mujian'"]
