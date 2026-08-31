package mcp

import (
	"context"
	"mujian/internal/db"
	"mujian/internal/models"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------- 输入类型 ----------

type CreateRecordInput struct {
	Name              string             `json:"name"`
	Channel           string             `json:"channel,omitempty"`
	City              string             `json:"city,omitempty"`
	Address           string             `json:"address,omitempty"`
	Coordinate        *models.Coordinate `json:"coordinate,omitempty"`
	CoverFile         string             `json:"cover_file,omitempty"`
	CoverThumb        string             `json:"cover_thumb,omitempty"`
	CategoryName      string             `json:"category_name,omitempty"`
	CategoryNames     StringOrArray      `json:"category_names,omitempty"`
	ArtistIDs         StringOrArray      `json:"artist_ids,omitempty"`
	ArtistNames       StringOrArray      `json:"artist_names,omitempty"`
	Guest             StringOrArray      `json:"guest,omitempty"`
	Play              StringOrArray      `json:"play,omitempty"`
	DramaIDs          StringOrArray      `json:"drama_ids,omitempty"`
	ZheziIDs          StringOrArray      `json:"zhezi_ids,omitempty"`
	DateText          string             `json:"date_text,omitempty"`
	Rating            int                `json:"rating,omitempty"`
	Duration          int                `json:"duration,omitempty"`
	Seat              string             `json:"seat,omitempty"`
	Friends           string             `json:"friends,omitempty"`
	Company           string             `json:"company,omitempty"`
	Remark            string             `json:"remark,omitempty"`
	ActiveStatus      int                `json:"active_status,omitempty"`
	Price             float64            `json:"price,omitempty"`
	PriceCurrency     string             `json:"price_currency,omitempty"`
	PayPrice          float64            `json:"pay_price,omitempty"`
	PayPriceCurrency  string             `json:"pay_price_currency,omitempty"`
	OtherCost         float64            `json:"other_cost,omitempty"`
	OtherCostCurrency string             `json:"other_cost_currency,omitempty"`
	DryRun            *bool              `json:"dry_run,omitempty"`
}

type UpdateRecordInput struct {
	ID                string             `json:"id"`
	Name              *string            `json:"name,omitempty"`
	Channel           *string            `json:"channel,omitempty"`
	City              *string            `json:"city,omitempty"`
	Address           *string            `json:"address,omitempty"`
	Coordinate        *models.Coordinate `json:"coordinate,omitempty"`
	CoverFile         *string            `json:"cover_file,omitempty"`
	CoverThumb        *string            `json:"cover_thumb,omitempty"`
	CategoryName      *string            `json:"category_name,omitempty"`
	CategoryNames     *ArrayOp           `json:"category_names,omitempty"`
	ArtistIDs         *ArrayOp           `json:"artist_ids,omitempty"`
	ArtistNames       *ArrayOp           `json:"artist_names,omitempty"`
	Guest             *ArrayOp           `json:"guest,omitempty"`
	Play              *ArrayOp           `json:"play,omitempty"`
	DramaIDs          *ArrayOp           `json:"drama_ids,omitempty"`
	ZheziIDs          *ArrayOp           `json:"zhezi_ids,omitempty"`
	DateText          *string            `json:"date_text,omitempty"`
	Rating            *int               `json:"rating,omitempty"`
	Duration          *int               `json:"duration,omitempty"`
	Seat              *string            `json:"seat,omitempty"`
	Friends           *string            `json:"friends,omitempty"`
	Company           *string            `json:"company,omitempty"`
	Remark            *string            `json:"remark,omitempty"`
	ActiveStatus      *int               `json:"active_status,omitempty"`
	Price             *float64           `json:"price,omitempty"`
	PriceCurrency     *string            `json:"price_currency,omitempty"`
	PayPrice          *float64           `json:"pay_price,omitempty"`
	PayPriceCurrency  *string            `json:"pay_price_currency,omitempty"`
	OtherCost         *float64           `json:"other_cost,omitempty"`
	OtherCostCurrency *string            `json:"other_cost_currency,omitempty"`
	DryRun            *bool              `json:"dry_run,omitempty"`
}

type DeleteRecordInput struct {
	ID     string `json:"id"`
	DryRun *bool  `json:"dry_run,omitempty"`
}

type BatchDeleteRecordsInput struct {
	IDs    []string `json:"ids"`
	DryRun *bool    `json:"dry_run,omitempty"`
}

// ---------- 工具实现 ----------

func (s *Server) handleCreateRecord(ctx context.Context, req *mcp.CallToolRequest, in CreateRecordInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Name) == "" {
		return errResult("name 不能为空")
	}

	r := models.RecordRequest{
		Name:              in.Name,
		Channel:           in.Channel,
		City:              in.City,
		Address:           in.Address,
		Coordinate:        in.Coordinate,
		CoverFile:         in.CoverFile,
		CoverThumb:        in.CoverThumb,
		CategoryName:      in.CategoryName,
		CategoryNames:     in.CategoryNames,
		ArtistIDs:         in.ArtistIDs,
		ArtistNames:       in.ArtistNames,
		Guest:             in.Guest,
		Play:              in.Play,
		DramaIDs:          in.DramaIDs,
		ZheziIDs:          in.ZheziIDs,
		DateText:          in.DateText,
		Rating:            in.Rating,
		Duration:          in.Duration,
		Seat:              in.Seat,
		Friends:           in.Friends,
		Company:           in.Company,
		Remark:            in.Remark,
		ActiveStatus:      in.ActiveStatus,
		Price:             in.Price,
		PriceCurrency:     in.PriceCurrency,
		PayPrice:          in.PayPrice,
		PayPriceCurrency:  in.PayPriceCurrency,
		OtherCost:         in.OtherCost,
		OtherCostCurrency: in.OtherCostCurrency,
	}

	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run": true,
			"record":  r,
		})
	}

	rec, err := s.db.CreateRecord(r)
	if err != nil {
		return errResult("创建演出记录失败：%v", err)
	}
	return jsonResult(rec)
}

func (s *Server) handleUpdateRecord(ctx context.Context, req *mcp.CallToolRequest, in UpdateRecordInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errResult("id 不能为空")
	}

	existing, err := s.db.GetRecord(in.ID)
	if err != nil {
		return errResult("未找到演出记录「%s」", in.ID)
	}

	r := models.RecordRequest{
		Name:              existing.Name,
		Channel:           existing.Channel,
		City:              existing.City,
		Address:           existing.Address,
		Coordinate:        existing.Coordinate,
		CoverFile:         existing.CoverFile,
		CoverThumb:        existing.CoverThumb,
		CategoryName:      existing.CategoryName,
		CategoryNames:     existing.CategoryNames,
		ArtistIDs:         existing.ArtistIDs,
		ArtistNames:       existing.ArtistNames,
		Guest:             existing.Guest,
		Play:              existing.Play,
		DramaIDs:          existing.DramaIDs,
		ZheziIDs:          existing.ZheziIDs,
		DateText:          existing.DateText,
		Rating:            existing.Rating,
		Seat:              existing.Seat,
		Friends:           existing.Friends,
		Company:           existing.Company,
		Remark:            existing.Remark,
		ActiveStatus:      existing.ActiveStatus,
		Price:             existing.Price,
		PriceCurrency:     existing.PriceCurrency,
		PayPrice:          existing.PayPrice,
		PayPriceCurrency:  existing.PayPriceCurrency,
		OtherCost:         existing.OtherCost,
		OtherCostCurrency: existing.OtherCostCurrency,
	}

	if in.Name != nil {
		r.Name = *in.Name
	}
	if in.Channel != nil {
		r.Channel = *in.Channel
	}
	if in.City != nil {
		r.City = *in.City
	}
	if in.Address != nil {
		r.Address = *in.Address
	}
	if in.Coordinate != nil {
		r.Coordinate = in.Coordinate
	}
	if in.CoverFile != nil {
		r.CoverFile = *in.CoverFile
	}
	if in.CoverThumb != nil {
		r.CoverThumb = *in.CoverThumb
	}
	if in.CategoryName != nil {
		r.CategoryName = *in.CategoryName
	}
	if in.CategoryNames != nil {
		r.CategoryNames = applyArrayOp(r.CategoryNames, in.CategoryNames)
	}
	if in.ArtistIDs != nil {
		r.ArtistIDs = applyArrayOp(r.ArtistIDs, in.ArtistIDs)
	}
	if in.ArtistNames != nil {
		r.ArtistNames = applyArrayOp(r.ArtistNames, in.ArtistNames)
	}
	if in.Guest != nil {
		r.Guest = applyArrayOp(r.Guest, in.Guest)
	}
	if in.Play != nil {
		r.Play = applyArrayOp(r.Play, in.Play)
	}
	if in.DramaIDs != nil {
		r.DramaIDs = applyArrayOp(r.DramaIDs, in.DramaIDs)
	}
	if in.ZheziIDs != nil {
		r.ZheziIDs = applyArrayOp(r.ZheziIDs, in.ZheziIDs)
	}
	if in.DateText != nil {
		r.DateText = *in.DateText
	}
	if in.Rating != nil {
		r.Rating = *in.Rating
	}
	if in.Duration != nil {
		r.Duration = *in.Duration
	}
	if in.Seat != nil {
		r.Seat = *in.Seat
	}
	if in.Friends != nil {
		r.Friends = *in.Friends
	}
	if in.Company != nil {
		r.Company = *in.Company
	}
	if in.Remark != nil {
		r.Remark = *in.Remark
	}
	if in.ActiveStatus != nil {
		r.ActiveStatus = *in.ActiveStatus
	}
	if in.Price != nil {
		r.Price = *in.Price
	}
	if in.PriceCurrency != nil {
		r.PriceCurrency = *in.PriceCurrency
	}
	if in.PayPrice != nil {
		r.PayPrice = *in.PayPrice
	}
	if in.PayPriceCurrency != nil {
		r.PayPriceCurrency = *in.PayPriceCurrency
	}
	if in.OtherCost != nil {
		r.OtherCost = *in.OtherCost
	}
	if in.OtherCostCurrency != nil {
		r.OtherCostCurrency = *in.OtherCostCurrency
	}

	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run":  true,
			"id":       in.ID,
			"original": existing,
			"updated":  r,
		})
	}

	rec, err := s.db.UpdateRecord(in.ID, r)
	if err != nil {
		return errResult("更新演出记录失败：%v", err)
	}
	return jsonResult(rec)
}

func (s *Server) handleDeleteRecord(ctx context.Context, req *mcp.CallToolRequest, in DeleteRecordInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ID) == "" {
		return errResult("id 不能为空")
	}
	rec, err := s.db.GetRecord(in.ID)
	if err != nil {
		return errResult("未找到演出记录「%s」", in.ID)
	}
	if dryRun(in.DryRun) {
		return jsonResult(map[string]any{
			"dry_run": true,
			"id":      rec.ID,
			"name":    rec.Name,
			"date":    rec.DateText,
		})
	}
	if err := s.db.DeleteRecord(in.ID); err != nil {
		return errResult("删除失败：%v", err)
	}
	return jsonResult(map[string]any{"deleted": in.ID})
}

func (s *Server) handleBatchDeleteRecords(ctx context.Context, req *mcp.CallToolRequest, in BatchDeleteRecordsInput) (*mcp.CallToolResult, any, error) {
	if len(in.IDs) == 0 {
		return errResult("ids 不能为空")
	}
	if dryRun(in.DryRun) {
		var items []map[string]any
		for _, id := range in.IDs {
			if rec, err := s.db.GetRecord(id); err == nil {
				items = append(items, map[string]any{"id": rec.ID, "name": rec.Name, "date": rec.DateText})
			} else {
				items = append(items, map[string]any{"id": id, "error": "未找到"})
			}
		}
		return jsonResult(map[string]any{
			"dry_run":   true,
			"requested": len(in.IDs),
			"records":   items,
		})
	}
	n, err := s.db.BatchDeleteRecords(in.IDs)
	if err != nil {
		return errResult("批量删除失败：%v", err)
	}
	return jsonResult(map[string]any{"deleted": n, "requested": len(in.IDs)})
}

// applyArrayOp applies set/append/remove operations to a string slice.
func applyArrayOp(current []string, op *ArrayOp) []string {
	if op == nil {
		return current
	}
	switch op.Op {
	case "set":
		return op.Value
	case "append":
		seen := make(map[string]bool, len(current))
		for _, v := range current {
			seen[v] = true
		}
		for _, v := range op.Value {
			if !seen[v] {
				current = append(current, v)
			}
		}
		return current
	case "remove":
		remove := make(map[string]bool, len(op.Value))
		for _, v := range op.Value {
			remove[v] = true
		}
		var result []string
		for _, v := range current {
			if !remove[v] {
				result = append(result, v)
			}
		}
		return result
	default:
		return current
	}
}

// RecordFilter is a convenience alias for the db package's filter.
type RecordFilter = db.RecordFilter
