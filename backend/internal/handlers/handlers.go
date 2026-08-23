package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
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
}

func New(database *db.DB, cfg *config.Config, st storage.Storage) *Handler {
	return &Handler{db: database, cfg: cfg, storage: st}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

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
	r.Get("/calendar", h.getCalendar)
	r.Get("/calendar.ics", h.getICS)

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

	r.Post("/upload", h.uploadFile)
	r.Get("/export", h.exportRecords)
	r.Post("/backup/restore", h.backupRestore)

	return r
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

	if f.Query != "" && f.Category == "" && f.City == "" && f.Year == 0 && f.Start == "" && f.End == "" {
		// pure search uses the dedicated search path but list supports it too
	}

	recs, err := h.db.ListRecords(f)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, recs)
}

func (h *Handler) listAllRecords(w http.ResponseWriter, r *http.Request) {
	recs, err := h.db.ListRecords(db.RecordFilter{})
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
	recs, err := h.db.ListRecords(db.RecordFilter{Query: q})
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
		CategoryName *string `json:"category_name,omitempty"`
		CategoryNames *models.BatchArrayOp `json:"category_names,omitempty"`
		Rating       *int    `json:"rating,omitempty"`
		ActiveStatus *int    `json:"active_status,omitempty"`
		City         *string `json:"city,omitempty"`
		Address      *string `json:"address,omitempty"`
		Channel      *string `json:"channel,omitempty"`
		Company      *string `json:"company,omitempty"`
		Friends      *string `json:"friends,omitempty"`
		Remark       *string `json:"remark,omitempty"`
		Seat         *string `json:"seat,omitempty"`

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
		TagIDs      *models.BatchArrayOp `json:"tag_ids,omitempty"`
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
		TagIDs:            req.TagIDs,
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
	d, err := h.db.SaveDrama(models.Drama{Name: strings.TrimSpace(req.Name), CategoryName: req.CategoryName, CategoryNames: req.CategoryNames, Remark: req.Remark})
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
	d, err := h.db.SaveDrama(models.Drama{ID: id, Name: strings.TrimSpace(req.Name), CategoryName: req.CategoryName, CategoryNames: req.CategoryNames, Remark: req.Remark})
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
	stats, err := h.db.GetStats()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, stats)
}

func (h *Handler) getDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.GetDashboardStats()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, stats)
}

func (h *Handler) getCalendar(w http.ResponseWriter, r *http.Request) {
	year := time.Now().Year()
	month := int(time.Now().Month())
	if y := r.URL.Query().Get("year"); y != "" {
		year, _ = strconv.Atoi(y)
	}
	if m := r.URL.Query().Get("month"); m != "" {
		month, _ = strconv.Atoi(m)
	}
	events, err := h.db.GetCalendarEvents(year, month)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, events)
}

func (h *Handler) getICS(w http.ResponseWriter, r *http.Request) {
	recs, err := h.db.ListRecords(db.RecordFilter{})
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=mujian.ics")
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
	jsonResp(w, 200, h.cfg.GetSettingsResponse())
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req config.SettingsUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	h.cfg.Update(&req)
	h.cfg.SaveToFile(filepath.Join(h.cfg.DBPath, "..", "settings.json"))
	jsonResp(w, 200, h.cfg.GetSettingsResponse())
}

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.AllowLocalStorage && h.cfg.StorageType != "s3" {
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

	data, err := h.db.Export()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	filename := fmt.Sprintf("mujian_export_%s.json", time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write(b)
}

// exportZIP downloads the converted-format archive: data.json + binary
// covers/ (read from the uploads dir), ready to be re-imported directly.
func (h *Handler) exportZIP(w http.ResponseWriter, r *http.Request) {
	data, err := h.db.Export()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writeZipEntryBytes(zw, "data.json", jsonBytes); err != nil {
		jsonErr(w, 500, "failed to write data.json: "+err.Error())
		return
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
			jsonErr(w, 500, "failed to write cover: "+err.Error())
			return
		}
		seen[name] = true
		covers++
	}
	if err := zw.Close(); err != nil {
		jsonErr(w, 500, "failed to finalize zip: "+err.Error())
		return
	}

	filename := fmt.Sprintf("mujian_export_%s.zip", time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write(buf.Bytes())
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
