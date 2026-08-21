# Build frontend (cached unless frontend files change)
FROM node:24-alpine AS frontend
RUN npm install -g pnpm
WORKDIR /app
COPY frontend/package*.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ .
RUN pnpm run build

# Build backend (cached unless Go files change, frontend dist is now available)
FROM golang:1.27-trixie AS backend
RUN apt-get update && apt-get install -y --no-install-recommends gcc libavif-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY --from=frontend /app/dist ./dist
RUN CGO_ENABLED=1 go build -o /mujian .

# Final image
FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata libavif-dev curl && rm -rf /var/lib/apt/lists/* \
    && useradd -u 1000 -m -s /sbin/nologin mujian \
    && mkdir -p /app/data/uploads \
    && chown -R mujian:mujian /app
ENV TZ=Asia/Shanghai PUID=1000 PGID=1000 ALLOW_LOCAL_STORAGE=true
WORKDIR /app
COPY --from=backend --chown=mujian:mujian /mujian .
COPY --from=frontend --chown=mujian:mujian /app/dist ./dist
EXPOSE 8080
CMD ["sh", "-c", "exec su -s /bin/sh mujian -c './mujian'"]
