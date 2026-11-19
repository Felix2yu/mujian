package handlers

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"mujian/internal/db"
	"mujian/internal/models"
)

// upcomingMaxDays 限制回看跨度，避免 ?days= 被放大成一次全表扫描。
const upcomingMaxDays = 90

// upcomingMaxItems 限制返回条数。消费方（众妙）只会渲染最近几场，
// 而且每条最终要塞进 ActivityKit 的 4KB 预算里，多给没有意义。
const upcomingMaxItems = 50

// upcomingDefaultDays 是众妙侧的默认口径：两周足够覆盖提醒排程。
const upcomingDefaultDays = 14

// upcomingRecord 是给原生壳的精简投影，只保留通知与灵动岛能渲染的字段。
// 完整的 models.Record 会撑爆 ActivityKit 的 4KB 预算，且大半字段那边根本不读。
type upcomingRecord struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Date         int64    `json:"date"`
	Duration     int      `json:"duration"`
	Address      string   `json:"address"`
	City         string   `json:"city"`
	Seat         string   `json:"seat"`
	CategoryName string   `json:"categoryName"`
	ArtistNames  []string `json:"artist_names"`
	// CoverURL 是封面缩略图的可访问地址（绝对，或以 / 开头的站内路径）。
	// 众妙拿它做本地通知的缩略图；没有封面时为空。
	CoverURL string `json:"coverUrl"`
}

// upcomingCoverURL 把记录里的封面 key 转成消费方可直接 GET 的地址。
//
// 封面 key 可能来自本地存储（covers/xxx.webp）或 S3；老数据还可能是外部
// URL、data URI 或裸 base64。只返回能通过网络取到的形态：data URI / 裸 base64
// 会撑大响应且无法被 URLSession 直接下载，一律跳过。
func (h *Handler) upcomingCoverURL(rec models.Record) string {
	key := rec.CoverThumb
	if key == "" {
		key = rec.CoverFile
	}
	if key == "" {
		key = rec.Cover
	}
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		return key
	}
	if strings.HasPrefix(key, "/uploads/") {
		return key
	}
	if strings.HasPrefix(key, "data:") || len(key) > 500 {
		return ""
	}
	// 老数据里 cover 可能是无扩展名的 uuid（导入格式），拼成 /uploads/ 只会 404。
	if !strings.Contains(key, ".") {
		return ""
	}
	escaped := escapeURLPath(key)
	if h.cfg != nil && h.cfg.StorageType == "s3" {
		if h.cfg.S3PublicURL == "" {
			return ""
		}
		return strings.TrimRight(h.cfg.S3PublicURL, "/") + "/" + escaped
	}
	return "/uploads/" + escaped
}

// escapeURLPath 逐段转义，保留路径分隔符，同时让中文/空格文件名可用。
func escapeURLPath(p string) string {
	segments := strings.Split(p, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

// listUpcomingRecords 服务 GET /api/records/upcoming?days=14。
//
// 与 /api/records 的区别在于语义固定：只要未来的、按开演时间升序、有上限，
// 并且剔除已取消与未赴约——一场取消掉的演出不该变成一条提醒。
func (h *Handler) listUpcomingRecords(w http.ResponseWriter, r *http.Request) {
	days := upcomingDefaultDays
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	if days > upcomingMaxDays {
		days = upcomingMaxDays
	}

	loc := h.cfg.Location()
	now := time.Now().In(loc)
	recs, err := h.db.ListRecordsContext(r.Context(), db.RecordFilter{
		Start:    now.Format("2006-01-02"),
		End:      now.AddDate(0, 0, days).Format("2006-01-02"),
		Statuses: []int{models.StatusNormal, models.StatusWantWatch},
		NoLimit:  true,
	})
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Start/End 按自然日过滤，今天已开演的场次仍在区间内，按时间戳再剔一次；
	// 否则众妙会为一场几小时前就开演的演出排提醒。
	cutoff := now.Unix()
	items := make([]upcomingRecord, 0, len(recs))
	for _, rec := range recs {
		if rec.Date < cutoff {
			continue
		}
		items = append(items, upcomingRecord{
			ID:           rec.ID,
			Name:         rec.Name,
			Date:         rec.Date,
			Duration:     rec.Duration,
			Address:      rec.Address,
			City:         rec.City,
			Seat:         rec.Seat,
			CategoryName: rec.CategoryName,
			ArtistNames:  rec.ArtistNames,
			CoverURL:     h.upcomingCoverURL(rec),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Date < items[j].Date })
	if len(items) > upcomingMaxItems {
		items = items[:upcomingMaxItems]
	}

	jsonResp(w, http.StatusOK, map[string]any{"items": items, "days": days})
}
