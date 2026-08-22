package handlers

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"mujian/internal/config"
	"mujian/internal/db"
	"mujian/internal/models"
	"mujian/internal/storage"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// ---------- helpers ----------

func newTestServer(t *testing.T, mutate func(*config.Config)) (*httptest.Server, *Handler, *db.DB, *storage.LocalStorage) {
	t.Helper()
	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "data", "mujian.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(database.Close)
	store := storage.NewLocalStorage(dir, func() string { return "avif" })
	cfg := &config.Config{
		AllowLocalStorage: true,
		DBPath:            filepath.Join(dir, "data", "mujian.db"),
		UploadDir:         dir,
		Port:              "8080",
		Timezone:          "UTC",
		Theme:             "auto",
		StorageType:       "local",
		ImageFormat:       "avif",
	}
	if mutate != nil {
		mutate(cfg)
	}
	h := New(database, cfg, store)
	mux := chi.NewRouter()
	mux.Mount("/api", h.Routes())
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, h, database, store
}

func doReq(t *testing.T, method, url string, body io.Reader, contentType string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

func doJSON(t *testing.T, method, url string, payload interface{}) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if payload != nil {
		b, _ := json.Marshal(payload)
		buf.Write(b)
	}
	return doReq(t, method, url, &buf, "application/json")
}

func expectStatus(t *testing.T, res *http.Response, want int, what string) {
	t.Helper()
	if res.StatusCode != want {
		t.Fatalf("%s: got status %d, want %d", what, res.StatusCode, want)
	}
}

func decodeResp(t *testing.T, b []byte, v interface{}) {
	t.Helper()
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("decode json %s: %v", b, err)
	}
}

func jpgFixture() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 8), uint8(y * 8), 100, 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

func uploadFile(t *testing.T, url, field, filename string, data []byte, contentType string) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(data)
	w.Close()
	return doReq(t, "POST", url, &buf, w.FormDataContentType())
}

// ---------- records ----------

func TestRecordsEndpoints(t *testing.T) {
	ts, h, _, _ := newTestServer(t, nil)

	// Empty list.
	res, b := doJSON(t, "GET", ts.URL+"/api/records", nil)
	expectStatus(t, res, 200, "list records")
	var list []models.Record
	decodeResp(t, b, &list)
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %v", list)
	}

	// Create.
	res, b = doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{
		"name": "牡丹亭", "city": "上海", "date": time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC).Unix(),
		"price": 280, "artist_names": []string{"张军"}, "play": []string{"惊梦"},
	})
	expectStatus(t, res, 201, "create record")
	var created models.Record
	decodeResp(t, b, &created)
	if created.ID == "" || created.Name != "牡丹亭" {
		t.Fatalf("create: %+v", created)
	}
	// Play derived from drama? no drama exists, so Play kept as given.

	// Invalid body.
	res, _ = doJSON(t, "POST", ts.URL+"/api/records", []byte("{"))
	expectStatus(t, res, 400, "create invalid body")
	// Missing name.
	res, _ = doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{"city": "x"})
	expectStatus(t, res, 400, "create missing name")

	// Get.
	res, b = doJSON(t, "GET", ts.URL+"/api/records/"+created.ID, nil)
	expectStatus(t, res, 200, "get record")
	res, _ = doJSON(t, "GET", ts.URL+"/api/records/missing", nil)
	expectStatus(t, res, 404, "get missing")

	// Update.
	res, b = doJSON(t, "PUT", ts.URL+"/api/records/"+created.ID, map[string]interface{}{"name": "改", "dateText": "2026-08-22 19:30"})
	expectStatus(t, res, 200, "update record")
	var upd models.Record
	decodeResp(t, b, &upd)
	if upd.Name != "改" {
		t.Fatalf("update: %+v", upd)
	}
	res, _ = doJSON(t, "PUT", ts.URL+"/api/records/missing", map[string]interface{}{"name": "x"})
	expectStatus(t, res, 404, "update missing")
	res, _ = doJSON(t, "PUT", ts.URL+"/api/records/"+created.ID, []byte("{"))
	expectStatus(t, res, 400, "update invalid body")

	// List all / search.
	res, b = doJSON(t, "GET", ts.URL+"/api/records/all", nil)
	expectStatus(t, res, 200, "list all")
	decodeResp(t, b, &list)
	if len(list) != 1 {
		t.Fatalf("list all: %v", list)
	}

	// Artist tree (picker source) returns lightweight id+name pairs.
	res, b = doJSON(t, "GET", ts.URL+"/api/artists/tree", nil)
	expectStatus(t, res, 200, "artist tree")
	var tree []models.ArtistTree
	decodeResp(t, b, &tree)
	if len(tree) == 0 {
		t.Fatalf("artist tree should not be empty: %v", tree)
	}
	res, b = doJSON(t, "GET", ts.URL+"/api/records/search?q=改", nil)
	expectStatus(t, res, 200, "search")
	decodeResp(t, b, &list)
	if len(list) != 1 {
		t.Fatalf("search: %v", list)
	}
	res, b = doJSON(t, "GET", ts.URL+"/api/records/search?q=", nil)
	expectStatus(t, res, 200, "search empty")
	decodeResp(t, b, &list)
	if len(list) != 0 {
		t.Fatalf("search empty should return []: %v", list)
	}

	// Batch update / delete.
	res, b = doJSON(t, "POST", ts.URL+"/api/records/batch", map[string]interface{}{
		"ids": []string{created.ID}, "city": "北京", "play": map[string]interface{}{"op": "append", "value": []string{"寻梦"}},
	})
	expectStatus(t, res, 200, "batch update")
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/batch", map[string]interface{}{"ids": []string{}})
	expectStatus(t, res, 400, "batch update empty ids")
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/batch", []byte("{"))
	expectStatus(t, res, 400, "batch update invalid body")

	res, b = doJSON(t, "POST", ts.URL+"/api/records/batch/delete", map[string]interface{}{"ids": []string{created.ID}})
	expectStatus(t, res, 200, "batch delete")
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/batch/delete", map[string]interface{}{})
	expectStatus(t, res, 400, "batch delete empty")

	// Align venues.
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/align-venues", nil)
	expectStatus(t, res, 200, "align venues")

	// Delete.
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/records/missing", nil)
	expectStatus(t, res, 200, "delete missing is ok")

	_ = h
}

// ---------- categories / dramas / zhezis ----------

func TestCategoriesEndpoints(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)
	res, _ := doJSON(t, "GET", ts.URL+"/api/categories", nil)
	expectStatus(t, res, 200, "list categories")

	res, b := doJSON(t, "POST", ts.URL+"/api/categories", map[string]interface{}{"name": "昆曲", "activeIds": []string{"a"}})
	expectStatus(t, res, 201, "create category")
	var cat models.Category
	decodeResp(t, b, &cat)
	res, _ = doJSON(t, "POST", ts.URL+"/api/categories", map[string]interface{}{})
	expectStatus(t, res, 400, "create category missing name")
	res, _ = doJSON(t, "POST", ts.URL+"/api/categories", []byte("{"))
	expectStatus(t, res, 400, "create category invalid body")

	res, _ = doJSON(t, "PUT", ts.URL+"/api/categories/"+cat.ID, map[string]interface{}{"name": "越剧"})
	expectStatus(t, res, 200, "update category")
	res, _ = doJSON(t, "PUT", ts.URL+"/api/categories/x", []byte("{"))
	expectStatus(t, res, 400, "update category invalid body")

	res, _ = doJSON(t, "DELETE", ts.URL+"/api/categories/"+cat.ID, nil)
	expectStatus(t, res, 200, "delete category")

	// Manual reorder.
	res, _ = doJSON(t, "POST", ts.URL+"/api/categories/reorder", map[string]interface{}{"ids": []string{"x", cat.ID}})
	expectStatus(t, res, 200, "reorder categories")
	res, _ = doJSON(t, "POST", ts.URL+"/api/categories/reorder", []byte("{"))
	expectStatus(t, res, 400, "reorder categories invalid body")
}

func TestDramasAndZhezisEndpoints(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)
	res, _ := doJSON(t, "GET", ts.URL+"/api/dramas", nil)
	expectStatus(t, res, 200, "list dramas")
	res, _ = doJSON(t, "GET", ts.URL+"/api/dramas/tree", nil)
	expectStatus(t, res, 200, "drama tree")

	res, b := doJSON(t, "POST", ts.URL+"/api/dramas", map[string]interface{}{"name": " 牡丹亭 ", "categoryName": "昆曲"})
	expectStatus(t, res, 201, "create drama")
	var d models.Drama
	decodeResp(t, b, &d)
	if d.Name != "牡丹亭" {
		t.Fatalf("drama name should be trimmed: %+v", d)
	}
	res, _ = doJSON(t, "POST", ts.URL+"/api/dramas", map[string]interface{}{"name": "  "})
	expectStatus(t, res, 400, "create drama blank name")
	res, _ = doJSON(t, "POST", ts.URL+"/api/dramas", []byte("{"))
	expectStatus(t, res, 400, "create drama invalid body")

	res, b = doJSON(t, "GET", ts.URL+"/api/dramas/"+d.ID, nil)
	expectStatus(t, res, 200, "get drama")
	decodeResp(t, b, &d)
	res, _ = doJSON(t, "GET", ts.URL+"/api/dramas/missing", nil)
	expectStatus(t, res, 404, "get drama missing")

	res, _ = doJSON(t, "PUT", ts.URL+"/api/dramas/"+d.ID, map[string]interface{}{"name": "牡丹亭2"})
	expectStatus(t, res, 200, "update drama")
	res, _ = doJSON(t, "PUT", ts.URL+"/api/dramas/"+d.ID, map[string]interface{}{"name": ""})
	expectStatus(t, res, 400, "update drama blank name")

	res, b = doJSON(t, "POST", ts.URL+"/api/dramas/"+d.ID+"/zhezis", map[string]interface{}{"name": "惊梦", "aliases": []string{"游园"}})
	expectStatus(t, res, 201, "create zhezi")
	var z models.Zhezi
	decodeResp(t, b, &z)
	res, _ = doJSON(t, "POST", ts.URL+"/api/dramas/"+d.ID+"/zhezis", map[string]interface{}{"name": ""})
	expectStatus(t, res, 400, "create zhezi blank name")

	res, _ = doJSON(t, "PUT", ts.URL+"/api/zhezis/"+z.ID, map[string]interface{}{"name": "惊梦改"})
	expectStatus(t, res, 200, "update zhezi")
	res, _ = doJSON(t, "PUT", ts.URL+"/api/zhezis/missing", map[string]interface{}{"name": "x"})
	expectStatus(t, res, 404, "update zhezi missing")

	res, _ = doJSON(t, "POST", ts.URL+"/api/dramas/"+d.ID+"/zhezis/reorder", map[string]interface{}{"ids": []string{z.ID}})
	expectStatus(t, res, 200, "reorder zhezis")
	res, _ = doJSON(t, "POST", ts.URL+"/api/dramas/"+d.ID+"/zhezis/reorder", []byte("{"))
	expectStatus(t, res, 400, "reorder invalid body")

	res, _ = doJSON(t, "POST", ts.URL+"/api/dramas/reorder", map[string]interface{}{"ids": []string{d.ID}})
	expectStatus(t, res, 200, "reorder dramas")
	res, _ = doJSON(t, "POST", ts.URL+"/api/dramas/reorder", []byte("{"))
	expectStatus(t, res, 400, "reorder dramas invalid body")

	res, _ = doJSON(t, "DELETE", ts.URL+"/api/zhezis/"+z.ID, nil)
	expectStatus(t, res, 200, "delete zhezi")
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/dramas/"+d.ID, nil)
	expectStatus(t, res, 200, "delete drama")
}

// ---------- stats / calendar / autocomplete / settings ----------

func TestStatsCalendarSettings(t *testing.T) {
	ts, _, database, _ := newTestServer(t, nil)
	_ = database.UpsertRecord(models.Record{ID: "st1", Name: "A", City: "上海", CategoryName: "昆曲",
		Date: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC).Unix(), Rating: 5})

	res, b := doJSON(t, "GET", ts.URL+"/api/stats", nil)
	expectStatus(t, res, 200, "stats")
	var stats models.Stats
	decodeResp(t, b, &stats)
	if stats.TotalRecords != 1 {
		t.Fatalf("stats: %+v", stats)
	}

	res, b = doJSON(t, "GET", ts.URL+"/api/dashboard", nil)
	expectStatus(t, res, 200, "dashboard")
	var dash models.DashboardStats
	decodeResp(t, b, &dash)
	if dash.TotalRecords != 1 {
		t.Fatalf("dashboard: %+v", dash)
	}

	res, b = doJSON(t, "GET", ts.URL+"/api/calendar?year=2026&month=8", nil)
	expectStatus(t, res, 200, "calendar")
	var evs []models.CalendarEvent
	decodeResp(t, b, &evs)
	if len(evs) != 1 {
		t.Fatalf("calendar: %+v", evs)
	}
	res, _ = doJSON(t, "GET", ts.URL+"/api/calendar", nil)
	expectStatus(t, res, 200, "calendar default")

	res, b = doJSON(t, "GET", ts.URL+"/api/calendar.ics", nil)
	expectStatus(t, res, 200, "ics")
	if !strings.Contains(string(b), "BEGIN:VCALENDAR") {
		t.Fatalf("ics body: %s", b)
	}

	res, b = doJSON(t, "GET", ts.URL+"/api/autocomplete/city", nil)
	expectStatus(t, res, 200, "autocomplete")
	var cities []string
	decodeResp(t, b, &cities)
	if len(cities) != 1 || cities[0] != "上海" {
		t.Fatalf("autocomplete: %v", cities)
	}
	res, _ = doJSON(t, "GET", ts.URL+"/api/autocomplete/bogus", nil)
	expectStatus(t, res, 400, "autocomplete invalid")

	res, b = doJSON(t, "GET", ts.URL+"/api/field/name/A", nil)
	expectStatus(t, res, 200, "field")
	var recs []models.Record
	decodeResp(t, b, &recs)
	if len(recs) != 1 {
		t.Fatalf("field: %v", recs)
	}
	res, _ = doJSON(t, "GET", ts.URL+"/api/field/bogus/x", nil)
	expectStatus(t, res, 400, "field invalid")

	res, b = doJSON(t, "GET", ts.URL+"/api/settings", nil)
	expectStatus(t, res, 200, "get settings")
	res, b = doJSON(t, "PUT", ts.URL+"/api/settings", map[string]interface{}{"theme": "dark", "image_format": "webp"})
	expectStatus(t, res, 200, "update settings")
	var s map[string]interface{}
	decodeResp(t, b, &s)
	if s["theme"] != "dark" {
		t.Fatalf("settings: %v", s)
	}
	res, _ = doJSON(t, "PUT", ts.URL+"/api/settings", []byte("{"))
	expectStatus(t, res, 400, "update settings invalid body")
}

// ---------- upload / export / import ----------

func TestUploadExportImport(t *testing.T) {
	ts, _, database, store := newTestServer(t, nil)

	// Upload.
	res, b := uploadFile(t, ts.URL+"/api/upload", "file", "poster.jpg", jpgFixture(), "")
	expectStatus(t, res, 200, "upload")
	var up map[string]interface{}
	decodeResp(t, b, &up)
	if up["key"] == "" {
		t.Fatalf("upload: %v", up)
	}
	res, _ = doJSON(t, "POST", ts.URL+"/api/upload", nil)
	expectStatus(t, res, 400, "upload no file")

	// Export json + zip.
	_ = database.UpsertRecord(models.Record{ID: "exp1", Name: "导出", CoverFile: up["key"].(string), Date: time.Now().Unix()})
	res, b = doReq(t, "GET", ts.URL+"/api/export", nil, "")
	expectStatus(t, res, 200, "export json")
	if !strings.Contains(string(b), "导出") {
		t.Fatalf("export body: %s", b)
	}
	res, b = doReq(t, "GET", ts.URL+"/api/export?format=zip", nil, "")
	expectStatus(t, res, 200, "export zip")
	if res.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("zip content type: %s", res.Header.Get("Content-Type"))
	}
	if !bytes.Contains(b, []byte("PK")) {
		t.Fatalf("zip magic missing")
	}

	// Import JSON.
	exp := models.ExportData{
		Source: "mujian", Records: []models.Record{{ID: "imp1", Name: "导入", Date: time.Now().Unix()}},
		Categories: []models.Category{{ID: "c1", Name: "昆曲"}},
	}
	eb, _ := json.Marshal(exp)
	res, b = uploadFile(t, ts.URL+"/api/records/import", "file", "data.json", eb, "")
	expectStatus(t, res, 200, "import json")
	if !strings.Contains(string(b), `"records":1`) {
		t.Fatalf("import json body: %s", b)
	}
	// Bad json.
	res, _ = uploadFile(t, ts.URL+"/api/records/import", "file", "bad.json", []byte("{bad"), "")
	expectStatus(t, res, 400, "import bad json")
	// Unsupported ext.
	res, _ = uploadFile(t, ts.URL+"/api/records/import", "file", "x.txt", []byte("hi"), "")
	expectStatus(t, res, 400, "import unsupported ext")

	// Import zip (converted layout: data.json + covers/). The record carries a
	// coverFile that must be materialized from the archive.
	expWithCover := exp
	expWithCover.Records[0].CoverFile = "covers/abc123.jpg"
	ebWithCover, _ := json.Marshal(expWithCover)
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	df, _ := zw.Create("data.json")
	df.Write(ebWithCover)
	cf, _ := zw.Create("covers/abc123.jpg")
	cf.Write(jpgFixture())
	zw.Close()
	res, b = uploadFile(t, ts.URL+"/api/records/import", "file", "backup.zip", zbuf.Bytes(), "")
	expectStatus(t, res, 200, "import zip")
	if !strings.Contains(string(b), `"covers_imported":1`) {
		t.Fatalf("import zip body: %s", b)
	}

	// Import zip missing data file.
	var zbad bytes.Buffer
	zw2 := zip.NewWriter(&zbad)
	f2, _ := zw2.Create("nope.txt")
	f2.Write([]byte("x"))
	zw2.Close()
	res, _ = uploadFile(t, ts.URL+"/api/records/import", "file", "bad.zip", zbad.Bytes(), "")
	expectStatus(t, res, 400, "import zip no data")

	// Import raw-deflate JI_LU_XIAN_CHANG.android layout with base64 covers.
	var raw bytes.Buffer
	fw, _ := flate.NewWriter(&raw, flate.DefaultCompression)
	androidJSON := map[string]interface{}{
		"active": []map[string]interface{}{
			{"id": "jl1", "name": "现场导入", "cover": "coveruuid", "customCategoryId": "cat1", "date": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()},
		},
		"customCategory": []map[string]interface{}{{"id": "cat1", "name": "越剧"}},
	}
	ajb, _ := json.Marshal(androidJSON)
	fw.Write(ajb)
	fw.Close()

	var zbuf2 bytes.Buffer
	zw3 := zip.NewWriter(&zbuf2)
	af, _ := zw3.Create("JI_LU_XIAN_CHANG.android")
	af.Write(raw.Bytes())
	cvf, _ := zw3.Create("covers/coveruuid")
	cvf.Write([]byte(base64.StdEncoding.EncodeToString(jpgFixture())))
	zw3.Close()
	res, b = uploadFile(t, ts.URL+"/api/records/import", "file", "JI_LU_XIAN_CHANG.android.zip", zbuf2.Bytes(), "")
	expectStatus(t, res, 200, "import android zip")
	if !strings.Contains(string(b), `"covers_imported":1`) {
		t.Fatalf("import android zip body: %s", b)
	}

	// backup/restore endpoint.
	res, b = uploadFile(t, ts.URL+"/api/backup/restore", "file", "data.json", eb, "")
	expectStatus(t, res, 200, "backup restore")
	res, _ = uploadFile(t, ts.URL+"/api/backup/restore", "file", "bad.json", []byte("{bad"), "")
	expectStatus(t, res, 400, "backup restore bad json")
	res, _ = doJSON(t, "POST", ts.URL+"/api/backup/restore", nil)
	expectStatus(t, res, 400, "backup restore no file")

	_ = store
}

// ---------- covers ----------

func seedCover(t *testing.T, h *Handler, data []byte, id string) string {
	t.Helper()
	key, _, err := h.storage.SaveCoverBytes(data, storage.DetectExt(data))
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		_ = h.db.UpsertRecord(models.Record{ID: id, Name: "演出" + id, CoverFile: key, Date: time.Now().Unix()})
	}
	hash := storage.HashBytes(data)
	_ = h.db.UpsertCoverMeta(hash, key, filepath.Ext(key), int64(len(data)))
	return key
}

func TestCoversEndpoints(t *testing.T) {
	ts, h, _, store := newTestServer(t, nil)
	jpg := jpgFixture()
	key := seedCover(t, h, jpg, "cov1")

	res, b := doJSON(t, "GET", ts.URL+"/api/covers?q=&limit=10", nil)
	expectStatus(t, res, 200, "list covers")
	var picker map[string]interface{}
	decodeResp(t, b, &picker)
	if picker["total"].(float64) < 1 {
		t.Fatalf("picker: %v", picker)
	}

	res, b = doJSON(t, "GET", ts.URL+"/api/covers/duplicates", nil)
	expectStatus(t, res, 200, "duplicates")
	var dup map[string]interface{}
	decodeResp(t, b, &dup)
	if _, ok := dup["groups"]; !ok {
		t.Fatalf("duplicates response shape: %v", dup)
	}

	// Create an orphan (unreferenced cover) and list orphans.
	orphanKey, _, err := store.SaveCoverBytes([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}, ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	_ = h.db.UpsertCoverMeta("orphanhash", orphanKey, ".jpg", 10)
	res, b = doJSON(t, "GET", ts.URL+"/api/covers/orphans", nil)
	expectStatus(t, res, 200, "orphans")
	var orph map[string]interface{}
	decodeResp(t, b, &orph)
	if orph["count"].(float64) < 1 {
		t.Fatalf("orphans: %v", orph)
	}

	// Cleanup (all) moves the orphan to trash.
	res, b = doJSON(t, "POST", ts.URL+"/api/covers/cleanup", map[string]interface{}{"all": true})
	expectStatus(t, res, 200, "cleanup all")
	var clean map[string]interface{}
	decodeResp(t, b, &clean)
	if clean["moved"].(float64) < 1 {
		t.Fatalf("cleanup: %v", clean)
	}
	res, _ = doJSON(t, "POST", ts.URL+"/api/covers/cleanup", []byte("{"))
	expectStatus(t, res, 400, "cleanup invalid body")

	res, b = doJSON(t, "POST", ts.URL+"/api/covers/trash/purge", nil)
	expectStatus(t, res, 200, "purge trash")
	var purge map[string]interface{}
	decodeResp(t, b, &purge)
	if purge["purged"].(float64) < 1 {
		t.Fatalf("purge: %v", purge)
	}

	// Merge: create a real duplicate group — two distinct files with identical
	// bytes (SaveCoverBytes would dedupe, so write the second file directly).
	dupPath := filepath.Join(h.cfg.UploadDir, "covers", "manualdup.jpg")
	if err := os.WriteFile(dupPath, jpg, 0644); err != nil {
		t.Fatal(err)
	}
	dupKey := "covers/manualdup.jpg"
	_ = h.db.UpsertRecord(models.Record{ID: "cov2", Name: "演出cov2", CoverFile: dupKey, Date: time.Now().Unix()})
	_ = h.db.UpsertCoverMeta(storage.HashBytes(jpg), dupKey, ".jpg", int64(len(jpg)))

	groups, _ := h.db.GetDuplicateGroups()
	var hash string
	for _, g := range groups {
		hash = g.Hash
	}
	if hash == "" {
		t.Fatal("expected duplicate groups for merge")
	}
	res, b = doJSON(t, "POST", ts.URL+"/api/covers/merge", map[string]interface{}{"hashes": []string{hash}})
	expectStatus(t, res, 200, "merge")
	var merge map[string]interface{}
	decodeResp(t, b, &merge)
	if merge["merged_groups"].(float64) < 1 {
		t.Fatalf("merge: %v", merge)
	}
	res, _ = doJSON(t, "POST", ts.URL+"/api/covers/merge", map[string]interface{}{})
	expectStatus(t, res, 400, "merge empty hashes")
	res, _ = doJSON(t, "POST", ts.URL+"/api/covers/merge", []byte("{"))
	expectStatus(t, res, 400, "merge invalid body")

	// Single convert.
	res, b = doJSON(t, "POST", ts.URL+"/api/covers/convert", map[string]interface{}{"key": key, "format": "webp"})
	expectStatus(t, res, 200, "convert single")
	var conv map[string]interface{}
	decodeResp(t, b, &conv)
	newKey, _ := conv["key"].(string)
	if newKey == "" {
		t.Fatal("convert should return a key")
	}
	res, _ = doJSON(t, "POST", ts.URL+"/api/covers/convert", map[string]interface{}{})
	expectStatus(t, res, 400, "convert missing key")
	res, _ = doJSON(t, "POST", ts.URL+"/api/covers/convert", map[string]interface{}{"key": newKey, "format": "gif"})
	expectStatus(t, res, 400, "convert unsupported format")
	res, _ = doJSON(t, "POST", ts.URL+"/api/covers/convert", []byte("{"))
	expectStatus(t, res, 400, "convert invalid body")

	// Thumbnail regeneration (streams NDJSON).
	res, b = doJSON(t, "POST", ts.URL+"/api/covers/thumbs", nil)
	expectStatus(t, res, 200, "thumbs")
	if !strings.Contains(string(b), `"done":true`) {
		t.Fatalf("thumbs should stream done line: %s", b)
	}

	// Batch convert (streams NDJSON). Empty target -> start + done.
	res, b = doJSON(t, "POST", ts.URL+"/api/covers/convert-batch", map[string]interface{}{"format": "webp"})
	expectStatus(t, res, 200, "convert batch")
	if !strings.Contains(string(b), `"done":true`) {
		t.Fatalf("batch should stream done line: %s", b)
	}
	res, _ = doJSON(t, "POST", ts.URL+"/api/covers/convert-batch", []byte("{"))
	expectStatus(t, res, 400, "batch invalid body")
	res, _ = doJSON(t, "POST", ts.URL+"/api/covers/convert-batch", map[string]interface{}{"format": "gif"})
	expectStatus(t, res, 400, "batch unsupported format")
}

func TestUploadDisabledLocalStorage(t *testing.T) {
	ts, _, _, _ := newTestServer(t, func(c *config.Config) {
		c.AllowLocalStorage = false
		c.StorageType = "local"
	})
	res, _ := uploadFile(t, ts.URL+"/api/upload", "file", "p.jpg", jpgFixture(), "")
	expectStatus(t, res, 403, "upload disabled")
}

func TestListCoversPagination(t *testing.T) {
	ts, h, _, _ := newTestServer(t, nil)
	base := jpgFixture()
	for i := 0; i < 3; i++ {
		// Distinct content so content-addressed storage keeps 3 files.
		data := append(append([]byte{}, base...), byte(i))
		seedCover(t, h, data, fmt.Sprintf("pg%d", i))
	}
	res, b := doJSON(t, "GET", ts.URL+"/api/covers?limit=2&page=0", nil)
	expectStatus(t, res, 200, "paginated covers")
	var out map[string]interface{}
	decodeResp(t, b, &out)
	if out["total"].(float64) < 3 {
		t.Fatalf("total: %v", out)
	}
	// invalid limit falls back to default 30
	res, _ = doJSON(t, "GET", ts.URL+"/api/covers?limit=9999", nil)
	expectStatus(t, res, 200, "invalid limit")
}

func TestBatchConvertPathsAndFormatNormalization(t *testing.T) {
	ts, h, _, _ := newTestServer(t, nil)
	jpg := jpgFixture()

	// Cover that will be converted (jpeg content).
	keyA := seedCover(t, h, jpg, "bc1")
	// Cover already webp -> skipped.
	webpKey, _, err := h.storage.ConvertCover(keyA, "webp")
	if err != nil {
		t.Fatal(err)
	}
	_ = h.db.UpsertRecord(models.Record{ID: "bc2", Name: "已是webp", CoverFile: webpKey, Date: time.Now().Unix()})
	// Cover that fails to convert (undecodable content).
	garbageKey, _, err := h.storage.SaveCoverBytes([]byte("this is not an image at all"), ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	_ = h.db.UpsertCoverMeta("garbagehash", garbageKey, ".jpg", 26)
	// Another jpeg cover with distinct content -> converted.
	other := append(append([]byte{}, jpg...), 0x01)
	seedCover(t, h, other, "bc3")

	res, b := doJSON(t, "POST", ts.URL+"/api/covers/convert-batch", map[string]interface{}{"format": "webp"})
	expectStatus(t, res, 200, "batch paths")
	body := string(b)
	if !strings.Contains(body, `"status":"converted"`) {
		t.Errorf("expected converted item: %s", body)
	}
	if !strings.Contains(body, `"status":"skipped"`) {
		t.Errorf("expected skipped item: %s", body)
	}
	if !strings.Contains(body, `"status":"error"`) {
		t.Errorf("expected error item: %s", body)
	}
	if !strings.Contains(body, `"done":true`) {
		t.Errorf("expected done line: %s", body)
	}

	// Format normalization variants.
	for _, f := range []string{"jpg", "png", ""} {
		res, _ = doJSON(t, "POST", ts.URL+"/api/covers/convert-batch", map[string]interface{}{"format": f})
		expectStatus(t, res, 200, "batch normalized format "+f)
	}
}

func TestImportAndroidZlib(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)
	ajb, _ := json.Marshal(map[string]interface{}{
		"active": []map[string]interface{}{
			{"id": "z1", "name": "zlib导入", "cover": "zcover", "customCategoryId": "zc", "date": time.Now().Unix()},
		},
		"customCategory": []map[string]interface{}{{"id": "zc", "name": "京剧"}},
	})
	// zlib-compressed (not raw deflate) payload.
	var zc bytes.Buffer
	zw := zlib.NewWriter(&zc)
	zw.Write(ajb)
	zw.Close()

	var zbuf bytes.Buffer
	z3 := zip.NewWriter(&zbuf)
	zf, _ := z3.Create("JI_LU_XIAN_CHANG.android")
	zf.Write(zc.Bytes())
	cv, _ := z3.Create("covers/zcover")
	cv.Write([]byte(base64.StdEncoding.EncodeToString(jpgFixture())))
	z3.Close()

	res, b := uploadFile(t, ts.URL+"/api/records/import", "file", "JI_LU_XIAN_CHANG.android.zip", zbuf.Bytes(), "")
	expectStatus(t, res, 200, "import zlib android zip")
	if !strings.Contains(string(b), `"covers_imported":1`) {
		t.Fatalf("zlib import body: %s", b)
	}
}

// ---------- artists endpoints (entity table + reverse lookup) ----------

func TestArtistsEndpoints(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)

	res, b := doJSON(t, "GET", ts.URL+"/api/artists", nil)
	expectStatus(t, res, 200, "list artists (empty)")

	// Create with trimming + aliases.
	res, b = doJSON(t, "POST", ts.URL+"/api/artists", map[string]interface{}{
		"name":    " 张军 ",
		"aliases": []string{"张三"},
		"bio":     "昆曲演员",
	})
	expectStatus(t, res, 201, "create artist")
	var a models.Artist
	decodeResp(t, b, &a)
	if a.Name != "张军" {
		t.Fatalf("artist name should be trimmed: %+v", a)
	}
	// Blank name -> 400.
	res, _ = doJSON(t, "POST", ts.URL+"/api/artists", map[string]interface{}{"name": "  "})
	expectStatus(t, res, 400, "create artist blank name")
	// Invalid body -> 400.
	res, _ = doJSON(t, "POST", ts.URL+"/api/artists", []byte("{"))
	expectStatus(t, res, 400, "create artist invalid body")

	// Link a record to the artist; verify reverse lookup counts it.
	res, b = doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{
		"name":         "牡丹亭",
		"date":         time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC).Unix(),
		"artist_names": []string{"张军"},
	})
	expectStatus(t, res, 201, "create record linking artist")
	res, b = doJSON(t, "GET", ts.URL+"/api/artists/"+a.ID, nil)
	expectStatus(t, res, 200, "get artist detail")
	var detail models.ArtistDetail
	decodeResp(t, b, &detail)
	if detail.Artist.Name != "张军" || len(detail.Records) != 1 {
		t.Fatalf("artist detail wrong: %+v", detail)
	}
	res, _ = doJSON(t, "GET", ts.URL+"/api/artists/missing", nil)
	expectStatus(t, res, 404, "get artist missing")

	// Update (by id) with new name + bio.
	res, b = doJSON(t, "PUT", ts.URL+"/api/artists/"+a.ID, map[string]interface{}{"name": "张军(改)", "bio": "国家一级演员"})
	expectStatus(t, res, 200, "update artist")
	decodeResp(t, b, &a)
	if a.Name != "张军(改)" || a.Bio != "国家一级演员" {
		t.Fatalf("artist update wrong: %+v", a)
	}
	// Update blank name -> 400.
	res, _ = doJSON(t, "PUT", ts.URL+"/api/artists/"+a.ID, map[string]interface{}{"name": ""})
	expectStatus(t, res, 400, "update artist blank name")

	// Second artist + reorder.
	res, b = doJSON(t, "POST", ts.URL+"/api/artists", map[string]interface{}{"name": "单雯"})
	expectStatus(t, res, 201, "create second artist")
	var other models.Artist
	decodeResp(t, b, &other)
	res, _ = doJSON(t, "POST", ts.URL+"/api/artists/reorder", map[string]interface{}{"ids": []string{other.ID, a.ID}})
	expectStatus(t, res, 200, "reorder artists")
	res, _ = doJSON(t, "POST", ts.URL+"/api/artists/reorder", []byte("{"))
	expectStatus(t, res, 400, "reorder artists invalid body")
	res2, b2 := doJSON(t, "GET", ts.URL+"/api/artists", nil)
	expectStatus(t, res2, 200, "list artists after reorder")
	var arts []models.Artist
	decodeResp(t, b2, &arts)
	if len(arts) != 2 || arts[0].ID != other.ID {
		t.Fatalf("reorder did not take effect: %+v", arts)
	}

	// Delete cascades the record_artists link, then artist is gone.
	res, _ = doJSON(t, "DELETE", ts.URL+"/api/artists/"+a.ID, nil)
	expectStatus(t, res, 200, "delete artist")
	res, _ = doJSON(t, "GET", ts.URL+"/api/artists/"+a.ID, nil)
	expectStatus(t, res, 404, "deleted artist gone")
}

// TestRecordsAggregationCoverage exercises the stats/dashboard/calendar/ICS and
// venue-alignment code paths with multiple records of varying city/rating/month
// so the aggregation branches get covered (pushes handlers coverage past 85%).
func TestRecordsAggregationCoverage(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)

	cities := []string{"上海", "北京", "南京"}
	months := []int{1, 3, 5, 7, 9, 11}
	for i, m := range months {
		res, _ := doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{
			"name":         fmt.Sprintf("演出%d", i),
			"city":         cities[i%len(cities)],
			"rating":       (i % 5) + 1,
			"date":         time.Date(2026, time.Month(m), 15, 19, 30, 0, 0, time.UTC).Unix(),
			"artist_names": []string{"张军"},
			"price":        100 + i*10,
		})
		expectStatus(t, res, 201, fmt.Sprintf("create record %d", i))
	}

	getPaths := []string{
		"/stats", "/dashboard",
		"/calendar?year=2026&month=5",
		"/calendar",
		"/calendar.ics",
		"/autocomplete/city",
		"/field/city/北京",
		"/records/all",
	}
	for _, path := range getPaths {
		res, _ := doJSON(t, "GET", ts.URL+"/api"+path, nil)
		expectStatus(t, res, 200, "GET "+path)
	}
	// align-venues is a POST endpoint.
	ares, _ := doJSON(t, "POST", ts.URL+"/api/records/align-venues", nil)
	expectStatus(t, ares, 200, "POST align-venues")

	// Export paths.
	res, _ := doJSON(t, "GET", ts.URL+"/api/export", nil)
	expectStatus(t, res, 200, "export json")
	res, _ = doJSON(t, "GET", ts.URL+"/api/export?format=zip", nil)
	expectStatus(t, res, 200, "export zip")
}

// TestRecordFieldBatchBranches covers getByField / autocomplete across many
// field values and batchUpdate with various array ops, exercising branches that
// the basic CRUD tests leave untouched.
func TestRecordFieldBatchBranches(t *testing.T) {
	ts, _, database, _ := newTestServer(t, nil)

	// Seed a drama + zhezi + artist, then a record referencing them.
	res, b := doJSON(t, "POST", ts.URL+"/api/dramas", map[string]interface{}{"name": "牡丹亭"})
	expectStatus(t, res, 201, "create drama")
	var d models.Drama
	decodeResp(t, b, &d)
	res, b = doJSON(t, "POST", ts.URL+"/api/dramas/"+d.ID+"/zhezis", map[string]interface{}{"name": "惊梦"})
	expectStatus(t, res, 201, "create zhezi")
	var z models.Zhezi
	decodeResp(t, b, &z)
	res, b = doJSON(t, "POST", ts.URL+"/api/artists", map[string]interface{}{"name": "张军"})
	expectStatus(t, res, 201, "create artist")
	var art models.Artist
	decodeResp(t, b, &art)
	_ = art

	res, b = doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{
		"name":         "游园",
		"city":         "上海",
		"categoryName": "昆曲",
		"company":      "江苏省昆剧院",
		"play":         []string{"游园"},
		"rating":       4,
		"date":         time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC).Unix(),
		"artist_names": []string{"张军"},
		"drama_ids":    []string{d.ID},
		"zhezi_ids":    []string{z.ID},
	})
	expectStatus(t, res, 201, "create record")
	var rec models.Record
	decodeResp(t, b, &rec)

	// getByField across supported text fields (drama/zhezi/artist are not
	// textFields and intentionally return 400).
	for _, f := range []string{"name/游园", "city/上海", "category_name/昆曲", "company/江苏省昆剧院"} {
		res, _ := doJSON(t, "GET", ts.URL+"/api/field/"+f, nil)
		expectStatus(t, res, 200, "field "+f)
	}
	res, _ = doJSON(t, "GET", ts.URL+"/api/field/drama/"+d.ID, nil)
	expectStatus(t, res, 400, "field drama invalid")
	res, _ = doJSON(t, "GET", ts.URL+"/api/field/bogus/x", nil)
	expectStatus(t, res, 400, "field invalid")

	// autocomplete across supported text fields.
	for _, f := range []string{"city", "name", "category_name", "company", "seat", "address"} {
		res, _ := doJSON(t, "GET", ts.URL+"/api/autocomplete/"+f, nil)
		expectStatus(t, res, 200, "autocomplete "+f)
	}
	res, _ = doJSON(t, "GET", ts.URL+"/api/autocomplete/play", nil)
	expectStatus(t, res, 400, "autocomplete play invalid")

	// batchUpdate with scalar + array ops.
	res, b = doJSON(t, "POST", ts.URL+"/api/records/batch", map[string]interface{}{
		"ids":           []string{rec.ID},
		"rating":        5,
		"active_status": 1,
		"city":          "北京",
		"price":         320.5,
		"play":          map[string]interface{}{"op": "append", "value": []string{"惊梦"}},
		"artist_names":  map[string]interface{}{"op": "append", "value": []string{"单雯"}},
		"drama_ids":     map[string]interface{}{"op": "set", "value": []string{}},
		"tag_ids":       map[string]interface{}{"op": "remove", "value": []string{"x"}},
	})
	expectStatus(t, res, 200, "batch update ops")
	decodeResp(t, b, &map[string]interface{}{})
	_ = database
}

// TestBatchUpdateAllFields drives db.BatchUpdateRecords through every scalar
// field and every array op (set/append/remove) so the large optional-branch
// function is fully exercised.
func TestBatchUpdateAllFields(t *testing.T) {
	ts, _, _, _ := newTestServer(t, nil)

	res, b := doJSON(t, "POST", ts.URL+"/api/records", map[string]interface{}{
		"name":  "批量字段",
		"date":  time.Date(2026, 8, 22, 19, 30, 0, 0, time.UTC).Unix(),
		"play":  []string{"原"},
		"guest": []string{"客"},
	})
	expectStatus(t, res, 201, "create record")
	var rec models.Record
	decodeResp(t, b, &rec)

	op := func(o string, v ...string) map[string]interface{} { return map[string]interface{}{"op": o, "value": v} }

	res, b = doJSON(t, "POST", ts.URL+"/api/records/batch", map[string]interface{}{
		"ids":                []string{rec.ID},
		"category_name":      "昆曲",
		"rating":             5,
		"active_status":      2,
		"city":               "北京",
		"address":            "长安大戏院",
		"channel":            "大麦",
		"company":            "某院团",
		"friends":            "友人",
		"remark":             "备注",
		"seat":               "A1",
		"price":              200.0,
		"price_currency":     "CNY",
		"pay_price":          180.0,
		"pay_price_currency": "CNY",
		"other_cost":         20.0,
		"other_cost_currency": "CNY",
		"drama_ids":          op("set", "d1"),
		"zhezi_ids":          op("set", "z1"),
		"play":               op("append", "新折子"),
		"guest":              op("append", "新客"),
		"artist_names":       op("append", "张军"),
		"tag_ids":            op("remove", "x"),
	})
	expectStatus(t, res, 200, "batch update all fields")
	decodeResp(t, b, &map[string]interface{}{})

	// A second pass exercising remove on array fields.
	res, b = doJSON(t, "POST", ts.URL+"/api/records/batch", map[string]interface{}{
		"ids":          []string{rec.ID},
		"play":         op("remove", "原"),
		"guest":        op("remove", "客"),
		"artist_names": op("set", "单雯"),
		"drama_ids":    op("append", "d2"),
		"zhezi_ids":    op("append", "z2"),
	})
	expectStatus(t, res, 200, "batch update array remove/set")
	decodeResp(t, b, &map[string]interface{}{})

	// Invalid body / empty ids.
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/batch", []byte("{"))
	expectStatus(t, res, 400, "batch invalid body")
	res, _ = doJSON(t, "POST", ts.URL+"/api/records/batch", map[string]interface{}{"ids": []string{}})
	expectStatus(t, res, 400, "batch empty ids")
}
