package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"mujian/internal/models"
	"mujian/internal/storage"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// coverMu serializes merge/cleanup/thumbnail operations that touch both the
// database and the filesystem (single-user app, in-process mutex is enough).
var coverMu sync.Mutex

// GET /covers — distinct covers for the reuse picker.
func (h *Handler) listCovers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := 30
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 200 {
			limit = v
		}
	}
	page := 0
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v >= 0 {
			page = v
		}
	}

	covers, total, err := h.db.ListCoverPicker(q, limit, page*limit)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{
		"covers": covers,
		"total":  total,
		"page":   page,
		"limit":  limit,
	})
}

// GET /covers/duplicates — sync cover hashes and report duplicate groups.
func (h *Handler) getCoverDuplicates(w http.ResponseWriter, r *http.Request) {
	coverMu.Lock()
	defer coverMu.Unlock()

	scanned, err := h.db.SyncCovers(h.storage)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	groups, err := h.db.GetDuplicateGroups()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if groups == nil {
		groups = []models.DupGroup{}
	}
	jsonResp(w, 200, map[string]interface{}{"groups": groups, "scanned": scanned})
}

// POST /covers/merge {"hashes":[...]} — repoint all records in each group to
// the canonical covers/<hash>.<ext> file, then remove orphaned duplicates.
func (h *Handler) mergeCovers(w http.ResponseWriter, r *http.Request) {
	coverMu.Lock()
	defer coverMu.Unlock()

	var req struct {
		Hashes []string `json:"hashes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if len(req.Hashes) == 0 {
		jsonErr(w, 400, "no hashes provided")
		return
	}

	h.db.SyncCovers(h.storage)

	mergedGroups, updated := 0, int64(0)
	freed := int64(0)
	for _, hash := range req.Hashes {
		cover, err := h.db.GetCoverByHash(hash)
		if err != nil {
			continue
		}
		recs, err := h.db.GetRecordsByCoverHash(hash)
		if err != nil || len(recs) == 0 {
			continue
		}

		canonical := "covers/" + hash + cover.Ext

		// 1) ensure the canonical file exists (idempotent)
		if !h.storage.CoverExists(canonical) {
			data, rerr := h.storage.ReadCover(recs[0].CoverFile)
			if rerr != nil {
				continue
			}
			if _, _, err := h.storage.SaveCoverBytes(data, cover.Ext); err != nil {
				continue
			}
		}

		// 2) single-transaction repoint of all group members
		var ids []string
		oldFiles := map[string]bool{}
		for _, rec := range recs {
			ids = append(ids, rec.ID)
			if rec.CoverFile != canonical {
				oldFiles[rec.CoverFile] = true
			}
		}
		n, err := h.db.UpdateRecordsCoverFile(ids, canonical)
		if err != nil {
			continue
		}
		updated += n

		// 3) remove now-orphaned duplicate files (re-check ref count)
		for old := range oldFiles {
			if cnt, _ := h.db.CountCoverRefs(old); cnt > 0 {
				continue
			}
			size, _ := h.db.CoverSize(old)
			if err := h.storage.DeleteCover(old); err == nil {
				freed += size
				h.db.DeleteCoverMeta(old)
			}
		}

		h.db.UpsertCoverMeta(hash, canonical, cover.Ext, cover.Size)
		mergedGroups++
	}

	jsonResp(w, 200, map[string]interface{}{
		"merged_groups":   mergedGroups,
		"updated_records": updated,
		"freed_bytes":     freed,
	})
}

// GET /covers/orphans — files in storage not referenced by any record.
func (h *Handler) getCoverOrphans(w http.ResponseWriter, r *http.Request) {
	keys, err := h.storage.ListCoverKeys()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	files := []models.OrphanItem{}
	total := int64(0)
	for _, k := range keys {
		cnt, err := h.db.CountCoverRefs(k)
		if err != nil || cnt > 0 {
			continue
		}
		size, _ := h.db.CoverSize(k)
		if size == 0 {
			if data, err := h.storage.ReadCover(k); err == nil {
				size = int64(len(data))
			}
		}
		files = append(files, models.OrphanItem{FileName: filepath.Base(k), Size: size})
		total += size
	}
	jsonResp(w, 200, map[string]interface{}{
		"files":      files,
		"count":      len(files),
		"total_size": total,
	})
}

// POST /covers/cleanup {"files":[names]} | {"all":true} — move orphans to trash.
func (h *Handler) cleanupCovers(w http.ResponseWriter, r *http.Request) {
	coverMu.Lock()
	defer coverMu.Unlock()

	var req struct {
		Files []string `json:"files"`
		All   bool     `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}

	keys, err := h.storage.ListCoverKeys()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	moved := 0
	freed := int64(0)
	for _, k := range keys {
		name := filepath.Base(k)
		if !req.All && !slices.Contains(req.Files, name) {
			continue
		}
		cnt, err := h.db.CountCoverRefs(k)
		if err != nil || cnt > 0 {
			continue // never trash a referenced file
		}
		size, _ := h.db.CoverSize(k)
		if err := h.storage.MoveCoverToTrash(k); err == nil {
			moved++
			freed += size
			h.db.DeleteCoverMeta(k)
		}
	}
	jsonResp(w, 200, map[string]interface{}{"moved": moved, "freed_bytes": freed})
}

// POST /covers/trash/purge — permanently delete all trashed covers.
func (h *Handler) purgeTrash(w http.ResponseWriter, r *http.Request) {
	coverMu.Lock()
	defer coverMu.Unlock()
	n, err := h.storage.PurgeTrash()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{"purged": n})
}

// POST /covers/thumbs — (re)generate unified JPEG thumbnails for all records
// that have a cover file, storing base64 into records.cover_thumb.
// POST /covers/thumbs — regenerate every record's thumbnail in the current
// image format. Streams newline-delimited JSON progress (like convert-batch):
// regenerating thumbnails for a large library can exceed client request
// timeouts, and flushing after each cover keeps reverse-proxy idle timeouts
// from killing the connection.
func (h *Handler) regenerateThumbs(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		// Streaming unsupported by the ResponseWriter; fall back to a single
		// JSON response (legacy behavior).
		coverMu.Lock()
		updated, lerr := h.runRegenerateThumbs(r, func(map[string]interface{}) {})
		coverMu.Unlock()
		if lerr != nil {
			jsonErr(w, 500, lerr.Error())
			return
		}
		jsonResp(w, 200, map[string]interface{}{"updated": updated})
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	flusher.Flush()

	coverMu.Lock()
	defer coverMu.Unlock()

	updated, lerr := h.runRegenerateThumbs(r, func(v map[string]interface{}) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "%s\n", b)
		flusher.Flush()
	})
	if lerr != nil {
		b, _ := json.Marshal(map[string]interface{}{"error": lerr.Error(), "done": true})
		fmt.Fprintf(w, "%s\n", b)
		flusher.Flush()
		return
	}
	b, _ := json.Marshal(map[string]interface{}{"done": true, "updated": updated})
	fmt.Fprintf(w, "%s\n", b)
	flusher.Flush()
}

// runRegenerateThumbs groups records by cover file (so a cover shared by many
// records is only re-encoded once), regenerates each thumbnail in the current
// image format, and repoints the records' cover_thumb. Returns the number of
// records updated. The caller must hold coverMu.
func (h *Handler) runRegenerateThumbs(r *http.Request, emit func(map[string]interface{})) (int, error) {
	list, err := h.db.ListCoverFiles()
	if err != nil {
		return 0, err
	}
	byCover := make(map[string][]string) // coverFile -> record ids
	for _, item := range list {
		byCover[item.CoverFile] = append(byCover[item.CoverFile], item.ID)
	}
	covers := make([]string, 0, len(byCover))
	for k := range byCover {
		covers = append(covers, k)
	}
	slices.Sort(covers)

	total := len(covers)
	if emit != nil {
		emit(map[string]interface{}{"phase": "start", "total": total, "records": len(list)})
	}

	updated := 0
	for i, key := range covers {
		// Bail out if the client went away; no point burning CPU.
		if r.Context().Err() != nil {
			break
		}
		data, rerr := h.storage.ReadCover(key)
		if rerr != nil {
			if emit != nil {
				emit(map[string]interface{}{
					"phase": "item", "index": i, "total": total, "key": key,
					"status": "error", "error": "read failed", "updated": updated,
				})
			}
			continue
		}
		thumbKey, terr := h.storage.MakeThumbnail(key, data, 400, h.cfg.ImageFormat)
		if terr != nil {
			if emit != nil {
				emit(map[string]interface{}{
					"phase": "item", "index": i, "total": total, "key": key,
					"status": "error", "error": terr.Error(), "updated": updated,
				})
			}
			continue
		}
		okCount := 0
		for _, id := range byCover[key] {
			if err := h.db.SetRecordThumb(id, thumbKey); err == nil {
				okCount++
			}
		}
		updated += okCount
		if emit != nil {
			emit(map[string]interface{}{
				"phase": "item", "index": i, "total": total, "key": key,
				"status": "ok", "records": okCount, "updated": updated,
			})
		}
	}
	return updated, nil
}

// POST /covers/convert {"key":"...", "format":"avif|webp|jpeg"}
// Re-encode a single cover file to the target format and repoint all referencing records.
func (h *Handler) convertCover(w http.ResponseWriter, r *http.Request) {
	coverMu.Lock()
	defer coverMu.Unlock()

	var req struct {
		Key    string `json:"key"`
		Format string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if req.Key == "" {
		jsonErr(w, 400, "key is required")
		return
	}
	format := normalizeImageFormat(req.Format)
	if !isSupportedImageFormat(format) {
		jsonErr(w, 400, "unsupported format")
		return
	}

	newKey, converted, err := h.storage.ConvertCover(req.Key, format)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	if newKey != req.Key {
		if err := h.db.RepointCoverRefs(req.Key, newKey); err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		// regenerate thumbnails for affected records
		h.regenerateThumbsForCover(req.Key, newKey, format)
	}

	jsonResp(w, 200, map[string]interface{}{
		"key":       newKey,
		"converted": converted,
	})
}

// POST /covers/convert-batch {"format":"avif|webp|jpeg"}
// Re-encode every cover in storage to the target format, repoint records, and
// regenerate thumbnails. Skips files already in the target format.
//
// The work is CPU-heavy and can run far longer than a client's request timeout,
// so progress is streamed as newline-delimited JSON (one object per line, each
// flushed immediately). That (a) keeps reverse-proxy idle timeouts from
// killing the connection and (b) lets the UI show live progress. The final
// line carries "done": true with the overall summary.
func (h *Handler) convertBatchCovers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Format string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	format := normalizeImageFormat(req.Format)
	if !isSupportedImageFormat(format) {
		jsonErr(w, 400, "unsupported format")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Streaming unsupported by the ResponseWriter; fall back to a single
		// JSON response (legacy behavior, requires the client to wait).
		coverMu.Lock()
		res, lerr := h.runBatchConvert(r, format, func(map[string]interface{}) {})
		coverMu.Unlock()
		if lerr != nil {
			jsonErr(w, 500, lerr.Error())
			return
		}
		jsonResp(w, 200, map[string]interface{}{
			"converted":     res.Converted,
			"skipped":       res.Skipped,
			"freed_bytes":   res.Freed,
			"target_format": format,
		})
		return
	}

	// Establish the streaming response up front so the client and any
	// reverse proxy see activity immediately, then flush after every line.
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx proxy buffering
	w.WriteHeader(200)
	flusher.Flush()

	coverMu.Lock()
	defer coverMu.Unlock()

	res, lerr := h.runBatchConvert(r, format, func(v map[string]interface{}) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "%s\n", b)
		flusher.Flush()
	})
	if lerr != nil {
		b, _ := json.Marshal(map[string]interface{}{"error": lerr.Error(), "done": true})
		fmt.Fprintf(w, "%s\n", b)
		flusher.Flush()
		return
	}
	b, _ := json.Marshal(map[string]interface{}{
		"done":          true,
		"converted":     res.Converted,
		"skipped":       res.Skipped,
		"freed_bytes":   res.Freed,
		"total":         res.Total,
		"target_format": format,
	})
	fmt.Fprintf(w, "%s\n", b)
	flusher.Flush()
}

// batchConvertResult aggregates the outcome of a batch conversion run.
type batchConvertResult struct {
	Converted int
	Skipped   int
	Freed     int64
	Total     int
}

// runBatchConvert performs the batch re-encode, invoking emit (if non-nil)
// with a progress object after each cover and at the start. It stops early if
// the client disconnects (r.Context() cancelled). The caller must hold
// coverMu. The emit callback is invoked synchronously on the serving goroutine.
func (h *Handler) runBatchConvert(r *http.Request, format string, emit func(map[string]interface{})) (batchConvertResult, error) {
	keys, err := h.storage.ListCoverKeys()
	if err != nil {
		return batchConvertResult{}, err
	}
	total := len(keys)
	if emit != nil {
		emit(map[string]interface{}{"phase": "start", "total": total, "format": format})
	}

	res := batchConvertResult{Total: total}
	for i, k := range keys {
		// Bail out if the client went away; no point burning CPU.
		if r.Context().Err() != nil {
			break
		}
		status, perr := h.convertOneCover(k, format, &res)
		if emit != nil {
			line := map[string]interface{}{
				"phase":       "item",
				"index":       i,
				"total":       total,
				"key":         k,
				"status":      status,
				"converted":   res.Converted,
				"skipped":     res.Skipped,
				"freed_bytes": res.Freed,
			}
			if perr != nil {
				line["error"] = perr.Error()
			}
			emit(line)
		}
	}
	return res, nil
}

// convertOneCover re-encodes a single cover to the target format, repointing
// database references and refreshing thumbnails as needed. It recovers from
// panics so one corrupt file can never crash the whole batch (and the server
// with it). status is one of "converted", "skipped", "error".
func (h *Handler) convertOneCover(k, format string, res *batchConvertResult) (status string, err error) {
	defer func() {
		if p := recover(); p != nil {
			status = "error"
			err = fmt.Errorf("panic converting cover %s: %v", k, p)
			slog.Error("cover conversion panic", "key", k, "panic", p)
		}
	}()

	data, rerr := h.storage.ReadCover(k)
	if rerr != nil {
		return "error", fmt.Errorf("read %s: %w", k, rerr)
	}
	// Skip by actual encoded format, not the file extension, so a real
	// AVIF saved under a wrong extension is not pointlessly re-encoded,
	// and a misnamed non-AVIF file is still converted.
	if storage.DetectImageFormat(data) == format {
		res.Skipped++
		return "skipped", nil
	}
	newKey, _, cerr := h.storage.ConvertCover(k, format)
	if cerr != nil {
		return "error", fmt.Errorf("convert %s: %w", k, cerr)
	}
	if newKey != k {
		if err := h.db.RepointCoverRefs(k, newKey); err == nil {
			h.regenerateThumbsForCover(k, newKey, format)
			if cnt, _ := h.db.CountCoverRefs(k); cnt == 0 {
				if sz, ok := h.db.CoverSize(k); ok {
					res.Freed += sz
					h.db.DeleteCoverMeta(k)
				}
			}
		}
	}
	res.Converted++
	return "converted", nil
}

// regenerateThumbsForCover finds records referencing either the old or new key
// and refreshes their cover_thumb from the current cover file.
func (h *Handler) regenerateThumbsForCover(oldKey, newKey string, format string) {
	for _, k := range []string{oldKey, newKey} {
		data, err := h.storage.ReadCover(k)
		if err != nil {
			continue
		}
		thumbKey, err := h.storage.MakeThumbnail(k, data, 400, format)
		if err != nil {
			continue
		}
		recs, err := h.db.GetRecordsByCoverFile(k)
		if err != nil {
			continue
		}
		for _, rec := range recs {
			_ = h.db.SetRecordThumb(rec.ID, thumbKey)
		}
	}
}

func isSupportedImageFormat(f string) bool {
	switch f {
	case "avif", "webp", "jpeg":
		return true
	}
	return false
}

func normalizeImageFormat(f string) string {
	f = strings.ToLower(strings.TrimSpace(f))
	switch f {
	case "jpg":
		return "jpeg"
	case "png":
		return "jpeg"
	case "":
		return "avif"
	}
	return f
}
