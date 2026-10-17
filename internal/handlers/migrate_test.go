package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mujian/internal/config"
	"mujian/internal/models"
)

// miniS3 is a tiny in-memory S3-compatible server covering just the
// Head/Put/Get subset that /storage/migrate-to-s3 exercises.
type miniS3 struct{ objects map[string][]byte }

func newMiniS3() *miniS3 { return &miniS3{objects: map[string][]byte{}} }

func (m *miniS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if i := strings.Index(p, "/"); i >= 0 {
		p = p[i+1:]
	} else {
		p = ""
	}
	switch r.Method {
	case http.MethodHead:
		if _, ok := m.objects[p]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		m.objects[p] = b
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		if r.URL.Query().Get("list-type") == "2" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>`)
			return
		}
		if b, ok := m.objects[p]; ok {
			w.WriteHeader(http.StatusOK)
			w.Write(b)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ndjsonLast returns the final line of an NDJSON response body.
func ndjsonLast(t *testing.T, body []byte) string {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	return lines[len(lines)-1]
}

func TestMigrateToS3Endpoint(t *testing.T) {
	s3srv := newMiniS3()
	ts3 := httptest.NewServer(s3srv)
	defer ts3.Close()

	ts, _, database, store := newTestServer(t, func(c *config.Config) {
		c.S3Endpoint = ts3.URL
		c.S3Bucket = "bkt"
		c.S3Region = "us-east-1"
		c.S3AccessKey = "ak"
		c.S3SecretKey = "sk"
	})

	key, _, err := store.SaveCoverBytes(jpgFixture(), ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	_ = database.UpsertRecord(models.Record{ID: "mig1", Name: "迁移", CoverFile: key})

	res, body := doReq(t, "POST", ts.URL+"/api/storage/migrate-to-s3", nil, "")
	if res.StatusCode != 200 {
		t.Fatalf("migrate: status %d, body %s", res.StatusCode, body)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(ndjsonLast(t, body)), &out); err != nil {
		t.Fatalf("final line: %v (%s)", err, ndjsonLast(t, body))
	}
	if out["done"] != true || out["migrated"].(float64) != 1 || out["skipped"].(float64) != 0 {
		t.Fatalf("migrate result: %v", out)
	}
	got, ok := s3srv.objects[key]
	if !ok || string(got) != string(jpgFixture()) {
		t.Fatalf("object not uploaded correctly: present=%v len=%d", ok, len(got))
	}

	// Re-run: idempotent, all skipped.
	res, body = doReq(t, "POST", ts.URL+"/api/storage/migrate-to-s3", nil, "")
	if res.StatusCode != 200 {
		t.Fatalf("re-migrate: status %d", res.StatusCode)
	}
	out = map[string]interface{}{}
	json.Unmarshal([]byte(ndjsonLast(t, body)), &out)
	if out["skipped"].(float64) != 1 || out["migrated"].(float64) != 0 {
		t.Fatalf("re-migrate result: %v", out)
	}
}

func TestMigrateToS3RequiresConfig(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil) // no S3 config set
	res, _ := doReq(t, "POST", ts.URL+"/api/storage/migrate-to-s3", nil, "")
	expectStatus(t, res, 400, "migrate without s3 config")
}
