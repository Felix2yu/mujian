package handlers

import (
	"encoding/json"
	"fmt"
	"mujian/internal/config"
	"mujian/internal/db"
	"mujian/internal/ics"
	"mujian/internal/models"
	"mujian/internal/storage"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db       *db.DB
	cfg      *config.Config
	storage  storage.Storage
	sessions *db.SessionStore
}

func New(database *db.DB, cfg *config.Config, st storage.Storage) *Handler {
	return &Handler{db: database, cfg: cfg, storage: st, sessions: db.NewSessionStore()}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", h.register)
		r.Post("/login", h.login)
		r.Post("/logout", h.logout)
		r.Group(func(r chi.Router) {
			r.Use(h.AuthMiddleware)
			r.Get("/me", h.me)
			r.Put("/password", h.changePassword)
			r.Delete("/account", h.deleteAccount)
		})
	})

	r.Group(func(r chi.Router) {
		r.Use(h.AuthMiddleware)

		r.Get("/calendar", h.getCalendar)
		r.Get("/calendar.ics", h.getICS)
		r.Get("/stats", h.getStats)
		r.Get("/dashboard", h.getDashboard)

		r.Route("/shows", func(r chi.Router) {
			r.Get("/", h.listShows)
			r.Get("/all", h.listAllShows)
			r.Get("/search", h.searchShows)
			r.Get("/upcoming", h.getUpcoming)
			r.Get("/recent", h.getRecent)
			r.Post("/", h.createShow)
			r.Post("/import", h.importShows)
			r.Post("/batch", h.batchUpdate)
			r.Post("/batch/delete", h.batchDelete)
			r.Get("/{id}", h.getShow)
			r.Put("/{id}", h.updateShow)
			r.Delete("/{id}", h.deleteShow)
		})

		r.Route("/categories", func(r chi.Router) {
			r.Get("/", h.listCategories)
			r.Post("/", h.createCategory)
			r.Put("/{id}", h.updateCategory)
			r.Delete("/{id}", h.deleteCategory)
			r.Patch("/sort", h.updateCategorySort)
		})

		r.Get("/autocomplete/{field}", h.getAutocomplete)
		r.Get("/field/{field}/{value}", h.getByField)

		r.Route("/settings", func(r chi.Router) {
			r.Get("/", h.getSettings)
			r.Put("/", h.updateSettings)
		})

		r.Post("/upload", h.uploadFile)
		r.Get("/import/template", h.getImportTemplate)
		r.Get("/export", h.exportShows)

		r.Route("/backup", func(r chi.Router) {
			r.Get("/download", h.backupDownload)
			r.Post("/restore", h.backupRestore)
		})

		r.Route("/scene-sorts", func(r chi.Router) {
			r.Get("/", h.getSceneSorts)
			r.Put("/", h.updateSceneSort)
			r.Delete("/{play}", h.deleteSceneSort)
		})

		r.Route("/actors", func(r chi.Router) {
			r.Get("/", h.listActors)
			r.Get("/{name}", h.getActor)
			r.Put("/{name}", h.updateActor)
			r.Get("/{name}/shows", h.getActorShows)
		})

		r.Get("/plays", h.listPlays)
	})

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

func (h *Handler) listShows(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")

	var shows []models.Show
	var err error

	if startDate != "" || endDate != "" {
		shows, err = h.db.ListShowsByDateRange(userID, startDate, endDate)
	} else {
		year := r.URL.Query().Get("year")
		month := r.URL.Query().Get("month")

		if year != "" && month == "" {
			startDate = year + "-01-01"
			endDate = year + "-12-31"
			shows, err = h.db.ListShowsByDateRange(userID, startDate, endDate)
		} else {
			y := time.Now().Year()
			m := int(time.Now().Month())
			if year != "" {
				y, _ = strconv.Atoi(year)
			}
			if month != "" {
				m, _ = strconv.Atoi(month)
			}
			shows, err = h.db.ListShows(userID, y, m)
		}
	}

	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if shows == nil {
		shows = []models.Show{}
	}
	jsonResp(w, 200, shows)
}

func (h *Handler) listAllShows(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	shows, err := h.db.ListAllShows(userID)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if shows == nil {
		shows = []models.Show{}
	}
	jsonResp(w, 200, shows)
}

func (h *Handler) getShow(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}

	show, err := h.db.GetShow(userID, id)
	if err != nil {
		jsonErr(w, 404, "show not found")
		return
	}
	jsonResp(w, 200, show)
}

func (h *Handler) createShow(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	var req models.ShowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}

	if req.Name == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	req.Setlist = strings.ReplaceAll(strings.ReplaceAll(req.Setlist, "·", "•"), "*", "•")

	show, err := h.db.CreateShow(userID, req)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 201, show)
}

func (h *Handler) updateShow(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}

	var req models.ShowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	req.Setlist = strings.ReplaceAll(strings.ReplaceAll(req.Setlist, "·", "•"), "*", "•")

	show, err := h.db.UpdateShow(userID, id, req)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, show)
}

func (h *Handler) deleteShow(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}

	show, _ := h.db.GetShow(userID, id)
	if show != nil && show.PosterURL != "" {
		h.storage.Delete(show.PosterURL)
	}

	if err := h.db.DeleteShow(userID, id); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "deleted"})
}

func (h *Handler) getCalendar(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	year := time.Now().Year()
	month := int(time.Now().Month())

	if y := r.URL.Query().Get("year"); y != "" {
		year, _ = strconv.Atoi(y)
	}
	if m := r.URL.Query().Get("month"); m != "" {
		month, _ = strconv.Atoi(m)
	}

	events, err := h.db.GetCalendarEvents(userID, year, month)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if events == nil {
		events = []models.CalendarEvent{}
	}
	jsonResp(w, 200, events)
}

func (h *Handler) getICS(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	shows, err := h.db.ListAllShows(userID)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=mujian.ics")
	w.Write([]byte(ics.GenerateCalendar(shows, h.cfg.Location())))
}

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	stats, err := h.db.GetStats(userID)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, stats)
}

func (h *Handler) getDashboard(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	stats, err := h.db.GetDashboardStats(userID)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, stats)
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	cats, err := h.db.ListCategories(userID)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, cats)
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if req.Name == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	if req.Color == "" {
		req.Color = "#4A90D9"
	}

	cat, err := h.db.CreateCategory(userID, req.Name, req.Color)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 201, cat)
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}

	var req struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}

	if err := h.db.UpdateCategory(userID, id, req.Name, req.Color); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "updated"})
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		jsonErr(w, 400, "invalid id")
		return
	}
	if err := h.db.DeleteCategory(userID, id); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "deleted"})
}

func (h *Handler) updateCategorySort(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if err := h.db.UpdateCategorySort(userID, req.IDs); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "updated"})
}

func (h *Handler) searchShows(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	q := r.URL.Query().Get("q")
	if q == "" {
		jsonResp(w, 200, []models.Show{})
		return
	}

	shows, err := h.db.SearchShows(userID, q)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if shows == nil {
		shows = []models.Show{}
	}
	jsonResp(w, 200, shows)
}

func (h *Handler) batchUpdate(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	var req struct {
		IDs        []int64  `json:"ids"`
		CategoryID *int64   `json:"category_id"`
		Rating     *int     `json:"rating"`
		Status     *string  `json:"status"`
		TicketCost *float64 `json:"ticket_cost"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		jsonErr(w, 400, "no ids provided")
		return
	}

	updated, err := h.db.BatchUpdateShows(userID, req.IDs, req.CategoryID, req.Rating, req.Status, req.TicketCost)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{"updated": updated})
}

func (h *Handler) batchDelete(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if len(req.IDs) == 0 {
		jsonErr(w, 400, "no ids provided")
		return
	}

	deleted, err := h.db.BatchDeleteShows(userID, req.IDs)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{"deleted": deleted})
}

func (h *Handler) getUpcoming(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}
	shows, err := h.db.GetUpcomingShows(userID, limit)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if shows == nil {
		shows = []models.Show{}
	}
	jsonResp(w, 200, shows)
}

func (h *Handler) getRecent(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}
	shows, err := h.db.GetRecentShows(userID, limit)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if shows == nil {
		shows = []models.Show{}
	}
	jsonResp(w, 200, shows)
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

func (h *Handler) getAutocomplete(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	field := chi.URLParam(r, "field")
	values, err := h.db.GetAutocomplete(userID, field)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	jsonResp(w, 200, values)
}

func (h *Handler) getByField(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	field := chi.URLParam(r, "field")
	value := chi.URLParam(r, "value")
	shows, err := h.db.GetShowsByField(userID, field, value)
	if err != nil {
		jsonErr(w, 400, err.Error())
		return
	}
	if shows == nil {
		shows = []models.Show{}
	}
	jsonResp(w, 200, shows)
}

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.AllowLocalStorage && h.cfg.StorageType != "s3" {
		jsonErr(w, 403, "local storage is disabled")
		return
	}

	r.ParseMultipartForm(32 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		jsonErr(w, 400, "no file provided")
		return
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	url, err := h.storage.Save(header, filename)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonResp(w, 200, map[string]string{"url": url})
}

func (h *Handler) backupDownload(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	data, err := h.db.Export(userID)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	filename := fmt.Sprintf("mujian_backup_%s.json", time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Write(b)
}

func (h *Handler) backupRestore(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	r.ParseMultipartForm(32 << 20)
	file, _, err := r.FormFile("file")
	if err != nil {
		jsonErr(w, 400, "no file provided")
		return
	}
	defer file.Close()

	var data db.BackupData
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		jsonErr(w, 400, "invalid backup file: "+err.Error())
		return
	}

	importResult, err := h.db.Import(userID, &data)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	jsonResp(w, 200, map[string]interface{}{
		"message":    "restore completed",
		"categories": importResult.Categories,
		"shows":      importResult.Shows,
		"skipped":    importResult.Skipped,
	})
}

func (h *Handler) getSceneSorts(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	sorts, err := h.db.GetSceneSorts(userID)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if sorts == nil {
		sorts = []models.SceneSort{}
	}
	jsonResp(w, 200, sorts)
}

func (h *Handler) updateSceneSort(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	var req struct {
		Play   string `json:"play"`
		Scenes string `json:"scenes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if req.Play == "" {
		jsonErr(w, 400, "play is required")
		return
	}
	if err := h.db.UpdateSceneSort(userID, req.Play, req.Scenes); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "updated"})
}

func (h *Handler) deleteSceneSort(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	play := chi.URLParam(r, "play")
	if play == "" {
		jsonErr(w, 400, "play is required")
		return
	}
	if err := h.db.DeleteSceneSort(userID, play); err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]string{"message": "deleted"})
}

func (h *Handler) listActors(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	actors, err := h.db.ListActors(userID)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if actors == nil {
		actors = []models.Actor{}
	}
	jsonResp(w, 200, actors)
}

func (h *Handler) getActor(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	name := chi.URLParam(r, "name")
	if name == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	actor, err := h.db.GetActor(userID, name)
	if err != nil {
		jsonErr(w, 404, err.Error())
		return
	}
	jsonResp(w, 200, actor)
}

func (h *Handler) updateActor(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	name := chi.URLParam(r, "name")
	if name == "" {
		jsonErr(w, 400, "name is required")
		return
	}

	var req models.ActorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	req.Name = name

	actor, err := h.db.UpsertActor(userID, req)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, actor)
}

func (h *Handler) getActorShows(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	name := chi.URLParam(r, "name")
	if name == "" {
		jsonErr(w, 400, "name is required")
		return
	}
	shows, err := h.db.GetShowsByActor(userID, name)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	if shows == nil {
		shows = []models.Show{}
	}
	jsonResp(w, 200, shows)
}

func (h *Handler) listPlays(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	shows, err := h.db.ListAllShows(userID)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	type PlayInfo struct {
		Name       string `json:"name"`
		SceneCount int    `json:"scene_count"`
		ShowCount  int    `json:"show_count"`
	}

	playMap := make(map[string]*PlayInfo)
	for _, s := range shows {
		if s.Setlist == "" {
			continue
		}
		lines := strings.Split(s.Setlist, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				idx := strings.Index(part, "•")
				var playName string
				var sceneCount int
				if idx == -1 {
					playName = part
					sceneCount = 0
				} else {
					playName = strings.TrimSpace(part[:idx])
					scenes := strings.Split(part[idx+1:], "•")
					seen := make(map[string]bool)
					for _, sc := range scenes {
						sc = strings.TrimSpace(sc)
						if sc != "" && !seen[sc] {
							seen[sc] = true
							sceneCount++
						}
					}
				}
				if playName == "" {
					continue
				}
				if _, exists := playMap[playName]; !exists {
					playMap[playName] = &PlayInfo{Name: playName}
				}
				playMap[playName].ShowCount++
				if sceneCount > playMap[playName].SceneCount {
					playMap[playName].SceneCount = sceneCount
				}
			}
		}
	}

	plays := make([]PlayInfo, 0, len(playMap))
	for _, p := range playMap {
		plays = append(plays, *p)
	}
	sort.Slice(plays, func(i, j int) bool {
		return plays[i].Name < plays[j].Name
	})
	jsonResp(w, 200, plays)
}
