package web

import (
	"io/fs"
	"testing"
)

// 两种状态都必须自洽：仓库内只有 dist/.gitkeep 占位（未构建），
// 或 CI / 构建脚本已把 web/dist 拷进来（已构建）。
// HasBuild 的判定必须与 FS() 的实际内容一致，否则启动时会把空前端当成可用。
func TestHasBuildMatchesFS(t *testing.T) {
	hasBuild := HasBuild()
	_, err := fs.Stat(FS(), "index.html")

	switch {
	case hasBuild && err != nil:
		t.Fatalf("HasBuild()=true 但 FS() 读不到 index.html: %v", err)
	case !hasBuild && err == nil:
		t.Fatal("HasBuild()=false 但 FS() 却能读到 index.html")
	}
}

// 占位态下静态资源路由仍会调用 FS()，因此它必须始终可遍历、不能 panic。
func TestFSWalkable(t *testing.T) {
	if _, err := fs.ReadDir(FS(), "."); err != nil {
		t.Fatalf("FS() 根目录不可读: %v", err)
	}
}
