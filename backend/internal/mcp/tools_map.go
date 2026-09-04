package mcp

import (
	"context"
	"mujian/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type ListMapPointsInput struct {
	City     *string `json:"city,omitempty"`
	Category *string `json:"category,omitempty"`
}

// ---------- 工具实现 ----------

func (s *Server) handleListMapPoints(ctx context.Context, req *mcp.CallToolRequest, in ListMapPointsInput) (*mcp.CallToolResult, any, error) {
	points, err := s.db.ListMapPoints()
	if err != nil {
		return errResult("查询地图点位失败：%v", err)
	}

	// 可选过滤
	var filtered []mapPointResult
	for _, p := range points {
		if in.City != nil && *in.City != "" && p.City != *in.City {
			continue
		}
		if in.Category != nil && *in.Category != "" && p.CategoryName != *in.Category {
			continue
		}
		filtered = append(filtered, mapPointResult{
			ID:         p.ID,
			Name:       p.Name,
			City:       p.City,
			Address:    p.Address,
			Coordinate: p.Coordinate,
			CoverFile:  p.CoverFile,
			CoverThumb: p.CoverThumb,
			DateText:   p.DateText,
			Rating:     p.Rating,
			Category:   p.CategoryName,
		})
	}

	if filtered == nil {
		filtered = []mapPointResult{}
	}

	return jsonResult(map[string]any{
		"total":  len(filtered),
		"points": filtered,
	})
}

type mapPointResult struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	City       string             `json:"city"`
	Address    string             `json:"address"`
	Coordinate *models.Coordinate `json:"coordinate"`
	CoverFile  string             `json:"coverFile"`
	CoverThumb string             `json:"coverThumb"`
	DateText   string             `json:"dateText"`
	Rating     int                `json:"rating"`
	Category   string             `json:"categoryName"`
}
