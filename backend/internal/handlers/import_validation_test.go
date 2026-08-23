package handlers

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// ---------- import lock ----------

// A second concurrent import must get an explicit 409 instead of a cryptic
// SQLite lock error (the server-side half of the SQLITE_BUSY fix).
func TestImportConflictReturns409(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)

	importMu.Lock() // simulate another import in flight
	defer importMu.Unlock()

	res, body := uploadFile(t, ts.URL+"/api/records/import", "file", "data.json", []byte(`{"records":[]}`), "")
	if res.StatusCode != 409 {
		t.Fatalf("expected 409 while import in flight, got %d: %s", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "已有另一个导入正在进行") {
		t.Errorf("conflict message: %s", body)
	}
}

// ---------- request validation ----------

func TestImportValidationErrors(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)

	// Unsupported extension.
	res, body := uploadFile(t, ts.URL+"/api/records/import", "file", "backup.tar", []byte("x"), "")
	expectStatus(t, res, 400, "unsupported extension")
	if !strings.Contains(string(body), "仅支持") {
		t.Errorf("extension error body: %s", body)
	}

	// .json that is not valid JSON.
	res, body = uploadFile(t, ts.URL+"/api/records/import", "file", "bad.json", []byte("{nope"), "")
	expectStatus(t, res, 400, "invalid json")
	if !strings.Contains(string(body), "failed to parse export") {
		t.Errorf("invalid json body: %s", body)
	}

	// Valid JSON without records or categories.
	res, body = uploadFile(t, ts.URL+"/api/records/import", "file", "empty.json", []byte(`{"source":"mujian"}`), "")
	expectStatus(t, res, 400, "json without records")
	if !strings.Contains(string(body), "no records") {
		t.Errorf("empty json body: %s", body)
	}

	// .zip whose bytes are not a zip archive.
	res, body = uploadFile(t, ts.URL+"/api/records/import", "file", "broken.zip", []byte("not a zip"), "")
	expectStatus(t, res, 400, "invalid zip")
	if !strings.Contains(string(body), "invalid zip") {
		t.Errorf("invalid zip body: %s", body)
	}

	// Zip without any recognized data entry.
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	e, _ := zw.Create("readme.txt")
	e.Write([]byte("nothing here"))
	zw.Close()
	res, body = uploadFile(t, ts.URL+"/api/records/import", "file", "nodata.zip", zbuf.Bytes(), "")
	expectStatus(t, res, 400, "zip without data file")
	if !strings.Contains(string(body), "未找到") {
		t.Errorf("missing data file body: %s", body)
	}

	// Zip whose data.json holds no records.
	zbuf.Reset()
	zw = zip.NewWriter(&zbuf)
	e, _ = zw.Create("data.json")
	e.Write([]byte(`{"records":[],"categories":[]}`))
	zw.Close()
	res, body = uploadFile(t, ts.URL+"/api/records/import", "file", "empty.zip", zbuf.Bytes(), "")
	expectStatus(t, res, 400, "zip with empty data")
	if !strings.Contains(string(body), "no records") {
		t.Errorf("zip empty data body: %s", body)
	}
}
