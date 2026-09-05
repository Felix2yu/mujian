package mcp

import (
	"context"
	"fmt"
	"mujian/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

// MergeArtistsInput merges one artist profile into another (重复建档清理).
// Either id or name resolves each side.
type MergeArtistsInput struct {
	SourceID   string `json:"source_id,omitempty"`
	SourceName string `json:"source_name,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	TargetName string `json:"target_name,omitempty"`
	DryRun     *bool  `json:"dry_run,omitempty"`
}

// MergeDramasInput merges one drama into another, moving record links and
// zhezis. Either id or name resolves each side.
type MergeDramasInput struct {
	SourceID   string `json:"source_id,omitempty"`
	SourceName string `json:"source_name,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	TargetName string `json:"target_name,omitempty"`
	DryRun     *bool  `json:"dry_run,omitempty"`
}

// resolveArtistArg resolves an artist by ID or by name/alias.
func (s *Server) resolveArtistArg(id, name string) (*models.Artist, error) {
	if id != "" {
		return s.db.GetArtist(id)
	}
	artist, partial, err := s.findArtist(name)
	if err != nil {
		return nil, err
	}
	if artist == nil {
		return nil, fmt.Errorf("未找到演员「%s」，候选：%v", name, partial)
	}
	return artist, nil
}

// ---------- 工具实现 ----------

func (s *Server) handleMergeArtists(ctx context.Context, req *mcp.CallToolRequest, in MergeArtistsInput) (*mcp.CallToolResult, any, error) {
	if in.SourceID == "" && in.SourceName == "" {
		return errResult("需要提供 source_id 或 source_name")
	}
	if in.TargetID == "" && in.TargetName == "" {
		return errResult("需要提供 target_id 或 target_name")
	}
	source, err := s.resolveArtistArg(in.SourceID, in.SourceName)
	if err != nil {
		return errResult("source 解析失败：%v", err)
	}
	target, err := s.resolveArtistArg(in.TargetID, in.TargetName)
	if err != nil {
		return errResult("target 解析失败：%v", err)
	}
	if source.ID == target.ID {
		return errResult("source 与 target 是同一个演员，无需合并")
	}

	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run": true,
			"source":  map[string]any{"id": source.ID, "name": source.Name, "aliases": source.Aliases, "record_count": source.RecordCount},
			"target":  map[string]any{"id": target.ID, "name": target.Name, "aliases": target.Aliases, "record_count": target.RecordCount},
			"message": "将把 source 的演出关联并入 target，别名并入 target，然后删除 source 档案",
		})
	}

	res, err := s.db.MergeArtists(source.ID, target.ID)
	if err != nil {
		return errResult("合并失败：%v", err)
	}
	return jsonResult(res)
}

func (s *Server) handleMergeDramas(ctx context.Context, req *mcp.CallToolRequest, in MergeDramasInput) (*mcp.CallToolResult, any, error) {
	if in.SourceID == "" && in.SourceName == "" {
		return errResult("需要提供 source_id 或 source_name")
	}
	if in.TargetID == "" && in.TargetName == "" {
		return errResult("需要提供 target_id 或 target_name")
	}
	source, err := s.resolveDrama(in.SourceID, in.SourceName)
	if err != nil {
		return errResult("source 解析失败：%v", err)
	}
	target, err := s.resolveDrama(in.TargetID, in.TargetName)
	if err != nil {
		return errResult("target 解析失败：%v", err)
	}
	if source.ID == target.ID {
		return errResult("source 与 target 是同一个剧目，无需合并")
	}

	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run": true,
			"source":  map[string]any{"id": source.ID, "name": source.Name, "aliases": source.Aliases, "record_count": source.RecordCount, "zhezi_count": source.ZheziCount},
			"target":  map[string]any{"id": target.ID, "name": target.Name, "aliases": target.Aliases, "record_count": target.RecordCount, "zhezi_count": target.ZheziCount},
			"message": "将把 source 的演出关联与折子并入 target（同名折子去重），别名并入 target，然后删除 source 剧目",
		})
	}

	res, err := s.db.MergeDramas(source.ID, target.ID)
	if err != nil {
		return errResult("合并失败：%v", err)
	}
	return jsonResult(res)
}
