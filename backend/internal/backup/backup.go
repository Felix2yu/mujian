// Package backup implements scheduled on-disk snapshots of the SQLite
// database via VACUUM INTO. Snapshots are plain .db files in the backup dir
// (<DB dir>/backups), restorable by swapping the file back (or opening it
// directly); a retention count prunes the oldest ones after each run.
package backup

import (
	"errors"
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

const (
	filePrefix = "mujian-backup-"

	// retryBaseDelay 是备份失败后第一次重试的等待时间，之后每失败一次翻倍，
	// 上限为配置的备份间隔。没有它时，RunNow 失败不会推进调度基准，
	// 定时器 duration 恒为 0，会以「写代码的速度」反复重跑并不断产出快照
	// （表现为：设置了每天备份，备份历史却每分钟多一份）。
	retryBaseDelay = 5 * time.Minute

	// maxDailyBackups 是单日写出的快照硬上限（兜底闸）：调度器即使再出缺陷
	// （重复触发、定时器算错、远端推送失败重试），一天也最多写这么多份，
	// 到额即跳过本次，次日配额自动重置。
	maxDailyBackups = 24

	// defaultKeep 是未配置保留份数时的兜底值。
	defaultKeep = 10
)

// errDailyQuota 标记「当日写出的快照已达上限」而跳过的备份。它不算故障，
// 因此不参与失败退避（下一轮仍按正常间隔排定）。
var errDailyQuota = errors.New("当日备份数量已达上限")

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
	// remoteDelete removes one object from S3 by its full key; nil means the
	// local-only mode (nothing to clean up remotely).
	remoteDelete RemoteDeleter

	mu      sync.Mutex
	lastRun time.Time
	lastErr string

	// nextRun 是已排定的下一次运行时刻；为零表示尚未排定（首次启动 /
	// 自动备份曾被关闭），此时由 lastRun + 间隔推导。
	nextRun time.Time
	// failures 统计连续失败次数，用于 failures 的指数退避。
	failures int

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

// RemoteDeleter removes one object from remote object storage by its full key.
// Wired from main.go; nil means deletions stay local-only.
type RemoteDeleter func(key string) error

// SetRemoteDeleter wires the S3 remover used to keep backups/ in sync with
// locally deleted snapshots.
func (m *Manager) SetRemoteDeleter(d RemoteDeleter) {
	m.remoteDelete = d
}

// deleteRemoteLocked 尽力同步删除远端副本：未开启推送、未装配删除器、远端本就
// 没有该对象（例如当初上传失败）都不算错误。返回是否确实删掉了远端对象。
func (m *Manager) deleteRemoteLocked(name string) bool {
	if m.remoteDelete == nil || !m.cfg.GetBackupRemote() {
		return false
	}
	if err := m.remoteDelete(remoteKey(name)); err != nil {
		slog.Warn("delete remote backup", "file", name, "err", err)
		return false
	}
	slog.Info("remote backup deleted", "file", name)
	return true
}

func remoteKey(name string) string { return "backups/" + name }

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

// Dir returns the backup directory. Used by the MCP export tool to drop
// full-data JSON files next to the snapshots, where list/delete/restore can
// manage them like any other artifact.
func (m *Manager) Dir() string {
	return m.dir
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
	if err != nil {
		return err
	}
	// 本地删掉的同时把 S3 上的同名副本删掉（若已开启推送）。
	m.deleteRemoteLocked(name)
	return nil
}

// PruneTo deletes every snapshot except the newest keep ones and returns how
// many were removed locally and how many matching S3 objects were removed with
// them. keep < 1 falls back to the configured retention count.
// 用于手工收敛历史堆积（例如一次调度缺陷留下数千份快照），不改变配置本身。
func (m *Manager) PruneTo(keep int) (deleted, remoteDeleted int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if keep < 1 {
		keep = m.cfg.GetBackupKeep()
	}
	deleted, remoteDeleted = m.pruneToLocked(keep)
	return deleted, remoteDeleted, nil
}

// DeleteBefore removes every snapshot older than the given instant. 判定用文件
// 的修改时间，因此手动拷进去的老文件也能被算准。
func (m *Manager) DeleteBefore(before time.Time) (deleted, remoteDeleted int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range listBackupFiles(m.dir) {
		info, statErr := os.Stat(filepath.Join(m.dir, f))
		if statErr != nil {
			continue
		}
		if !info.ModTime().Before(before) {
			continue
		}
		if rmErr := os.Remove(filepath.Join(m.dir, f)); rmErr != nil && !os.IsNotExist(rmErr) {
			return deleted, remoteDeleted, fmt.Errorf("remove %s: %w", f, rmErr)
		}
		deleted++
		if m.deleteRemoteLocked(f) {
			remoteDeleted++
		}
	}
	return deleted, remoteDeleted, nil
}

// DeleteAll removes every snapshot in the backup dir. 清空后当日配额随之释放。
func (m *Manager) DeleteAll() (deleted, remoteDeleted int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range listBackupFiles(m.dir) {
		if rmErr := os.Remove(filepath.Join(m.dir, f)); rmErr != nil && !os.IsNotExist(rmErr) {
			slog.Warn("delete backup", "file", f, "err", rmErr)
			continue
		}
		deleted++
		if m.deleteRemoteLocked(f) {
			remoteDeleted++
		}
	}
	slog.Info("all backups deleted", "count", deleted)
	return deleted, remoteDeleted, nil
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
	m.nextRun = time.Time{}
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
		timer := time.NewTimer(m.nextDelay(time.Duration(interval) * time.Hour))
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

// nextDelay 返回距下一次备份的等待时间（最小 0）。nextRun 已排定时直接按其
// 计算；未排定时回落到「上次成功时间 + 间隔」，从未成功过则 0（立即补跑），
// 这样重启进程不会把调度周期重新计时。
func (m *Manager) nextDelay(interval time.Duration) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.nextRun
	if next.IsZero() {
		if m.lastRun.IsZero() {
			return 0
		}
		next = m.lastRun.Add(interval)
	}
	d := time.Until(next)
	if d < 0 {
		return 0
	}
	return d
}

// scheduleNextLocked 排定下一次运行：成功走正常间隔，失败走指数退避。
// 调用方必须持有 mu。
func (m *Manager) scheduleNextLocked(failed bool) {
	interval := time.Duration(m.cfg.GetBackupIntervalHours()) * time.Hour
	if interval <= 0 {
		// 自动备份关闭：置空以便再次开启时按「上次成功 + 间隔」重算。
		m.nextRun = time.Time{}
		return
	}
	if !failed {
		m.failures = 0
		m.nextRun = time.Now().Add(interval)
		return
	}
	m.failures++
	backoff := retryBaseDelay
	if backoff > interval {
		backoff = interval
	}
	for i := 1; i < m.failures; i++ {
		backoff *= 2
		if backoff <= 0 || backoff >= interval {
			backoff = interval
			break
		}
	}
	m.nextRun = time.Now().Add(backoff)
	slog.Warn("backup failed, retry scheduled", "failures", m.failures, "retry_in", backoff)
}

// RunNow performs one backup immediately and returns the snapshot file name.
// Safe to call concurrently with the scheduler (serialized by mu). 无论本次
// 成功与否，它都会更新 lastRun / 下一次调度时刻，避免失败被无退避地重试。
func (m *Manager) RunNow() (name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 失败路径同样会留下快照文件，必须每次都裁剪；原先只在成功分支末尾裁剪，
	// 远端推送失败时整个跳过，快照只增不减。
	defer m.pruneLocked()
	// 排程必须放在 defer 里：runLocked 若 panic（如推送器空指针），也要当成一次
	// 失败推进 nextRun，否则 nextRun 不前进会退化成「无退避立即重试」的紧密循环。
	defer func() {
		if p := recover(); p != nil {
			slog.Error("backup panicked", "panic", p)
			err = fmt.Errorf("备份过程 panic: %v", p)
		}
		if errors.Is(err, errDailyQuota) {
			// 到额跳过不是故障：按正常间隔再排，避免被当成失败一路退避到
			// 「间隔」之外；UI 仍能从 lastErr 看到跳过原因。
			slog.Warn("backup skipped by daily quota", "err", err)
			m.scheduleNextLocked(false)
			return
		}
		m.scheduleNextLocked(err != nil)
	}()
	return m.runLocked()
}

// runLocked 执行一次快照写入，调用方必须持有 mu。
func (m *Manager) runLocked() (string, error) {
	if err := os.MkdirAll(m.dir, 0755); err != nil {
		m.lastErr = err.Error()
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	// 单日配额闸：无论上层以什么频率调用（调度器、手动、MCP），当天的产出
	// 都被顶在 dailyBackupLimit 以内。
	if n, limit := todaySnapshotCount(m.dir, time.Now()), dailyBackupLimit(m.cfg); n >= limit {
		m.lastErr = fmt.Sprintf("当日备份已达上限 %d 份（已写 %d 份），跳过本次；次日自动恢复", limit, n)
		return "", fmt.Errorf("%w：%d/%d", errDailyQuota, n, limit)
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
		s3key := remoteKey(name)
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
	slog.Info("backup written", "file", name)
	return name, nil
}

// pruneLocked deletes the oldest snapshots beyond the retention count.
func (m *Manager) pruneLocked() {
	m.pruneToLocked(m.cfg.GetBackupKeep())
}

// pruneToLocked keeps the newest keep snapshots; it returns how many were
// removed locally and how many S3 objects were removed with them.
// 调用方必须持有 mu。
func (m *Manager) pruneToLocked(keep int) (deleted, remoteDeleted int) {
	if keep <= 0 {
		keep = defaultKeep
	}
	files := listBackupFiles(m.dir)
	if len(files) <= keep {
		return 0, 0
	}
	for _, f := range files[:len(files)-keep] {
		if err := os.Remove(filepath.Join(m.dir, f)); err != nil {
			slog.Warn("prune old backup", "file", f, "err", err)
			continue
		}
		deleted++
		if m.deleteRemoteLocked(f) {
			remoteDeleted++
		}
		slog.Info("pruned old backup", "file", f)
	}
	return deleted, remoteDeleted
}

// dailyBackupLimit 返回单日允许的快照份数：至少 maxDailyBackups，若间隔短于
// 1 小时则按「24 小时 / 间隔」上抬，保证任何合法配置都不会被这道闸误伤，
// 而失控循环（分钟级 / 秒级）会被硬顶住。
func dailyBackupLimit(cfg *config.Config) int {
	interval := time.Duration(cfg.GetBackupIntervalHours()) * time.Hour
	limit := maxDailyBackups
	if interval > 0 && interval < 24*time.Hour {
		if n := int((24*time.Hour + interval - 1) / interval); n > limit {
			limit = n
		}
	}
	return limit
}

// todaySnapshotCount 统计当天（按 now 的本地日期）已写出的快照份数。以文件名
// 里的日期戳为准，进程重启后仍然准确。
func todaySnapshotCount(dir string, now time.Time) int {
	stamp := now.Format("20060102")
	var n int
	for _, f := range listBackupFiles(dir) {
		if strings.Contains(f, stamp) {
			n++
		}
	}
	return n
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
