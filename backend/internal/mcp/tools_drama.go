package mcp

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UpdateDramaInput edits a drama archive. Nil fields keep their current value;
// category_names replaces the full list (a drama usually has few categories).
type UpdateDramaInput struct {
	ID            string   `json:"id"`
	Name          *string  `json:"name,omitempty"`
	CategoryNames []string `json:"category_names,omitempty"`
	Remark        *string  `json:"remark,omitempty"`
}

// handleUpdateDrama updates a drama's editable fields via SaveDrama (upsert),
// which also keeps the scalar primary category in sync with category_names.
func (s *Server) handleUpdateDrama(ctx context.Context, req *mcp.CallToolRequest, in UpdateDramaInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errResult("id 不能为空")
	}
	d, err := s.db.GetDrama(in.ID)
	if err != nil {
		return errResult("未找到剧目「%s」", in.ID)
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return errResult("name 不能为空字符串")
		}
		d.Name = strings.TrimSpace(*in.Name)
	}
	if in.CategoryNames != nil {
		names := make([]string, 0, len(in.CategoryNames))
		for _, n := range in.CategoryNames {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
		d.CategoryNames = names
	}
	if in.Remark != nil {
		d.Remark = *in.Remark
	}
	updated, err := s.db.SaveDrama(*d)
	if err != nil {
		return errResult("更新剧目失败：%v", err)
	}
	return jsonResult(updated)
}
