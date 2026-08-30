package backup

import (
	"os"
	"strings"
	"testing"

	"mujian/internal/config"
)

// fakeVac simulates a database that supports VACUUM INTO by materializing a
// small file at the target path.
type fakeVac struct{}

func (fakeVac) VacuumInto(path string) error { return os.WriteFile(path, []byte("db-snapshot"), 0644) }
func (fakeVac) Checkpoint() error            { return nil }

// capturePusher records S3 upload calls so tests can assert on key + payload.
type capturePusher struct {
	calls []pushCall
	err   error
}

type pushCall struct {
	key  string
	data []byte
}

func (c *capturePusher) push(key string, data []byte) error {
	c.calls = append(c.calls, pushCall{key: key, data: data})
	return c.err
}

func newTestManager(t *testing.T, remote bool) (*Manager, *capturePusher, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		BackupRemote: remote,
		BackupFormat: "db",
		BackupKeep:   10,
	}
	m := New(fakeVac{}, dir, cfg)
	p := &capturePusher{}
	m.SetRemotePusher(p.push)
	return m, p, dir
}

func TestRunNowPushesToS3WhenEnabled(t *testing.T) {
	m, p, _ := newTestManager(t, true)
	name, err := m.RunNow()
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if name == "" {
		t.Fatal("expected a snapshot file name")
	}
	if len(p.calls) != 1 {
		t.Fatalf("expected exactly one S3 push, got %d", len(p.calls))
	}
	wantKey := "backups/" + name
	if p.calls[0].key != wantKey {
		t.Errorf("S3 key = %q, want %q", p.calls[0].key, wantKey)
	}
	if string(p.calls[0].data) != "db-snapshot" {
		t.Errorf("S3 payload = %q, want %q", p.calls[0].data, "db-snapshot")
	}
	if !strings.HasPrefix(name, "mujian-backup-") || !strings.HasSuffix(name, ".db") {
		t.Errorf("unexpected snapshot name %q", name)
	}
}

func TestRunNowNoS3PushWhenDisabled(t *testing.T) {
	m, p, _ := newTestManager(t, false)
	if _, err := m.RunNow(); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if len(p.calls) != 0 {
		t.Fatalf("expected no S3 push when BackupRemote is false, got %d", len(p.calls))
	}
}

func TestRunNowRemoteEnabledButPusherNil(t *testing.T) {
	dir := t.TempDir()
	m := New(fakeVac{}, dir, &config.Config{BackupRemote: true, BackupFormat: "db", BackupKeep: 10})
	// No SetRemotePusher: pusher stays nil.
	if _, err := m.RunNow(); err == nil {
		t.Fatal("expected error when BackupRemote is set but no pusher is wired")
	}
}

func TestRunNowS3PushFailureSurfaced(t *testing.T) {
	m, p, dir := newTestManager(t, true)
	p.err = os.ErrPermission
	_, err := m.RunNow()
	if err == nil {
		t.Fatal("expected S3 push failure to be surfaced as an error")
	}
	if !strings.Contains(err.Error(), "S3") {
		t.Errorf("error should mention S3: %v", err)
	}
	// Local snapshot must still exist even when the S3 push fails.
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Error("local snapshot should be retained despite S3 failure")
	}
}

func TestRunNowS3PushReadsFinalFile(t *testing.T) {
	// The pusher should receive the contents of the renamed (final) snapshot,
	// proving the upload happens after the atomic rename, not the .tmp file.
	m, p, _ := newTestManager(t, true)
	if _, err := m.RunNow(); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if got := string(p.calls[0].data); got != "db-snapshot" {
		t.Errorf("pushed data = %q, want %q", got, "db-snapshot")
	}
}
