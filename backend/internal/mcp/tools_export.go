package mcp

import (
	"context"
	"encoding/json"

	"mujian/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type ExportDataInput struct {
	Format *string `json:"format,omitempty"` // "json" 或 "zip"，默认 "json"
}

type ImportDataInput struct {
	JSONData string `json:"json_data"`
	DryRun   *bool  `json:"dry_run,omitempty"`
}

// ---------- 工具实现 ----------

func (s *Server) handleExportData(ctx context.Context, req *mcp.CallToolRequest, in ExportDataInput) (*mcp.CallToolResult, any, error) {
	data, err := s.db.Export()
	if err != nil {
		return errResult("导出失败：%v", err)
	}

	return jsonResult(map[string]any{
		"source":        data.Source,
		"exported_at":   data.ExportedAt,
		"record_count":  data.RecordCount,
		"categories":    len(data.Categories),
		"record_photos": len(data.RecordPhotos),
		"records":       data.Records,
		"category_list": data.Categories,
	})
}

func (s *Server) handleImportData(ctx context.Context, req *mcp.CallToolRequest, in ImportDataInput) (*mcp.CallToolResult, any, error) {
	if in.JSONData == "" {
		return errResult("json_data 不能为空")
	}

	var data models.ExportData
	if err := json.Unmarshal([]byte(in.JSONData), &data); err != nil {
		return errResult("JSON 解析失败：%v", err)
	}

	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run":        true,
			"records":        len(data.Records),
			"categories":     len(data.Categories),
			"record_photos":  len(data.RecordPhotos),
			"record_count":   data.RecordCount,
			"source":         data.Source,
		})
	}

	result, err := s.db.ImportData(&data)
	if err != nil {
		return errResult("导入失败：%v", err)
	}

	return jsonResult(map[string]any{
		"imported_records":    result.Records,
		"imported_categories": result.Categories,
	})
}
