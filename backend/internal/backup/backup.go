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

// Reschedule nudges the loop to recompute the next run time after the
// interval setting changed.
func (m *Manager) Reschedule() {
	select {
	case m.reschedule <- struct{}{}:
	default:
	}
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
