.PHONY: dev build docker clean

dev-frontend:
	cd frontend && pnpm run dev

dev-backend:
	./dev.sh

build:
	cd frontend && pnpm run build
	# backend/dist 若已存在，`cp -r src dst` 会把源目录整体嵌套成 backend/dist/dist，
	# 而 go:embed 读到的仍是上一次的旧外壳——新构建的前端根本没进二进制。CI 里同样先 rm -rf。
	rm -rf backend/dist
	cp -r frontend/dist backend/dist
	cd backend && CGO_ENABLED=1 go build -o mujian .

docker:
	docker compose up -d

clean:
	rm -rf frontend/dist backend/dist backend/mujian backend/data
