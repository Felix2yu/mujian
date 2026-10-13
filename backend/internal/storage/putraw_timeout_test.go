package storage

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// PutRaw 原先使用 context.TODO()：端点不可达时请求会一直挂着，把一次失败拖成
// 数十秒。这里用「永远不返回」的服务端验证超时确实生效。
func TestPutRawRespectsTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	prev := s3OpTimeout
	s3OpTimeout = 200 * time.Millisecond
	t.Cleanup(func() { s3OpTimeout = prev })

	s := newS3Store(t, ts.URL)
	start := time.Now()
	err := s.PutRaw("backups/mujian-backup.json", []byte("payload"))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error when the upload exceeds the timeout")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("PutRaw 未在超时后返回，耗时 %v", elapsed)
	}
}

// DeleteRaw 是 PutRaw 的对偶操作：本地快照被删除时用它同步清掉 S3 副本。
// 对象不存在（如当初上传失败）要按成功处理。
func TestDeleteRaw(t *testing.T) {
	fs := newFakeS3()
	ts := httptest.NewServer(fs)
	defer ts.Close()
	s := newS3Store(t, ts.URL)

	key := "backups/mujian-backup-20261012-093142.json"
	if err := s.PutRaw(key, []byte("payload")); err != nil {
		t.Fatalf("PutRaw: %v", err)
	}
	if err := s.DeleteRaw(key); err != nil {
		t.Fatalf("DeleteRaw: %v", err)
	}
	if _, ok := fs.objects[key]; ok {
		t.Fatal("对象未被删除")
	}
	// 幂等：重复删除 / 删除不存在的键都不报错。
	if err := s.DeleteRaw(key); err != nil {
		t.Fatalf("重复 DeleteRaw 应视为成功，实际 %v", err)
	}
	if err := s.DeleteRaw("backups/never-existed.json"); err != nil {
		t.Fatalf("删除不存在的键应视为成功，实际 %v", err)
	}
}

// 正常上传不受超时影响：小对象应在远小于超时预算内落盘。
func TestPutRawSucceedsWithinTimeout(t *testing.T) {
	ts := httptest.NewServer(newFakeS3())
	defer ts.Close()

	prev := s3OpTimeout
	s3OpTimeout = 60 * time.Second
	t.Cleanup(func() { s3OpTimeout = prev })

	s := newS3Store(t, ts.URL)
	if err := s.PutRaw("backups/mujian-backup.json", []byte("payload")); err != nil {
		t.Fatalf("PutRaw: %v", err)
	}
}
