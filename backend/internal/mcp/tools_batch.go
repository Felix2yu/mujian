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
	DryRun *bool   `json:"dry_run,omitempty"`
}

type BatchMergeVenuesInput struct {
	SourceAddress   string `json:"source_address"`
	TargetAddress   string `json:"target_address"`
	SyncCoordinates bool   `json:"sync_coordinates,omitempty"`
	DryRun *bool   `json:"dry_run,omitempty"`
}

type ArrayOp struct {
	Op    string   `json:"op"` // set | append | remove
	Value []string `json:"value,omitempty"`
}

type BatchUpdateRecordsInput struct {
	IDs []string `json:"ids"`

	Name          *string  `json:"name,omitempty"`
	CategoryName  *string  `json:"category_name,omitempty"`
	CategoryNames *ArrayOp `json:"category_names,omitempty"`
	Rating        *int     `json:"rating,omitempty"`
	ActiveStatus  *int     `json:"active_status,omitempty"`
	City          *string  `json:"city,omitempty"`
	Address       *string  `json:"address,omitempty"`
	Channel       *string  `json:"channel,omitempty"`
	Company       *string  `json:"company,omitempty"`
	Friends       *string  `json:"friends,omitempty"`
	Remark        *string  `json:"remark,omitempty"`
	Seat          *string  `json:"seat,omitempty"`
	// 演出时间文本（如 "2026-08-23 19:30"），解析后联动 date；空串清空
	DateText   *string            `json:"date_text,omitempty"`
	Coordinate *models.Coordinate `json:"coordinate,omitempty"`

	Price             *float64 `json:"price,omitempty"`
	PriceCurrency     *string  `json:"price_currency,omitempty"`
	PayPrice          *float64 `json:"pay_price,omitempty"`
	PayPriceCurrency  *string  `json:"pay_price_currency,omitempty"`
	OtherCost         *float64 `json:"other_cost,omitempty"`
	OtherCostCurrency *string  `json:"other_cost_currency,omitempty"`

	DramaIDs    *ArrayOp `json:"drama_ids,omitempty"`
	ZheziIDs    *ArrayOp `json:"zhezi_ids,omitempty"`
	Play        *ArrayOp `json:"play,omitempty"`
	Guest       *ArrayOp `json:"guest,omitempty"`
	ArtistNames *ArrayOp `json:"artist_names,omitempty"`
	TagIDs      *ArrayOp `json:"tag_ids,omitempty"`

	DryRun *bool `json:"dry_run,omitempty"`
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

	if dryRun(in.DryRun) {
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

	if dryRun(in.DryRun) {
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

	// 收集非零的更新字段名，用于 dry_run 预览
	if dryRun(in.DryRun) {
		type fieldChange struct {
			Field string `json:"field"`
			Value any    `json:"value"`
		}
		var changes []fieldChange
		if in.Name != nil {
			changes = append(changes, fieldChange{"name", *in.Name})
		}
		if in.CategoryName != nil {
			changes = append(changes, fieldChange{"category_name", *in.CategoryName})
		}
		if in.CategoryNames != nil {
			changes = append(changes, fieldChange{"category_names", in.CategoryNames})
		}
		if in.Rating != nil {
			changes = append(changes, fieldChange{"rating", *in.Rating})
		}
		if in.ActiveStatus != nil {
			changes = append(changes, fieldChange{"active_status", *in.ActiveStatus})
		}
		if in.City != nil {
			changes = append(changes, fieldChange{"city", *in.City})
		}
		if in.Address != nil {
			changes = append(changes, fieldChange{"address", *in.Address})
		}
		if in.Channel != nil {
			changes = append(changes, fieldChange{"channel", *in.Channel})
		}
		if in.Company != nil {
			changes = append(changes, fieldChange{"company", *in.Company})
		}
		if in.Friends != nil {
			changes = append(changes, fieldChange{"friends", *in.Friends})
		}
		if in.Remark != nil {
			changes = append(changes, fieldChange{"remark", *in.Remark})
		}
		if in.Seat != nil {
			changes = append(changes, fieldChange{"seat", *in.Seat})
		}
		if in.DateText != nil {
			changes = append(changes, fieldChange{"date_text", *in.DateText})
		}
		if in.Coordinate != nil {
			changes = append(changes, fieldChange{"coordinate", in.Coordinate})
		}
		if in.Price != nil {
			changes = append(changes, fieldChange{"price", *in.Price})
		}
		if in.PriceCurrency != nil {
			changes = append(changes, fieldChange{"price_currency", *in.PriceCurrency})
		}
		if in.PayPrice != nil {
			changes = append(changes, fieldChange{"pay_price", *in.PayPrice})
		}
		if in.PayPriceCurrency != nil {
			changes = append(changes, fieldChange{"pay_price_currency", *in.PayPriceCurrency})
		}
		if in.OtherCost != nil {
			changes = append(changes, fieldChange{"other_cost", *in.OtherCost})
		}
		if in.OtherCostCurrency != nil {
			changes = append(changes, fieldChange{"other_cost_currency", *in.OtherCostCurrency})
		}
		if in.DramaIDs != nil {
			changes = append(changes, fieldChange{"drama_ids", in.DramaIDs})
		}
		if in.ZheziIDs != nil {
			changes = append(changes, fieldChange{"zhezi_ids", in.ZheziIDs})
		}
		if in.Play != nil {
			changes = append(changes, fieldChange{"play", in.Play})
		}
		if in.Guest != nil {
			changes = append(changes, fieldChange{"guest", in.Guest})
		}
		if in.ArtistNames != nil {
			changes = append(changes, fieldChange{"artist_names", in.ArtistNames})
		}
		if in.TagIDs != nil {
			changes = append(changes, fieldChange{"tag_ids", in.TagIDs})
		}
		return jsonResult(map[string]any{
			"dry_run":  true,
			"requested": len(in.IDs),
			"ids":      in.IDs,
			"changes":  changes,
		})
	}

	params := models.BatchUpdateParams{
		IDs:               in.IDs,
		Name:              in.Name,
		CategoryName:      in.CategoryName,
		CategoryNames:     (*models.BatchArrayOp)(in.CategoryNames),
		Rating:            in.Rating,
		ActiveStatus:      in.ActiveStatus,
		City:              in.City,
		Address:           in.Address,
		Channel:           in.Channel,
		Company:           in.Company,
		Friends:           in.Friends,
		Remark:            in.Remark,
		Seat:              in.Seat,
		DateText:          in.DateText,
		Coordinate:        in.Coordinate,
		Price:             in.Price,
		PriceCurrency:     in.PriceCurrency,
		PayPrice:          in.PayPrice,
		PayPriceCurrency:  in.PayPriceCurrency,
		OtherCost:         in.OtherCost,
		OtherCostCurrency: in.OtherCostCurrency,
		DramaIDs:          (*models.BatchArrayOp)(in.DramaIDs),
		ZheziIDs:          (*models.BatchArrayOp)(in.ZheziIDs),
		Play:              (*models.BatchArrayOp)(in.Play),
		Guest:             (*models.BatchArrayOp)(in.Guest),
		ArtistNames:       (*models.BatchArrayOp)(in.ArtistNames),
		TagIDs:            (*models.BatchArrayOp)(in.TagIDs),
	}
	n, err := s.db.BatchUpdateRecords(params)
	if err != nil {
		return errResult("批量更新失败：%v", err)
	}
	return jsonResult(map[string]any{"updated": n, "requested": len(in.IDs)})
}
