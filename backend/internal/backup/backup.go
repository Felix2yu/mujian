// Package backup implements scheduled on-disk snapshots of the SQLite
// database via VACUUM INTO. Snapshots are plain .db files in the backup dir
// (<DB dir>/backups), restorable by swapping the file back (or opening it
// directly); a retention count prunes the oldest ones after each run.
package backup

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"mujian/internal/config"
)

// Vacuumer is the DB capability the manager needs; *db.DB implements it.
type Vacuumer interface {
	VacuumInto(path string) error
	Checkpoint() error
}

const filePrefix = "mujian-backup-"

type Manager struct {
	db  Vacuumer
	dir string
	cfg *config.Config

	// exportJSON / exportZip produce the export-style payloads for the "json"
	// and "zip" backup formats; wired by the handlers package (they reuse the
	// download endpoint builders). db format does not use them.
	exportJSON func() ([]byte, error)
	exportZip  func() ([]byte, error)

	// remote uploads the snapshot to S3 (backups/ prefix) when BackupRemote is
	// enabled; nil means remote push is unavailable.
	remote RemotePusher

	mu      sync.Mutex
	lastRun time.Time
	lastErr string

	reschedule chan struct{}
	stop       chan struct{}
	done       chan struct{}
}

func New(db Vacuumer, dir string, cfg *config.Config) *Manager {
	return &Manager{
		db:         db,
		dir:        dir,
		cfg:        cfg,
		reschedule: make(chan struct{}, 1),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// Start launches the scheduler goroutine. The next run time is derived from
// the newest existing backup file, so restarting the process does not reset
// the schedule.
func (m *Manager) Start() {
	go m.loop()
}

func (m *Manager) Stop() {
	close(m.stop)
	<-m.done
}

// SetExporter wires the JSON / ZIP payload builders (see Manager.exportJSON).
func (m *Manager) SetExporter(jsonFn, zipFn func() ([]byte, error)) {
	m.exportJSON = jsonFn
	m.exportZip = zipFn
}

// RemotePusher uploads a snapshot to remote object storage under the given key.
// Wired from main.go (it has access to the S3 client); nil means "no remote
// upload configured". Backup.RunNow only invokes it when BackupRemote is set.
type RemotePusher func(key string, data []byte) error

// SetRemotePusher wires the S3 uploader used when BackupRemote is enabled.
func (m *Manager) SetRemotePusher(p RemotePusher) {
	m.remote = p
}

// Reschedule nudges the loop to recompute the next run time after the
// interval setting changed.
func (m *Manager) Reschedule() {
	select {
	case m.reschedule <- struct{}{}:
	default:
	}
}

// BackupInfo describes one snapshot file in the backup dir.
type BackupInfo struct {
	Name    string `json:"file"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"modified"`
}

// List returns all snapshots, newest first.
func (m *Manager) List() []BackupInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	files := listBackupFiles(m.dir)
	out := make([]BackupInfo, 0, len(files))
	for i := len(files) - 1; i >= 0; i-- {
		info, err := os.Stat(filepath.Join(m.dir, files[i]))
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{Name: files[i], Size: info.Size(), ModTime: info.ModTime().Unix()})
	}
	return out
}

// ValidateName guards against path traversal: only snapshot files inside the
// backup dir may be addressed. Exported for the restore endpoint.
func ValidateName(name string) error {
	if strings.HasPrefix(name, filePrefix) && !strings.ContainsAny(name, "/\\") && filepath.Base(name) == name {
		switch filepath.Ext(name) {
		case ".db", ".json", ".zip":
			return nil
		}
	}
	return fmt.Errorf("invalid backup file name: %q", name)
}

// Read returns the raw bytes of a snapshot (download / restore).
func (m *Manager) Read(name string) ([]byte, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return os.ReadFile(filepath.Join(m.dir, name))
}

// Delete removes one snapshot from the backup dir.
func (m *Manager) Delete(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	err := os.Remove(filepath.Join(m.dir, name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Status reports the last successful backup (unix seconds, 0 = never) and the
// last error message for the settings response.
func (m *Manager) Status() (lastRunAt int64, lastErr string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastRun.IsZero() {
		return 0, m.lastErr
	}
	return m.lastRun.Unix(), m.lastErr
}

func (m *Manager) loop() {
	defer close(m.done)
	m.mu.Lock()
	m.lastRun = newestBackupTime(m.dir)
	m.mu.Unlock()

	for {
		interval := m.cfg.GetBackupIntervalHours()
		if interval <= 0 {
			// 关闭状态：不建 timer（nil timer 的 timer.C 会在 select 里解引用
			// nil），只等停止或重新调度。
			select {
			case <-m.stop:
				return
			case <-m.reschedule:
			}
			continue
		}
		next := m.lastRun.Add(time.Duration(interval) * time.Hour)
		d := time.Until(next)
		if d < 0 {
			d = 0
		}
		timer := time.NewTimer(d)
		select {
		case <-m.stop:
			timer.Stop()
			return
		case <-m.reschedule:
			timer.Stop()
		case <-timer.C:
			if _, err := m.RunNow(); err != nil {
				slog.Error("scheduled backup failed", "err", err)
			}
		}
	}
}

// RunNow performs one backup immediately and returns the snapshot file name.
// Safe to call concurrently with the scheduler (serialized by mu).
func (m *Manager) RunNow() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(m.dir, 0755); err != nil {
		m.lastErr = err.Error()
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	format := m.cfg.GetBackupFormat()
	ext := map[string]string{"db": ".db", "json": ".json", "zip": ".zip"}[format]
	if ext == "" {
		m.lastErr = "unknown backup format: " + format
		return "", fmt.Errorf("unknown backup format: %s", format)
	}
	var payload []byte
	if format == "json" {
		if m.exportJSON == nil {
			m.lastErr = "json exporter not configured"
			return "", fmt.Errorf("json exporter not configured")
		}
		b, err := m.exportJSON()
		if err != nil {
			m.lastErr = err.Error()
			return "", err
		}
		payload = b
	} else if format == "zip" {
		if m.exportZip == nil {
			m.lastErr = "zip exporter not configured"
			return "", fmt.Errorf("zip exporter not configured")
		}
		b, err := m.exportZip()
		if err != nil {
			m.lastErr = err.Error()
			return "", err
		}
		payload = b
	}

	// 写临时文件再原子重命名，半成品快照永远不可见；同一秒内多次备份时
	// 文件名追加序号，避免互相覆盖。
	stamp := time.Now().Format("20060102-150405")
	name := fmt.Sprintf("%s%s%s", filePrefix, stamp, ext)
	for i := 2; func() bool {
		_, err := os.Stat(filepath.Join(m.dir, name))
		return err == nil
	}(); i++ {
		// 序号放扩展名之前，保证 listBackupFiles 的扩展名过滤仍能识别
		name = fmt.Sprintf("%s%s-%d%s", filePrefix, stamp, i, ext)
	}
	tmp := filepath.Join(m.dir, name+".tmp")
	final := filepath.Join(m.dir, name)

	if payload != nil {
		if err := os.WriteFile(tmp, payload, 0644); err != nil {
			m.lastErr = err.Error()
			os.Remove(tmp)
			return "", fmt.Errorf("write backup: %w", err)
		}
	} else {
		// db 快照：VACUUM INTO 要求目标不存在
		if err := m.db.VacuumInto(tmp); err != nil {
			m.lastErr = err.Error()
			os.Remove(tmp)
			return "", fmt.Errorf("vacuum into %s: %w", tmp, err)
		}
	}
	if err := os.Rename(tmp, final); err != nil {
		m.lastErr = err.Error()
		os.Remove(tmp)
		return "", fmt.Errorf("rename snapshot: %w", err)
	}

	// 备份成功后若开启了「上传 S3」，把快照推送到桶内 backups/ 目录。
	// 本地快照已经落盘（上面的 Rename 已生效），即便 S3 上传失败，本机
	// 备份仍然保留，仅本次操作的 S3 部分失败会被上报。
	if m.cfg.GetBackupRemote() {
		if m.remote == nil {
			m.lastErr = "BackupRemote 已开启但未配置 S3 上传器"
			return name, fmt.Errorf("BackupRemote 已开启，但 S3 客户端未注入（请确认 S3 凭据已配置）")
		}
		data, rerr := os.ReadFile(final)
		if rerr != nil {
			m.lastErr = rerr.Error()
			return name, fmt.Errorf("read snapshot for S3 upload: %w", rerr)
		}
		s3key := "backups/" + name
		if perr := m.remote(s3key, data); perr != nil {
			m.lastErr = "S3 上传失败: " + perr.Error()
			return name, fmt.Errorf("S3 上传失败（本地备份已保留）: %w", perr)
		}
		slog.Info("backup pushed to S3", "key", s3key)
	}

	// db 快照之后把 WAL 收缩进主文件：磁盘上的 .db 从此自洽，
	// 即使进程随后崩溃，快照对应的恢复流程也不依赖 WAL。
	if payload == nil {
		if err := m.db.Checkpoint(); err != nil {
			slog.Warn("wal checkpoint after backup", "err", err)
		}
	}

	m.lastRun = time.Now()
	m.lastErr = ""
	m.pruneLocked()
	slog.Info("backup written", "file", name)
	return name, nil
}

// pruneLocked deletes the oldest snapshots beyond the retention count.
func (m *Manager) pruneLocked() {
	keep := m.cfg.GetBackupKeep()
	if keep <= 0 {
		keep = 10
	}
	files := listBackupFiles(m.dir)
	if len(files) <= keep {
		return
	}
	for _, f := range files[:len(files)-keep] {
		if err := os.Remove(filepath.Join(m.dir, f)); err != nil {
			slog.Warn("prune old backup", "file", f, "err", err)
		} else {
			slog.Info("pruned old backup", "file", f)
		}
	}
}

// listBackupFiles returns snapshot file names (no tmp files), oldest first.
func listBackupFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, filePrefix) {
			continue
		}
		switch filepath.Ext(name) {
		case ".db", ".json", ".zip":
		default:
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out) // timestamped names sort chronologically
	return out
}

func newestBackupTime(dir string) time.Time {
	files := listBackupFiles(dir)
	if len(files) == 0 {
		return time.Time{}
	}
	info, err := os.Stat(filepath.Join(dir, files[len(files)-1]))
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
