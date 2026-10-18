// 备份导出（外部备份工具 / CLI 子命令用的流式路径）。
//
// 与 internal/backup 的调度器分工不同：那边按间隔把快照写成数据目录里的文件，
// 这里把同一份 VACUUM INTO 快照直接流到调用方给的 writer 上，供 `mujian backup`
// 经 stdout 交给外部工具（docker exec 采集），全程不把整包读进内存。
package db

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// 备份压缩包内部的固定布局。包内名字与界面导出（data.json + covers/）保持一致，
// 这样这份压缩包仍能被「恢复备份」/ restore_from_backup 在线导入；改这里要同步改那边。
const (
	// ZipDataEntry 是包内的全量 JSON 导出，与 GET /api/export 同构。
	ZipDataEntry = "data.json"
	// ZipDBEntry 是包内的 SQLite 一致性快照：停机后把它放回数据目录即完成恢复。
	// 在线导入通道只读 data.json，认不得 SQLite 快照——它是冷备用的一份文件。
	ZipDBEntry = "mujian.db"
)

// OpenForExport 以「只做导出」的方式打开既有数据库：不建目录、不跑迁移、不写种子。
//
// 外部备份工具（docker-db-auto-backup）经 docker exec 触发 `mujian backup` 时，应用主进程
// 正在读写这个库。表结构升级只应由应用自己执行，一个"顺手打开数据库"的导出进程不该把
// schema 改了，更不该与它抢写。文件不存在时直接报错——那是路径配错，不是要新建空库，
// 导出进程绝不能产出一份空数据库。
func OpenForExport(dbPath string) (*DB, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("数据文件不可用: %w", err)
	}
	// DSN 刻意不设 journal_mode：导出连接不得改写库头部（WAL 会持久在库头里，
	// 已有的 -wal 数据照常读取）；busy_timeout 兜住与主进程偶发的读锁冲突。
	conn, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", dbPath))
	if err != nil {
		return nil, fmt.Errorf("open db for export: %w", err)
	}
	// 导出是顺序读，一条连接足够，也避免多余连接各自开 -shm 句柄。
	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping db for export: %w", err)
	}
	return &DB{conn: conn, loc: time.UTC, artistNameCache: map[string]string{}}, nil
}

// WriteSQLiteSnapshot 把数据库以 SQLite 自身的一致性快照写到 w。
//
// 用 VACUUM INTO 而不是复制文件：热拷贝正在被写入的 db/-wal/-shm 不是原子操作，
// 可能拷出损坏的副本或漏掉 WAL 里已提交的数据；VACUUM INTO 由 SQLite 在事务里
// 生成一份自洽的快照，WAL 模式下也不阻塞写入。快照与自动备份共用同一条
// VacuumInto，只是落点从数据目录里的文件换成了一条流。
//
// 快照先落在临时目录再流式输出：VACUUM INTO 要求目标不存在，且这样无论调用方
// 读到一半断开，都不会在数据目录里留下半成品。
func (db *DB) WriteSQLiteSnapshot(w io.Writer) error {
	dir, err := os.MkdirTemp("", "mujian-snapshot-")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(dir)

	target := filepath.Join(dir, "snapshot.db")
	if err := db.VacuumInto(target); err != nil {
		return fmt.Errorf("导出数据库快照失败: %w", err)
	}
	if err := streamFile(w, target); err != nil {
		return fmt.Errorf("写出快照失败: %w", err)
	}
	return nil
}

// WriteBackupZIP 把「SQLite 快照 + data.json + uploads 目录」边生成边写进 w，
// 返回打进包里的封面文件数。
//
// 为什么要这三种形态：快照是库的一致性冷备（停机换回文件即可）；data.json 让这份包
// 仍能走现有的在线导入通道；封面文件在库外（UPLOAD_DIR），只备份数据库等于静默漏掉
// 整个媒体库。压缩包里的文件是字节流，绝不整包进内存——封面动辄上百 MB。
//
// 与界面导出的差别：那里只带记录引用到的封面，这里带 uploads 目录下的每个文件
// （含缩略图与回收站里尚未清空的封面）。备份的底线是漏不得，宁大不缺。
// S3 存储模式下封面不在本地目录，此时包内只有数据库与 data.json。
func (db *DB) WriteBackupZIP(w io.Writer, uploadDir string) (int, error) {
	data, err := db.Export()
	if err != nil {
		return 0, fmt.Errorf("导出 JSON 失败: %w", err)
	}

	dir, err := os.MkdirTemp("", "mujian-backup-")
	if err != nil {
		return 0, fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(dir)
	snapshot := filepath.Join(dir, "snapshot.db")
	if err := db.VacuumInto(snapshot); err != nil {
		return 0, fmt.Errorf("导出数据库快照失败: %w", err)
	}

	zw := zip.NewWriter(w)
	if err := writeZipJSON(zw, ZipDataEntry, data); err != nil {
		_ = zw.Close() // 先收掉半截包的错误，真正的失败原因由上面返回
		return 0, err
	}
	if _, err := zipOneFile(zw, ZipDBEntry, snapshot); err != nil {
		_ = zw.Close()
		return 0, err
	}
	files, err := zipUploadDir(zw, uploadDir)
	if err != nil {
		_ = zw.Close()
		return files, err
	}
	if err := zw.Close(); err != nil {
		return files, fmt.Errorf("完成压缩包失败: %w", err)
	}
	return files, nil
}

// writeZipJSON 把一个值作为缩进 JSON 条目写进压缩包（与界面导出的排版一致）。
func writeZipJSON(zw *zip.Writer, name string, v any) error {
	f, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("创建压缩包条目 %s 失败: %w", name, err)
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("写入压缩包条目 %s 失败: %w", name, err)
	}
	return nil
}

// zipUploadDir 把 uploads 目录下的每个文件按相对路径打进压缩包，于是包内布局
// 与磁盘完全一致（covers/<hash>.avif、covers_trash/…），导入侧按 covers/ 前缀认领封面。
// 目录不存在（纯 S3 部署、或还没上传过封面）不算失败，只是没有文件可带。
func zipUploadDir(zw *zip.Writer, root string) (int, error) {
	clean := filepath.Clean(root)
	info, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Warn("uploads 目录不存在，压缩包只含数据库", "dir", root)
			return 0, nil
		}
		return 0, fmt.Errorf("读取 uploads 目录失败: %w", err)
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("uploads 路径不是目录: %s", root)
	}

	files := 0
	walkErr := filepath.WalkDir(clean, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // 符号链接与设备文件不进包
		}
		rel, rerr := filepath.Rel(clean, path)
		if rerr != nil {
			return rerr
		}
		n, zerr := zipOneFile(zw, filepath.ToSlash(rel), path)
		files += n
		return zerr
	})
	if walkErr != nil {
		return files, fmt.Errorf("打包封面文件失败: %w", walkErr)
	}
	return files, nil
}

// zipOneFile 把一个文件流式写进压缩包条目，返回写入份数（0 表示已跳过）。
// 打开时文件已经没了（回收站清空、格式转换改名）按跳过处理——备份不该因为
// 一张正在消失的图整体失败。
func zipOneFile(zw *zip.Writer, name, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("读取 %s 失败: %w", name, err)
	}
	defer f.Close()

	w, err := zw.Create(name)
	if err != nil {
		return 0, fmt.Errorf("创建压缩包条目 %s 失败: %w", name, err)
	}
	if _, err := io.Copy(w, f); err != nil {
		return 0, fmt.Errorf("写入压缩包条目 %s 失败: %w", name, err)
	}
	return 1, nil
}

// streamFile 把磁盘上的一个文件原样拷到 w。
func streamFile(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}
