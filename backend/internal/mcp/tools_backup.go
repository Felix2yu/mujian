package mcp

import (
	"context"
	"encoding/json"
	"mujian/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type RunBackupInput struct {
	DryRun *bool `json:"dry_run,omitempty"`
}

type DeleteBackupInput struct {
	File   string `json:"file"`
	DryRun *bool  `json:"dry_run,omitempty"`
}

type RestoreFromBackupInput struct {
	File   string `json:"file"`
	DryRun *bool  `json:"dry_run,omitempty"`
}

// ---------- 工具实现 ----------

func (s *Server) handleRunBackup(ctx context.Context, req *mcp.CallToolRequest, in RunBackupInput) (*mcp.CallToolResult, any, error) {
	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run": true,
			"message": "将触发一次备份",
		})
	}

	name, err := s.backup.RunNow()
	if err != nil {
		return errResult("备份失败：%v", err)
	}

	return jsonResult(map[string]any{
		"file": name,
	})
}

func (s *Server) handleListBackups(ctx context.Context, req *mcp.CallToolRequest, _ noInput) (*mcp.CallToolResult, any, error) {
	backups := s.backup.List()
	return jsonResult(map[string]any{
		"total":   len(backups),
		"backups": backups,
	})
}

func (s *Server) handleDeleteBackup(ctx context.Context, req *mcp.CallToolRequest, in DeleteBackupInput) (*mcp.CallToolResult, any, error) {
	if in.File == "" {
		return errResult("file 不能为空")
	}

	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run": true,
			"file":    in.File,
		})
	}

	if err := s.backup.Delete(in.File); err != nil {
		return errResult("删除备份失败：%v", err)
	}

	return jsonResult(map[string]any{"deleted": in.File})
}

func (s *Server) handleRestoreFromBackup(ctx context.Context, req *mcp.CallToolRequest, in RestoreFromBackupInput) (*mcp.CallToolResult, any, error) {
	if in.File == "" {
		return errResult("file 不能为空")
	}

	data, err := s.backup.Read(in.File)
	if err != nil {
		return errResult("读取备份失败：%v", err)
	}

	// 根据文件扩展名处理
	switch ext := in.File[len(in.File)-4:]; ext {
	case ".json":
		var exportData models.ExportData
		if err := json.Unmarshal(data, &exportData); err != nil {
			return errResult("JSON 解析失败：%v", err)
		}

		if dryRun(in.DryRun) {
			return jsonResult(map[string]any{
				"dry_run":        true,
				"file":           in.File,
				"records":        len(exportData.Records),
				"categories":     len(exportData.Categories),
			})
		}

		result, err := s.db.ImportData(&exportData)
		if err != nil {
			return errResult("恢复失败：%v", err)
		}

		return jsonResult(map[string]any{
			"restored_records":    result.Records,
			"restored_categories": result.Categories,
		})
	case ".db":
		return errResult("在线恢复 .db 文件不支持，请停机后手动替换数据库文件")
	default:
		return errResult("不支持的备份格式：%s", ext)
	}
}
