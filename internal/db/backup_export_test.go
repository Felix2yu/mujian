package db

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"mujian/internal/models"
)

// sqliteHeader 是 SQLite 文件的固定头 16 字节，快照与解压出来的库都按它验货。
const sqliteHeader = "SQLite format 3\x00"

// countingWriter 记录被写了几次，用来证明导出是边生成边写而不是一次性灌进来的。
type countingWriter struct {
	buf    bytes.Buffer
	chunks int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.chunks++
	return c.buf.Write(p)
}

// newExportFixture 走生产同一条路径建库（建目录 + 迁移 + WAL），返回库路径与句柄。
// 句柄**不关**：外部备份工具正是在应用还在读写的时候被 docker exec 触发的。
func newExportFixture(t *testing.T) (string, *DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mujian.db")
	database, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(database.Close)
	return path, database
}

func seedRecord(t *testing.T, database *DB, id string, when time.Time) {
	t.Helper()
	rec := sampleRecord(id, when.Unix())
	if err := database.UpsertRecord(rec); err != nil {
		t.Fatalf("UpsertRecord %s: %v", id, err)
	}
}

func readFileHex(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// TestWriteSQLiteSnapshotMagicAndRows 覆盖契约里的 db 形态：产物以 SQLite 头开始、
// 可以重新打开并读回记录，且 WAL 里最后一条已提交的数据也在快照里
// （这正是热拷贝 .db 文件做不到的：写到一半的 WAL 不会被合并进主文件）。
func TestWriteSQLiteSnapshotMagicAndRows(t *testing.T) {
	path, writer := newExportFixture(t)
	base := time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC)
	seedRecord(t, writer, "snap-1", base)
	seedRecord(t, writer, "snap-2", base.Add(time.Hour)) // 只落在 -wal 里的那一条

	wal := path + "-wal"
	if info, err := os.Stat(wal); err != nil || info.Size() == 0 {
		t.Fatalf("前置条件不成立：-wal 应存在且非空（err=%v）", err)
	}

	export, err := OpenForExport(path)
	if err != nil {
		t.Fatalf("OpenForExport: %v", err)
	}
	defer export.Close()

	var out countingWriter
	if err := export.WriteSQLiteSnapshot(&out); err != nil {
		t.Fatalf("WriteSQLiteSnapshot: %v", err)
	}
	blob := out.buf.Bytes()
	if len(blob) < 512 {
		t.Fatalf("快照过小: %d 字节", len(blob))
	}
	if got := string(blob[:16]); got != sqliteHeader {
		t.Errorf("快照头 16 字节 = %q, want %q", got, sqliteHeader)
	}

	// 落盘后重新打开快照读回记录
	snapPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := os.WriteFile(snapPath, blob, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ro, err := OpenForExport(snapPath)
	if err != nil {
		t.Fatalf("OpenForExport(snapshot): %v", err)
	}
	defer ro.Close()
	records, err := ro.ListRecords(RecordFilter{NoLimit: true})
	if err != nil {
		t.Fatalf("ListRecords: %v", err)
	}
	ids := make([]string, 0, len(records))
	for _, r := range records {
		ids = append(ids, r.ID)
	}
	for _, want := range []string{"snap-1", "snap-2"} {
		if !slices.Contains(ids, want) {
			t.Errorf("快照里缺少 %s（实际 %v）", want, ids)
		}
	}
}

// TestOpenForExportDoesNotMigrateOrModifySource 覆盖「导出进程不改库」：
// 用旧 schema 的 fixture（migrate() 会无条件 DROP users/shows/scene_sorts/actors）
// 加前后 sha256 比对。迁移一旦跑起来，这两条都会立刻失败。
func TestOpenForExportDoesNotMigrateOrModifySource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")

	// 一份"应用还没升级过"的库：只有旧表，没有当前 schema 的任何东西。
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := conn.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO users (name) VALUES ('ada')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	before := readFileHex(t, path)

	export, err := OpenForExport(path)
	if err != nil {
		t.Fatalf("OpenForExport: %v", err)
	}
	var blob bytes.Buffer
	if err := export.WriteSQLiteSnapshot(&blob); err != nil {
		t.Fatalf("WriteSQLiteSnapshot: %v", err)
	}
	export.Close()

	if got := readFileHex(t, path); got != before {
		t.Errorf("导出进程改写了源库: sha256 %s -> %s", before[:12], got[:12])
	}

	// 源库里旧表仍在、当前 schema 的表没被建出来，且没有多出目录。
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopen source: %v", err)
	}
	defer check.Close()
	for name, want := range map[string]int{"users": 1, "records": 0} {
		var n int
		if err := check.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil {
			t.Fatalf("query %s: %v", name, err)
		}
		if n != want {
			t.Errorf("源库中表 %s 存在性 = %d, want %d（导出不得跑迁移）", name, n, want)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("导出进程在数据目录里建了目录 %s", e.Name())
		}
		if suffix := strings.TrimPrefix(e.Name(), filepath.Base(path)); suffix != "" &&
			suffix != "-wal" && suffix != "-shm" && suffix != "-journal" {
			t.Errorf("导出进程多写出了 %s", e.Name())
		}
	}

	// 快照本身也必须是同一份旧 schema：证明它带的是源库的内容而不是迁移后的空库。
	snapPath := filepath.Join(t.TempDir(), "legacy-snapshot.db")
	if err := os.WriteFile(snapPath, blob.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	snap, err := sql.Open("sqlite", snapPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer snap.Close()
	var name string
	if err := snap.QueryRow(`SELECT name FROM users LIMIT 1`).Scan(&name); err != nil {
		t.Fatalf("read legacy row from snapshot: %v", err)
	}
	if name != "ada" {
		t.Errorf("快照里的旧数据 = %q, want ada", name)
	}
}

// TestOpenForExportRejectsMissingDB 覆盖「库不存在时报错且不产出任何东西」：
// 导出进程绝不新建空库，也不替主进程建目录。
func TestOpenForExportRejectsMissingDB(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nested", "mujian.db")

	export, err := OpenForExport(missing)
	if err == nil {
		export.Close()
		t.Fatal("库文件不存在时 OpenForExport 应报错")
	}
	if !strings.Contains(err.Error(), "数据文件不可用") {
		t.Errorf("报错内容应说明数据文件不可用, got %q", err.Error())
	}
	if _, err := os.Stat(filepath.Join(dir, "nested")); !os.IsNotExist(err) {
		t.Errorf("导出进程创建了缺失的父目录: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("数据目录应保持为空, got %v", entries)
	}
}

// TestWriteBackupZIP 覆盖 zip 形态：Go 的 zip 读得开，包内有 SQLite 快照（魔数完好）、
// 可在线导入的 data.json，以及 uploads 目录下的封面文件（含缩略图与回收站）。
func TestWriteBackupZIP(t *testing.T) {
	path, writer := newExportFixture(t)
	uploadDir := filepath.Join(filepath.Dir(path), "uploads")

	seedRecord(t, writer, "zip-1", time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC))
	cover := []byte("fake-avif-bytes-for-test")
	for key, data := range map[string][]byte{
		"covers/aaaa.avif":       cover,
		"covers/aaaa.thumb.avif": append([]byte("thumb:"), cover...),
		"covers_trash/bbbb.avif": cover,
		"stray/readme.txt":       []byte("uploads 下的其它文件也要带走"),
	} {
		full := filepath.Join(uploadDir, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(full, data, 0o600); err != nil {
			t.Fatalf("WriteFile %s: %v", key, err)
		}
	}

	export, err := OpenForExport(path)
	if err != nil {
		t.Fatalf("OpenForExport: %v", err)
	}
	defer export.Close()

	var out countingWriter
	files, err := export.WriteBackupZIP(&out, uploadDir)
	if err != nil {
		t.Fatalf("WriteBackupZIP: %v", err)
	}
	if files != 4 {
		t.Errorf("打包文件数 = %d, want 4", files)
	}
	// 压缩包必须边生成边写（整包进内存会被媒体库撑爆），一次以上才算流式。
	if out.chunks < 2 {
		t.Errorf("压缩包写入次数 = %d, 期望分多次流式写出", out.chunks)
	}

	blob := out.buf.Bytes()
	if string(blob[:4]) != "PK\x03\x04" {
		t.Errorf("zip 魔数 = %q, want PK\\x03\\x04", blob[:4])
	}
	zr, err := zip.NewReader(bytes.NewReader(blob), int64(len(blob)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	entries := map[string][]byte{}
	for _, zf := range zr.File {
		entries[zf.Name] = readZipEntry(t, zf)
	}

	snap, ok := entries[ZipDBEntry]
	if !ok {
		t.Fatalf("包内缺少 %s, got %v", ZipDBEntry, sortedKeys(entries))
	}
	if len(snap) < 16 || string(snap[:16]) != sqliteHeader {
		t.Errorf("包内 %s 不是 SQLite 快照（头 %q）", ZipDBEntry, snap[:min(16, len(snap))])
	}
	for _, want := range []string{"covers/aaaa.avif", "covers/aaaa.thumb.avif", "covers_trash/bbbb.avif", "stray/readme.txt"} {
		if _, ok := entries[want]; !ok {
			t.Errorf("包内缺少 %s, got %v", want, sortedKeys(entries))
		}
	}
	if got := string(entries["covers/aaaa.avif"]); got != string(cover) {
		t.Errorf("封面内容被改动: %q", got)
	}

	data, ok := entries[ZipDataEntry]
	if !ok {
		t.Fatalf("包内缺少 %s, got %v", ZipDataEntry, sortedKeys(entries))
	}
	var payload models.ExportData
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("data.json 解析失败: %v", err)
	}
	if !slices.ContainsFunc(payload.Records, func(r models.Record) bool { return r.ID == "zip-1" }) {
		t.Errorf("data.json 里没有 zip-1 这条记录")
	}
}

// TestWriteBackupZIPWithoutUploads 说明 S3 / 首次部署这种「库有、目录没有」的情况：
// 压缩包照常产出，只是没有封面文件。
func TestWriteBackupZIPWithoutUploads(t *testing.T) {
	path, writer := newExportFixture(t)
	_ = writer

	export, err := OpenForExport(path)
	if err != nil {
		t.Fatalf("OpenForExport: %v", err)
	}
	defer export.Close()

	var blob bytes.Buffer
	files, err := export.WriteBackupZIP(&blob, filepath.Join(t.TempDir(), "never-created"))
	if err != nil {
		t.Fatalf("WriteBackupZIP: %v", err)
	}
	if files != 0 {
		t.Errorf("目录不存在时不该有文件进包, got %d", files)
	}
	zr, err := zip.NewReader(bytes.NewReader(blob.Bytes()), int64(blob.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	names := make([]string, 0, len(zr.File))
	for _, zf := range zr.File {
		names = append(names, zf.Name)
	}
	for _, want := range []string{ZipDataEntry, ZipDBEntry} {
		if !slices.Contains(names, want) {
			t.Errorf("包内缺少 %s, got %v", want, names)
		}
	}
}

func readZipEntry(t *testing.T, zf *zip.File) []byte {
	t.Helper()
	rc, err := zf.Open()
	if err != nil {
		t.Fatalf("open %s: %v", zf.Name, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read %s: %v", zf.Name, err)
	}
	return b
}

func sortedKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
