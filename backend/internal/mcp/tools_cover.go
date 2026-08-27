package mcp

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type ListCoversInput struct {
	Query  string `json:"query,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

type CoverDuplicatesInput struct{}

type MergeCoversInput struct {
	Target string   `json:"target"`
	Sources []string `json:"sources"`
	DryRun  bool     `json:"dry_run,omitempty"`
}

type CoverOrphansInput struct{}

type CleanupCoversInput struct {
	DryRun bool `json:"dry_run,omitempty"`
}

// ---------- 工具实现 ----------

func (s *Server) handleListCovers(ctx context.Context, req *mcp.CallToolRequest, in ListCoversInput) (*mcp.CallToolResult, any, error) {
	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	covers, total, err := s.db.ListCoverPicker(in.Query, limit, in.Offset)
	if err != nil {
		return errResult("查询封面失败：%v", err)
	}
	return jsonResult(map[string]any{
		"covers": covers,
		"total":  total,
		"limit":  limit,
		"offset": in.Offset,
	})
}

func (s *Server) handleCoverDuplicates(ctx context.Context, req *mcp.CallToolRequest, in CoverDuplicatesInput) (*mcp.CallToolResult, any, error) {
	groups, err := s.db.GetDuplicateGroups()
	if err != nil {
		return errResult("查询重复封面失败：%v", err)
	}
	return jsonResult(map[string]any{
		"groups": groups,
		"count":  len(groups),
	})
}

func (s *Server) handleMergeCovers(ctx context.Context, req *mcp.CallToolRequest, in MergeCoversInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Target) == "" {
		return errResult("target 不能为空")
	}
	if len(in.Sources) == 0 {
		return errResult("sources 不能为空")
	}

	// 收集引用信息
	var merged []map[string]any
	for _, src := range in.Sources {
		refs, err := s.db.CountCoverRefs(src)
		if err != nil {
			return errResult("查询封面引用失败：%v", err)
		}
		merged = append(merged, map[string]any{"source": src, "ref_count": refs})
	}

	if in.DryRun {
		return jsonResult(map[string]any{
			"dry_run":   true,
			"target":    in.Target,
			"sources":   in.Sources,
			"references": merged,
		})
	}

	for _, src := range in.Sources {
		// 获取引用该封面的记录
		records, err := s.db.GetRecordsByCoverFile(src)
		if err != nil {
			continue
		}
		var ids []string
		for _, r := range records {
			ids = append(ids, r.ID)
		}
		if len(ids) > 0 {
			s.db.UpdateRecordsCoverFile(ids, in.Target)
		}
		s.db.DeleteCoverMeta(src)
	}

	return jsonResult(map[string]any{
		"target":  in.Target,
		"merged":  len(in.Sources),
	})
}

func (s *Server) handleCoverOrphans(ctx context.Context, req *mcp.CallToolRequest, in CoverOrphansInput) (*mcp.CallToolResult, any, error) {
	// 通过 ListCoverPicker 获取所有封面，再逐个检查引用数
	covers, _, err := s.db.ListCoverPicker("", 200, 0)
	if err != nil {
		return errResult("查询封面失败：%v", err)
	}
	var orphans []map[string]any
	for _, c := range covers {
		refs, err := s.db.CountCoverRefs(c.FileName)
		if err != nil {
			continue
		}
		if refs == 0 {
			orphans = append(orphans, map[string]any{
				"file_name": c.FileName,
				"size":      c.Size,
			})
		}
	}
	return jsonResult(map[string]any{
		"orphans": orphans,
		"count":   len(orphans),
	})
}

func (s *Server) handleCleanupCovers(ctx context.Context, req *mcp.CallToolRequest, in CleanupCoversInput) (*mcp.CallToolResult, any, error) {
	covers, _, err := s.db.ListCoverPicker("", 500, 0)
	if err != nil {
		return errResult("查询封面失败：%v", err)
	}
	var cleaned []string
	for _, c := range covers {
		refs, err := s.db.CountCoverRefs(c.FileName)
		if err != nil || refs > 0 {
			continue
		}
		if in.DryRun {
			cleaned = append(cleaned, c.FileName)
			continue
		}
		s.db.DeleteCoverMeta(c.FileName)
		cleaned = append(cleaned, c.FileName)
	}
	result := map[string]any{
		"cleaned": cleaned,
		"count":   len(cleaned),
	}
	if in.DryRun {
		result["dry_run"] = true
	}
	return jsonResult(result)
}
