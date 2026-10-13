package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mujian/internal/config"
)

// newFailingRemoteManager 复现「设置了每天备份，却在每分钟产一份」的触发现场：
// json 格式 + 开启 S3 推送 + 推送必然失败。
func newFailingRemoteManager(t *testing.T, intervalHours, keep int) (*Manager, *capturePusher, string) {
	t.Helper()
	bdir := filepath.Join(t.TempDir(), "backups")
	cfg := &config.Config{
		BackupIntervalHours: intervalHours,
		BackupKeep:          keep,
		BackupFormat:        "json",
		BackupRemote:        true,
	}
	m := New(fakeVac{}, bdir, cfg)
	m.SetExporter(func() ([]byte, error) { return []byte(`{"records":[]}`), nil }, nil)
	p := &capturePusher{err: fmt.Errorf("simulated s3 failure")}
	m.SetRemotePusher(p.push)
	return m, p, bdir
}

// 修复前：RunNow 失败不推进调度基准，定时器 duration 恒为 0，2 秒能写出上千份
// 快照；修复后失败一次即转入退避，间隔 24 小时只应产出这一份。
func TestFailedBackupDoesNotRetryInTightLoop(t *testing.T) {
	m, p, bdir := newFailingRemoteManager(t, 24, 10)
	m.Start()
	defer m.Stop()

	time.Sleep(1500 * time.Millisecond)

	files, err := os.ReadDir(bdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 1 {
		t.Fatalf("远端推送失败导致 %d 次重试并写出 %d 份快照（间隔为 24 小时）", len(p.calls), len(files))
	}
	// 下一次运行必须已被推到退避时间之后，而不是立刻再来一次。
	if wait := time.Until(m.nextRun); wait < 4*time.Minute {
		t.Fatalf("失败后未排定退避重试，nextRun 距今仅 %v", wait)
	}
}

func TestBackoffGrowsAndResetsOnSuccess(t *testing.T) {
	m, p, _ := newFailingRemoteManager(t, 24, 10)

	if _, err := m.RunNow(); err == nil {
		t.Fatal("expected first backup to fail")
	}
	first := time.Until(m.nextRun)
	if _, err := m.RunNow(); err == nil {
		t.Fatal("expected second backup to fail")
	}
	second := time.Until(m.nextRun)
	if second <= first+time.Minute {
		t.Fatalf("退避未随连续失败放大: %v → %v", first, second)
	}

	// 成功一次后回到正常间隔，连续失败计数清零。
	p.err = nil
	if _, err := m.RunNow(); err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}
	if wait := time.Until(m.nextRun); wait < 23*time.Hour || wait > 25*time.Hour {
		t.Fatalf("成功后下一次运行应回到 24 小时，实际 %v", wait)
	}
	if _, lastErr := m.Status(); lastErr != "" {
		t.Fatalf("成功后仍残留错误: %s", lastErr)
	}
	if n := len(m.List()); n == 0 {
		t.Fatal("成功备份后应至少有一份快照")
	}
}

// 单日配额闸：无论以多快的节奏调用 RunNow，当天写出的快照都不超过
// dailyBackupLimit（正常间隔下 = 24）份。
func TestDailyQuotaCapsRunawayWrites(t *testing.T) {
	bdir := filepath.Join(t.TempDir(), "backups")
	cfg := &config.Config{
		BackupIntervalHours: 24,
		BackupKeep:          1000, // 关闭裁剪，单独考察配额闸
		BackupFormat:        "json",
	}
	m := New(fakeVac{}, bdir, cfg)
	m.SetExporter(func() ([]byte, error) { return []byte(`{"records":[]}`), nil }, nil)

	limit := dailyBackupLimit(cfg)
	var quotaErr error
	for i := 0; i < limit+5; i++ {
		if _, err := m.RunNow(); err != nil {
			quotaErr = err
		}
	}
	if !errors.Is(quotaErr, errDailyQuota) {
		t.Fatalf("超过单日上限后应返回配额错误，实际 %v", quotaErr)
	}
	files, err := os.ReadDir(bdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != limit {
		t.Fatalf("单日应最多 %d 份，实际 %d 份", limit, len(files))
	}
	// 到额跳过不算故障：下一次仍按正常间隔排定，而不是失败退避。
	if wait := time.Until(m.nextRun); wait < 23*time.Hour {
		t.Fatalf("配额跳过被误判为故障（退避到 %v）", wait)
	}
}

// DeleteBefore 按修改时间成批删除，边界时刻本身保留。
func TestDeleteBefore(t *testing.T) {
	bdir := filepath.Join(t.TempDir(), "backups")
	m := New(fakeVac{}, bdir, &config.Config{BackupFormat: "db", BackupKeep: 100})
	old, recent := "", ""
	for i := 0; i < 4; i++ {
		name, err := m.RunNow()
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(bdir, name)
		// 前两份是 24/23 小时前的旧快照，后两份是 1 小时内的新快照。
		hours := -24 + 0
		if i >= 2 {
			hours = -1
		}
		when := time.Now().Add(time.Duration(hours) * time.Hour)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			old = name
		} else {
			recent = name
		}
		time.Sleep(1100 * time.Millisecond) // 文件名戳按秒区分
	}

	n, _, err := m.DeleteBefore(time.Now().Add(-12 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("应删除 2 份旧快照，实际 %d", n)
	}
	if _, err := os.Stat(filepath.Join(bdir, old)); !os.IsNotExist(err) {
		t.Fatal("旧快照未删除")
	}
	if _, err := os.Stat(filepath.Join(bdir, recent)); err != nil {
		t.Fatalf("新快照被误删: %v", err)
	}
}

// PruneTo / DeleteAll 是清理历史堆积的手工入口。
func TestPruneToAndDeleteAll(t *testing.T) {
	bdir := filepath.Join(t.TempDir(), "backups")
	m := New(fakeVac{}, bdir, &config.Config{BackupFormat: "db", BackupKeep: 100})
	for i := 0; i < 6; i++ {
		if _, err := m.RunNow(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1100 * time.Millisecond)
	}

	// 6 份 → 保留 2 份；未开启 S3 推送，远端删除数应为 0。
	deleted, remoteDeleted, err := m.PruneTo(2)
	if err != nil {
		t.Fatal(err)
	}
	if remoteDeleted != 0 {
		t.Fatalf("未开启推送时不应产生远端删除，实际 %d", remoteDeleted)
	}
	if deleted != 4 || len(m.List()) != 2 {
		t.Fatalf("PruneTo(2): deleted=%d remaining=%d", deleted, len(m.List()))
	}

	deleted, _, err = m.DeleteAll()
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 || len(m.List()) != 0 {
		t.Fatalf("DeleteAll: deleted=%d remaining=%d", deleted, len(m.List()))
	}
}

// 本地删除会同步删掉 S3 上的同名副本；删除器报错不影响本地结果。
func TestLocalDeleteSyncsRemote(t *testing.T) {
	bdir := filepath.Join(t.TempDir(), "backups")
	m := New(fakeVac{}, bdir, &config.Config{BackupFormat: "db", BackupKeep: 100, BackupRemote: true})
	var removed []string
	m.SetRemoteDeleter(func(key string) error {
		removed = append(removed, key)
		return nil
	})
	// 上传器只用于让本次备份走完成功分支，远端真实副本由上面的删除器记录模拟。
	m.SetRemotePusher(func(key string, data []byte) error { return nil })

	name, err := m.RunNow()
	if err != nil {
		t.Fatal(err)
	}

	deleted, remoteDeleted, err := m.DeleteAll()
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 || remoteDeleted != 1 {
		t.Fatalf("删除 %d 份本地 / %d 份远端，期望 1/1", deleted, remoteDeleted)
	}
	if len(removed) != 1 || removed[0] != "backups/"+name {
		t.Fatalf("远端删除键不符: %v", removed)
	}
}

// runLocked 内部 panic（例如推送器空指针）也要被当成一次失败推进调度：否则
// nextRun 不前进，就会退化成「无退避立即重试」的紧密循环。
func TestRunNowRecoversFromPanic(t *testing.T) {
	bdir := filepath.Join(t.TempDir(), "backups")
	cfg := &config.Config{
		BackupIntervalHours: 24,
		BackupKeep:          10,
		BackupFormat:        "json",
		BackupRemote:        true,
	}
	m := New(fakeVac{}, bdir, cfg)
	m.SetExporter(func() ([]byte, error) { return []byte(`{"records":[]}`), nil }, nil)
	m.SetRemotePusher(func(key string, data []byte) error { panic("simulated pusher panic") })

	if _, err := m.RunNow(); err == nil {
		t.Fatal("panic 应被转成错误返回")
	} else if !strings.Contains(err.Error(), "panic") {
		t.Fatalf("错误信息应说明是 panic，实际 %v", err)
	}
	if wait := time.Until(m.nextRun); wait < 4*time.Minute {
		t.Fatalf("panic 后未排定退避重试，nextRun 距今仅 %v", wait)
	}

	// panic 不得破坏管理器状态：换回正常推送器后应能继续备份。
	m.SetRemotePusher(func(key string, data []byte) error { return nil })
	if _, err := m.RunNow(); err != nil {
		t.Fatalf("panic 之后无法继续备份: %v", err)
	}
	if _, lastErr := m.Status(); lastErr != "" {
		t.Fatalf("成功后仍残留错误: %s", lastErr)
	}
}

// 失败路径同样会留下快照文件，必须每次都裁剪（原先只在成功分支末尾裁剪，
// 远端推送失败时整个跳过 → 快照只增不减）。
func TestPruneRunsEvenWhenRemotePushFails(t *testing.T) {
	m, _, bdir := newFailingRemoteManager(t, 24, 3)

	for i := 0; i < 6; i++ {
		if _, err := m.RunNow(); err == nil {
			t.Fatalf("run %d: expected failure", i)
		}
	}

	files, err := os.ReadDir(bdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("keep=3 但失败 6 次后留下 %d 份快照", len(files))
	}
}
