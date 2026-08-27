package mcp

import (
	"context"
	"mujian/internal/models"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type CreateArtistInput struct {
	Name    string        `json:"name"`
	Aliases StringOrArray `json:"aliases,omitempty"`
	Remark  string        `json:"remark,omitempty"`
	Bio     string        `json:"bio,omitempty"`
	DryRun  bool          `json:"dry_run,omitempty"`
}

type UpdateArtistInput struct {
	ID      string    `json:"id"`
	Name    *string   `json:"name,omitempty"`
	Aliases *ArrayOp  `json:"aliases,omitempty"`
	Remark  *string   `json:"remark,omitempty"`
	Bio     *string   `json:"bio,omitempty"`
	DryRun  bool      `json:"dry_run,omitempty"`
}

type DeleteArtistInput struct {
	ID     string `json:"id"`
	DryRun bool   `json:"dry_run,omitempty"`
}

// ---------- 工具实现 ----------

func (s *Server) handleCreateArtist(ctx context.Context, req *mcp.CallToolRequest, in CreateArtistInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Name) == "" {
		return errResult("name 不能为空")
	}

	a := models.Artist{
		Name:    strings.TrimSpace(in.Name),
		Aliases: in.Aliases,
		Remark:  in.Remark,
		Bio:     in.Bio,
	}

	if in.DryRun {
		return jsonResult(map[string]any{
			"dry_run": true,
			"artist":  a,
		})
	}

	created, err := s.db.SaveArtist(a)
	if err != nil {
		return errResult("创建演员失败：%v", err)
	}
	return jsonResult(created)
}

func (s *Server) handleUpdateArtist(ctx context.Context, req *mcp.CallToolRequest, in UpdateArtistInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errResult("id 不能为空")
	}

	existing, err := s.db.GetArtist(in.ID)
	if err != nil {
		return errResult("未找到演员「%s」", in.ID)
	}

	origName := existing.Name
	origAliases := existing.Aliases
	origRemark := existing.Remark
	origBio := existing.Bio

	if in.Name != nil {
		existing.Name = strings.TrimSpace(*in.Name)
	}
	if in.Aliases != nil {
		existing.Aliases = applyArrayOp(existing.Aliases, in.Aliases)
	}
	if in.Remark != nil {
		existing.Remark = *in.Remark
	}
	if in.Bio != nil {
		existing.Bio = *in.Bio
	}

	if in.DryRun {
		return jsonResult(map[string]any{
			"dry_run":          true,
			"artist_id":        existing.ID,
			"original_name":    origName,
			"name":             existing.Name,
			"original_aliases": origAliases,
			"aliases":          existing.Aliases,
			"original_remark":  origRemark,
			"remark":           existing.Remark,
			"original_bio":     origBio,
			"bio":              existing.Bio,
		})
	}

	updated, err := s.db.SaveArtist(*existing)
	if err != nil {
		return errResult("更新演员失败：%v", err)
	}
	return jsonResult(updated)
}

func (s *Server) handleDeleteArtist(ctx context.Context, req *mcp.CallToolRequest, in DeleteArtistInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errResult("id 不能为空")
	}
	a, err := s.db.GetArtist(in.ID)
	if err != nil {
		return errResult("未找到演员「%s」", in.ID)
	}
	if in.DryRun {
		return jsonResult(map[string]any{
			"dry_run":     true,
			"artist_id":   a.ID,
			"name":        a.Name,
			"aliases":     a.Aliases,
			"record_count": a.RecordCount,
		})
	}
	if err := s.db.DeleteArtist(in.ID); err != nil {
		return errResult("删除演员失败：%v", err)
	}
	return jsonResult(map[string]any{"deleted": in.ID})
}
