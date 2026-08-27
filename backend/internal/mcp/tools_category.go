package mcp

import (
	"context"
	"mujian/internal/models"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type ListCategoriesInput struct{}

type CreateCategoryInput struct {
	Name   string `json:"name"`
	DryRun *bool   `json:"dry_run,omitempty"`
}

type UpdateCategoryInput struct {
	ID     string  `json:"id"`
	Name   *string `json:"name,omitempty"`
	DryRun *bool    `json:"dry_run,omitempty"`
}

type DeleteCategoryInput struct {
	ID     string `json:"id"`
	DryRun *bool   `json:"dry_run,omitempty"`
}

// ---------- 工具实现 ----------

func (s *Server) handleListCategories(ctx context.Context, req *mcp.CallToolRequest, in ListCategoriesInput) (*mcp.CallToolResult, any, error) {
	cats, err := s.db.ListCategories()
	if err != nil {
		return errResult("查询分类失败：%v", err)
	}
	return jsonResult(cats)
}

func (s *Server) handleCreateCategory(ctx context.Context, req *mcp.CallToolRequest, in CreateCategoryInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Name) == "" {
		return errResult("name 不能为空")
	}
	c := &models.Category{Name: strings.TrimSpace(in.Name)}
	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run":  true,
			"category": c,
		})
	}
	if err := s.db.UpsertCategory(c); err != nil {
		return errResult("创建分类失败：%v", err)
	}
	return jsonResult(c)
}

func (s *Server) handleUpdateCategory(ctx context.Context, req *mcp.CallToolRequest, in UpdateCategoryInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errResult("id 不能为空")
	}
	cats, err := s.db.ListCategories()
	if err != nil {
		return errResult("查询分类失败：%v", err)
	}
	var existing *models.Category
	for i := range cats {
		if cats[i].ID == in.ID {
			existing = &cats[i]
			break
		}
	}
	if existing == nil {
		return errResult("未找到分类「%s」", in.ID)
	}
	origName := existing.Name
	if in.Name != nil {
		existing.Name = strings.TrimSpace(*in.Name)
	}
	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run":        true,
			"category_id":    existing.ID,
			"original_name":  origName,
			"name":           existing.Name,
		})
	}
	if err := s.db.UpsertCategory(existing); err != nil {
		return errResult("更新分类失败：%v", err)
	}
	return jsonResult(existing)
}

func (s *Server) handleDeleteCategory(ctx context.Context, req *mcp.CallToolRequest, in DeleteCategoryInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errResult("id 不能为空")
	}
	cats, err := s.db.ListCategories()
	if err != nil {
		return errResult("查询分类失败：%v", err)
	}
	var existing *models.Category
	for i := range cats {
		if cats[i].ID == in.ID {
			existing = &cats[i]
			break
		}
	}
	if existing == nil {
		return errResult("未找到分类「%s」", in.ID)
	}
	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run":     true,
			"category_id": existing.ID,
			"name":        existing.Name,
		})
	}
	if err := s.db.DeleteCategory(in.ID); err != nil {
		return errResult("删除分类失败：%v", err)
	}
	return jsonResult(map[string]any{"deleted": in.ID})
}
