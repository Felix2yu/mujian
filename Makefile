# mujian —— 目标约定见 Felix2yu/.github/docs/makefile.md
#
# 七个必备目标：build / dev / test / lint / fmt / docker / clean
.PHONY: dev dev-frontend dev-backend build test lint lint-strict fmt docker up clean

# 同时起前后端：dev.sh 用 fswatch 监听 .go 变化，自动重建并重启
dev:
	./dev.sh

dev-frontend:
	cd web && pnpm run dev

dev-backend:
	./dev.sh

build:
	cd web && pnpm run build
	# internal/web/dist 若已存在，`cp -r src dst` 会把源目录整体嵌套成
	# internal/web/dist/dist，而 go:embed 读到的仍是上一次的旧外壳——新构建的前端
	# 根本没进二进制。CI 里同样先 rm -rf。
	rm -rf internal/web/dist
	mkdir -p internal/web/dist
	cp -r web/dist/. internal/web/dist/
	# tracked 的 .gitkeep 会被上面的 rm -rf 一起删掉，工作区就永久挂着一条删除记录
	touch internal/web/dist/.gitkeep
	mkdir -p bin
	CGO_ENABLED=1 go build -o bin/mujian ./cmd/mujian

test:
	go test ./...

# gofmt -l 只列出未格式化文件、不写盘，属于检查而非格式化
lint:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "未格式化："; echo "$$out"; exit 1; fi
	go vet ./...
	cd web && pnpm run check

# 深度检查：在 lint 之上加 golangci-lint（存量告警未清零前会红，故不并入 lint）
lint-strict: lint
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run; else echo "  （未安装 golangci-lint，跳过）"; fi

fmt:
	gofmt -w $$(git ls-files '*.go')

# 装配式镜像：先出 bin/mujian（前端已内嵌），镜像只拼装。
# 起容器用 make up（docker compose 需要挂载与网络配置，不是镜像构建）。
docker: build
	docker build -t mujian:dev .

up:
	docker compose up -d

clean:
	# internal/web/dist/* 不匹配点开头文件，.gitkeep 占位会幸存
	rm -rf web/dist internal/web/dist/* bin data
