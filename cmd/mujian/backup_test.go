package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mujian/internal/db"
)

// sqliteMagic 是裸快照的头 16 字节；zipMagic 是压缩包的头 4 字节。
// 两者用来判定 `mujian backup` 默认到底给了哪种形态。
const (
	sqliteMagic = "SQLite format 3\x00"
	zipMagic    = "PK\x03\x04"
)

// preparedDB 用生产路径（建目录 + 迁移 + WAL）建一份库，返回库文件路径。
// 建完即关：这些测试只验 CLI 的形态选择，不需要与写进程并存。
func preparedDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mujian.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	database.Close()
	return path
}

// TestRunBackupDefaultFormatIsDB 覆盖契约「默认格式 db」：不带任何参数时，
// stdout 拿到的是 SQLite 快照而不是压缩包。库路径也按 DB_PATH 解析，
// 备份工具无需再传一次路径。
func TestRunBackupDefaultFormatIsDB(t *testing.T) {
	path := preparedDB(t)
	t.Setenv("DB_PATH", path)

	var blob bytes.Buffer
	if err := runBackup(nil, &blob); err != nil {
		t.Fatalf("runBackup: %v", err)
	}
	got := blob.Bytes()
	if len(got) < 16 {
		t.Fatalf("产物过小: %d 字节", len(got))
	}
	if string(got[:16]) != sqliteMagic {
		t.Errorf("默认产物应是 SQLite 快照（头 %q）", got[:16])
	}
	if string(got[:4]) == zipMagic {
		t.Errorf("默认不该输出 zip")
	}
}

func TestRunBackupZipFormat(t *testing.T) {
	path := preparedDB(t)
	uploadDir := filepath.Join(filepath.Dir(path), "uploads")
	if err := os.MkdirAll(filepath.Join(uploadDir, "covers"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "covers", "cc.avif"), []byte("cover-bytes"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("DB_PATH", path)
	t.Setenv("UPLOAD_DIR", uploadDir)

	var blob bytes.Buffer
	if err := runBackup([]string{"--format", "zip"}, &blob); err != nil {
		t.Fatalf("runBackup --format zip: %v", err)
	}
	got := blob.Bytes()
	if len(got) < 4 || string(got[:4]) != zipMagic {
		t.Fatalf("zip 形态的产物应以 zip 魔数开始, got %q", got[:min(4, len(got))])
	}
	for _, want := range []string{"mujian.db", "data.json", "covers/cc.avif"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("压缩包里应有 %s", want)
		}
	}
}

// TestRunBackupMissingDBWritesNothing 覆盖契约第 7 条：库不存在时
// stdout 必须是零字节（工具那边按魔数校验才不会收下坏产物），并给出报错。
func TestRunBackupMissingDBWritesNothing(t *testing.T) {
	dir := t.TempDir()
	// uploads 也指到临时目录：库都不存在时，导出进程更不该碰数据目录。
	t.Setenv("UPLOAD_DIR", filepath.Join(dir, "uploads"))
	for _, args := range [][]string{nil, {"--format", "zip"}, {"--format", "bogus"}} {
		var blob bytes.Buffer
		err := runBackupWithDB(filepath.Join(dir, "absent", "mujian.db"), args, &blob)
		if err == nil {
			t.Fatalf("%v: 库不存在时应报错", args)
		}
		if blob.Len() != 0 {
			t.Errorf("%v: stdout 应零字节, got %d", args, blob.Len())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "absent")); !os.IsNotExist(err) {
		t.Errorf("导出进程不该把缺失的父目录建出来")
	}
	if _, err := os.Stat(filepath.Join(dir, "uploads")); !os.IsNotExist(err) {
		t.Errorf("导出进程不该建 uploads 目录")
	}
}

// TestRunBackupUnknownFormat 覆盖参数校验：格式写错时不打开数据库、不写字节。
func TestRunBackupUnknownFormat(t *testing.T) {
	path := preparedDB(t)
	var blob bytes.Buffer
	if err := runBackup([]string{"--db", path, "--format", "tar"}, &blob); err == nil {
		t.Fatal("未知格式应报错")
	} else if !strings.Contains(err.Error(), "未知格式") {
		t.Errorf("报错应说明格式未知, got %q", err.Error())
	}
	if blob.Len() != 0 {
		t.Errorf("stdout 应零字节, got %d", blob.Len())
	}
}

// runBackupWithDB 走 --db 参数指定库路径（与 DB_PATH 等价的另一条入口）。
func runBackupWithDB(path string, args []string, w *bytes.Buffer) error {
	return runBackup(append([]string{"--db", path}, args...), w)
}
