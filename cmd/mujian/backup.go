package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"mujian/internal/config"
	"mujian/internal/db"
)

// runBackup 实现 `mujian backup`：把一份备份写到 w（由调用方给 os.Stdout）。
//
// 外部备份工具（docker-db-auto-backup）经 `docker exec <容器> sh -c "… backup"` 采集
// stdout，所以这里只用应用自己的路径解析（DB_PATH，默认 ./data/mujian.db），不建目录、
// 不跑迁移、不写种子、不起后台任务与 HTTP。config.Load 只读环境变量，与主进程共用
// 同一套解析口径，备份工具无需再配一份路径。
//
// 两种形态：
//
//	db  （默认）：SQLite VACUUM INTO 的事务级一致快照，恢复即把文件放回数据目录；
//	zip          ：快照 + data.json + uploads 目录下的全部封面文件（媒体库大，产物也大）。
//
// 日志一律走 stderr，stdout 只承载备份数据本身。
func runBackup(args []string, w io.Writer) error {
	cfg := config.Load()

	fs := flag.NewFlagSet("mujian backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr) // 用法与解析错误都走 stderr：stdout 只留给备份字节
	dbPath := fs.String("db", cfg.DBPath, "SQLite 数据文件路径（默认取 DB_PATH）")
	format := fs.String("format", "db", "导出格式：db（SQLite 一致性快照）或 zip（快照 + data.json + 封面文件）")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	selected := strings.ToLower(strings.TrimSpace(*format))
	if selected != "db" && selected != "zip" {
		return fmt.Errorf("未知格式 %q（可选 db / zip）", *format)
	}

	// 导出专用的打开方式：不建目录、不跑迁移、不写种子，文件不存在直接报错。
	database, err := db.OpenForExport(*dbPath)
	if err != nil {
		return err
	}
	defer database.Close()

	if selected == "zip" {
		files, err := database.WriteBackupZIP(w, cfg.UploadDir)
		if err != nil {
			return err
		}
		slog.Info("backup zip written", "db", *dbPath, "covers", files)
		return nil
	}
	return database.WriteSQLiteSnapshot(w)
}
