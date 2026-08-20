package handlers

import (
	"encoding/json"
	"mujian/internal/models"
	"mujian/internal/storage"
	"net/http"
	"path/filepath"
	"strconv"
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
		"merged_groups": mergedGroups,
		"updated_records": updated,
		"freed_bytes":   freed,
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
		if !req.All {
			matched := false
			for _, n := range req.Files {
				if n == name {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
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
func (h *Handler) regenerateThumbs(w http.ResponseWriter, r *http.Request) {
	coverMu.Lock()
	defer coverMu.Unlock()

	list, err := h.db.ListCoverFiles()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	done := 0
	for _, item := range list {
		data, err := h.storage.ReadCover(item.CoverFile)
		if err != nil {
			continue
		}
		thumb, err := storage.ThumbBase64FromBytes(data, 400)
		if err != nil {
			continue
		}
		if err := h.db.SetRecordThumb(item.ID, thumb); err == nil {
			done++
		}
	}
	jsonResp(w, 200, map[string]interface{}{"updated": done})
}
