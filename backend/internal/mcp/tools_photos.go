package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// list_record_photos exposes the per-record photo list (file names + order)
// so agents can tell which photos/票根 a record carries — the binary payloads
// themselves stay out of band.

type RecordPhotosInput struct {
	RecordID string `json:"record_id"`
}

func (s *Server) handleListRecordPhotos(ctx context.Context, req *mcp.CallToolRequest, in RecordPhotosInput) (*mcp.CallToolResult, any, error) {
	if in.RecordID == "" {
		return errResult("record_id 不能为空")
	}
	photos, err := s.db.ListRecordPhotos(in.RecordID)
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	return jsonResult(map[string]any{
		"record_id": in.RecordID,
		"total":     len(photos),
		"photos":    photos,
	})
}
