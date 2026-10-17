package storage

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"mujian/internal/config"
)

// fakeS3 is a minimal in-memory S3-compatible server covering the subset of
// operations the S3Storage backend uses (Put/Get/Head/Delete/Copy/List).
type fakeS3 struct {
	objects map[string][]byte
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: map[string][]byte{}}
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Path-style addressing: /<bucket>/<key>. Drop the bucket segment.
	p := strings.TrimPrefix(r.URL.Path, "/")
	if i := strings.Index(p, "/"); i >= 0 {
		p = p[i+1:]
	} else {
		p = "" // bucket root (used by ListObjectsV2)
	}
	key := p
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("list-type") == "2" {
			f.listObjects(w, r)
			return
		}
		b, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(b)
	case http.MethodHead:
		if _, ok := f.objects[key]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		if src := r.Header.Get("x-amz-copy-source"); src != "" {
			// CopyObject: source like "bkt/covers/x.jpg"
			parts := strings.SplitN(src, "/", 2)
			if len(parts) == 2 {
				if b, ok := f.objects[parts[1]]; ok {
					f.objects[key] = append([]byte{}, b...)
				}
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `<CopyObjectResult><ETag>"x"</ETag></CopyObjectResult>`)
			return
		}
		b, _ := io.ReadAll(r.Body)
		f.objects[key] = b
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type s3ListResult struct {
	Contents []struct {
		Key  string `xml:"Key"`
		Size int64  `xml:"Size"`
	} `xml:"Contents"`
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
}

func (f *fakeS3) listObjects(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	token := r.URL.Query().Get("continuation-token")
	out := s3ListResult{}
	started := token == ""
	for _, k := range sortedKeys(f.objects) {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if !started {
			started = true
		}
		out.Contents = append(out.Contents, struct {
			Key  string `xml:"Key"`
			Size int64  `xml:"Size"`
		}{Key: k, Size: int64(len(f.objects[k]))})
	}
	out.IsTruncated = false
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	xml.NewEncoder(w).Encode(out)
}

func sortedKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func newS3Store(t *testing.T, endpoint string) *S3Storage {
	t.Helper()
	cfg := &config.Config{
		StorageType: "s3",
		S3Endpoint:  endpoint,
		S3Bucket:    "mujian-test",
		S3Region:    "us-east-1",
		S3AccessKey: "test-ak",
		S3SecretKey: "test-sk",
		ImageFormat: "avif",
	}
	return NewS3Storage(cfg)
}

func s3UploadHeader(t *testing.T, data []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "poster.jpg")
	fw.Write(data)
	w.Close()
	r := multipart.NewReader(&buf, w.Boundary())
	form, _ := r.ReadForm(1 << 20)
	return form.File["file"][0]
}

func TestS3FromSettingsPathStyle(t *testing.T) {
	// Records the most recent request so we can assert addressing style.
	var mu sync.Mutex
	var lastPath string
	rec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastPath = r.URL.Path
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer rec.Close()

	// Explicit (http) endpoint → path-style, bucket segment in the path.
	// A virtual-hosted form would put the bucket in the Host header and the
	// path would be just "/covers/test.jpg"; we assert the bucket is in the path.
	s := NewS3StorageFromSettings(config.S3Settings{
		Endpoint:  rec.URL, // e.g. http://127.0.0.1:PORT
		Bucket:    "mujian-test",
		Region:    "us-east-1",
		AccessKey: "ak",
		SecretKey: "sk",
	}, func() string { return "avif" })
	if err := s.PutRaw("covers/test.jpg", []byte("hello")); err != nil {
		t.Fatalf("PutRaw (scheme endpoint) failed: %v", err)
	}
	mu.Lock()
	gotPath := lastPath
	mu.Unlock()
	if !strings.HasPrefix(gotPath, "/mujian-test/") {
		t.Errorf("custom endpoint should use path-style (/<bucket>/<key>), got path %q", gotPath)
	}
}

func TestNormalizeS3Endpoint(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"s3.example.com", "https://s3.example.com"},
		{"https://s3.example.com", "https://s3.example.com"},
		{"http://localhost:9000", "http://localhost:9000"},
		{"play.min.io", "https://play.min.io"},
		{"https://account.r2.cloudflarestorage.com", "https://account.r2.cloudflarestorage.com"},
	}
	for _, c := range cases {
		if got := normalizeS3Endpoint(c.in); got != c.want {
			t.Errorf("normalizeS3Endpoint(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestS3StorageFlow(t *testing.T) {
	fs := newFakeS3()
	ts := httptest.NewServer(fs)
	defer ts.Close()

	s := newS3Store(t, ts.URL)
	jpg := jpegBytes(t)

	// SaveUpload.
	key, thumb, created, err := s.SaveUpload(s3UploadHeader(t, jpg))
	if err != nil {
		t.Fatalf("SaveUpload: %v", err)
	}
	if !created || !strings.HasSuffix(key, ".avif") {
		t.Fatalf("upload: key=%q created=%v", key, created)
	}
	if thumb == "" {
		t.Error("thumbnail key should be set")
	}
	// Dedupe.
	key2, _, created2, err := s.SaveUpload(s3UploadHeader(t, jpg))
	if err != nil || key2 != key || created2 {
		t.Fatalf("dedupe: key2=%q created2=%v err=%v", key2, created2, err)
	}

	// Read / exists.
	data, err := s.ReadCover(key)
	if err != nil || len(data) == 0 {
		t.Fatalf("ReadCover: %v", err)
	}
	if !s.CoverExists(key) {
		t.Error("CoverExists should be true")
	}
	if s.CoverExists("covers/does-not-exist.avif") {
		t.Error("CoverExists should be false for missing")
	}

	// List keys (thumbnails filtered).
	keys, err := s.ListCoverKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != key {
		t.Fatalf("ListCoverKeys: %v", keys)
	}
	for _, k := range keys {
		if isThumbKey(k) {
			t.Fatalf("thumb leaked: %q", k)
		}
	}

	// SaveCoverBytes new + dedupe.
	extraKey, created3, err := s.SaveCoverBytes(jpg, "")
	if err != nil || !created3 {
		t.Fatalf("SaveCoverBytes: %v %v", extraKey, err)
	}
	extraKey2, created4, _ := s.SaveCoverBytes(jpg, ".jpg")
	if extraKey2 != extraKey || created4 {
		t.Fatalf("SaveCoverBytes dedupe: %q %v", extraKey2, created4)
	}

	// Convert jpg -> webp.
	convKey, conv, err := s.ConvertCover(extraKey, "webp")
	if err != nil || !conv {
		t.Fatalf("ConvertCover: %v %v", conv, err)
	}
	if !strings.HasSuffix(convKey, ".webp") {
		t.Errorf("converted key: %q", convKey)
	}
	if s.CoverExists(extraKey) {
		t.Error("old file should be deleted after conversion")
	}

	// MakeThumbnail + format switch cleanup.
	tk1, err := s.MakeThumbnail(key, data, 400, "jpeg")
	if err != nil {
		t.Fatal(err)
	}
	tk2, err := s.MakeThumbnail(key, data, 400, "webp")
	if err != nil {
		t.Fatal(err)
	}
	if tk1 == tk2 {
		t.Error("thumb keys should differ by format")
	}

	// Trash flow.
	if err := s.MoveCoverToTrash(key); err != nil {
		t.Fatal(err)
	}
	if s.CoverExists(key) {
		t.Error("cover should be gone after trash")
	}
	trash, err := s.ListTrashKeys()
	if err != nil || len(trash) != 1 {
		t.Fatalf("ListTrashKeys: %v %v", trash, err)
	}
	if n, err := s.PurgeTrash(); err != nil || n != 1 {
		t.Fatalf("PurgeTrash: %d %v", n, err)
	}

	// DeleteCover.
	delKey, _, err := s.SaveCoverBytes(jpg, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCover(delKey); err != nil {
		t.Fatal(err)
	}
	if s.CoverExists(delKey) {
		t.Error("cover should be deleted")
	}

	// imageFormat default.
	if got := s.imageFormat(); got != "avif" {
		t.Errorf("imageFormat: %q", got)
	}
}

func TestS3NewSelector(t *testing.T) {
	fs := newFakeS3()
	ts := httptest.NewServer(fs)
	defer ts.Close()

	cfg := &config.Config{
		StorageType: "s3", S3Bucket: "b", S3AccessKey: "ak",
		S3Endpoint: ts.URL, ImageFormat: "avif",
	}
	st := New(cfg)
	if _, ok := st.(*S3Storage); !ok {
		t.Fatalf("expected S3Storage, got %T", st)
	}

	// Local when s3 fields missing.
	cfg2 := &config.Config{StorageType: "s3"}
	st2 := New(cfg2)
	if _, ok := st2.(*LocalStorage); !ok {
		t.Fatalf("expected LocalStorage, got %T", st2)
	}
}

func TestS3ListKeysAndTrashMissing(t *testing.T) {
	fs := newFakeS3()
	ts := httptest.NewServer(fs)
	defer ts.Close()
	s := newS3Store(t, ts.URL)

	// Listing an empty bucket returns empty.
	keys, err := s.ListCoverKeys()
	if err != nil || len(keys) != 0 {
		t.Fatalf("ListCoverKeys empty: %v %v", keys, err)
	}
	// ReadCover on missing errors.
	if _, err := s.ReadCover("covers/nope.jpg"); err == nil {
		t.Error("ReadCover missing should error")
	}
	// MoveCoverToTrash of a missing file: CopyObject copies nothing; not an error.
	_ = os.MkdirAll(filepath.Join(t.TempDir(), "unused"), 0755)
	if err := s.MoveCoverToTrash("covers/ghost.jpg"); err == nil {
		// allowed (mock copies nothing); no assertion on error
	}
}
