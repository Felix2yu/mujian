package handlers

import (
	"testing"

	"mujian/internal/config"
	"mujian/internal/models"
)

// costPendingResp 对应 GET /api/costs/pending 的响应体。
type costPendingResp struct {
	Records []models.CostPendingRecord `json:"records"`
	Summary models.CostSummary         `json:"summary"`
}

func costFieldPtr(v float64) *float64 { return &v }

// TestCostsEndpoints 覆盖费用补全三个端点的端到端行为：筛选、批量标记、
// 设置金额、dry_run 预览、撤销，以及非法入参的拒绝。
func TestCostsEndpoints(t *testing.T) {
	// 费用补全测试覆盖全部三个字段的待确认/标记语义，故临时把参与字段开到全量
	// （默认仅 "price"）。其余行为不受影响。
	allCost := "price,pay_price,other_cost"
	ts, _, database, _ := newTestServer(t, func(c *config.Config) {
		c.Update(&config.SettingsUpdate{CostEnabledFields: &allCost})
	})

	empty, err := database.CreateRecord(models.RecordRequest{Name: "全空", Date: 1_750_000_000})
	if err != nil {
		t.Fatalf("seed empty: %v", err)
	}
	filled, err := database.CreateRecord(models.RecordRequest{
		Name: "票价已填", Date: 1_750_000_001, Price: costFieldPtr(280),
	})
	if err != nil {
		t.Fatalf("seed filled: %v", err)
	}

	pending := func() costPendingResp {
		t.Helper()
		res, b := doReq(t, "GET", ts.URL+"/api/costs/pending", nil, "")
		expectStatus(t, res, 200, "GET costs/pending")
		var out costPendingResp
		decodeResp(t, b, &out)
		return out
	}

	// 1) 初始：两条记录都出现在清单里 —— 全空记录的三个字段待确认；已填
	//    票价 280 的记录其 price 不算待确认，但未填的实付/其他花费仍需确认。
	got := pending()
	if len(got.Records) != 2 {
		t.Fatalf("初始待确认记录数 = %d, want 2", len(got.Records))
	}
	if got.Summary.Pending[models.CostFieldPrice] != 1 {
		t.Fatalf("待确认票价 = %d, want 1（仅全空记录）", got.Summary.Pending[models.CostFieldPrice])
	}
	if got.Summary.Pending[models.CostFieldPayPrice] != 2 {
		t.Fatalf("待确认实付 = %d, want 2", got.Summary.Pending[models.CostFieldPayPrice])
	}
	if got.Summary.PendingRecords != 2 {
		t.Fatalf("待确认记录数(汇总) = %d, want 2", got.Summary.PendingRecords)
	}

	// 2) dry_run：只回报计划，不落库。
	res, b := doJSON(t, "POST", ts.URL+"/api/costs/review", map[string]interface{}{
		"ids": []string{empty.ID, filled.ID}, "field": "price", "action": "zero", "dry_run": true,
	})
	expectStatus(t, res, 200, "POST costs/review dry_run")
	var dry map[string]interface{}
	decodeResp(t, b, &dry)
	if dry["dry_run"] != true {
		t.Fatalf("dry_run 响应应标记 dry_run=true: %v", dry)
	}
	if again := pending(); again.Summary.Pending[models.CostFieldPrice] != 1 {
		t.Fatalf("dry_run 不应改动数据，待确认票价 = %d", again.Summary.Pending[models.CostFieldPrice])
	}

	// 3) 批量标记为 0 元：未填写记录落 0，已填 280 的记录保持不变。
	res, b = doJSON(t, "POST", ts.URL+"/api/costs/review", map[string]interface{}{
		"ids": []string{empty.ID, filled.ID}, "field": "price", "action": "zero",
	})
	expectStatus(t, res, 200, "POST costs/review zero")
	got = pending()
	if got.Summary.Pending[models.CostFieldPrice] != 0 {
		t.Fatalf("标记后待确认票价 = %d, want 0", got.Summary.Pending[models.CostFieldPrice])
	}
	// 只有「全空」记录真的被标记：票价 280 的记录被守卫排除，不会产生
	// 标记（它本来就不是待确认项）。
	if got.Summary.Reviewed[models.CostFieldPrice] != 1 {
		t.Fatalf("标记后 reviewed 票价 = %d, want 1", got.Summary.Reviewed[models.CostFieldPrice])
	}
	rec, err := database.GetRecord(filled.ID)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if rec.Price == nil || *rec.Price != 280 {
		t.Fatalf("已填写的票价不得被批量操作改写，实际 %v", rec.Price)
	}
	recEmpty, err := database.GetRecord(empty.ID)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if recEmpty.Price == nil || *recEmpty.Price != 0 {
		t.Fatalf("未填写记录标记后票价应为 0，实际 %v", recEmpty.Price)
	}

	// 4) 单条填入明确金额。
	res, b = doJSON(t, "POST", ts.URL+"/api/costs/review", map[string]interface{}{
		"items": []map[string]interface{}{
			{"id": empty.ID, "field": "pay_price", "action": "amount", "amount": 88},
		},
	})
	expectStatus(t, res, 200, "POST costs/review amount")
	recEmpty, err = database.GetRecord(empty.ID)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if recEmpty.PayPrice == nil || *recEmpty.PayPrice != 88 {
		t.Fatalf("实付应为 88，实际 %v", recEmpty.PayPrice)
	}

	// 5) 撤销：重新回到待确认清单。
	res, b = doJSON(t, "POST", ts.URL+"/api/costs/unreview", map[string]interface{}{
		"ids": []string{empty.ID}, "field": "price",
	})
	expectStatus(t, res, 200, "POST costs/unreview")
	var un map[string]interface{}
	decodeResp(t, b, &un)
	if un["restored"].(float64) != 1 {
		t.Fatalf("撤销数 = %v, want 1", un["restored"])
	}
	if got = pending(); got.Summary.Pending[models.CostFieldPrice] != 1 {
		t.Fatalf("撤销后待确认票价 = %d, want 1", got.Summary.Pending[models.CostFieldPrice])
	}

	// 6) 非法入参：未知字段 / 未知动作 / 缺少 amount。
	for _, payload := range []map[string]interface{}{
		{"ids": []string{empty.ID}, "field": "name", "action": "zero"},
		{"ids": []string{empty.ID}, "field": "price", "action": "explode"},
		{"ids": []string{empty.ID}, "field": "price", "action": "amount"},
		{"ids": []string{}, "field": "price", "action": "zero"},
	} {
		res, _ = doJSON(t, "POST", ts.URL+"/api/costs/review", payload)
		expectStatus(t, res, 400, "POST costs/review invalid")
	}
}

// TestCostsUnreviewAllByField 覆盖「撤销该字段全部标记」。
func TestCostsUnreviewAllByField(t *testing.T) {
	allCost := "price,pay_price,other_cost"
	ts, _, database, _ := newTestServer(t, func(c *config.Config) {
		c.Update(&config.SettingsUpdate{CostEnabledFields: &allCost})
	})

	for i := 0; i < 3; i++ {
		if _, err := database.CreateRecord(models.RecordRequest{Name: "r", Date: int64(1_750_000_000 + i)}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	rows, err := database.ListCostPending(t.Context(), 0, []string{models.CostFieldPrice, models.CostFieldPayPrice, models.CostFieldOtherCost})
	if err != nil {
		t.Fatalf("ListCostPending: %v", err)
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	res, _ := doJSON(t, "POST", ts.URL+"/api/costs/review", map[string]interface{}{
		"ids": ids, "field": "other_cost", "action": "skip",
	})
	expectStatus(t, res, 200, "POST costs/review skip")

	res, b := doJSON(t, "POST", ts.URL+"/api/costs/unreview", map[string]interface{}{
		"all": true, "field": "other_cost",
	})
	expectStatus(t, res, 200, "POST costs/unreview all")
	var un map[string]interface{}
	decodeResp(t, b, &un)
	if un["restored"].(float64) != 3 {
		t.Fatalf("撤销数 = %v, want 3", un["restored"])
	}
	if s := un["summary"].(map[string]interface{}); s["reviewed_total"].(float64) != 0 {
		t.Fatalf("撤销后 reviewed_total 应为 0，实际 %v", s["reviewed_total"])
	}
}
