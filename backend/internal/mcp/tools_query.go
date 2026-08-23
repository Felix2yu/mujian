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
	var partial []string
	for i := range dramas {
		d := &dramas[i]
		if d.Name == name {
			exact = d
			break
		}
		if strings.Contains(strings.ToLower(d.Name), lower) {
			partial = append(partial, fmt.Sprintf("%s (id=%s)", d.Name, d.ID))
		}
	}
	if exact != nil {
		return exact, nil
	}
	if len(partial) == 1 {
		// 唯一的部分匹配直接采用，方便 AI 用不完整剧名定位。
		for i := range dramas {
			if fmt.Sprintf("%s (id=%s)", dramas[i].Name, dramas[i].ID) == partial[0] {
				return &dramas[i], nil
			}
		}
	}
	if len(partial) > 1 {
		return nil, fmt.Errorf("剧名「%s」匹配到多个剧目，请指定 drama_id：%v", name, partial)
	}
	return nil, fmt.Errorf("未找到剧目「%s」", name)
}

// ---------- 工具实现 ----------

func (s *Server) handleSearchRecords(ctx context.Context, req *mcp.CallToolRequest, in SearchRecordsInput) (*mcp.CallToolResult, any, error) {
	filter := db.RecordFilter{
		Query:    in.Query,
		City:     in.City,
		Category: in.Category,
		Year:     in.Year,
		Month:    in.Month,
		Start:    in.Start,
		End:      in.End,
		ZheziID:  in.ZheziID,
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

	records, err := s.db.ListRecords(filter)
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
	return jsonResult(map[string]any{
		"total":     total,
		"returned":  len(records),
		"truncated": truncated,
		"records":   records,
	})
}

func (s *Server) handleGetRecord(ctx context.Context, req *mcp.CallToolRequest, in IDInput) (*mcp.CallToolResult, any, error) {
	rec, err := s.db.GetRecord(in.ID)
	if err != nil {
		return errResult("记录不存在：%v", err)
	}
	return jsonResult(rec)
}

func (s *Server) handleListArtists(ctx context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	artists, err := s.db.ListArtists()
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	return jsonResult(artists)
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

func (s *Server) handleListDramas(ctx context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	dramas, err := s.db.ListDramas()
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	return jsonResult(dramas)
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

// noInput is the input type for tools that take no parameters.
type noInput struct{}
