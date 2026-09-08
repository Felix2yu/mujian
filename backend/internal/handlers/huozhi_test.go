package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"mujian/internal/config"
	"mujian/internal/models"
)

// billGroupJSON mirrors the breakdown bucket returned by the bills endpoints.
type billGroupJSON struct {
	Role     string                   `json:"role"`
	Label    string                   `json:"label"`
	Total    float64                  `json:"total"`
	Currency string                   `json:"currency"`
	Mixed    bool                     `json:"mixed_currency"`
	Bills    []map[string]interface{} `json:"bills"`
}

type recordBillsJSON struct {
	Enabled           bool                   `json:"enabled"`
	Ready             bool                   `json:"ready"`
	BillIDs           []int64                `json:"bill_ids"`
	Bills             []map[string]interface{} `json:"bills"`
	Total             float64                `json:"total"`
	Currency          string                 `json:"currency"`
	MixedCurrency     bool                   `json:"mixed_currency"`
	Breakdown         []billGroupJSON        `json:"breakdown"`
	LocalTotal        float64                `json:"local_total"`
	LocalCurrency     string                 `json:"local_currency"`
	EffectiveTotal    float64                `json:"effective_total"`
	EffectiveCurrency string                 `json:"effective_currency"`
	EffectiveSource   string                 `json:"effective_source"`
	Errors            map[string]string      `json:"errors"`
	Error             string                 `json:"error"`
}

// huozhiMock stands up a fake 货殖 public-bills API. When forceStatus is non-zero
// every request is answered with that status; otherwise bills[id] is returned.
func huozhiMock(t *testing.T, apiKey string, bills map[int64]map[string]interface{}, forceStatus int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/public/bills/", func(w http.ResponseWriter, r *http.Request) {
		if forceStatus != 0 {
			w.WriteHeader(forceStatus)
			json.NewEncoder(w).Encode(map[string]interface{}{"code": forceStatus, "message": "forced", "data": nil})
			return
		}
		if r.Header.Get("X-API-Key") != apiKey {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 401, "message": "unauthorized", "data": nil})
			return
		}
		idStr := strings.TrimPrefix(r.URL.Path, "/api/public/bills/")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		data, ok := bills[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "message": "ok", "data": nil})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "message": "ok", "data": data})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func huozhiMutate(mockURL, apiKey string, enabled bool) func(*config.Config) {
	return func(c *config.Config) {
		c.HuozhiEnabled = enabled
		c.HuozhiDomain = mockURL
		c.HuozhiAPIKey = apiKey
		c.HuozhiBillPath = "/bills/{id}"
	}
}

// ---------- GET /api/records/{id}/bills ----------

func TestHuozhiRecordBills(t *testing.T) {
	bills := map[int64]map[string]interface{}{
		101: {"id": 101, "description": "门票", "amount": 280, "currency": "CNY", "category": "演出"},
		102: {"id": 102, "description": "酒店", "amount": 300, "currency": "CNY", "category": "酒店"},
	}

	t.Run("no bill ids", func(t *testing.T) {
		ts, _, db, _ := newTestServer(t, nil)
		_ = db.UpsertRecord(models.Record{ID: "r0", Name: "无账单", Date: time.Now().Unix(), PayPriceCurrency: "CNY"})
		res, b := doJSON(t, "GET", ts.URL+"/api/records/r0/bills", nil)
		expectStatus(t, res, 200, "no bill ids")
		var out recordBillsJSON
		decodeResp(t, b, &out)
		if out.Enabled || out.Ready {
			t.Errorf("expected disabled/!ready, got enabled=%v ready=%v", out.Enabled, out.Ready)
		}
		if out.EffectiveSource != "local" {
			t.Errorf("effective_source = %q, want local", out.EffectiveSource)
		}
		if len(out.BillIDs) != 0 {
			t.Errorf("bill_ids should be empty, got %v", out.BillIDs)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		mock := huozhiMock(t, "k", bills, 0)
		ts, _, db, _ := newTestServer(t, huozhiMutate(mock.URL, "k", false))
		_ = db.UpsertRecord(models.Record{ID: "r1", Name: "x", HuozhiBillIDs: []int64{101}, Date: time.Now().Unix()})
		res, b := doJSON(t, "GET", ts.URL+"/api/records/r1/bills", nil)
		expectStatus(t, res, 200, "disabled")
		var out recordBillsJSON
		decodeResp(t, b, &out)
		if out.Error == "" || !strings.Contains(out.Error, "未启用") {
			t.Errorf("expected 未启用 error, got %q", out.Error)
		}
	})

	t.Run("enabled but unconfigured", func(t *testing.T) {
		mock := huozhiMock(t, "k", bills, 0)
		ts, _, db, _ := newTestServer(t, func(c *config.Config) {
			c.HuozhiEnabled = true
			c.HuozhiDomain = mock.URL
			c.HuozhiAPIKey = "" // 缺 key
		})
		_ = db.UpsertRecord(models.Record{ID: "r2", Name: "x", HuozhiBillIDs: []int64{101}, Date: time.Now().Unix()})
		res, b := doJSON(t, "GET", ts.URL+"/api/records/r2/bills", nil)
		expectStatus(t, res, 200, "unconfigured")
		var out recordBillsJSON
		decodeResp(t, b, &out)
		if !strings.Contains(out.Error, "未配置完整") {
			t.Errorf("expected 未配置完整 error, got %q", out.Error)
		}
	})

	t.Run("success with breakdown", func(t *testing.T) {
		mock := huozhiMock(t, "k", bills, 0)
		ts, _, db, _ := newTestServer(t, huozhiMutate(mock.URL, "k", true))
		_ = db.UpsertRecord(models.Record{ID: "r3", Name: "x", HuozhiBillIDs: []int64{101, 102}, Date: time.Now().Unix(), PayPriceCurrency: "CNY"})
		res, b := doJSON(t, "GET", ts.URL+"/api/records/r3/bills", nil)
		expectStatus(t, res, 200, "success")
		var out recordBillsJSON
		decodeResp(t, b, &out)
		if out.EffectiveSource != "huozhi" {
			t.Errorf("effective_source = %q, want huozhi", out.EffectiveSource)
		}
		if out.Total != 580 {
			t.Errorf("total = %v, want 580", out.Total)
		}
		if len(out.Bills) != 2 {
			t.Fatalf("bills = %d, want 2", len(out.Bills))
		}
		if len(out.Breakdown) != 2 {
			t.Fatalf("breakdown groups = %d, want 2 (ticket + other)", len(out.Breakdown))
		}
		byRole := map[string]billGroupJSON{}
		for _, g := range out.Breakdown {
			byRole[g.Role] = g
		}
		if g, ok := byRole["ticket"]; !ok || g.Total != 280 {
			t.Errorf("ticket group = %+v, want total 280", g)
		}
		if g, ok := byRole["other"]; !ok || g.Total != 300 {
			t.Errorf("other group = %+v, want total 300", g)
		}
	})

	t.Run("fetch error falls back to local", func(t *testing.T) {
		mock := huozhiMock(t, "k", nil, http.StatusUnauthorized) // 所有请求 401
		ts, _, db, _ := newTestServer(t, huozhiMutate(mock.URL, "k", true))
		_ = db.UpsertRecord(models.Record{ID: "r4", Name: "x", HuozhiBillIDs: []int64{101}, Date: time.Now().Unix()})
		res, b := doJSON(t, "GET", ts.URL+"/api/records/r4/bills", nil)
		expectStatus(t, res, 200, "fetch error")
		var out recordBillsJSON
		decodeResp(t, b, &out)
		if out.EffectiveSource != "local" {
			t.Errorf("expected fallback to local, got %q", out.EffectiveSource)
		}
		if len(out.Errors) == 0 {
			t.Errorf("expected an error entry for the failed fetch, got %v", out.Errors)
		}
	})

	t.Run("record not found", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, nil)
		res, _ := doJSON(t, "GET", ts.URL+"/api/records/missing/bills", nil)
		expectStatus(t, res, 404, "missing record")
	})
}

// ---------- POST /api/huozhi/bills ----------

func TestHuozhiLookupBills(t *testing.T) {
	bills := map[int64]map[string]interface{}{
		101: {"id": 101, "description": "门票", "amount": 280, "currency": "CNY", "category": "演出"},
		102: {"id": 102, "description": "酒店", "amount": 300, "currency": "CNY", "category": "酒店"},
	}

	t.Run("empty ids", func(t *testing.T) {
		mock := huozhiMock(t, "k", bills, 0)
		ts, _, _, _ := newTestServer(t, huozhiMutate(mock.URL, "k", true))
		res, b := doJSON(t, "POST", ts.URL+"/api/huozhi/bills", map[string]interface{}{"ids": []int64{}})
		expectStatus(t, res, 200, "empty ids")
		var out map[string]interface{}
		decodeResp(t, b, &out)
		if arr, ok := out["bills"].([]interface{}); !ok || len(arr) != 0 {
			t.Errorf("bills should be empty, got %v", out["bills"])
		}
	})

	t.Run("disabled", func(t *testing.T) {
		mock := huozhiMock(t, "k", bills, 0)
		ts, _, _, _ := newTestServer(t, huozhiMutate(mock.URL, "k", false))
		res, _ := doJSON(t, "POST", ts.URL+"/api/huozhi/bills", map[string]interface{}{"ids": []int64{101}})
		expectStatus(t, res, 400, "disabled")
	})

	t.Run("success", func(t *testing.T) {
		mock := huozhiMock(t, "k", bills, 0)
		ts, _, _, _ := newTestServer(t, huozhiMutate(mock.URL, "k", true))
		res, b := doJSON(t, "POST", ts.URL+"/api/huozhi/bills", map[string]interface{}{"ids": []int64{101, 102}})
		expectStatus(t, res, 200, "success")
		var out struct {
			Bills     []map[string]interface{} `json:"bills"`
			Total     float64                  `json:"total"`
			Breakdown []billGroupJSON          `json:"breakdown"`
		}
		decodeResp(t, b, &out)
		if len(out.Bills) != 2 {
			t.Fatalf("bills = %d, want 2", len(out.Bills))
		}
		if out.Total != 580 {
			t.Errorf("total = %v, want 580", out.Total)
		}
		if len(out.Breakdown) != 2 {
			t.Errorf("breakdown groups = %d, want 2", len(out.Breakdown))
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, nil)
		res, _ := doJSON(t, "POST", ts.URL+"/api/huozhi/bills", []byte("{"))
		expectStatus(t, res, 400, "invalid body")
	})
}

// ---------- POST /api/settings/test-huozhi ----------

func TestHuozhiTestConnection(t *testing.T) {
	bills := map[int64]map[string]interface{}{
		101: {"id": 101, "description": "门票", "amount": 280, "currency": "CNY", "category": "演出"},
	}

	t.Run("missing domain/key", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, func(c *config.Config) {
			c.HuozhiEnabled = true // enabled but no domain/key
		})
		res, b := doJSON(t, "POST", ts.URL+"/api/settings/test-huozhi", map[string]interface{}{})
		expectStatus(t, res, 200, "missing")
		var out map[string]interface{}
		decodeResp(t, b, &out)
		if out["ok"] == true {
			t.Errorf("expected ok:false when domain/key missing, got %v", out)
		}
	})

	t.Run("reachable probe (bill 1 missing)", func(t *testing.T) {
		mock := huozhiMock(t, "k", bills, 0)
		ts, _, _, _ := newTestServer(t, huozhiMutate(mock.URL, "k", true))
		res, b := doJSON(t, "POST", ts.URL+"/api/settings/test-huozhi", map[string]interface{}{})
		expectStatus(t, res, 200, "reachable")
		var out map[string]interface{}
		decodeResp(t, b, &out)
		if out["ok"] != true {
			t.Errorf("expected ok:true for reachable endpoint, got %v", out)
		}
	})

	t.Run("bill fetch", func(t *testing.T) {
		mock := huozhiMock(t, "k", bills, 0)
		ts, _, _, _ := newTestServer(t, huozhiMutate(mock.URL, "k", true))
		res, b := doJSON(t, "POST", ts.URL+"/api/settings/test-huozhi", map[string]interface{}{"bill_id": 101})
		expectStatus(t, res, 200, "bill fetch")
		var out map[string]interface{}
		decodeResp(t, b, &out)
		if out["ok"] != true {
			t.Errorf("expected ok:true, got %v", out)
		}
		bill, ok := out["bill"].(map[string]interface{})
		if !ok || int(bill["id"].(float64)) != 101 {
			t.Errorf("bill payload wrong: %v", out["bill"])
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, nil)
		res, _ := doJSON(t, "POST", ts.URL+"/api/settings/test-huozhi", []byte("{"))
		expectStatus(t, res, 400, "invalid body")
	})
}

// ---------- parseBillIDs ----------

func TestParseBillIDs(t *testing.T) {
	cases := []struct {
		name string
		in   []interface{}
		want []int64
	}{
		{"floats", []interface{}{float64(1), float64(2)}, []int64{1, 2}},
		{"strings", []interface{}{"3", "4"}, []int64{3, 4}},
		{"json.Number", []interface{}{json.Number("5")}, []int64{5}},
		{"mixed", []interface{}{"6", float64(7), json.Number("8")}, []int64{6, 7, 8}},
		{"dedup", []interface{}{float64(1), "1"}, []int64{1}},
		{"invalid skipped", []interface{}{"abc", float64(2)}, []int64{2}},
		{"zero/negative skipped", []interface{}{float64(0), float64(-3), float64(4)}, []int64{4}},
		{"empty", []interface{}{}, []int64{}},
		{"garbage skipped", []interface{}{true, nil, "x"}, []int64{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseBillIDs(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("parseBillIDs(%v) = %v, want %v", c.in, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("parseBillIDs(%v)[%d] = %d, want %d", c.in, i, got[i], c.want[i])
				}
			}
		})
	}
}
