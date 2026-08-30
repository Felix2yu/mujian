package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mujian/internal/backup"
	"mujian/internal/config"
	"mujian/internal/db"
	"mujian/internal/ics"
	"mujian/internal/models"
	"mujian/internal/storage"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db      *db.DB
	cfg     *config.Config
	storage storage.Storage
	backup  *backup.Manager
	// TTL caches for the read-only aggregate endpoints; invalidated by
	// statsInvalidationMiddleware after every successful mutation.
	statsCache     *statsCache[*models.Stats]
	dashboardCache *statsCache[*models.DashboardStats]
	analyticsCache *statsCache[*models.AnalyticsData]
}

func New(database *db.DB, cfg *config.Config, st storage.Storage, bm *backup.Manager) *Handler {
	h := &Handler{
		db:             database,
		cfg:            cfg,
		storage:        st,
		backup:         bm,
		statsCache:     newStatsCache[*models.Stats](5 * time.Second),
		dashboardCache: newStatsCache[*models.DashboardStats](5 * time.Second),
		analyticsCache: newStatsCache[*models.AnalyticsData](5 * time.Second),
	}
	// 自动备份的 JSON/ZIP 两种格式复用导出端点的构建逻辑；数据库快照格式
	// 由 backup.Manager 自己执行 VACUUM INTO，无需导出器。
	if bm != nil {
		bm.SetExporter(h.exportDataJSON, h.exportZipBytes)
	}
	return h
}

// invalidateStats drops all aggregate caches (called after mutations).
func (h *Handler) invalidateStats() {
	h.statsCache.invalidate()
	h.dashboardCache.invalidate()
	h.analyticsCache.invalidate()
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	// Clear the aggregate caches after any successful mutating request.
	r.Use(statsInvalidationMiddleware(h.invalidateStats))
	// Cap request bodies so an unauthenticated client cannot force the server
	// to buffer unbounded JSON. Multipart endpoints get a larger allowance.
	r.Use(bodyLimitMiddleware)

	r.Get("/records", h.listRecords)
	r.Get("/records/all", h.listAllRecords)
	r.Get("/records/search", h.searchRecords)
	r.Post("/records", h.createRecord)
	r.Post("/records/import", h.importRecords)
	r.Post("/records/align-venues", h.alignVenues)
	r.Post("/records/batch", h.batchUpdate)
	r.Post("/records/batch/delete", h.batchDelete)
	r.Get("/records/{id}", h.getRecord)
	r.Put("/records/{id}", h.updateRecord)
	r.Delete("/records/{id}", h.deleteRecord)

	// 票根/现场照（多图）
	r.Get("/records/{id}/photos", h.listRecordPhotos)
	r.Post("/records/{id}/photos", h.addRecordPhoto)
	r.Delete("/records/{id}/photos/{pid}", h.deleteRecordPhoto)
	r.Post("/records/{id}/photos/reorder", h.reorderRecordPhotos)

	// 回收站（软删除，30 天后自动清理）
	r.Get("/records/deleted", h.listDeletedRecords)
	r.Post("/records/trash/purge", h.purgeRecordsTrash)
	r.Post("/records/{id}/restore", h.restoreRecord)
	r.Delete("/records/{id}/purge", h.purgeRecord)

	r.Get("/categories", h.listCategories)
	r.Post("/categories", h.createCategory)
	r.Post("/categories/reorder", h.reorderCategories)
	r.Put("/categories/{id}", h.updateCategory)
	r.Delete("/categories/{id}", h.deleteCategory)

	r.Get("/dramas", h.listDramas)
	r.Get("/dramas/tree", h.listDramaTree)
	r.Post("/dramas", h.createDrama)
	r.Get("/dramas/{id}", h.getDramaDetail)
	r.Put("/dramas/{id}", h.updateDrama)
	r.Delete("/dramas/{id}", h.deleteDrama)
	r.Post("/dramas/reorder", h.reorderDramas)
	r.Post("/dramas/{id}/zhezis", h.createZhezi)
	r.Post("/dramas/{id}/zhezis/reorder", h.reorderZhezis)
	r.Put("/zhezis/{id}", h.updateZhezi)
	r.Delete("/zhezis/{id}", h.deleteZhezi)

	r.Get("/artists", h.listArtists)
	r.Post("/artists", h.createArtist)
	r.Get("/artists/{id}", h.getArtistDetail)
	r.Put("/artists/{id}", h.updateArtist)
	r.Delete("/artists/{id}", h.deleteArtist)
	r.Post("/artists/reorder", h.reorderArtists)
	r.Get("/artists/tree", h.listArtistTree)

	r.Get("/stats", h.getStats)
	r.Get("/dashboard", h.getDashboard)
	r.Get("/analytics", h.getAnalytics)
	r.Get("/calendar", h.getCalendar)
	r.Get("/calendar.ics", h.getICS)
	r.Get("/map/points", h.getMapPoints)

	r.Route("/covers", func(r chi.Router) {
		r.Get("/", h.listCovers)
		r.Get("/duplicates", h.getCoverDuplicates)
		r.Post("/merge", h.mergeCovers)
		r.Get("/orphans", h.getCoverOrphans)
		r.Post("/cleanup", h.cleanupCovers)
		r.Post("/trash/purge", h.purgeTrash)
		r.Post("/thumbs", h.regenerateThumbs)
		r.Post("/convert", h.convertCover)
		r.Post("/convert-batch", h.convertBatchCovers)
	})

	r.Get("/autocomplete/{field}", h.getAutocomplete)
	r.Get("/field/{field}/{value}", h.getByField)

	r.Get("/settings", h.getSettings)
	r.Put("/settings", h.updateSettings)
	r.Post("/settings/test-s3", h.testS3Connection)

	r.Post("/upload", h.uploadFile)
	r.Get("/export", h.exportRecords)
	r.Post("/backup/restore", h.backupRestore)
	r.Post("/backup/run", h.runBackupNow)
	r.Get("/backup/list", h.listBackups)
	r.Get("/backup/download", h.downloadBackup)
	r.Post("/backup/restore-from", h.restoreFromBackup)
	r.Delete("/backup", h.deleteBackup)
	r.Post("/storage/migrate-to-s3", h.migrateToS3)

	return r
}

// Request-body caps. JSON endpoints get a small allowance; the multipart
// import/restore endpoints legitimately carry full export archives.
const (
	defaultBodyLimit = 4 << 20   // 4MB for JSON payloads
	importBodyLimit  = 640 << 20 // import/restore archives (original exports can be hundreds of MB)
	uploadBodyLimit  = 40 << 20  // single cover upload (8MB file + multipart overhead)
)

// bodyLimitMiddleware wraps r.Body with http.MaxBytesReader using a
// per-endpoint limit. The endpoints below are matched by suffix because this
// router is mounted under /api.
func bodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(defaultBodyLimit)
		switch {
		case strings.HasSuffix(r.URL.Path, "/records/import"),
			strings.HasSuffix(r.URL.Path, "/backup/restore"):
			limit = importBodyLimit
		case strings.HasSuffix(r.URL.Path, "/upload"):
			limit = uploadBodyLimit
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}

func jsonResp(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, status int, msg string) {
	jsonResp(w, status, map[string]string{"error": msg})
}

// normalizeRecord fills derived fields (id, date<->dateText) before persist.
func normalizeRecord(r *models.RecordRequest) {
	if r.Date == 0 && r.DateText != "" {
		for _, f := range []string{"2006-01-02 15:04", "2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02"} {
			if t, err := time.ParseInLocation(f, r.DateText, time.Local); err == nil {
				r.Date = t.Unix()
				break
			}
		}
	}
	if r.Date != 0 && r.DateText == "" {
		r.DateText = time.Unix(r.Date, 0).Format("2006-01-02 15:04")
	}
	if r.PriceCurrency == "" {
		r.PriceCurrency = "CNY"
	}
	if r.PayPriceCurrency == "" {
		r.PayPriceCurrency = "CNY"
	}
	if r.OtherCostCurrency == "" {
		r.OtherCostCurrency = "CNY"
	}
}

func (h *Handler) listRecords(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := db.RecordFilter{}
	f.Query = q.Get("q")
	f.Category = q.Get("category")
	f.City = q.Get("city")
	if y := q.Get("year"); y != "" {
		f.Year, _ = strconv.Atoi(y)
	}
	if m := q.Get("month"); m != "" {
		f.Month, _ = strconv.Atoi(m)
	}
	f.Start = q.Get("start")
	f.End = q.Get("end")
	f.DramaID = q.Get("drama")
	f.ZheziID = q.Get("zhezi")
	f.Missing = q.Get("missing")
	f.Channel = q.Get("channel")
	f.Company = q.Get("company")
	if v := q.Get("rating_min"); v != "" {
		f.RatingMin, _ = strconv.Atoi(v)
	}
	if v := q.Get("price_min"); v != "" {
		f.PriceMin, _ = strconv.ParseFloat(v, 64)
	}
	if v := q.Get("price_max"); v != "" {
		f.PriceMax, _ = strconv.ParseFloat(v, 64)
	}
	if v := q.Get("status"); v != "" {
		f.ActiveStatus, _ = strconv.Atoi(v)
	}
	// active_status=0,2 — multi-select from the client's status preferences.
	// Applied server-side so `total` only counts statuses the user displays.
	if v := q.Get("active_status"); v != "" {
		for _, p := range strings.Split(v, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				f.Statuses = append(f.Statuses, n)
			}
		}
	}
	if q.Get("exact") == "1" || q.Get("exact") == "true" {
		f.Exact = true
	}
	if v := q.Get("limit"); v != "" {
		f.Limit, _ = strconv.Atoi(v)
	}
	if v := q.Get("offset"); v != "" {
		f.Offset, _ = strconv.Atoi(v)
	}

	// total 计数不受 limit/offset 影响，始终返回完整匹配数
	countFilter := f
	countFilter.Limit = 0
	countFilter.Offset = 0
	total, err := h.db.CountRecordsContext(r.Context(), countFilter)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	recs, err := h.db.ListRecordsContext(r.Context(), f)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{
		"records": recs,
		"total":   total,
	})
}

func (h *Handler) listAllRecords(w http.ResponseWriter, r *http.Request) {
	// Explicit "everything" endpoint: exempt from the list row caps.
	recs, err := h.db.ListRecordsContext(r.Context(), db.RecordFilter{NoLimit: true})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, recs)
}

func (h *Handler) searchRecords(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		jsonResp(w, 200, []models.Record{})
		return
	}
	recs, err := h.db.ListRecordsContext(r.Context(), db.RecordFilter{Query: q})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, recs)
}

func (h *Handler) getRecord(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rec, err := h.db.GetRecord(id)
	if err != nil {
		jsonErr(w, 404, "record not found")
		return
	}
	jsonResp(w, 200, rec)
}

func (h *Handler) createRecord(w http.ResponseWriter, r *http.Request) {
	var req models.RecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if req.Name == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	normalizeRecord(&req)

	rec, err := h.db.CreateRecord(req)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 201, rec)
}

func (h *Handler) updateRecord(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req models.RecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	normalizeRecord(&req)

	rec, err := h.db.UpdateRecord(id, req)
	if err != nil {
		jsonErr(w, 404, "record not found")
		return
	}
	jsonResp(w, 200, rec)
}

func (h *Handler) deleteRecord(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.db.DeleteRecord(id); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "deleted"})
}

// GET /api/records/{id}/photos — 票根/现场照列表。
func (h *Handler) listRecordPhotos(w http.ResponseWriter, r *http.Request) {
	photos, err := h.db.ListRecordPhotos(chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{"photos": photos})
}

// POST /api/records/{id}/photos {"key": ...} — 关联一张已上传的图片。
func (h *Handler) addRecordPhoto(w http.ResponseWriter, r *http.Request) {
	recordID := chi.URLParam(r, "id")
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		jsonErr(w, 400, "key is required")
		return
	}
	if !h.storage.CoverExists(req.Key) {
		jsonErr(w, 400, "unknown image key: "+req.Key)
		return
	}
	photo, err := h.db.AddRecordPhoto(recordID, req.Key)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 201, photo)
}

// DELETE /api/records/{id}/photos/{pid} — 移除一张照片的关联
// （图片本体内容寻址，留待封面清理统一回收）。
func (h *Handler) deleteRecordPhoto(w http.ResponseWriter, r *http.Request) {
	if err := h.db.DeleteRecordPhoto(chi.URLParam(r, "id"), chi.URLParam(r, "pid")); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "deleted"})
}

// POST /api/records/{id}/photos/reorder {"ids": [...]} — 照片排序。
func (h *Handler) reorderRecordPhotos(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if err := h.db.ReorderRecordPhotos(chi.URLParam(r, "id"), req.IDs); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "reordered"})
}

// GET /api/records/deleted — 回收站列表。
func (h *Handler) listDeletedRecords(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	recs, err := h.db.ListDeletedRecords(limit, offset)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	total, _ := h.db.DeletedCount()
	jsonResp(w, 200, map[string]interface{}{"records": recs, "total": total})
}

// POST /api/records/{id}/restore — 从回收站恢复。
func (h *Handler) restoreRecord(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.db.RestoreRecord(id); err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	rec, err := h.db.GetRecord(id)
	if err != nil {
		jsonErr(w, 404, "record not found")
		return
	}
	jsonResp(w, 200, rec)
}

// DELETE /api/records/{id}/purge — 彻底删除（不可恢复）。
func (h *Handler) purgeRecord(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.db.PurgeRecord(id); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "purged"})
}

// POST /api/records/trash/purge — 清空回收站。
func (h *Handler) purgeRecordsTrash(w http.ResponseWriter, r *http.Request) {
	recs, err := h.db.ListDeletedRecords(0, 0)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	purged := 0
	for _, rec := range recs {
		if err := h.db.PurgeRecord(rec.ID); err == nil {
			purged++
		}
	}
	jsonResp(w, 200, map[string]interface{}{"purged": purged})
}

// POST /records/align-venues — 存量对齐：按地址分组，用各组里已有的坐标
// 回填同地址的其他记录，保证「同场馆唯一经纬度」。
func (h *Handler) alignVenues(w http.ResponseWriter, r *http.Request) {
	res, err := h.db.AlignVenueCoordinates()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, res)
}

func (h *Handler) batchUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`

		// 每个字段为 nil 表示不修改；非 nil 则按指定操作更新
		Name          *string              `json:"name,omitempty"`
		CategoryName  *string              `json:"category_name,omitempty"`
		CategoryNames *models.BatchArrayOp `json:"category_names,omitempty"`
		Rating        *int                 `json:"rating,omitempty"`
		ActiveStatus  *int                 `json:"active_status,omitempty"`
		City          *string              `json:"city,omitempty"`
		Address       *string              `json:"address,omitempty"`
		Channel       *string              `json:"channel,omitempty"`
		Company       *string              `json:"company,omitempty"`
		Friends       *string              `json:"friends,omitempty"`
		Remark        *string              `json:"remark,omitempty"`
		Seat          *string              `json:"seat,omitempty"`
		DateText      *string              `json:"date_text,omitempty"`
		Coordinate    *models.Coordinate   `json:"coordinate,omitempty"`

		Price             *float64 `json:"price,omitempty"`
		PriceCurrency     *string  `json:"price_currency,omitempty"`
		PayPrice          *float64 `json:"pay_price,omitempty"`
		PayPriceCurrency  *string  `json:"pay_price_currency,omitempty"`
		OtherCost         *float64 `json:"other_cost,omitempty"`
		OtherCostCurrency *string  `json:"other_cost_currency,omitempty"`

		// 数组字段支持三种操作：set(替换)、append(追加)、remove(移除)
		DramaIDs    *models.BatchArrayOp `json:"drama_ids,omitempty"`
		ZheziIDs    *models.BatchArrayOp `json:"zhezi_ids,omitempty"`
		Play        *models.BatchArrayOp `json:"play,omitempty"`
		Guest       *models.BatchArrayOp `json:"guest,omitempty"`
		ArtistNames *models.BatchArrayOp `json:"artist_names,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		jsonErr(w, 400, "no ids provided")
		return
	}
	updated, err := h.db.BatchUpdateRecords(models.BatchUpdateParams{
		IDs:               req.IDs,
		Name:              req.Name,
		CategoryName:      req.CategoryName,
		CategoryNames:     req.CategoryNames,
		Rating:            req.Rating,
		ActiveStatus:      req.ActiveStatus,
		City:              req.City,
		Address:           req.Address,
		Channel:           req.Channel,
		Company:           req.Company,
		Friends:           req.Friends,
		Remark:            req.Remark,
		Seat:              req.Seat,
		DateText:          req.DateText,
		Coordinate:        req.Coordinate,
		Price:             req.Price,
		PriceCurrency:     req.PriceCurrency,
		PayPrice:          req.PayPrice,
		PayPriceCurrency:  req.PayPriceCurrency,
		OtherCost:         req.OtherCost,
		OtherCostCurrency: req.OtherCostCurrency,
		DramaIDs:          req.DramaIDs,
		ZheziIDs:          req.ZheziIDs,
		Play:              req.Play,
		Guest:             req.Guest,
		ArtistNames:       req.ArtistNames,
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{"updated": updated})
}

func (h *Handler) batchDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		jsonErr(w, 400, "no ids provided")
		return
	}
	deleted, err := h.db.BatchDeleteRecords(req.IDs)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{"deleted": deleted})
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.db.ListCategories()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, cats)
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		ActiveIDs   []string `json:"activeIds"`
		RecordCount int      `json:"recordCount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if req.Name == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	cat := models.Category{ID: req.ID, Name: req.Name, ActiveIDs: req.ActiveIDs, RecordCount: req.RecordCount}
	if cat.ActiveIDs == nil {
		cat.ActiveIDs = []string{}
	}
	if err := h.db.UpsertCategory(&cat); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 201, cat)
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name        string   `json:"name"`
		ActiveIDs   []string `json:"activeIds"`
		RecordCount int      `json:"recordCount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	cat := models.Category{ID: id, Name: req.Name, ActiveIDs: req.ActiveIDs, RecordCount: req.RecordCount}
	if cat.ActiveIDs == nil {
		cat.ActiveIDs = []string{}
	}
	if err := h.db.UpsertCategory(&cat); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, cat)
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.db.DeleteCategory(id); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "deleted"})
}

// ---------- Dramas & Zhezis ----------

type dramaReq struct {
	Name          string   `json:"name"`
	Aliases       []string `json:"aliases"`
	CategoryName  string   `json:"categoryName"`
	CategoryNames []string `json:"categoryNames"`
	Remark        string   `json:"remark"`
}

type zheziReq struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Remark  string   `json:"remark"`
}

func (h *Handler) listDramaTree(w http.ResponseWriter, r *http.Request) {
	tree, err := h.db.ListDramaTree()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, tree)
}

func (h *Handler) listDramas(w http.ResponseWriter, r *http.Request) {
	dramas, err := h.db.ListDramas()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, dramas)
}

func (h *Handler) createDrama(w http.ResponseWriter, r *http.Request) {
	var req dramaReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	d, err := h.db.SaveDrama(models.Drama{Name: strings.TrimSpace(req.Name), Aliases: req.Aliases, CategoryNames: req.CategoryNames, Remark: req.Remark})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 201, d)
}

func (h *Handler) getDramaDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	d, err := h.db.GetDramaDetail(id)
	if err != nil {
		jsonErr(w, 404, "drama not found")
		return
	}
	jsonResp(w, 200, d)
}

func (h *Handler) updateDrama(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req dramaReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	d, err := h.db.SaveDrama(models.Drama{ID: id, Name: strings.TrimSpace(req.Name), Aliases: req.Aliases, CategoryNames: req.CategoryNames, Remark: req.Remark})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, d)
}

func (h *Handler) deleteDrama(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.db.DeleteDrama(id); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "deleted"})
}

func (h *Handler) createZhezi(w http.ResponseWriter, r *http.Request) {
	dramaID := chi.URLParam(r, "id")
	var req zheziReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	z, err := h.db.CreateZhezi(models.Zhezi{DramaID: dramaID, Name: strings.TrimSpace(req.Name), Aliases: req.Aliases, Remark: req.Remark})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 201, z)
}

func (h *Handler) updateZhezi(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req zheziReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	z, err := h.db.UpdateZhezi(models.Zhezi{ID: id, Name: strings.TrimSpace(req.Name), Aliases: req.Aliases, Remark: req.Remark})
	if err != nil {
		jsonErr(w, 404, "zhezi not found")
		return
	}
	jsonResp(w, 200, z)
}

func (h *Handler) deleteZhezi(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.db.DeleteZhezi(id); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "deleted"})
}

func (h *Handler) reorderZhezis(w http.ResponseWriter, r *http.Request) {
	dramaID := chi.URLParam(r, "id")
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if err := h.db.ReorderZhezis(dramaID, req.IDs); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "reordered"})
}

// POST /dramas/reorder {"ids":[...]} — manual ordering of dramas (first = top).
func (h *Handler) reorderDramas(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if err := h.db.ReorderDramas(req.IDs); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "reordered"})
}

// ---------- Artists (演员) ----------

func (h *Handler) listArtists(w http.ResponseWriter, r *http.Request) {
	artists, err := h.db.ListArtists()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, artists)
}

func (h *Handler) createArtist(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string   `json:"name"`
		Aliases    []string `json:"aliases"`
		Remark     string   `json:"remark"`
		Cover      string   `json:"cover"`
		CoverFile  string   `json:"coverFile"`
		CoverThumb string   `json:"coverThumb"`
		Bio        string   `json:"bio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	a, err := h.db.SaveArtist(models.Artist{
		Name:       strings.TrimSpace(req.Name),
		Aliases:    req.Aliases,
		Remark:     req.Remark,
		Cover:      req.Cover,
		CoverFile:  req.CoverFile,
		CoverThumb: req.CoverThumb,
		Bio:        req.Bio,
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 201, a)
}

func (h *Handler) getArtistDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := h.db.GetArtistDetail(id)
	if err != nil {
		jsonErr(w, 404, "artist not found")
		return
	}
	jsonResp(w, 200, a)
}

func (h *Handler) updateArtist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		Name       string   `json:"name"`
		Aliases    []string `json:"aliases"`
		Remark     string   `json:"remark"`
		Cover      string   `json:"cover"`
		CoverFile  string   `json:"coverFile"`
		CoverThumb string   `json:"coverThumb"`
		Bio        string   `json:"bio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	a, err := h.db.SaveArtist(models.Artist{
		ID:         id,
		Name:       strings.TrimSpace(req.Name),
		Aliases:    req.Aliases,
		Remark:     req.Remark,
		Cover:      req.Cover,
		CoverFile:  req.CoverFile,
		CoverThumb: req.CoverThumb,
		Bio:        req.Bio,
	})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, a)
}

func (h *Handler) deleteArtist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.db.DeleteArtist(id); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "deleted"})
}

// POST /artists/reorder {"ids":[...]} — manual ordering of artists (first = top).
func (h *Handler) reorderArtists(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if err := h.db.ReorderArtists(req.IDs); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "reordered"})
}

// GET /artists/tree — lightweight id+name pairs for the record-form picker.
func (h *Handler) listArtistTree(w http.ResponseWriter, r *http.Request) {
	tree, err := h.db.ListArtistTree()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, tree)
}

// POST /categories/reorder {"ids":[...]} — manual ordering of categories.
func (h *Handler) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if err := h.db.ReorderCategories(req.IDs); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "reordered"})
}

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	if s, ok := h.statsCache.get(); ok {
		jsonResp(w, 200, s)
		return
	}
	stats, err := h.db.GetStats()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	h.statsCache.set(stats)
	jsonResp(w, 200, stats)
}

func (h *Handler) getDashboard(w http.ResponseWriter, r *http.Request) {
	if s, ok := h.dashboardCache.get(); ok {
		jsonResp(w, 200, s)
		return
	}
	stats, err := h.db.GetDashboardStats()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	h.dashboardCache.set(stats)
	jsonResp(w, 200, stats)
}

func (h *Handler) getAnalytics(w http.ResponseWriter, r *http.Request) {
	if d, ok := h.analyticsCache.get(); ok {
		jsonResp(w, 200, d)
		return
	}
	data, err := h.db.GetAnalytics()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	h.analyticsCache.set(data)
	jsonResp(w, 200, data)
}

// getMapPoints serves the lightweight payload for the map page: only records
// that carry coordinates, and only the fields the map renders. The generic
// /api/records response is ~12x larger for the same data (720KB vs ~60KB),
// most of it remark/artist JSON the map never reads.
func (h *Handler) getMapPoints(w http.ResponseWriter, r *http.Request) {
	pts, err := h.db.ListMapPoints()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, pts)
}

func (h *Handler) getCalendar(w http.ResponseWriter, r *http.Request) {
	year := time.Now().Year()
	month := int(time.Now().Month())
	// 解析失败或越界时保留「当前月」默认值：
	// - Atoi 失败得到 0 会查询公元元年，表现为莫名空日历；
	// - month=0/13 会被 time.Date 规范化到相邻年份，静默错位一个月。
	if y := r.URL.Query().Get("year"); y != "" {
		if v, err := strconv.Atoi(y); err == nil {
			year = v
		}
	}
	if m := r.URL.Query().Get("month"); m != "" {
		if v, err := strconv.Atoi(m); err == nil && v >= 1 && v <= 12 {
			month = v
		}
	}
	events, err := h.db.GetCalendarEvents(year, month)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, events)
}

func (h *Handler) getICS(w http.ResponseWriter, r *http.Request) {
	// Calendar subscription must contain every record: bypass the row caps.
	recs, err := h.db.ListRecordsContext(r.Context(), db.RecordFilter{NoLimit: true})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	// 默认 inline 以便日历客户端直接订阅；?dl=1 时作为文件下载。
	if r.URL.Query().Get("dl") == "1" {
		w.Header().Set("Content-Disposition", "attachment; filename=mujian.ics")
	} else {
		w.Header().Set("Content-Disposition", "inline; filename=mujian.ics")
	}
	w.Write([]byte(ics.GenerateCalendar(recs, h.cfg.Location())))
}

func (h *Handler) getAutocomplete(w http.ResponseWriter, r *http.Request) {
	field := chi.URLParam(r, "field")
	values, err := h.db.GetAutocomplete(field)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	jsonResp(w, 200, values)
}

func (h *Handler) getByField(w http.ResponseWriter, r *http.Request) {
	field := chi.URLParam(r, "field")
	value := chi.URLParam(r, "value")
	recs, err := h.db.GetByField(field, value)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	jsonResp(w, 200, recs)
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	resp := h.cfg.GetSettingsResponse()
	lastRun, lastErr := h.backup.Status()
	resp["last_backup_at"] = lastRun
	resp["backup_last_error"] = lastErr
	jsonResp(w, 200, resp)
}

// POST /api/backup/run — 手动触发一次备份，返回快照文件名。
func (h *Handler) runBackupNow(w http.ResponseWriter, r *http.Request) {
	name, err := h.backup.RunNow()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{"file": name})
}

// GET /api/backup/list — 备份快照清单（新→旧）。
func (h *Handler) listBackups(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, 200, map[string]interface{}{"backups": h.backup.List()})
}

// GET /api/backup/download?file= — 下载一份快照。
func (h *Handler) downloadBackup(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("file")
	data, err := h.backup.Read(name)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", name))
	w.Write(data)
}

// DELETE /api/backup?file= — 删除一份快照。
func (h *Handler) deleteBackup(w http.ResponseWriter, r *http.Request) {
	if err := h.backup.Delete(r.URL.Query().Get("file")); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "deleted"})
}

// POST /api/backup/restore-from {"file": ...} — 从已有快照恢复。
// json/zip 走导入通道在线恢复；.db 快照无法热恢复，提示停机换文件。
func (h *Handler) restoreFromBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File string `json:"file"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if err := backup.ValidateName(req.File); err != nil {
		jsonErr(w, 400, err.Error())
		return
	}

	switch filepath.Ext(req.File) {
	case ".db":
		jsonErr(w, 400, ".db 快照无法在服务运行时恢复：请下线服务后用该文件替换数据库文件，或改用 json/zip 快照在线恢复")
		return
	case ".json", ".zip":
	default:
		jsonErr(w, 400, "unsupported backup file")
		return
	}

	data, err := h.backup.Read(req.File)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	h.withImportLock(w, func() {
		if filepath.Ext(req.File) == ".zip" {
			zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				jsonErr(w, 400, "invalid zip archive: "+err.Error())
				return
			}
			h.importZipArchive(w, zr)
			return
		}
		var export models.ExportData
		if err := json.Unmarshal(data, &export); err != nil {
			jsonErr(w, 400, "invalid export file: "+err.Error())
			return
		}
		result, err := h.db.ImportData(&export)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonResp(w, 200, map[string]interface{}{
			"message":    "restore completed",
			"records":    result.Records,
			"categories": result.Categories,
		})
	})
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req config.SettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	h.cfg.Update(&req)
	h.cfg.SaveToFile(filepath.Join(h.cfg.DBPath, "..", "settings.json"))
	// 备份间隔可能变了：让调度器重算下次运行时间
	h.backup.Reschedule()
	jsonResp(w, 200, h.cfg.GetSettingsResponse())
}

// effectiveS3Settings merges the S3 fields submitted by the client with the
// currently-saved config. The merge rules differ per field kind:
//   - Endpoint/Bucket/Region/PublicURL: a non-empty body value overrides saved;
//     an empty body value means "not provided" and falls back to saved.
//   - AccessKey/SecretKey: a non-empty, non-masked body value overrides saved
//     (the user typed a new key); an empty body value falls back to saved; a
//     masked value (suffix "****", echoed from GET) is ignored so the real saved
//     secret is kept. An empty body field must NOT inherit the saved secret —
//     otherwise a user who never configured creds and tests before typing the
//     key would see a false positive.
func effectiveS3Settings(saved, req config.S3Settings) config.S3Settings {
	out := saved
	if req.Endpoint != "" {
		out.Endpoint = req.Endpoint
	}
	if req.Bucket != "" {
		out.Bucket = req.Bucket
	}
	if req.Region != "" {
		out.Region = req.Region
	}
	if req.PublicURL != "" {
		out.PublicURL = req.PublicURL
	}
	if req.AccessKey != "" && !strings.HasSuffix(req.AccessKey, "****") {
		out.AccessKey = req.AccessKey
	}
	// Secret: empty body => no override (so a never-saved secret stays empty and
	// the test correctly reports "incomplete"). Masked => keep saved. Plain => use.
	if req.SecretKey != "" && !strings.HasSuffix(req.SecretKey, "****") {
		out.SecretKey = req.SecretKey
	}
	return out
}

// POST /api/settings/test-s3 — probe the (merged) S3 config with a real
// put+delete of a tiny marker object. Returns {ok:bool, error?:string}.
// Intentionally does NOT persist anything.
//
// The body uses the same s3_* field names as PUT /api/settings (config.S3Settings
// has no JSON tags, so it cannot be decoded directly from that payload). Masked
// values (suffix "****") and empties fall back to the saved config.
type s3TestRequest struct {
	Endpoint  string `json:"s3_endpoint"`
	Bucket    string `json:"s3_bucket"`
	Region    string `json:"s3_region"`
	AccessKey string `json:"s3_access_key"`
	SecretKey string `json:"s3_secret_key"`
	PublicURL string `json:"s3_public_url"`
}

func (h *Handler) testS3Connection(w http.ResponseWriter, r *http.Request) {
	var req s3TestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	eff := effectiveS3Settings(h.cfg.GetS3Settings(), config.S3Settings{
		Endpoint:  req.Endpoint,
		Bucket:    req.Bucket,
		Region:    req.Region,
		AccessKey: req.AccessKey,
		SecretKey: req.SecretKey,
		PublicURL: req.PublicURL,
	})
	if eff.Bucket == "" || eff.AccessKey == "" || eff.SecretKey == "" {
		jsonResp(w, 200, map[string]interface{}{
			"ok":    false,
			"error": "S3 未配置完整：需要 Bucket、Access Key、Secret Key（以及 Endpoint 或 Region）",
		})
		return
	}
	st := storage.NewS3StorageFromSettings(eff, h.cfg.GetImageFormat)
	if err := st.TestConnection(r.Context()); err != nil {
		jsonResp(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, 200, map[string]interface{}{"ok": true})
}

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	storageType, allowLocal := h.cfg.GetStorageMode()
	if !allowLocal && storageType != "s3" {
		jsonErr(w, 403, "local storage is disabled")
		return
	}
	r.ParseMultipartForm(32 << 20)
	_, header, err := r.FormFile("file")
	if err != nil {
		jsonErr(w, 400, "no file provided")
		return
	}

	if header.Size > 8<<20 {
		jsonErr(w, 400, "file too large, max 8MB")
		return
	}

	key, thumb, created, err := h.storage.SaveUpload(header)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{
		"key":     key,
		"thumb":   thumb,
		"created": created,
	})
}

func (h *Handler) exportRecords(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("format") == "zip" {
		h.exportZIP(w, r)
		return
	}
	b, err := h.exportDataJSON()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	filename := fmt.Sprintf("mujian_export_%s.json", time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write(b)
}

// exportDataJSON renders the full export payload (records + categories +
// meta) as indented JSON. Shared by the download endpoint and the auto-backup
// exporter.
func (h *Handler) exportDataJSON() ([]byte, error) {
	data, err := h.db.Export()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(data, "", "  ")
}

// exportZIP downloads the converted-format archive: data.json + binary
// covers/ (read from the uploads dir), ready to be re-imported directly.
func (h *Handler) exportZIP(w http.ResponseWriter, r *http.Request) {
	b, err := h.exportZipBytes()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	filename := fmt.Sprintf("mujian_export_%s.zip", time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write(b)
}

// exportZipBytes builds the data.json + covers/ archive in memory. Shared by
// the download endpoint and the auto-backup exporter.
func (h *Handler) exportZipBytes() ([]byte, error) {
	data, err := h.db.Export()
	if err != nil {
		return nil, err
	}
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writeZipEntryBytes(zw, "data.json", jsonBytes); err != nil {
		return nil, fmt.Errorf("failed to write data.json: %w", err)
	}

	uploadRoot := filepath.Clean(h.cfg.UploadDir)
	seen := map[string]bool{}
	covers := 0
	for _, rec := range data.Records {
		if rec.CoverFile == "" {
			continue
		}
		name := filepath.Base(rec.CoverFile)
		if name == "." || seen[name] {
			continue
		}
		path := filepath.Clean(filepath.Join(uploadRoot, rec.CoverFile))
		if path != uploadRoot && !strings.HasPrefix(path, uploadRoot+string(filepath.Separator)) {
			continue // path traversal guard
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := writeZipEntryBytes(zw, "covers/"+name, b); err != nil {
			return nil, fmt.Errorf("failed to write cover: %w", err)
		}
		seen[name] = true
		covers++
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize zip: %w", err)
	}
	return buf.Bytes(), nil
}

func writeZipEntryBytes(zw *zip.Writer, name string, data []byte) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

func (h *Handler) backupRestore(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)
	file, _, err := r.FormFile("file")
	if err != nil {
		jsonErr(w, 400, "no file provided")
		return
	}
	defer file.Close()

	var data models.ExportData
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		jsonErr(w, 400, "invalid export file: "+err.Error())
		return
	}

	h.withImportLock(w, func() {
		result, err := h.db.ImportData(&data)
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		jsonResp(w, 200, map[string]interface{}{
			"message":    "restore completed",
			"records":    result.Records,
			"categories": result.Categories,
		})
	})
}
