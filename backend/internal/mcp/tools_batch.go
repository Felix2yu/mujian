package mcp

import (
	"context"
	"mujian/internal/db"
	"mujian/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type BatchCompanyByArtistInput struct {
	ArtistName string `json:"artist_name"`
	Company    string `json:"company"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

type BatchMergeVenuesInput struct {
	SourceAddress   string `json:"source_address"`
	TargetAddress   string `json:"target_address"`
	SyncCoordinates bool   `json:"sync_coordinates,omitempty"`
	DryRun          bool   `json:"dry_run,omitempty"`
}

type ArrayOp struct {
	Op    string   `json:"op"` // set | append | remove
	Value []string `json:"value,omitempty"`
}

type BatchUpdateRecordsInput struct {
	IDs []string `json:"ids"`

	CategoryName *string `json:"category_name,omitempty"`
	Rating       *int    `json:"rating,omitempty"`
	ActiveStatus *int    `json:"active_status,omitempty"`
	City         *string `json:"city,omitempty"`
	Address      *string `json:"address,omitempty"`
	Channel      *string `json:"channel,omitempty"`
	Company      *string `json:"company,omitempty"`
	Friends      *string `json:"friends,omitempty"`
	Remark       *string `json:"remark,omitempty"`
	Seat         *string `json:"seat,omitempty"`

	DramaIDs    *ArrayOp `json:"drama_ids,omitempty"`
	ZheziIDs    *ArrayOp `json:"zhezi_ids,omitempty"`
	Play        *ArrayOp `json:"play,omitempty"`
	Guest       *ArrayOp `json:"guest,omitempty"`
	ArtistNames *ArrayOp `json:"artist_names,omitempty"`
}

// ---------- 工具实现 ----------

// handleBatchUpdateCompanyByArtist unifies the company field across every show
// that features a given artist.
func (s *Server) handleBatchUpdateCompanyByArtist(ctx context.Context, req *mcp.CallToolRequest, in BatchCompanyByArtistInput) (*mcp.CallToolResult, any, error) {
	artist, partial, err := s.findArtist(in.ArtistName)
	if err != nil {
		return errResult("%v", err)
	}
	if artist == nil {
		return errResult("未找到演员「%s」，候选：%v", in.ArtistName, partial)
	}

	records, err := s.db.ListRecords(db.RecordFilter{ArtistID: artist.ID})
	if err != nil {
		return errResult("查询演出失败：%v", err)
	}
	if len(records) == 0 {
		return jsonResult(map[string]any{
			"artist":  artist.Name,
			"matched": 0,
			"message": "该演员没有关联的演出记录",
		})
	}

	summary := map[string]any{
		"artist":  artist.Name,
		"company": in.Company,
		"matched": len(records),
	}
	var ids []string
	for _, r := range records {
		ids = append(ids, r.ID)
	}

	if in.DryRun {
		type preview struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Date    string `json:"date_text"`
			Company string `json:"company_current"`
		}
		items := make([]preview, 0, len(records))
		changed := 0
		for _, r := range records {
			if r.Company != in.Company {
				changed++
			}
			items = append(items, preview{ID: r.ID, Name: r.Name, Date: r.DateText, Company: r.Company})
		}
		summary["dry_run"] = true
		summary["will_change"] = changed
		summary["records"] = items
		return jsonResult(summary)
	}

	n, err := s.db.BatchUpdateRecords(models.BatchUpdateParams{IDs: ids, Company: &in.Company})
	if err != nil {
		return errResult("批量更新失败：%v", err)
	}
	summary["updated"] = n
	return jsonResult(summary)
}

// handleBatchMergeVenues rewrites every record at source_address to
// target_address and optionally propagates target's coordinates.
func (s *Server) handleBatchMergeVenues(ctx context.Context, req *mcp.CallToolRequest, in BatchMergeVenuesInput) (*mcp.CallToolResult, any, error) {
	if in.SourceAddress == "" || in.TargetAddress == "" {
		return errResult("source_address 与 target_address 均不能为空")
	}
	if in.SourceAddress == in.TargetAddress {
		return errResult("source_address 与 target_address 相同，无需合并")
	}

	sourceRecords, err := s.db.ListRecords(db.RecordFilter{Query: ""})
	if err != nil {
		return errResult("查询失败：%v", err)
	}
	var ids []string
	for _, r := range sourceRecords {
		if r.Address == in.SourceAddress {
			ids = append(ids, r.ID)
		}
	}

	targetCount := 0
	var targetCoord *models.Coordinate
	for _, r := range sourceRecords {
		if r.Address == in.TargetAddress {
			targetCount++
			if targetCoord == nil && r.Coordinate != nil {
				targetCoord = r.Coordinate
			}
		}
	}
	if len(ids) == 0 {
		return errResult("没有找到地址为「%s」的演出记录", in.SourceAddress)
	}

	summary := map[string]any{
		"source":         in.SourceAddress,
		"target":         in.TargetAddress,
		"source_records": len(ids),
		"target_records": targetCount,
	}

	if in.DryRun {
		summary["dry_run"] = true
		return jsonResult(summary)
	}

	addr := in.TargetAddress
	n, err := s.db.BatchUpdateRecords(models.BatchUpdateParams{IDs: ids, Address: &addr})
	if err != nil {
		return errResult("批量更新失败：%v", err)
	}
	summary["updated"] = n

	if in.SyncCoordinates && targetCoord != nil {
		synced, err := s.db.SyncVenueCoordinates(in.TargetAddress, targetCoord, "")
		if err != nil {
			summary["coordinate_sync_error"] = err.Error()
		} else {
			summary["coordinates_synced"] = synced
		}
	}
	return jsonResult(summary)
}

func (s *Server) handleBatchUpdateRecords(ctx context.Context, req *mcp.CallToolRequest, in BatchUpdateRecordsInput) (*mcp.CallToolResult, any, error) {
	if len(in.IDs) == 0 {
		return errResult("ids 不能为空")
	}
	params := models.BatchUpdateParams{
		IDs:          in.IDs,
		CategoryName: in.CategoryName,
		Rating:       in.Rating,
		ActiveStatus: in.ActiveStatus,
		City:         in.City,
		Address:      in.Address,
		Channel:      in.Channel,
		Company:      in.Company,
		Friends:      in.Friends,
		Remark:       in.Remark,
		Seat:         in.Seat,
		DramaIDs:     (*models.BatchArrayOp)(in.DramaIDs),
		ZheziIDs:     (*models.BatchArrayOp)(in.ZheziIDs),
		Play:         (*models.BatchArrayOp)(in.Play),
		Guest:        (*models.BatchArrayOp)(in.Guest),
		ArtistNames:  (*models.BatchArrayOp)(in.ArtistNames),
	}
	n, err := s.db.BatchUpdateRecords(params)
	if err != nil {
		return errResult("批量更新失败：%v", err)
	}
	return jsonResult(map[string]any{"updated": n, "requested": len(in.IDs)})
}
