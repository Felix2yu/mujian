package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"mujian/internal/config"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mujian/internal/storage"
)

func imgBytes(t *testing.T, format string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{200, 10, 10, 255})
	b, _, err := storage.EncodeImage(img, format)
	if err != nil {
		t.Skipf("%s encoder unavailable: %v", format, err)
	}
	return b
}

func oneFileZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	e, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	e.Write(content)
	zw.Close()
	return zbuf.Bytes()
}

func materializeFromZip(t *testing.T, content []byte) ([]byte, bool) {
	t.Helper()
	raw := oneFileZip(t, "covers/x", content)
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := materializeCover(zr.File[0])
	return data, err == nil
}

func TestMaterializeCoverFormats(t *testing.T) {
	for _, format := range []string{"jpeg", "png", "webp", "avif"} {
		raw := imgBytes(t, format)
		if data, ok := materializeFromZip(t, raw); !ok || len(data) == 0 {
			t.Errorf("binary %s cover should be recognized", format)
		}
		// Data-uri wrapped base64 text form (记录现场 legacy exports).
		uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
		if data, ok := materializeFromZip(t, []byte(uri)); !ok || len(data) == 0 {
			t.Errorf("base64-wrapped %s cover should be decoded", format)
		}
		// Bare base64 text with newlines/whitespace.
		bare := base64.StdEncoding.EncodeToString(raw)
		wrapped := bare[:10] + "\n" + bare[10:] + " \n"
		if data, ok := materializeFromZip(t, []byte(wrapped)); !ok || len(data) == 0 {
			t.Errorf("bare base64 %s cover should be decoded", format)
		}
	}

	// Garbage is rejected, not silently stored.
	if _, ok := materializeFromZip(t, []byte("this is not an image at all")); ok {
		t.Error("text garbage should not materialize")
	}
	if _, ok := materializeFromZip(t, nil); ok {
		t.Error("empty content should not materialize")
	}
}

// ---------- full zip import with mixed cover outcomes ----------

// Decodable covers increment covers_imported; undecodable ones count as
// missing while the record itself still imports.
func TestImportZIPMixedCoverOutcomes(t *testing.T) {
	ts, _, database, store := newTestServer(t, nil)

	good := imgBytes(t, "avif")

	payload := map[string]interface{}{
		"source":      "mujian",
		"recordCount": 2,
		"records": []map[string]interface{}{
			{"id": "mix-1", "name": "好封面", "coverFile": "covers/good.avif"},
			{"id": "mix-2", "name": "坏封面", "coverFile": "covers/broken.avif"},
			{"id": "mix-3", "name": "无封面"},
		},
		"categories": []map[string]interface{}{},
	}
	dataJSON, _ := json.Marshal(payload)

	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	e, _ := zw.Create("data.json")
	e.Write(dataJSON)
	cv, _ := zw.Create("covers/good.avif")
	cv.Write(good)
	bad, _ := zw.Create("covers/broken.avif")
	bad.Write([]byte("definitely not an avif payload"))
	zw.Close()

	res, body := uploadFile(t, ts.URL+"/api/records/import", "file", "mixed.zip", zbuf.Bytes(), "")
	if res.StatusCode != 200 {
		t.Fatalf("import: status %d, body %s", res.StatusCode, body)
	}
	if !strings.Contains(string(body), `"covers_imported":1`) || !strings.Contains(string(body), `"covers_missing":1`) {
		t.Fatalf("expected 1 imported + 1 missing, got: %s", body)
	}

	for _, id := range []string{"mix-1", "mix-2", "mix-3"} {
		rec, err := database.GetRecord(id)
		if err != nil {
			t.Fatalf("record %s missing after import: %v", id, err)
		}
		if rec.ID == "mix-1" {
			if rec.CoverFile == "" {
				t.Error("mix-1 should reference its cover")
			} else if _, err := store.ReadCover(rec.CoverFile); err != nil {
				t.Errorf("mix-1 cover not on disk: %v", err)
			}
		}
	}
}

// Re-encoding and thumbnail generation must NOT block the import response:
// a JPEG cover imported under the WebP setting keeps its original bytes at
// response time, then background processing converts it to .webp, repoints
// the record, and fills in cover_thumb.
func TestImportCoversProcessedInBackground(t *testing.T) {
	ts, _, database, store := newTestServer(t, func(c *config.Config) {
		c.ImageFormat = "webp"
	})

	jpg := imgBytes(t, "jpeg")
	payload := map[string]interface{}{
		"source":      "mujian",
		"recordCount": 1,
		"records": []map[string]interface{}{
			{"id": "bg-1", "name": "后台转码", "coverFile": "covers/photo.jpg"},
		},
		"categories": []map[string]interface{}{},
	}
	dataJSON, _ := json.Marshal(payload)

	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	e, _ := zw.Create("data.json")
	e.Write(dataJSON)
	cv, _ := zw.Create("covers/photo.jpg")
	cv.Write(jpg)
	zw.Close()

	// Freeze background processing by holding coverMu (the goroutine blocks on
	// it), so the "original bytes preserved" assertions below are deterministic
	// instead of racing the converter.
	coverMu.Lock()

	res, body := uploadFile(t, ts.URL+"/api/records/import", "file", "bg.zip", zbuf.Bytes(), "")
	if res.StatusCode != 200 {
		t.Fatalf("import: status %d, body %s", res.StatusCode, body)
	}

	// Response is immediate: original bytes stored as-is (still .jpg),
	// no blocking re-encode happened.
	rec, err := database.GetRecord("bg-1")
	if err != nil || rec.CoverFile == "" {
		t.Fatalf("record after import: %v %v", rec, err)
	}
	if filepath.Ext(rec.CoverFile) != ".jpg" {
		t.Fatalf("import response should not wait for conversion, ext already %q", filepath.Ext(rec.CoverFile))
	}
	if got, _ := store.ReadCover(rec.CoverFile); storage.DetectImageFormat(got) != "jpeg" {
		coverMu.Unlock()
		t.Fatal("original jpeg bytes should be preserved verbatim at import time")
	}
	coverMu.Unlock()

	// Background processing converges to the preferred format + thumbnail.
	deadline := time.Now().Add(15 * time.Second)
	for {
		rec, err = database.GetRecord("bg-1")
		if err != nil {
			t.Fatalf("record vanished: %v", err)
		}
		if filepath.Ext(rec.CoverFile) == ".webp" && rec.CoverThumb != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background conversion did not finish in time: coverFile=%q thumb=%q",
				rec.CoverFile, rec.CoverThumb)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := store.ReadCover(rec.CoverThumb); err != nil {
		t.Errorf("thumbnail not on disk: %v", err)
	}
	bgCoverWG.Wait()
}
