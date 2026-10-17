package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"mujian/internal/config"
	"mujian/internal/huozhi"

	"github.com/go-chi/chi/v5"
)

// ---------- GET /api/records/{id}/bills ----------

// recordBillsResponse is the bill panel payload for one performance. It keeps
// the locally entered cost and the 货殖 total side by side: the frontend shows
// 货殖 as the effective amount whenever the API returned at least one bill, and
// falls back to the built-in total otherwise.
type recordBillsResponse struct {
	Enabled bool           `json:"enabled"`
	Ready   bool           `json:"ready"`    // 已启用且配置完整
	BillIDs []int64        `json:"bill_ids"` // 记录上保存的关联账单
	Bills   []*huozhi.Bill `json:"bills"`
	// 货殖合计：仅统计成功拉取到的账单
	Total         float64 `json:"total"`
	Currency      string  `json:"currency"`
	MixedCurrency bool    `json:"mixed_currency"`
	// Breakdown 按 bill 角色分组（门票实付 / 其他开销），各组给出小计。
	// 前端据此分别展示，而不只是显示一个总和。
	Breakdown []billGroup `json:"breakdown"`
	// 内建费用合计（pay_price 优先，否则 price + other_cost）
	LocalTotal    float64 `json:"local_total"`
	LocalCurrency string  `json:"local_currency"`
	// 最终采用值：有货殖数据则取自货殖，否则取内建合计
	EffectiveTotal    float64 `json:"effective_total"`
	EffectiveCurrency string  `json:"effective_currency"`
	EffectiveSource   string  `json:"effective_source"` // "huozhi" | "local"
	// Errors maps bill id → failure reason; Error is an integration-level
	// failure (not configured / unreachable).
	Errors map[string]string `json:"errors,omitempty"`
	Error  string            `json:"error,omitempty"`
}

// billGroup is one role bucket of the 货殖 breakdown: 门票实付 or 其他开销.
type billGroup struct {
	Role     string         `json:"role"` // "ticket" | "other"
	Label    string         `json:"label"`
	Total    float64        `json:"total"`
	Currency string         `json:"currency"`
	Mixed    bool           `json:"mixed_currency"`
	Bills    []*huozhi.Bill `json:"bills"`
}

// billRoleLabels maps the role key to its Chinese label.
var billRoleLabels = map[string]string{
	"ticket": "门票实付",
	"other":  "其他开销",
}

// GET /api/records/{id}/bills — 解析该演出关联的货殖账单。外部接口不可用
// 时依然返回 200（降级为内建费用），避免详情页整体报错。
func (h *Handler) recordBills(w http.ResponseWriter, r *http.Request) {
	rec, err := h.db.GetRecord(chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, 404, "record not found")
		return
	}

	s := h.cfg.GetHuozhiSettings()
	resp := &recordBillsResponse{
		Enabled:           s.Enabled,
		Ready:             s.Configured(),
		BillIDs:           rec.HuozhiBillIDs,
		Bills:             []*huozhi.Bill{},
		LocalTotal:        rec.TotalCost,
		LocalCurrency:     firstNonEmpty(rec.PayPriceCurrency, rec.PriceCurrency, "CNY"),
		EffectiveTotal:    rec.TotalCost,
		EffectiveCurrency: firstNonEmpty(rec.PayPriceCurrency, rec.PriceCurrency, "CNY"),
		EffectiveSource:   "local",
	}
	if len(resp.BillIDs) == 0 {
		jsonResp(w, 200, resp)
		return
	}
	if !s.Enabled {
		resp.Error = "货殖关联未启用"
		jsonResp(w, 200, resp)
		return
	}
	if !s.Configured() {
		resp.Error = "货殖已启用但未配置完整（需要域名与 API Key）"
		jsonResp(w, 200, resp)
		return
	}

	client := huozhi.New(s)
	bills, errs := client.GetBills(r.Context(), rec.HuozhiBillIDs)
	resp.Bills = bills
	if len(errs) > 0 {
		resp.Errors = map[string]string{}
		for id, e := range errs {
			resp.Errors[strconv.FormatInt(id, 10)] = e.Error()
		}
	}
	if len(bills) > 0 {
		resp.Total, resp.Currency, resp.MixedCurrency = sumBills(bills)
		resp.Breakdown = groupBills(bills, s.TicketCategories)
		// 货殖优先级更高：接口返回了有效数据就采用接口数据
		resp.EffectiveTotal = resp.Total
		resp.EffectiveCurrency = firstNonEmpty(resp.Currency, resp.LocalCurrency)
		resp.EffectiveSource = "huozhi"
	} else if len(errs) > 0 {
		resp.Error = "未能拉取到任何货殖账单，已回退到内建费用"
	}
	jsonResp(w, 200, resp)
}

// sumBills totals bill amounts. Mixed currencies are still summed (the UI flags
// it) because refusing to total would hide the number the user asked for.
func sumBills(bills []*huozhi.Bill) (total float64, currency string, mixed bool) {
	for _, b := range bills {
		if b == nil {
			continue
		}
		total += b.Amount
		c := strings.ToUpper(strings.TrimSpace(b.Currency))
		if c == "" {
			continue
		}
		switch {
		case currency == "":
			currency = c
		case currency != c:
			mixed = true
		}
	}
	if currency == "" {
		currency = "CNY"
	}
	return total, currency, mixed
}

// groupBills buckets bills by role (ticket / other) using the configured
// ticket-category keywords, returning one group per non-empty role. Each group
// carries its own subtotal and currency so the UI can show 门票实付 and 其他开销
// separately instead of only the grand total.
func groupBills(bills []*huozhi.Bill, ticketCategories string) []billGroup {
	ticket := billGroup{Role: "ticket", Label: billRoleLabels["ticket"]}
	other := billGroup{Role: "other", Label: billRoleLabels["other"]}
	for _, b := range bills {
		if b == nil {
			continue
		}
		role := huozhi.ClassifyBillRole(b, ticketCategories)
		g := &other
		if role == "ticket" {
			g = &ticket
		}
		g.Total += b.Amount
		c := strings.ToUpper(strings.TrimSpace(b.Currency))
		if c != "" {
			switch {
			case g.Currency == "":
				g.Currency = c
			case g.Currency != c:
				g.Mixed = true
			}
		}
		g.Bills = append(g.Bills, b)
	}
	out := make([]billGroup, 0, 2)
	if len(ticket.Bills) > 0 {
		if ticket.Currency == "" {
			ticket.Currency = "CNY"
		}
		out = append(out, ticket)
	}
	if len(other.Bills) > 0 {
		if other.Currency == "" {
			other.Currency = "CNY"
		}
		out = append(out, other)
	}
	return out
}

// ---------- POST /api/huozhi/bills（表单填写时校验 / 预览） ----------

type billLookupRequest struct {
	// IDs 接受数字或数字字符串（表单里是文本输入）。
	IDs []interface{} `json:"ids"`
}

// POST /api/huozhi/bills — 批量查询账单，供演出表单在保存前预览。
func (h *Handler) lookupHuozhiBills(w http.ResponseWriter, r *http.Request) {
	var req billLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	ids := parseBillIDs(req.IDs)
	if len(ids) == 0 {
		jsonResp(w, 200, map[string]interface{}{"bills": []*huozhi.Bill{}, "total": 0})
		return
	}

	s := h.cfg.GetHuozhiSettings()
	if !s.Enabled {
		jsonErr(w, 400, "货殖关联未启用")
		return
	}
	if !s.Configured() {
		jsonErr(w, 400, "货殖已启用但未配置完整（需要域名与 API Key）")
		return
	}

	client := huozhi.New(s)
	bills, errs := client.GetBills(r.Context(), ids)
	errMap := map[string]string{}
	for id, e := range errs {
		errMap[strconv.FormatInt(id, 10)] = e.Error()
	}
	total, currency, mixed := sumBills(bills)
	jsonResp(w, 200, map[string]interface{}{
		"bills":          bills,
		"errors":         errMap,
		"total":          total,
		"currency":       currency,
		"mixed_currency": mixed,
		"breakdown":      groupBills(bills, s.TicketCategories),
	})
}

// parseBillIDs normalizes the loosely typed id list from the client.
func parseBillIDs(raw []interface{}) []int64 {
	out := make([]int64, 0, len(raw))
	seen := map[int64]bool{}
	for _, v := range raw {
		var id int64
		switch t := v.(type) {
		case float64:
			id = int64(t)
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
			if err != nil {
				continue
			}
			id = n
		case json.Number:
			n, err := t.Int64()
			if err != nil {
				continue
			}
			id = n
		default:
			continue
		}
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// ---------- POST /api/settings/test-huozhi ----------

type huozhiTestRequest struct {
	Domain   string `json:"huozhi_domain"`
	APIKey   string `json:"huozhi_api_key"`
	BillPath string `json:"huozhi_bill_path"`
	// BillID 可选：给了就真实拉取该账单以验证接口返回，不给则只验证连通性。
	BillID interface{} `json:"bill_id"`
}

// effectiveHuozhiSettings merges the submitted values with what is already
// saved, mirroring the S3 test semantics: empty means "not provided", a masked
// key (suffix "****") means "unchanged".
func effectiveHuozhiSettings(saved config.HuozhiSettings, req huozhiTestRequest) config.HuozhiSettings {
	out := saved
	out.Enabled = true // a test implies intent to use it
	if req.Domain != "" {
		out.Domain = req.Domain
	}
	if req.APIKey != "" && !strings.HasSuffix(req.APIKey, "****") {
		out.APIKey = req.APIKey
	}
	if req.BillPath != "" {
		out.BillPath = req.BillPath
	}
	return out
}

// POST /api/settings/test-huozhi — 验证货殖配置。给了 bill_id 就真实拉取一次
// 账单，否则只做一次接口探测。不落库。
func (h *Handler) testHuozhiConnection(w http.ResponseWriter, r *http.Request) {
	var req huozhiTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, "invalid request body")
		return
	}
	eff := effectiveHuozhiSettings(h.cfg.GetHuozhiSettings(), req)
	if eff.Domain == "" || eff.APIKey == "" {
		jsonResp(w, 200, map[string]interface{}{
			"ok":    false,
			"error": "货殖未配置完整：需要域名与 API Key",
		})
		return
	}

	client := huozhi.New(eff)
	ids := parseBillIDs([]interface{}{req.BillID})
	if len(ids) == 0 {
		// 未指定账单：用 1 探测接口是否可达。不存在（404/账单为空）也算连通
		// 成功——说明域名与密钥都被接受了；只有鉴权失败或网络不可达才报错。
		_, err := client.GetBill(r.Context(), 1)
		switch {
		case err == nil:
			jsonResp(w, 200, map[string]interface{}{"ok": true, "message": "接口可用（探测账单 1）"})
		case errors.Is(err, huozhi.ErrNotFound):
			jsonResp(w, 200, map[string]interface{}{"ok": true, "message": "接口连通（账单 1 不存在）"})
		case errors.Is(err, huozhi.ErrUnauthorized):
			jsonResp(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
		default:
			jsonResp(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
		}
		return
	}

	bill, err := client.GetBill(r.Context(), ids[0])
	if err != nil {
		jsonResp(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	jsonResp(w, 200, map[string]interface{}{
		"ok":      true,
		"bill":    bill,
		"message": fmt.Sprintf("已读取账单 #%d：%s", bill.ID, nonemptyOr(bill.Description, bill.Remark, "（无描述）")),
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func nonemptyOr(vals ...string) string {
	return firstNonEmpty(vals...)
}
