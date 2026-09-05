package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"mujian/internal/models"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type ExportDataInput struct {
	// ToFile 为 true 时把完整导出 JSON 写入备份目录（文件名 export-*.json，
	// 可用 list_backups/delete_backup 管理），响应只带路径和计数。
	ToFile *bool `json:"to_file,omitempty"`
	// IncludeRecords 为 true 时在响应里内联全部记录（数据量大时会占用
	// 大量上下文，仅在确需小库全量数据时使用；大库请用 to_file）。
	IncludeRecords *bool `json:"include_records,omitempty"`
}

type ImportDataInput struct {
	// JSONData 与 FilePath 二选一：内联 JSON 字符串，或服务器本地文件路径
	// （如 export_data 生成的 export-*.json / 备份 json 文件）。
	JSONData string `json:"json_data,omitempty"`
	FilePath string `json:"file_path,omitempty"`
	DryRun   *bool  `json:"dry_run,omitempty"`
}

// ---------- 工具实现 ----------

func (s *Server) handleExportData(ctx context.Context, req *mcp.CallToolRequest, in ExportDataInput) (*mcp.CallToolResult, any, error) {
	data, err := s.db.Export()
	if err != nil {
		return errResult("导出失败：%v", err)
	}

	summary := map[string]any{
		"source":        data.Source,
		"exported_at":   data.ExportedAt,
		"record_count":  data.RecordCount,
		"categories":    len(data.Categories),
		"record_photos": len(data.RecordPhotos),
		"category_list": data.Categories,
	}

	if in.ToFile != nil && *in.ToFile {
		name := fmt.Sprintf("export-%s.json", time.Now().Format("20060102-150405"))
		path := filepath.Join(s.backup.Dir(), name)
		b, err := json.Marshal(data)
		if err != nil {
			return errResult("导出序列化失败：%v", err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return errResult("写入导出文件失败：%v", err)
		}
		summary["file"] = name
		summary["path"] = path
		summary["bytes"] = len(b)
	}

	if in.IncludeRecords != nil && *in.IncludeRecords {
		summary["records"] = data.Records
	}
	return jsonResult(summary)
}

func (s *Server) handleImportData(ctx context.Context, req *mcp.CallToolRequest, in ImportDataInput) (*mcp.CallToolResult, any, error) {
	raw := []byte(in.JSONData)
	if in.FilePath != "" {
		b, err := os.ReadFile(in.FilePath)
		if err != nil {
			return errResult("读取文件失败：%v", err)
		}
		raw = b
	}
	if len(raw) == 0 {
		return errResult("json_data 与 file_path 至少提供一个")
	}

	var data models.ExportData
	if err := json.Unmarshal(raw, &data); err != nil {
		return errResult("JSON 解析失败：%v", err)
	}

	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run":       true,
			"records":       len(data.Records),
			"categories":    len(data.Categories),
			"record_photos": len(data.RecordPhotos),
			"record_count":  data.RecordCount,
			"source":        data.Source,
			"message":       "导入按记录 upsert：同 ID 覆盖，不删除未包含的现有数据",
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
