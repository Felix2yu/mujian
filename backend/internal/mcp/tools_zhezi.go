package mcp

import (
	"context"
	"mujian/internal/models"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type BatchCreateZhezisInput struct {
	DramaID   string   `json:"drama_id,omitempty"`
	DramaName string   `json:"drama_name,omitempty"`
	Names     []string `json:"names"`
	Remark    string   `json:"remark,omitempty"`
}

type UpdateZheziInput struct {
	ID      string   `json:"id"`
	Name    *string  `json:"name,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
	Remark  *string  `json:"remark,omitempty"`
}

// ---------- 工具实现 ----------

// handleBatchCreateZhezis writes a list of zhezi names (e.g. the frequently
// performed pieces of a drama gathered from the web) into one drama, skipping
// names or aliases that already exist there.
func (s *Server) handleBatchCreateZhezis(ctx context.Context, req *mcp.CallToolRequest, in BatchCreateZhezisInput) (*mcp.CallToolResult, any, error) {
	drama, err := s.resolveDrama(in.DramaID, in.DramaName)
	if err != nil {
		return errResult("%v", err)
	}
	if len(in.Names) == 0 {
		return errResult("names 不能为空")
	}

	existing, err := s.db.ListZhezisByDrama(drama.ID)
	if err != nil {
		return errResult("查询折子失败：%v", err)
	}
	have := map[string]bool{}
	for _, z := range existing {
		have[strings.TrimSpace(z.Name)] = true
		for _, al := range z.Aliases {
			have[strings.TrimSpace(al)] = true
		}
	}

	var created, skipped []string
	for _, raw := range in.Names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if have[name] {
			skipped = append(skipped, name)
			continue
		}
		z := models.Zhezi{DramaID: drama.ID, Name: name, Aliases: []string{}, Remark: in.Remark}
		if _, err := s.db.CreateZhezi(z); err != nil {
			return errResult("创建折子「%s」失败：%v", name, err)
		}
		have[name] = true
		created = append(created, name)
	}

	return jsonResult(map[string]any{
		"drama":          drama.Name,
		"created":        created,
		"skipped_exists": skipped,
		"total_zhezis":   len(existing) + len(created),
	})
}

// handleUpdateZhezi partially updates a zhezi: only provided fields change.
func (s *Server) handleUpdateZhezi(ctx context.Context, req *mcp.CallToolRequest, in UpdateZheziInput) (*mcp.CallToolResult, any, error) {
	current, err := s.db.GetZhezi(in.ID)
	if err != nil {
		return errResult("折子不存在：%v", err)
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		current.Name = strings.TrimSpace(*in.Name)
	}
	if in.Aliases != nil {
		current.Aliases = in.Aliases
	}
	if in.Remark != nil {
		current.Remark = *in.Remark
	}
	updated, err := s.db.UpdateZhezi(models.Zhezi{
		ID:      current.ID,
		Name:    current.Name,
		Aliases: current.Aliases,
		Remark:  current.Remark,
	})
	if err != nil {
		return errResult("更新失败：%v", err)
	}
	return jsonResult(updated)
}

func (s *Server) handleDeleteZhezi(ctx context.Context, req *mcp.CallToolRequest, in IDInput) (*mcp.CallToolResult, any, error) {
	if _, err := s.db.GetZhezi(in.ID); err != nil {
		return errResult("折子不存在：%v", err)
	}
	if err := s.db.DeleteZhezi(in.ID); err != nil {
		return errResult("删除失败：%v", err)
	}
	return jsonResult(map[string]any{"deleted": in.ID})
}
