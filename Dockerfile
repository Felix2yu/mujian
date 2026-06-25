# Build frontend (cached unless frontend files change)
FROM node:24-alpine AS frontend
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Build backend (cached unless Go files change, frontend dist is now available)
FROM golang:1.26-bookworm AS backend
RUN apt-get update && apt-get install -y --no-install-recommends gcc libavif-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN GOPROXY=https://goproxy.cn,direct go mod download
COPY backend/ .
COPY --from=frontend /app/dist ./dist
RUN CGO_ENABLED=1 GOPROXY=https://goproxy.cn,direct go build -o /mujian .

# Final image
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata libavif16 && rm -rf /var/lib/apt/lists/* \
    && useradd -u 1000 -m -s /sbin/nologin mujian \
    && mkdir -p /app/data/uploads \
    && chown -R mujian:mujian /app
ENV TZ=Asia/Shanghai PUID=1000 PGID=1000 ALLOW_LOCAL_STORAGE=true
WORKDIR /app
COPY --from=backend --chown=mujian:mujian /mujian .
COPY --from=frontend --chown=mujian:mujian /app/dist ./dist
EXPOSE 8080
CMD ["sh", "-c", "su mujian -c './mujian'"]
