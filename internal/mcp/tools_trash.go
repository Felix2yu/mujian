package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type ListDeletedRecordsInput struct {
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}

type RestoreRecordInput struct {
	ID     string `json:"id"`
	DryRun *bool  `json:"dry_run,omitempty"`
}

type PurgeRecordInput struct {
	ID     string `json:"id"`
	DryRun *bool  `json:"dry_run,omitempty"`
}

type PurgeDeletedRecordsInput struct {
	DryRun *bool `json:"dry_run,omitempty"`
}

// ---------- 工具实现 ----------

func (s *Server) handleListDeletedRecords(ctx context.Context, req *mcp.CallToolRequest, in ListDeletedRecordsInput) (*mcp.CallToolResult, any, error) {
	limit := 50
	if in.Limit != nil && *in.Limit > 0 {
		limit = *in.Limit
	}
	offset := 0
	if in.Offset != nil && *in.Offset > 0 {
		offset = *in.Offset
	}

	records, err := s.db.ListDeletedRecords(limit, offset)
	if err != nil {
		return errResult("查询回收站失败：%v", err)
	}

	total, err := s.db.DeletedCount()
	if err != nil {
		return errResult("查询回收站数量失败：%v", err)
	}

	return jsonResult(map[string]any{
		"total":  total,
		"limit":  limit,
		"offset": offset,
		"records": records,
	})
}

func (s *Server) handleRestoreRecord(ctx context.Context, req *mcp.CallToolRequest, in RestoreRecordInput) (*mcp.CallToolResult, any, error) {
	if in.ID == "" {
		return errResult("id 不能为空")
	}

	if dryRun(in.DryRun) {
		rec, err := s.db.GetRecord(in.ID)
		if err != nil {
			return errResult("未找到记录「%s」", in.ID)
		}
		return jsonResult(map[string]any{
			"dry_run": true,
			"id":      rec.ID,
			"name":    rec.Name,
			"date":    rec.DateText,
		})
	}

	if err := s.db.RestoreRecord(in.ID); err != nil {
		return errResult("恢复失败：%v", err)
	}

	rec, err := s.db.GetRecord(in.ID)
	if err != nil {
		return jsonResult(map[string]any{"restored": in.ID})
	}
	return jsonResult(rec)
}

func (s *Server) handlePurgeRecord(ctx context.Context, req *mcp.CallToolRequest, in PurgeRecordInput) (*mcp.CallToolResult, any, error) {
	if in.ID == "" {
		return errResult("id 不能为空")
	}

	if dryRun(in.DryRun) {
		rec, err := s.db.GetRecord(in.ID)
		if err != nil {
			return errResult("未找到记录「%s」", in.ID)
		}
		return jsonResult(map[string]any{
			"dry_run": true,
			"id":      rec.ID,
			"name":    rec.Name,
			"date":    rec.DateText,
		})
	}

	if err := s.db.PurgeRecord(in.ID); err != nil {
		return errResult("永久删除失败：%v", err)
	}

	return jsonResult(map[string]any{"purged": in.ID})
}

func (s *Server) handlePurgeDeletedRecords(ctx context.Context, req *mcp.CallToolRequest, in PurgeDeletedRecordsInput) (*mcp.CallToolResult, any, error) {
	if dryRun(in.DryRun) {
		records, err := s.db.ListDeletedRecords(100, 0)
		if err != nil {
			return errResult("查询回收站失败：%v", err)
		}
		total, _ := s.db.DeletedCount()
		return jsonResult(map[string]any{
			"dry_run":   true,
			"will_purge": total,
			"preview":   records,
		})
	}

	records, err := s.db.ListDeletedRecords(0, 0)
	if err != nil {
		return errResult("查询回收站失败：%v", err)
	}

	purged := 0
	for _, rec := range records {
		if err := s.db.PurgeRecord(rec.ID); err == nil {
			purged++
		}
	}

	return jsonResult(map[string]any{
		"purged": purged,
		"total":  len(records),
	})
}
