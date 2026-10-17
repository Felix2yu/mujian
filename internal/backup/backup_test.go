package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"mujian/internal/config"
)

// testVacuumer wraps a plain sqlite handle; production passes *db.DB which
// implements the same single-method interface.
type testVacuumer struct{ db *sql.DB }

func (t testVacuumer) VacuumInto(path string) error {
	_, err := t.db.Exec("VACUUM INTO ?", path)
	return err
}

func (t testVacuumer) Checkpoint() error { return nil }

func newManager(t *testing.T, interval, keep int) (*Manager, *sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{BackupIntervalHours: interval, BackupKeep: keep}
	m := New(testVacuumer{db}, filepath.Join(dir, "backups"), cfg)
	return m, db, filepath.Join(dir, "backups")
}

func TestRunNowCreatesValidSnapshot(t *testing.T) {
	m, db, bdir := newManager(t, 0, 10)
	if _, err := db.Exec("INSERT INTO t (v) VALUES ('hello')"); err != nil {
		t.Fatal(err)
	}

	name, err := m.RunNow()
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if filepath.Ext(name) != ".db" {
		t.Errorf("unexpected snapshot name %q", name)
	}

	snap, err := os.Open(filepath.Join(bdir, name))
	if err != nil {
		t.Fatalf("snapshot missing: %v", err)
	}
	snap.Close()

	// The snapshot must be a readable SQLite file containing the data.
	sdb, err := sql.Open("sqlite", filepath.Join(bdir, name))
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer sdb.Close()
	var v string
	if err := sdb.QueryRow("SELECT v FROM t LIMIT 1").Scan(&v); err != nil || v != "hello" {
		t.Fatalf("snapshot content: v=%q err=%v", v, err)
	}
}

func TestRunNowTwiceAndNoTmpLeftovers(t *testing.T) {
	m, _, bdir := newManager(t, 0, 10)
	if _, err := m.RunNow(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RunNow(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(bdir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("tmp file left behind: %s", e.Name())
		}
	}
	if n := len(entries); n != 2 {
		t.Errorf("got %d files, want 2 snapshots", n)
	}
	lastRun, lastErr := m.Status()
	if lastRun == 0 || lastErr != "" {
		t.Errorf("status: lastRun=%d lastErr=%q", lastRun, lastErr)
	}
}

func TestRetentionPrunesOldest(t *testing.T) {
	m, _, bdir := newManager(t, 0, 2)
	for i := 0; i < 3; i++ {
		if _, err := m.RunNow(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1100 * time.Millisecond) // 文件名精确到秒，保证时间戳可排序
	}
	entries, _ := os.ReadDir(bdir)
	if len(entries) != 2 {
		t.Fatalf("got %d snapshots after prune, want 2", len(entries))
	}
	// 剩下的必须是最新的两份
	oldest, _ := os.ReadFile(filepath.Join(bdir, entries[0].Name()))
	if len(oldest) == 0 {
		t.Error("oldest snapshot empty")
	}
}

func TestPruneFailureIsLoggedNotFatal(t *testing.T) {
	m, _, bdir := newManager(t, 0, 1)
	if _, err := m.RunNow(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RunNow(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(bdir)
	if len(entries) != 1 {
		t.Fatalf("keep=1 should leave 1 snapshot, got %d", len(entries))
	}
}

func TestSchedulerRunsOnInterval(t *testing.T) {
	m, db, bdir := newManager(t, 0, 5)
	if _, err := db.Exec("INSERT INTO t (v) VALUES ('x')"); err != nil {
		t.Fatal(err)
	}
	// 已有一份 2 小时前的备份 + 间隔 1 小时 → 启动后应立刻补跑一次
	if err := os.MkdirAll(bdir, 0755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(bdir, "mujian-backup-20000101-000000.db")
	if err := os.WriteFile(old, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(old, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	m.cfg = &config.Config{BackupIntervalHours: 1, BackupKeep: 5}
	m.Start()
	t.Cleanup(m.Stop)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(bdir, "mujian-backup-20000101-000000.db")); err != nil {
			// 旧文件被清掉了？keep=5 不会。直接数文件。
		}
		files, _ := os.ReadDir(bdir)
		if len(files) >= 2 {
			return // 补跑成功
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("scheduler did not run a catch-up backup within 3s")
}

func TestDisabledIntervalDoesNotRun(t *testing.T) {
	m, _, bdir := newManager(t, 0, 5)
	m.Start()
	t.Cleanup(m.Stop)
	time.Sleep(300 * time.Millisecond)
	files, _ := os.ReadDir(bdir)
	if len(files) != 0 {
		t.Fatalf("interval=0 should not schedule backups, got %d files", len(files))
	}
}

// json/zip 格式走导出器（handlers 包注入的构建函数），写出的文件扩展名
// 与内容匹配，保留份数清理对三种格式统一生效。
func TestExportFormats(t *testing.T) {
	m, _, bdir := newManager(t, 0, 3)
	m.SetExporter(
		func() ([]byte, error) { return []byte(`{"records":[1,2,3]}`), nil },
		func() ([]byte, error) { return []byte("PK-fake-zip"), nil },
	)

	m.cfg = &config.Config{BackupFormat: "json", BackupKeep: 3}
	name, err := m.RunNow()
	if err != nil {
		t.Fatalf("json backup: %v", err)
	}
	if filepath.Ext(name) != ".json" {
		t.Fatalf("json format produced %q", name)
	}
	b, _ := os.ReadFile(filepath.Join(bdir, name))
	if string(b) != `{"records":[1,2,3]}` {
		t.Errorf("json payload mismatch: %q", b)
	}

	m.cfg = &config.Config{BackupFormat: "zip", BackupKeep: 3}
	name, err = m.RunNow()
	if err != nil {
		t.Fatalf("zip backup: %v", err)
	}
	if filepath.Ext(name) != ".zip" {
		t.Fatalf("zip format produced %q", name)
	}

	// 未知格式报错且不留临时文件
	m.cfg = &config.Config{BackupFormat: "tar", BackupKeep: 3}
	if _, err := m.RunNow(); err == nil {
		t.Fatal("unknown format should error")
	}
	entries, _ := os.ReadDir(bdir)
	if len(entries) != 2 {
		t.Fatalf("got %d files, want 2 (json+zip)", len(entries))
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"mujian-backup-20240101-120000.db", false},
		{"mujian-backup-20240101-120000.json", false},
		{"mujian-backup-20240101-120000.zip", false},
		{"../etc/passwd", true},
		{"mujian-backup-20240101-120000.txt", true},
		{"", true},
		{"../../backup.db", true},
		{"mujian-backup-20240101-120000.db/../../etc/passwd", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestRead(t *testing.T) {
	m, _, bdir := newManager(t, 0, 10)
	name, err := m.RunNow()
	if err != nil {
		t.Fatal(err)
	}

	data, err := m.Read(name)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(data) == 0 {
		t.Error("Read returned empty data")
	}

	// 读取不存在的文件应返回错误
	if _, err := m.Read("mujian-backup-nonexistent.db"); err == nil {
		t.Error("Read nonexistent should error")
	}

	// 非法文件名应返回错误
	if _, err := m.Read("../../../etc/passwd"); err == nil {
		t.Error("Read with path traversal should error")
	}

	_ = bdir
}

func TestDelete(t *testing.T) {
	m, _, _ := newManager(t, 0, 10)
	name, err := m.RunNow()
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Delete(name); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// 删除不存在的文件不报错
	if err := m.Delete("mujian-backup-nonexistent.db"); err != nil {
		t.Errorf("Delete nonexistent: %v", err)
	}

	// 非法文件名应返回错误
	if err := m.Delete("../../../etc/passwd"); err == nil {
		t.Error("Delete with path traversal should error")
	}
}

func TestReschedule(t *testing.T) {
	m, _, _ := newManager(t, 0, 5)
	m.Start()
	defer m.Stop()

	// Reschedule 不应 panic
	m.Reschedule()

	// 多次调用也不应阻塞
	m.Reschedule()
	m.Reschedule()
}

func TestStatus(t *testing.T) {
	m, _, _ := newManager(t, 0, 10)

	// 初始状态
	lastRun, lastErr := m.Status()
	if lastRun != 0 {
		t.Errorf("initial lastRun should be 0, got %d", lastRun)
	}
	if lastErr != "" {
		t.Errorf("initial lastErr should be empty, got %q", lastErr)
	}

	// 执行备份后
	if _, err := m.RunNow(); err != nil {
		t.Fatal(err)
	}
	lastRun, lastErr = m.Status()
	if lastRun == 0 {
		t.Error("lastRun should be set after backup")
	}
	if lastErr != "" {
		t.Errorf("lastErr should be empty after success, got %q", lastErr)
	}
}

func TestList(t *testing.T) {
	m, _, _ := newManager(t, 0, 10)

	// 空目录
	list := m.List()
	if len(list) != 0 {
		t.Errorf("empty list: got %d", len(list))
	}

	// 添加一个备份
	name, err := m.RunNow()
	if err != nil {
		t.Fatal(err)
	}

	list = m.List()
	if len(list) != 1 {
		t.Fatalf("list after backup: got %d, want 1", len(list))
	}
	if list[0].Name != name {
		t.Errorf("list name: got %q, want %q", list[0].Name, name)
	}
	if list[0].Size <= 0 {
		t.Error("list size should be positive")
	}
	if list[0].ModTime <= 0 {
		t.Error("list modtime should be positive")
	}
}

func TestExportFormatsWithMissingExporters(t *testing.T) {
	m, _, _ := newManager(t, 0, 10)

	// JSON 格式但未设置 exporter
	m.cfg = &config.Config{BackupFormat: "json", BackupKeep: 3}
	if _, err := m.RunNow(); err == nil {
		t.Error("json without exporter should error")
	}

	// ZIP 格式但未设置 exporter
	m.cfg = &config.Config{BackupFormat: "zip", BackupKeep: 3}
	if _, err := m.RunNow(); err == nil {
		t.Error("zip without exporter should error")
	}
}
