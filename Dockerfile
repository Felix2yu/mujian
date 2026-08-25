# Final image assembled from CI-prebuilt artifacts:
#   - `frontend-dist`             -> ./dist    (prebuilt frontend static assets)
#   - `mujian-amd64`/`mujian-arm64` -> ./bin/mujian (Go binary built with CGO + libavif)
#
# The `build` job compiles everything; this Dockerfile only assembles the
# runtime image. The CGO + libavif ABI matches the build runner because both
# use ubuntu:24.04, so the dynamically linked libavif loads at runtime.
#
# The ubuntu base image already ships an `ubuntu` user at uid 1000, so we run
# the app as that user instead of creating a colliding uid-1000 account.

FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates tzdata libavif16 curl \
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
