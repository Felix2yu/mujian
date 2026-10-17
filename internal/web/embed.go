// Package web 提供前端产物托管：go:embed 内嵌 SvelteKit 静态产物。
//
// 构建脚本（scripts / CI 的 frontend-stage-command）把 web/dist 复制到
// internal/web/dist 后再编译，从而单二进制自带前端；未构建时该目录只有
// 占位文件，HasBuild 返回 false。
//
// 注意必须用 all: 前缀：SvelteKit 的全部 JS/CSS 在 _app/ 目录下，
// 而 go:embed 默认排除 _ 和 . 开头的条目（经典陷阱）。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embedded embed.FS

// HasBuild 判断是否内嵌了真实前端产物（dist 下存在 index.html）。
func HasBuild() bool {
	b, err := embedded.ReadFile("dist/index.html")
	return err == nil && len(b) > 0
}

// FS 返回内嵌产物的文件系统（调用方需先确认 HasBuild）。
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "dist")
	if err != nil {
		return embedded
	}
	return sub
}
