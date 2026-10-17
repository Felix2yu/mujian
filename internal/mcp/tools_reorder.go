package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type ReorderCategoriesInput struct {
	IDs    []string `json:"ids"`
	DryRun *bool    `json:"dry_run,omitempty"`
}

type ReorderDramasInput struct {
	IDs    []string `json:"ids"`
	DryRun *bool    `json:"dry_run,omitempty"`
}

type ReorderZhezisInput struct {
	DramaID string   `json:"drama_id"`
	IDs     []string `json:"ids"`
	DryRun  *bool    `json:"dry_run,omitempty"`
}

type ReorderArtistsInput struct {
	IDs    []string `json:"ids"`
	DryRun *bool    `json:"dry_run,omitempty"`
}

// ---------- 工具实现 ----------

func (s *Server) handleReorderCategories(ctx context.Context, req *mcp.CallToolRequest, in ReorderCategoriesInput) (*mcp.CallToolResult, any, error) {
	if len(in.IDs) == 0 {
		return errResult("ids 不能为空")
	}

	if dryRun(in.DryRun) {
		categories, err := s.db.ListCategories()
		if err != nil {
			return errResult("查询分类失败：%v", err)
		}
		return jsonResult(map[string]any{
			"dry_run":    true,
			"ids":        in.IDs,
			"categories": categories,
		})
	}

	if err := s.db.ReorderCategories(in.IDs); err != nil {
		return errResult("排序失败：%v", err)
	}

	return jsonResult(map[string]any{"reordered": len(in.IDs)})
}

func (s *Server) handleReorderDramas(ctx context.Context, req *mcp.CallToolRequest, in ReorderDramasInput) (*mcp.CallToolResult, any, error) {
	if len(in.IDs) == 0 {
		return errResult("ids 不能为空")
	}

	if dryRun(in.DryRun) {
		dramas, err := s.db.ListDramas()
		if err != nil {
			return errResult("查询剧目失败：%v", err)
		}
		return jsonResult(map[string]any{
			"dry_run":  true,
			"ids":      in.IDs,
			"dramas":   dramas,
		})
	}

	if err := s.db.ReorderDramas(in.IDs); err != nil {
		return errResult("排序失败：%v", err)
	}

	return jsonResult(map[string]any{"reordered": len(in.IDs)})
}

func (s *Server) handleReorderZhezis(ctx context.Context, req *mcp.CallToolRequest, in ReorderZhezisInput) (*mcp.CallToolResult, any, error) {
	if in.DramaID == "" {
		return errResult("drama_id 不能为空")
	}
	if len(in.IDs) == 0 {
		return errResult("ids 不能为空")
	}

	if dryRun(in.DryRun) {
		drama, err := s.db.GetDramaDetail(in.DramaID)
		if err != nil {
			return errResult("查询剧目失败：%v", err)
		}
		return jsonResult(map[string]any{
			"dry_run":   true,
			"drama_id":  in.DramaID,
			"drama_name": drama.Name,
			"ids":       in.IDs,
			"zhezis":    drama.Zhezis,
		})
	}

	if err := s.db.ReorderZhezis(in.DramaID, in.IDs); err != nil {
		return errResult("排序失败：%v", err)
	}

	return jsonResult(map[string]any{"reordered": len(in.IDs)})
}

func (s *Server) handleReorderArtists(ctx context.Context, req *mcp.CallToolRequest, in ReorderArtistsInput) (*mcp.CallToolResult, any, error) {
	if len(in.IDs) == 0 {
		return errResult("ids 不能为空")
	}

	if dryRun(in.DryRun) {
		artists, err := s.db.ListArtists()
		if err != nil {
			return errResult("查询演员失败：%v", err)
		}
		return jsonResult(map[string]any{
			"dry_run": true,
			"ids":     in.IDs,
			"artists": artists,
		})
	}

	if err := s.db.ReorderArtists(in.IDs); err != nil {
		return errResult("排序失败：%v", err)
	}

	return jsonResult(map[string]any{"reordered": len(in.IDs)})
}
