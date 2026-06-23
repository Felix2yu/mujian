# Build frontend (cached unless frontend files change)
FROM node:24-alpine AS frontend
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Build backend (cached unless Go files change, frontend dist is now available)
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN GOPROXY=https://goproxy.cn,direct go mod download
COPY backend/ .
COPY --from=frontend /app/dist ./dist
RUN CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct go build -o /mujian .

# Final image
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata su-exec shadow \
    && useradd -u 1000 -m -s /sbin/nologin mujian \
    && mkdir -p /app/data/uploads \
    && chown -R mujian:mujian /app
ENV TZ=Asia/Shanghai PUID=1000 PGID=1000 ALLOW_LOCAL_STORAGE=true
WORKDIR /app
COPY --from=backend --chown=mujian:mujian /mujian .
COPY --from=frontend --chown=mujian:mujian /app/dist ./dist
EXPOSE 8080
CMD ["sh", "-c", "su-exec ${PUID}:${PGID} ./mujian"]
