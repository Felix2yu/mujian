.PHONY: dev build docker clean

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

docker:
	docker compose up -d

clean:
	# internal/web/dist/* 不匹配点开头文件，.gitkeep 占位会幸存
	rm -rf web/dist internal/web/dist/* bin data
