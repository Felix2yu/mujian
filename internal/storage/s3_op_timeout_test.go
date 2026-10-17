package storage

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// 备份直传（PutRaw）之外，封面读写的每个 S3 调用原先都传 context.TODO()：
// 端点不可达时请求会一直挂着，一次封面写入能把 handler 拖到数分钟，
// goroutine 与连接随之堆积。这里用「永不返回」的服务端验证超时统一生效。
func TestCoverOpsRespectTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	prev := s3OpTimeout
	s3OpTimeout = 200 * time.Millisecond
	t.Cleanup(func() { s3OpTimeout = prev })

	s := newS3Store(t, ts.URL)
	ops := map[string]func() error{
		"SaveCoverBytes":   func() error { _, _, err := s.SaveCoverBytes([]byte("img"), ".jpg"); return err },
		"ReadCover":        func() error { _, err := s.ReadCover("covers/x.jpg"); return err },
		"DeleteCover":      func() error { return s.DeleteCover("covers/x.jpg") },
		"MoveCoverToTrash": func() error { return s.MoveCoverToTrash("covers/x.jpg") },
		"ListCoverKeys":    func() error { _, err := s.ListCoverKeys(); return err },
	}
	for name, op := range ops {
		start := time.Now()
		err := op()
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("%s 未在超时后返回，耗时 %v", name, elapsed)
		}
		if err == nil {
			t.Fatalf("%s 应在超时时报错，实际返回成功", name)
		}
	}

	// CoverExists 返回 bool：超时按「不存在」处理，同样不能挂住。
	start := time.Now()
	if s.CoverExists("covers/x.jpg") {
		t.Fatal("CoverExists 超时应按不存在处理")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("CoverExists 未在超时后返回，耗时 %v", elapsed)
	}
}

// listKeys 必须显式带 MaxKeys：不指定时 S3 默认每页 1000 条但仍要翻页到底，
// 桶里对象多时会把全部 key 读进内存。
func TestListKeysSetsPageSize(t *testing.T) {
	fs := newFakeS3()
	var got atomic.Value
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list-type") == "2" {
			got.Store(r.URL.Query().Get("max-keys"))
		}
		fs.ServeHTTP(w, r)
	}))
	defer ts.Close()

	s := newS3Store(t, ts.URL)
	if _, err := s.ListCoverKeys(); err != nil {
		t.Fatalf("ListCoverKeys: %v", err)
	}
	if v, _ := got.Load().(string); v != "1000" {
		t.Fatalf("应显式指定 max-keys=1000，实际 %q", v)
	}
}

// readAllLimited 是 ReadCover 的读取上限：异常巨大的对象要被拦下并报错，
// 而不是整块读进内存。
func TestReadAllLimited(t *testing.T) {
	small := bytes.Repeat([]byte("a"), 10)
	b, err := readAllLimited(bytes.NewReader(small), 100)
	if err != nil {
		t.Fatalf("10 字节应正常读回: %v", err)
	}
	if len(b) != 10 {
		t.Fatalf("读回 %d 字节，期望 10", len(b))
	}
	// 恰好等于上限也要放行：只有超过才算异常。
	if _, err := readAllLimited(bytes.NewReader(bytes.Repeat([]byte("a"), 100)), 100); err != nil {
		t.Fatalf("等于上限不应报错: %v", err)
	}
	if _, err := readAllLimited(bytes.NewReader(bytes.Repeat([]byte("a"), 101)), 100); err == nil {
		t.Fatal("超过上限应报错")
	}
}
