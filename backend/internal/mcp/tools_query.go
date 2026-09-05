package mcp

import (
	"context"
	"fmt"
	"mujian/internal/db"
	"mujian/internal/models"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型（JSON schema 由 SDK 自动生成） ----------

type SearchRecordsInput struct {
	Query      string `json:"query,omitempty"`
	ArtistName string `json:"artist_name,omitempty"`
	ArtistID   string `json:"artist_id,omitempty"`
	DramaName  string `json:"drama_name,omitempty"`
	DramaID    string `json:"drama_id,omitempty"`
	ZheziID    string `json:"zhezi_id,omitempty"`
	City       string `json:"city,omitempty"`
	Category   string `json:"category,omitempty"`
	Year       int    `json:"year,omitempty"`
	Month      int    `json:"month,omitempty"`
	Start      string `json:"start,omitempty"`
	End        string `json:"end,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
	// 扩展筛选维度（与前端筛选面板对齐）
	Channel      string  `json:"channel,omitempty"`
	Company      string  `json:"company,omitempty"`
	RatingMin    int     `json:"rating_min,omitempty"`
	PriceMin     float64 `json:"price_min,omitempty"`
	PriceMax     float64 `json:"price_max,omitempty"`
	ActiveStatus int     `json:"active_status,omitempty"` // 0=正常 1=想看 2=已取消 3=未赴约
	Statuses     []int   `json:"statuses,omitempty"`      // 多选，优先于 active_status
	Exact        bool    `json:"exact,omitempty"`         // 关键词按演出名精确匹配
	// Missing 是逗号分隔的字段列表，匹配任一为空的记录（数据卫生查询）。
	// 支持: category, city, address, company, channel, rating, price, cover,
	// coordinate, artist, drama, zhezi, friends, remark, seat, play, guest。
	Missing string `json:"missing,omitempty"`
	// Compact 为 true 时每条记录只返回核心字段，大幅降低输出体积。
	Compact bool `json:"compact,omitempty"`
}

type IDInput struct {
	ID string `json:"id"`
}

type NameOrIDInput struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type VenuesInput struct {
	Query string `json:"query,omitempty"`
}

type ValueCountsInput struct {
	Field string `json:"field"`
}

// findArtist resolves an artist by exact name, then alias, then case-insensitive
// substring match (returning candidates when ambiguous).
func (s *Server) findArtist(name string) (*models.Artist, []string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil, fmt.Errorf("演员名不能为空")
	}
	artists, err := s.db.ListArtists()
	if err != nil {
		return nil, nil, err
	}
	lower := strings.ToLower(name)
	var partial []string
	for i := range artists {
		a := &artists[i]
		if a.Name == name {
			return a, nil, nil
		}
		for _, al := range a.Aliases {
			if al == name {
				return a, nil, nil
			}
		}
	}
	for i := range artists {
		a := &artists[i]
		if strings.Contains(strings.ToLower(a.Name), lower) {
			partial = append(partial, fmt.Sprintf("%s (id=%s)", a.Name, a.ID))
			continue
		}
		for _, al := range a.Aliases {
			if strings.Contains(strings.ToLower(al), lower) {
				partial = append(partial, fmt.Sprintf("%s (id=%s)", a.Name, a.ID))
				break
			}
		}
	}
	if len(partial) == 0 {
		return nil, nil, fmt.Errorf("未找到演员「%s」", name)
	}
	return nil, partial, nil
}

// resolveDrama finds a drama by id or exact/substring name match.
func (s *Server) resolveDrama(id, name string) (*models.Drama, error) {
	if id != "" {
		return s.db.GetDrama(id)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("需要提供 drama_id 或 drama_name")
	}
	dramas, err := s.db.ListDramas()
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(name)
	var exact *models.Drama
	var partial []string            // 仅用于错误提示展示
	var partialRefs []*models.Drama // 与 partial 一一对应，唯一命中时直接采用
	for i := range dramas {
		d := &dramas[i]
		if d.Name == name {
			exact = d
			break
		}
		// Check aliases for exact match
		for _, alias := range d.Aliases {
			if alias == name {
				exact = d
				break
			}
		}
		if exact != nil {
			break
		}
		if strings.Contains(strings.ToLower(d.Name), lower) {
			partial = append(partial, fmt.Sprintf("%s (id=%s)", d.Name, d.ID))
			partialRefs = append(partialRefs, d)
			continue
		}
		// Check aliases for partial match
		for _, alias := range d.Aliases {
			if strings.Contains(strings.ToLower(alias), lower) {
				partial = append(partial, fmt.Sprintf("%s (id=%s)", d.Name, d.ID))
				partialRefs = append(partialRefs, d)
				break
			}
		}
	}
	if exact != nil {
		return exact, nil
	}
	if len(partialRefs) == 1 {
		// 唯一的部分匹配直接采用，方便 AI 用不完整剧名定位。
		// 直接持有剧目指针，避免用 "name (id=…)" 字符串反解析（剧目名
		// 本身可能包含 "(id="）。
		return partialRefs[0], nil
	}
	if len(partialRefs) > 1 {
		return nil, fmt.Errorf("剧名「%s」匹配到多个剧目，请指定 drama_id：%v", name, partial)
	}
	return nil, fmt.Errorf("未找到剧目「%s」", name)
}

// ---------- 工具实现 ----------

func (s *Server) handleSearchRecords(ctx context.Context, req *mcp.CallToolRequest, in SearchRecordsInput) (*mcp.CallToolResult, any, error) {
	filter := db.RecordFilter{
		Query:        in.Query,
		City:         in.City,
		Category:     in.Category,
		Year:         in.Year,
		Month:        in.Month,
		Start:        in.Start,
		End:          in.End,
		ZheziID:      in.ZheziID,
		Channel:      in.Channel,
		Company:      in.Company,
		RatingMin:    in.RatingMin,
		PriceMin:     in.PriceMin,
		PriceMax:     in.PriceMax,
		ActiveStatus: in.ActiveStatus,
		Statuses:     in.Statuses,
		Exact:        in.Exact,
		Missing:      in.Missing,
		Offset:       in.Offset,
	}

	if in.ArtistID != "" {
		filter.ArtistID = in.ArtistID
	} else if in.ArtistName != "" {
		artist, partial, err := s.findArtist(in.ArtistName)
		if err != nil {
			return errResult("%v", err)
		}
		if artist == nil {
			return errResult("未找到演员「%s」，候选：%v", in.ArtistName, partial)
		}
		filter.ArtistID = artist.ID
	}

	if in.DramaID != "" {
		filter.DramaID = in.DramaID
	} else if in.DramaName != "" {
		drama, err := s.resolveDrama("", in.DramaName)
		if err != nil {
			return errResult("%v", err)
		}
		filter.DramaID = drama.ID
	}

	records, err := s.db.ListRecordsContext(ctx, filter)
	if err != nil {
		return errResult("查询失败：%v", err)
	}

	total := len(records)
	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	truncated := false
	if total > limit {
		records = records[:limit]
		truncated = true
	}
	if in.Compact {
		compacted := make([]map[string]any, len(records))
		for i, r := range records {
			compacted[i] = compactRecord(&r)
		}
		return jsonResult(map[string]any{
			"total":     total,
			"returned":  len(compacted),
			"truncated": truncated,
			"offset":    in.Offset,
			"records":   compacted,
		})
	}
	return jsonResult(map[string]any{
		"total":     total,
		"returned":  len(records),
		"truncated": truncated,
		"offset":    in.Offset,
		"records":   records,
	})
}

// compactRecord projects a record onto the fields most list/cleanup workflows
// need, cutting the per-record payload roughly in half. Field names mirror
// models.Record's JSON tags.
func compactRecord(r *models.Record) map[string]any {
	out := map[string]any{
		"id":            r.ID,
		"name":          r.Name,
		"dateText":      r.DateText,
		"city":          r.City,
		"address":       r.Address,
		"company":       r.Company,
		"channel":       r.Channel,
		"categoryName":  r.CategoryName,
		"rating":        r.Rating,
		"active_status": r.ActiveStatus,
		"artist_names":  r.ArtistNames,
	}
	if r.Coordinate != nil {
		out["coordinate"] = r.Coordinate
	}
	return out
}

func (s *Server) handleGetRecord(ctx context.Context, req *mcp.CallToolRequest, in IDInput) (*mcp.CallToolResult, any, error) {
	rec, err := s.db.GetRecord(in.ID)
	if err != nil {
		return errResult("记录不存在：%v", err)
	}
	return jsonResult(rec)
}

// ListQueryInput is the input for list endpoints that support an optional
// in-memory substring filter over the entity's name and aliases.
type ListQueryInput struct {
	Query string `json:"query,omitempty"`
}

func filterByName[T any](items []T, query string, name func(*T) string, aliases func(*T) []string) []T {
	query = strings.TrimSpace(query)
	if query == "" {
		return items
	}
	lower := strings.ToLower(query)
	out := make([]T, 0, len(items))
	for i := range items {
		p := &items[i]
		if strings.Contains(strings.ToLower(name(p)), lower) {
			out = append(out, *p)
			continue
		}
		for _, al := range aliases(p) {
			if strings.Contains(strings.ToLower(al), lower) {
				out = append(out, *p)
				break
			}
		}
	}
	return out
}

func (s *Server) handleListArtists(ctx context.Context, req *mcp.CallToolRequest, in ListQueryInput) (*mcp.CallToolResult, any, error) {
	artists, err := s.db.ListArtists()
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	total := len(artists)
	artists = filterByName(artists, in.Query,
		func(a *models.Artist) string { return a.Name },
		func(a *models.Artist) []string { return a.Aliases })
	return jsonResult(map[string]any{
		"total":    total,
		"returned": len(artists),
		"artists":  artists,
	})
}

func (s *Server) handleGetArtistDetail(ctx context.Context, req *mcp.CallToolRequest, in NameOrIDInput) (*mcp.CallToolResult, any, error) {
	if in.ID == "" && in.Name == "" {
		return errResult("需要提供 id 或 name")
	}
	id := in.ID
	if id == "" {
		artist, partial, err := s.findArtist(in.Name)
		if err != nil {
			return errResult("%v", err)
		}
		if artist == nil {
			return errResult("未找到演员「%s」，候选：%v", in.Name, partial)
		}
		id = artist.ID
	}
	detail, err := s.db.GetArtistDetail(id)
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	return jsonResult(detail)
}

func (s *Server) handleListDramas(ctx context.Context, req *mcp.CallToolRequest, in ListQueryInput) (*mcp.CallToolResult, any, error) {
	dramas, err := s.db.ListDramas()
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	total := len(dramas)
	dramas = filterByName(dramas, in.Query,
		func(d *models.Drama) string { return d.Name },
		func(d *models.Drama) []string { return d.Aliases })
	return jsonResult(map[string]any{
		"total":    total,
		"returned": len(dramas),
		"dramas":   dramas,
	})
}

func (s *Server) handleGetDramaDetail(ctx context.Context, req *mcp.CallToolRequest, in NameOrIDInput) (*mcp.CallToolResult, any, error) {
	drama, err := s.resolveDrama(in.ID, in.Name)
	if err != nil {
		return errResult("%v", err)
	}
	detail, err := s.db.GetDramaDetail(drama.ID)
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	return jsonResult(detail)
}

func (s *Server) handleListVenues(ctx context.Context, req *mcp.CallToolRequest, in VenuesInput) (*mcp.CallToolResult, any, error) {
	groups, err := s.db.ListVenueGroups(in.Query)
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	return jsonResult(map[string]any{
		"total_groups": len(groups),
		"venues":       groups,
	})
}

func (s *Server) handleValueCounts(ctx context.Context, req *mcp.CallToolRequest, in ValueCountsInput) (*mcp.CallToolResult, any, error) {
	counts, err := s.db.GetValueCounts(in.Field)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{
		"field":  in.Field,
		"counts": counts,
	})
}

func (s *Server) handleGetStats(ctx context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	stats, err := s.db.GetStats()
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	return jsonResult(stats)
}

// ---------- 按坐标搜索 ----------

type SearchByLocationInput struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Radius    float64 `json:"radius"`               // 搜索半径（米）
	Limit     *int    `json:"limit,omitempty"`       // 默认 50
	Category  *string `json:"category,omitempty"`
	City      *string `json:"city,omitempty"`
	StartDate *string `json:"start_date,omitempty"` // "2024-01-01"
	EndDate   *string `json:"end_date,omitempty"`   // "2024-12-31"
}

func (s *Server) handleSearchByLocation(ctx context.Context, req *mcp.CallToolRequest, in SearchByLocationInput) (*mcp.CallToolResult, any, error) {
	limit := 50
	if in.Limit != nil && *in.Limit > 0 {
		limit = *in.Limit
	}

	results, err := s.db.SearchByLocation(in.Latitude, in.Longitude, in.Radius, limit, in.Category, in.City, in.StartDate, in.EndDate)
	if err != nil {
		return errResult("查询附近记录失败：%v", err)
	}

	return jsonResult(map[string]any{
		"center": map[string]any{
			"latitude":  in.Latitude,
			"longitude": in.Longitude,
		},
		"radius_m": in.Radius,
		"total":    len(results),
		"records":  results,
	})
}
