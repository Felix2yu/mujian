package handlers

import (
	"encoding/json"
	"fmt"
	"mujian/internal/storage"
	"net/http"
)

// POST /storage/migrate-to-s3 — upload every local cover (posters and
// thumbnails) to the configured S3 bucket, keeping the same keys. Existing
// remote objects are skipped, so the run is idempotent and can be repeated
// before flipping storage_type to "s3" for a seamless switchover.
//
// Progress is streamed as newline-delimited JSON like /covers/convert-batch:
// one object per line, flushed immediately, with the final line carrying
// "done": true plus the summary.
func (h *Handler) migrateToS3(w http.ResponseWriter, r *http.Request) {
	s3cfg := h.cfg.GetS3Settings()
	if s3cfg.Bucket == "" || s3cfg.AccessKey == "" {
		jsonErr(w, 400, "S3 未配置完整（需要 Bucket 与 Access Key）")
		return
	}

	local := storage.NewLocalStorage(h.cfg.UploadDir, nil)
	remote := storage.NewS3StorageFromSettings(s3cfg, h.cfg.GetImageFormat)

	flusher, ok := w.(http.Flusher)
	emit := func(done, total int) {}
	if ok {
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no") // disable nginx proxy buffering
		w.WriteHeader(200)
		flusher.Flush()
		emit = func(done, total int) {
			b, _ := json.Marshal(map[string]interface{}{"phase": "item", "processed": done, "total": total})
			fmt.Fprintf(w, "%s\n", b)
			flusher.Flush()
		}
	}

	// Serialize with merge/cleanup/batch-convert so nothing moves files around
	// mid-migration.
	coverMu.Lock()
	defer coverMu.Unlock()

	stats, err := storage.MigrateLocalToS3(local, remote, emit)
	if err != nil {
		if ok {
			b, _ := json.Marshal(map[string]interface{}{"error": err.Error(), "done": true})
			fmt.Fprintf(w, "%s\n", b)
			flusher.Flush()
			return
		}
		jsonErr(w, 500, err.Error())
		return
	}
	if ok {
		b, _ := json.Marshal(map[string]interface{}{
			"done":     true,
			"total":    stats.Total,
			"migrated": stats.Migrated,
			"skipped":  stats.Skipped,
			"failed":   stats.Failed,
			"bytes":    stats.Bytes,
		})
		fmt.Fprintf(w, "%s\n", b)
		flusher.Flush()
		return
	}
	jsonResp(w, 200, stats)
}
