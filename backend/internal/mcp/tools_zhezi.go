package mcp

import (
	"context"
	"encoding/json"
	"mujian/internal/models"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

// StringOrArray accepts both a JSON array and a comma-separated string,
// normalizing to []string. This works around MCP clients that stringify
// array parameters.
type StringOrArray []string

func (s *StringOrArray) UnmarshalJSON(data []byte) error {
	// Try array first.
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*s = arr
		return nil
	}
	// Fall back to comma-separated string.
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	var result []string
	for _, part := range strings.Split(str, ",") {
		if t := strings.TrimSpace(part); t != "" {
			result = append(result, t)
		}
	}
	*s = result
	return nil
}

type DeleteZheziInput struct {
	ID     string `json:"id"`
	DryRun *bool  `json:"dry_run,omitempty"`
}

type BatchCreateZhezisInput struct {
	DramaID   string        `json:"drama_id,omitempty"`
	DramaName string        `json:"drama_name,omitempty"`
	Names     StringOrArray `json:"names"`
	Remark    string        `json:"remark,omitempty"`
	DryRun    *bool         `json:"dry_run,omitempty"`
}

type UpdateZheziInput struct {
	ID      string   `json:"id"`
	Name    *string  `json:"name,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
	Remark  *string  `json:"remark,omitempty"`
	DryRun  *bool    `json:"dry_run,omitempty"`
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
		if dryRun(in.DryRun) {
			created = append(created, name)
			have[name] = true
			continue
		}
		z := models.Zhezi{DramaID: drama.ID, Name: name, Aliases: []string{}, Remark: in.Remark}
		if _, err := s.db.CreateZhezi(z); err != nil {
			return errResult("创建折子「%s」失败：%v", name, err)
		}
		have[name] = true
		created = append(created, name)
	}

	summary := map[string]any{
		"drama":          drama.Name,
		"created":        created,
		"skipped_exists": skipped,
	}
	if dryRun(in.DryRun) {
		summary["dry_run"] = true
	} else {
		summary["total_zhezis"] = len(existing) + len(created)
	}
	return jsonResult(summary)
}

// handleUpdateZhezi partially updates a zhezi: only provided fields change.
func (s *Server) handleUpdateZhezi(ctx context.Context, req *mcp.CallToolRequest, in UpdateZheziInput) (*mcp.CallToolResult, any, error) {
	current, err := s.db.GetZhezi(in.ID)
	if err != nil {
		return errResult("折子不存在：%v", err)
	}
	origName := current.Name
	origAliases := current.Aliases
	origRemark := current.Remark
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		current.Name = strings.TrimSpace(*in.Name)
	}
	if in.Aliases != nil {
		current.Aliases = in.Aliases
	}
	if in.Remark != nil {
		current.Remark = *in.Remark
	}

	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run":          true,
			"zhezi_id":         current.ID,
			"original_name":    origName,
			"name":             current.Name,
			"original_aliases": origAliases,
			"aliases":          current.Aliases,
			"original_remark":  origRemark,
			"remark":           current.Remark,
		})
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

func (s *Server) handleDeleteZhezi(ctx context.Context, req *mcp.CallToolRequest, in DeleteZheziInput) (*mcp.CallToolResult, any, error) {
	z, err := s.db.GetZhezi(in.ID)
	if err != nil {
		return errResult("折子不存在：%v", err)
	}
	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run":  true,
			"zhezi_id": z.ID,
			"name":     z.Name,
			"drama_id": z.DramaID,
			"aliases":  z.Aliases,
		})
	}
	if err := s.db.DeleteZhezi(in.ID); err != nil {
		return errResult("删除失败：%v", err)
	}
	return jsonResult(map[string]any{"deleted": in.ID})
}
