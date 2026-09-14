package db

import (
	"context"
	"testing"

	"mujian/internal/models"
)

// seedCostRecord 建一条只关心费用字段的记录；三个费用字段用裸指针传入，
// nil 表示「未填写」（落库为 NULL）。
func seedCostRecord(t *testing.T, d *DB, name string, date int64, price, pay, other *float64) *models.Record {
	t.Helper()
	rec, err := d.CreateRecord(models.RecordRequest{
		Name: name, City: "上海", Address: "上海大剧院", Date: date,
		Price: price, PayPrice: pay, OtherCost: other,
	})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return rec
}

// pendingByID 把待确认清单转成 id -> 待确认字段集合。
func pendingByID(t *testing.T, d *DB) map[string]map[string]bool {
	t.Helper()
	rows, err := d.ListCostPending(context.Background(), 0, costFields())
	if err != nil {
		t.Fatalf("ListCostPending: %v", err)
	}
	out := make(map[string]map[string]bool, len(rows))
	for _, r := range rows {
		set := make(map[string]bool, len(r.PendingFields))
		for _, f := range r.PendingFields {
			set[f] = true
		}
		out[r.ID] = set
	}
	return out
}

func costOf(t *testing.T, d *DB, id, field string) *float64 {
	t.Helper()
	rec, err := d.GetRecord(id)
	if err != nil {
		t.Fatalf("GetRecord %s: %v", id, err)
	}
	switch field {
	case models.CostFieldPrice:
		return rec.Price
	case models.CostFieldPayPrice:
		return rec.PayPrice
	case models.CostFieldOtherCost:
		return rec.OtherCost
	}
	t.Fatalf("unknown field %s", field)
	return nil
}

// TestCostPendingClassification 覆盖筛选语义：>0 永不进入待确认；NULL 与 0
// 都进入待确认；同一记录可同时在多个字段上待确认。
func TestCostPendingClassification(t *testing.T) {
	d := newTestDB(t)
	base := int64(1_750_000_000)

	// 全空：三个字段都待确认
	empty := seedCostRecord(t, d, "全空", base, nil, nil, nil)
	// 票价已填：只剩实付与其他花费待确认（两者都是历史默认 0）
	filledPrice := seedCostRecord(t, d, "票价已填", base+1, fltPtr(280), fltPtr(0), fltPtr(0))
	// 实付已填：票价(0) 与其他花费(0) 待确认
	filledPay := seedCostRecord(t, d, "实付已填", base+2, fltPtr(0), fltPtr(180), fltPtr(0))
	// 全部有效：完全不出现在清单里
	allFilled := seedCostRecord(t, d, "全部已填", base+3, fltPtr(280), fltPtr(180), fltPtr(20))

	got := pendingByID(t, d)

	want := map[string][]string{
		empty.ID:       {models.CostFieldPrice, models.CostFieldPayPrice, models.CostFieldOtherCost},
		filledPrice.ID: {models.CostFieldPayPrice, models.CostFieldOtherCost},
		filledPay.ID:   {models.CostFieldPrice, models.CostFieldOtherCost},
	}
	if _, ok := got[allFilled.ID]; ok {
		t.Fatalf("全部字段有效（>0）的记录不应进入待确认清单: %v", allFilled.ID)
	}
	for id, fields := range want {
		if len(got[id]) != len(fields) {
			t.Fatalf("record %s 待确认字段数 = %v, want %v", id, got[id], fields)
		}
		for _, f := range fields {
			if !got[id][f] {
				t.Fatalf("record %s 应包含待确认字段 %s，实际 %v", id, f, got[id])
			}
		}
	}

	summary, err := d.CostSummary(costFields())
	if err != nil {
		t.Fatalf("CostSummary: %v", err)
	}
	if summary.TotalRecords != 4 {
		t.Fatalf("TotalRecords = %d, want 4", summary.TotalRecords)
	}
	if summary.Pending[models.CostFieldPrice] != 2 { // empty, filledPay
		t.Fatalf("pending price = %d, want 2", summary.Pending[models.CostFieldPrice])
	}
	if summary.Pending[models.CostFieldPayPrice] != 2 { // empty, filledPrice
		t.Fatalf("pending pay_price = %d, want 2", summary.Pending[models.CostFieldPayPrice])
	}
	if summary.Pending[models.CostFieldOtherCost] != 3 { // empty, filledPrice, filledPay
		t.Fatalf("pending other_cost = %d, want 3", summary.Pending[models.CostFieldOtherCost])
	}
	if summary.PendingRecords != 3 {
		t.Fatalf("pending records = %d, want 3", summary.PendingRecords)
	}
	if summary.ReviewedTotal != 0 {
		t.Fatalf("reviewed total = %d, want 0", summary.ReviewedTotal)
	}
}

// TestCostReviewNeverTouchesFilledValues 是核心安全约束：批量标记 0 元时，
// 已填真实金额的记录必须原封不动（历史脏数据与有效数据混在同一次操作里）。
func TestCostReviewNeverTouchesFilledValues(t *testing.T) {
	d := newTestDB(t)
	base := int64(1_750_000_000)

	empty := seedCostRecord(t, d, "未填写", base, nil, nil, nil)
	filled := seedCostRecord(t, d, "已填 280", base+1, fltPtr(280), nil, nil)

	if _, err := d.MarkCostZero([]string{empty.ID, filled.ID}, models.CostFieldPrice); err != nil {
		t.Fatalf("MarkCostZero: %v", err)
	}

	// 未填写 → 落为真实 0；已填 280 → 保持不变
	if v := costOf(t, d, empty.ID, models.CostFieldPrice); v == nil || *v != 0 {
		t.Fatalf("未填写记录的票价应落为 0，实际 %v", v)
	}
	if v := costOf(t, d, filled.ID, models.CostFieldPrice); v == nil || *v != 280 {
		t.Fatalf("已填写记录的票价不得被改写，实际 %v", v)
	}

	got := pendingByID(t, d)
	if got[empty.ID][models.CostFieldPrice] {
		t.Fatalf("已标记为 0 元的字段不应再出现在待确认清单")
	}
	if _, ok := got[filled.ID]; ok && got[filled.ID][models.CostFieldPrice] {
		t.Fatalf(">0 的字段永远不应进入待确认清单")
	}
}

// TestCostReviewZeroSkipAmountAndUnreview 覆盖三种确认动作与撤销。
func TestCostReviewZeroSkipAmountAndUnreview(t *testing.T) {
	d := newTestDB(t)
	rec := seedCostRecord(t, d, "待补全", 1_750_000_000, nil, nil, nil)

	// 1) 跳过：列值不变（仍为 NULL），但移出待确认清单
	if _, err := d.MarkCostSkip([]string{rec.ID}, models.CostFieldPayPrice); err != nil {
		t.Fatalf("MarkCostSkip: %v", err)
	}
	if v := costOf(t, d, rec.ID, models.CostFieldPayPrice); v != nil {
		t.Fatalf("skip 不应改写列值，实际 %v", *v)
	}
	if pendingByID(t, d)[rec.ID][models.CostFieldPayPrice] {
		t.Fatalf("skip 后该字段应移出待确认清单")
	}

	// 2) 撤销：回到待确认清单，列值仍为 NULL
	restored, err := d.UnreviewCost([]string{rec.ID}, models.CostFieldPayPrice)
	if err != nil || restored != 1 {
		t.Fatalf("UnreviewCost restored=%d err=%v, want 1/nil", restored, err)
	}
	if !pendingByID(t, d)[rec.ID][models.CostFieldPayPrice] {
		t.Fatalf("撤销后该字段应回到待确认清单")
	}

	// 3) 填入明确金额：写值 + 清标记 + 刷新 total_cost
	if _, err := d.SetCostAmount([]string{rec.ID}, models.CostFieldOtherCost, 50); err != nil {
		t.Fatalf("SetCostAmount: %v", err)
	}
	if v := costOf(t, d, rec.ID, models.CostFieldOtherCost); v == nil || *v != 50 {
		t.Fatalf("other_cost 应为 50，实际 %v", v)
	}
	if pendingByID(t, d)[rec.ID][models.CostFieldOtherCost] {
		t.Fatalf("填入金额后该字段应移出待确认清单")
	}
	full, err := d.GetRecord(rec.ID)
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	// price/pay_price 仍为 NULL，只有 other_cost=50
	if full.TotalCost != 50 {
		t.Fatalf("total_cost = %v, want 50", full.TotalCost)
	}

	// 4) 撤销全部字段
	if _, err := d.UnreviewCost([]string{rec.ID}, ""); err != nil {
		t.Fatalf("UnreviewCost all: %v", err)
	}
	if s, err := d.CostSummary(costFields()); err != nil {
		t.Fatalf("CostSummary: %v", err)
	} else if s.ReviewedTotal != 0 {
		t.Fatalf("撤销后 reviewed total = %d, want 0", s.ReviewedTotal)
	}
}

// TestSetCostAmountZeroMeansFree 覆盖 amount <= 0 退化成本身即「真实 0 元」。
func TestSetCostAmountZeroMeansFree(t *testing.T) {
	d := newTestDB(t)
	rec := seedCostRecord(t, d, "免费票", 1_750_000_000, nil, nil, nil)

	if _, err := d.SetCostAmount([]string{rec.ID}, models.CostFieldPrice, 0); err != nil {
		t.Fatalf("SetCostAmount(0): %v", err)
	}
	if v := costOf(t, d, rec.ID, models.CostFieldPrice); v == nil || *v != 0 {
		t.Fatalf("amount=0 应落为真实 0，实际 %v", v)
	}
	if pendingByID(t, d)[rec.ID][models.CostFieldPrice] {
		t.Fatalf("amount=0 视为已确认，应移出待确认清单")
	}
	s, err := d.CostSummary(costFields())
	if err != nil {
		t.Fatalf("CostSummary: %v", err)
	}
	if s.Reviewed[models.CostFieldPrice] != 1 {
		t.Fatalf("reviewed price = %d, want 1", s.Reviewed[models.CostFieldPrice])
	}
}

// TestCostReviewClearedWhenFieldIsFilled 覆盖「编辑记录填上真实金额后，
// 陈旧的 0 元确认标记被自动清除」，避免已确认状态误导统计。
func TestCostReviewClearedWhenFieldIsFilled(t *testing.T) {
	d := newTestDB(t)
	rec := seedCostRecord(t, d, "先确认后填写", 1_750_000_000, nil, nil, nil)

	if _, err := d.MarkCostZero([]string{rec.ID}, models.CostFieldPrice); err != nil {
		t.Fatalf("MarkCostZero: %v", err)
	}
	if s, _ := d.CostSummary(costFields()); s.Reviewed[models.CostFieldPrice] != 1 {
		t.Fatalf("标记后 reviewed price 应为 1，实际 %d", s.Reviewed[models.CostFieldPrice])
	}

	// 走正常编辑路径把票价改成 120
	if _, err := d.UpdateRecord(rec.ID, models.RecordRequest{
		Name: "先确认后填写", City: "上海", Address: "上海大剧院", Date: 1_750_000_000,
		Price: fltPtr(120),
	}); err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}

	s, err := d.CostSummary(costFields())
	if err != nil {
		t.Fatalf("CostSummary: %v", err)
	}
	if s.Reviewed[models.CostFieldPrice] != 0 {
		t.Fatalf("字段已填真实金额后不应残留确认标记，实际 %d", s.Reviewed[models.CostFieldPrice])
	}
	if v := costOf(t, d, rec.ID, models.CostFieldPrice); v == nil || *v != 120 {
		t.Fatalf("票价应为 120，实际 %v", v)
	}
}

// TestCostReviewUnreviewAllByField 覆盖「撤销某字段全部标记」。
func TestCostReviewUnreviewAllByField(t *testing.T) {
	d := newTestDB(t)
	base := int64(1_750_000_000)
	a := seedCostRecord(t, d, "A", base, nil, nil, nil)
	b := seedCostRecord(t, d, "B", base+1, nil, nil, nil)

	if _, err := d.MarkCostZero([]string{a.ID, b.ID}, models.CostFieldPrice); err != nil {
		t.Fatalf("MarkCostZero price: %v", err)
	}
	if _, err := d.MarkCostSkip([]string{a.ID}, models.CostFieldPayPrice); err != nil {
		t.Fatalf("MarkCostSkip pay: %v", err)
	}

	n, err := d.UnreviewCostAll(models.CostFieldPrice)
	if err != nil || n != 2 {
		t.Fatalf("UnreviewCostAll(price) n=%d err=%v, want 2/nil", n, err)
	}
	got := pendingByID(t, d)
	if !got[a.ID][models.CostFieldPrice] || !got[b.ID][models.CostFieldPrice] {
		t.Fatalf("撤销后票价格式应重新待确认：%v", got)
	}
	if got[a.ID][models.CostFieldPayPrice] {
		t.Fatalf("撤销 price 不应影响 pay_price 的标记")
	}

	// 撤销全部字段
	if _, err := d.UnreviewCostAll(""); err != nil {
		t.Fatalf("UnreviewCostAll(all): %v", err)
	}
	if s, _ := d.CostSummary(costFields()); s.ReviewedTotal != 0 {
		t.Fatalf("撤销全部后 reviewed total = %d, want 0", s.ReviewedTotal)
	}
}

// TestCostReviewRejectsUnknownField 覆盖字段白名单校验。
func TestCostReviewRejectsUnknownField(t *testing.T) {
	d := newTestDB(t)
	rec := seedCostRecord(t, d, "x", 1, nil, nil, nil)
	if _, err := d.MarkCostZero([]string{rec.ID}, "name"); err == nil {
		t.Fatalf("非费用字段应被拒绝")
	}
	if _, err := d.SetCostAmount([]string{rec.ID}, models.CostFieldPrice, -1); err != nil {
		// 负数走 MarkCostZero 分支是允许的（等价 0 元），此处仅确认不 panic
		t.Fatalf("amount<0 不应报错: %v", err)
	}
}
