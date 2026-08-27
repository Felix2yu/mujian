package mcp

import (
	"context"
	"mujian/internal/models"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type CreateDramaInput struct {
	Name          string   `json:"name"`
	CategoryName  string   `json:"category_name,omitempty"`
	CategoryNames []string `json:"category_names,omitempty"`
	Remark        string   `json:"remark,omitempty"`
	DryRun        bool     `json:"dry_run,omitempty"`
}

type UpdateDramaInput struct {
	ID            string   `json:"id"`
	Name          *string  `json:"name,omitempty"`
	CategoryNames []string `json:"category_names,omitempty"`
	Remark        *string  `json:"remark,omitempty"`
	DryRun        bool     `json:"dry_run,omitempty"`
}

// handleUpdateDrama updates a drama's editable fields via SaveDrama (upsert).
func (s *Server) handleUpdateDrama(ctx context.Context, req *mcp.CallToolRequest, in UpdateDramaInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errResult("id 不能为空")
	}
	d, err := s.db.GetDrama(in.ID)
	if err != nil {
		return errResult("未找到剧目「%s」", in.ID)
	}
	origName := d.Name
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return errResult("name 不能为空字符串")
		}
		d.Name = strings.TrimSpace(*in.Name)
	}
	if in.CategoryNames != nil {
		d.CategoryNames = in.CategoryNames
	}
	if in.Remark != nil {
		d.Remark = *in.Remark
	}

	if in.DryRun {
		return jsonResult(map[string]any{
			"dry_run":        true,
			"drama_id":       d.ID,
			"original_name":  origName,
			"drama_name":     d.Name,
			"category_names": d.CategoryNames,
			"remark":         d.Remark,
		})
	}

	updated, err := s.db.SaveDrama(*d)
	if err != nil {
		return errResult("更新剧目失败：%v", err)
	}
	return jsonResult(updated)
}

// handleCreateDrama creates a new drama archive.
func (s *Server) handleCreateDrama(ctx context.Context, req *mcp.CallToolRequest, in CreateDramaInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Name) == "" {
		return errResult("name 不能为空")
	}

	d := models.Drama{
		Name:          strings.TrimSpace(in.Name),
		CategoryName:  in.CategoryName,
		CategoryNames: in.CategoryNames,
		Remark:        in.Remark,
	}

	if in.DryRun {
		return jsonResult(map[string]any{
			"dry_run":        true,
			"drama":          d,
		})
	}

	created, err := s.db.SaveDrama(d)
	if err != nil {
		return errResult("创建剧目失败：%v", err)
	}
	return jsonResult(created)
}

type DeleteDramaInput struct {
	ID     string `json:"id"`
	DryRun bool   `json:"dry_run,omitempty"`
}

// handleDeleteDrama deletes a drama and its zhezis.
func (s *Server) handleDeleteDrama(ctx context.Context, req *mcp.CallToolRequest, in DeleteDramaInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errResult("id 不能为空")
	}
	d, err := s.db.GetDrama(in.ID)
	if err != nil {
		return errResult("未找到剧目「%s」", in.ID)
	}
	if in.DryRun {
		return jsonResult(map[string]any{
			"dry_run":   true,
			"drama_id":  d.ID,
			"name":      d.Name,
			"zhezi_count": d.ZheziCount,
		})
	}
	if err := s.db.DeleteDrama(in.ID); err != nil {
		return errResult("删除剧目失败：%v", err)
	}
	return jsonResult(map[string]any{"deleted": in.ID})
}
