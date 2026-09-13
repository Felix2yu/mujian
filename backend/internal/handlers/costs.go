package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	"mujian/internal/models"
)

// 费用补全（cost review）HTTP 层。
//
// 三个端点：
//
//	GET  /api/costs/pending   待确认清单 + 概览计数
//	POST /api/costs/review    批量确认（标记为 0 元 / 跳过 / 填入金额）
//	POST /api/costs/unreview  撤销确认，使其回到待确认清单
//
// 语义与「真实 0 元 vs 历史遗留 0 元」的区分方式，详见
// internal/db/cost_review.go 与 models.CostPendingRecord 的注释。

// GET /api/costs/pending — 返回仍待确认的费用条目与全量概览。
//
// 查询参数：
//
//	limit  返回上限（默认及上限均为 5000；概览计数不受其影响）
func (h *Handler) getCostPending(w http.ResponseWriter, r *http.Request) {
	limit := 5000
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	records, err := h.db.ListCostPending(r.Context(), limit)
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	summary, err := h.db.CostSummary()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{
		"records": records,
		"summary": summary,
	})
}

// costReviewItem 是单条确认请求。Action 取 zero | skip | amount；
// amount 仅在 Action == "amount" 时必填。
type costReviewItem struct {
	ID     string   `json:"id"`
	Field  string   `json:"field"`
	Action string   `json:"action"`
	Amount *float64 `json:"amount,omitempty"`
}

// POST /api/costs/review — 批量确认待补全的费用字段。
//
// 请求体支持两种写法（items 优先）：
//
//	{ "items": [ { "id": "…", "field": "price", "action": "amount", "amount": 120 } ] }
//	{ "ids": ["…","…"], "field": "price", "action": "zero" }
//
// action 语义：
//
//	zero    确认为真实 0 元（免费 / 无支出）：列值落为 0，并写入确认记录
//	skip    确认暂不处理：列值不变，仅写入确认记录后移出待确认清单
//	amount  填入明确金额（>0）；amount <= 0 时等价于 zero
//
// 全部写操作只在字段当前为 NULL/0 时生效 —— 已填写的有效金额绝不会被覆盖。
// dry_run 为 true 时只统计将要影响的记录数，不落库。
func (h *Handler) postCostReview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs    []string         `json:"ids"`
		Field  string           `json:"field"`
		Action string           `json:"action"`
		Amount *float64         `json:"amount,omitempty"`
		Items  []costReviewItem `json:"items"`
		DryRun *bool            `json:"dry_run,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	dryRun := req.DryRun != nil && *req.DryRun

	items := req.Items
	if len(items) == 0 {
		if len(req.IDs) == 0 {
			jsonErr(w, 400, "no ids provided")
			return
		}
		// 简写形式：同一 field/action/amount 作用于一批 id，展开成逐条
		// item 以便统一校验、归并与执行。
		items = make([]costReviewItem, 0, len(req.IDs))
		for _, id := range req.IDs {
			items = append(items, costReviewItem{ID: id, Field: req.Field, Action: req.Action, Amount: req.Amount})
		}
	}

	// 1) 校验：整批任一项非法即拒绝，避免半成功状态难以排查。
	for i, it := range items {
		if it.ID == "" {
			jsonErr(w, 400, "item "+strconv.Itoa(i)+": missing id")
			return
		}
		if !isCostField(it.Field) {
			jsonErr(w, 400, "item "+strconv.Itoa(i)+": unknown field "+it.Field)
			return
		}
		switch it.Action {
		case models.CostStateZero, models.CostStateSkip:
			// ok
		case "amount":
			if it.Amount == nil || math.IsNaN(*it.Amount) || math.IsInf(*it.Amount, 0) || *it.Amount < 0 {
				jsonErr(w, 400, "item "+strconv.Itoa(i)+": invalid amount")
				return
			}
		default:
			jsonErr(w, 400, "item "+strconv.Itoa(i)+": unknown action "+it.Action)
			return
		}
	}

	// 2) 按 (field, action, amount) 归并，把 N 条同类操作压成一次 SQL。
	type groupKey struct {
		field  string
		action string
		amount float64
	}
	groups := make(map[groupKey][]string)
	order := make([]groupKey, 0, len(items))
	for _, it := range items {
		amt := 0.0
		if it.Action == "amount" && it.Amount != nil {
			amt = *it.Amount
		}
		k := groupKey{field: it.Field, action: it.Action, amount: amt}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], it.ID)
	}

	// 3) dry_run：只回报影响规模，不改动任何数据。
	if dryRun {
		plan := make([]map[string]interface{}, 0, len(order))
		for _, k := range order {
			plan = append(plan, map[string]interface{}{
				"field":  k.field,
				"action": k.action,
				"amount": k.amount,
				"ids":    len(groups[k]),
			})
		}
		jsonResp(w, 200, map[string]interface{}{"dry_run": true, "plan": plan})
		return
	}

	var affected int64
	for _, k := range order {
		ids := groups[k]
		var (
			n   int64
			err error
		)
		switch k.action {
		case models.CostStateZero:
			n, err = h.db.MarkCostZero(ids, k.field)
		case models.CostStateSkip:
			n, err = h.db.MarkCostSkip(ids, k.field)
		case "amount":
			n, err = h.db.SetCostAmount(ids, k.field, k.amount)
		}
		if err != nil {
			jsonErr(w, 500, err.Error())
			return
		}
		affected += n
	}

	summary, err := h.db.CostSummary()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{
		"affected": affected,
		"summary":  summary,
	})
}

// POST /api/costs/unreview — 撤销费用确认，使条目重新回到待确认清单。
//
// 请求体：
//
//	{ "ids": ["…"], "field": "price" }   撤销这些记录的 price 确认（field 省略 = 三个字段全撤）
//	{ "all": true,  "field": "price" }   撤销该字段的全部确认
//	{ "all": true }                      撤销全部确认
func (h *Handler) postCostUnreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs   []string `json:"ids"`
		Field string   `json:"field"`
		All   bool     `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	if req.Field != "" && !isCostField(req.Field) {
		jsonErr(w, 400, "unknown field "+req.Field)
		return
	}

	var (
		restored int64
		err      error
	)
	switch {
	case req.All:
		restored, err = h.db.UnreviewCostAll(req.Field)
	case len(req.IDs) > 0:
		restored, err = h.db.UnreviewCost(req.IDs, req.Field)
	default:
		jsonErr(w, 400, "no ids provided")
		return
	}
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}

	summary, err := h.db.CostSummary()
	if err != nil {
		jsonErr(w, 500, err.Error())
		return
	}
	jsonResp(w, 200, map[string]interface{}{
		"restored": restored,
		"summary":  summary,
	})
}

// isCostField 判断字段名是否属于费用补全管理的三个字段。
func isCostField(field string) bool {
	switch field {
	case models.CostFieldPrice, models.CostFieldPayPrice, models.CostFieldOtherCost:
		return true
	}
	return false
}
