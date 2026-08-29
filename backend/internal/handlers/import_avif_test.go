package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"

	"path/filepath"
	"strings"
	"testing"
	"time"

	"mujian/internal/storage"
)

// Regression: covers exported by mujian are AVIF binaries, but materializeCover's
// magic() only recognized jpeg/png/webp. Every AVIF entry fell through to the
// base64 branch, failed ("unsupported cover format"), and was counted missing —
// so a full re-import restored all records with their old coverFile paths while
// not a single image file existed on disk (every cover 404ed).
func TestImportMujianZipWithAvifCovers(t *testing.T) {
	ts, _, database, store := newTestServer(t, nil)

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 30), uint8(y * 30), 128, 255})
		}
	}
	avifBytes, ext, err := storage.EncodeImage(img, "avif")
	if err != nil {
		t.Skipf("avif encoder unavailable: %v", err)
	}
	if ext != ".avif" || storage.DetectImageFormat(avifBytes) != "avif" {
		t.Fatalf("encoder produced non-avif output (%s)", ext)
	}

	// Content-addressed name derived from the bytes, mirroring real exports.
	coverKey := "covers/" + storage.HashBytes(avifBytes) + ".avif"

	data := map[string]interface{}{
		"source":      "mujian",
		"recordCount": 1,
		"records": []map[string]interface{}{
			{"id": "avif-rec-1", "name": "AVIF 封面导入", "city": "上海", "dateText": "2026-08-23", "date": time.Now().Unix(), "coverFile": coverKey},
		},
		"categories": []map[string]interface{}{},
	}
	dataJSON, _ := json.Marshal(data)

	var zbuf bytes.Buffer
	zw := zip.NewWriter(&zbuf)
	entry, _ := zw.Create("data.json")
	entry.Write(dataJSON)
	cvEntry, _ := zw.Create(coverKey)
	cvEntry.Write(avifBytes)
	zw.Close()

	res, body := uploadFile(t, ts.URL+"/api/records/import", "file", "mujian_export.zip", zbuf.Bytes(), "")
	if res.StatusCode != 200 {
		t.Fatalf("import: status %d, body %s", res.StatusCode, body)
	}
	if !strings.Contains(string(body), `"covers_imported":1`) {
		t.Fatalf("expected the avif cover to import, got: %s", body)
	}

	// The record must reference a file that actually exists on disk.
	recJSON, _ := json.Marshal(map[string]string{})
	_ = recJSON
	got, err := database.GetRecord("avif-rec-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CoverFile == "" {
		t.Fatal("record has empty cover_file after import")
	}
	if _, err := store.ReadCover(got.CoverFile); err != nil {
		t.Fatalf("cover file %q not readable from storage: %v", got.CoverFile, err)
	}
	if filepath.Ext(got.CoverFile) != ".avif" {
		t.Errorf("stored cover should keep .avif extension, got %q", got.CoverFile)
	}
}
