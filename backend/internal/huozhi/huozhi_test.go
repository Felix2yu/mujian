package huozhi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------- ClassifyBillRole ----------

func TestClassifyBillRole(t *testing.T) {
	cases := []struct {
		name     string
		category string
		cats     string // empty => default set
		want     string
	}{
		{"empty category defaults to ticket", "", "", "ticket"},
		{"keyword 演出 -> ticket", "演出", "", "ticket"},
		{"keyword 门票 -> ticket", "门票", "", "ticket"},
		{"non-ticket 交通 -> other", "交通", "", "other"},
		{"non-ticket 酒店 -> other", "酒店住宿", "", "other"},
		{"case-insensitive ascii keyword", "Concert Ticket", "concert", "ticket"},
		{"substring 电影票 contains 电影", "电影票", "电影", "ticket"},
		// custom categories override the default set entirely.
		{"custom 餐饮 matches", "餐饮", "餐饮,交通", "ticket"},
		{"custom excludes 演出", "演出", "餐饮,交通", "other"},
		// empty custom cats falls back to defaults.
		{"empty custom falls back to default", "演出", "   ", "ticket"},
		{"empty custom falls back, 交通 still other", "交通", "   ", "other"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := &Bill{Category: c.category}
			got := ClassifyBillRole(b, c.cats)
			if got != c.want {
				t.Fatalf("ClassifyBillRole(%q, %q) = %q, want %q", c.category, c.cats, got, c.want)
			}
		})
	}
}

func TestSettingsClassifyBillRole(t *testing.T) {
	s := Settings{TicketCategories: "餐饮"}
	b := &Bill{Category: "餐饮"}
	if got := s.ClassifyBillRole(b); got != "ticket" {
		t.Fatalf("Settings.ClassifyBillRole = %q, want ticket", got)
	}
}

// ---------- BillURL / path template ----------

func TestBillURL(t *testing.T) {
	cases := []struct {
		name     string
		domain   string
		billPath string
		id       int64
		want     string
	}{
		{"default path", "https://hz.test", "", 123, "https://hz.test/bills/123"},
		{"custom path", "https://hz.test", "/b/{id}", 7, "https://hz.test/b/7"},
		{"bare host gets https", "hz.test", "/bills/{id}", 1, "https://hz.test/bills/1"},
		{"empty domain -> empty url", "", "/bills/{id}", 1, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Settings{Domain: c.domain, BillPath: c.billPath}
			got := New(s).BillURL(c.id)
			if got != c.want {
				t.Fatalf("BillURL(%d) = %q, want %q", c.id, got, c.want)
			}
		})
	}
}

// ---------- tolerant Bill unmarshalling ----------

func TestBillUnmarshalTolerant(t *testing.T) {
	raw := []byte(`{
		"id": 5,
		"description": "海宁站-杭州站",
		"amount": "11",
		"currency": "CNY",
		"type": "expense",
		"date": "2021-05-02",
		"merchant": "",
		"remark": "x",
		"account": {"id": 1, "name": "支付宝"},
		"category": "交通"
	}`)
	var b Bill
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if b.ID != 5 {
		t.Errorf("id = %d, want 5", b.ID)
	}
	if b.Amount != 11 {
		t.Errorf("amount (string) = %v, want 11", b.Amount)
	}
	if b.TxDate != "2021-05-02" {
		t.Errorf("tx_date fallback = %q, want 2021-05-02", b.TxDate)
	}
	if b.Account != "支付宝" {
		t.Errorf("account object flattened = %q, want 支付宝", b.Account)
	}
	if b.Category != "交通" {
		t.Errorf("category = %q, want 交通", b.Category)
	}

	// account as plain string
	var b2 Bill
	if err := json.Unmarshal([]byte(`{"id":2,"amount":3.5,"account":"微信"}`), &b2); err != nil {
		t.Fatalf("unmarshal2: %v", err)
	}
	if b2.Account != "微信" {
		t.Errorf("account string = %q, want 微信", b2.Account)
	}
	if b2.Amount != 3.5 {
		t.Errorf("amount number = %v, want 3.5", b2.Amount)
	}
}

// ---------- Configured ----------

func TestClientConfigured(t *testing.T) {
	cases := []struct {
		name string
		s    Settings
		want bool
	}{
		{"disabled", Settings{Enabled: false, Domain: "https://x", APIKey: "k"}, false},
		{"no domain", Settings{Enabled: true, APIKey: "k"}, false},
		{"no key", Settings{Enabled: true, Domain: "https://x"}, false},
		{"ready", Settings{Enabled: true, Domain: "https://x", APIKey: "k"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.Configured(); got != c.want {
				t.Fatalf("Configured() = %v, want %v", got, c.want)
			}
		})
	}
}

// ---------- HTTP client against a mock 货殖 server ----------

func TestClientGetBill(t *testing.T) {
	bills := map[int64]map[string]interface{}{
		101: {"id": 101, "description": "票", "amount": 280, "currency": "CNY", "category": "演出"},
		102: {"id": 102, "description": "酒店", "amount": "300", "currency": "CNY", "category": "酒店"},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 401, "message": "unauthorized", "data": nil})
			return
		}
		idStr := strings.TrimPrefix(r.URL.Path, "/api/public/bills/")
		switch idStr {
		case "101":
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "message": "ok", "data": bills[101]})
		case "102":
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "message": "ok", "data": bills[102]})
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "message": "ok", "data": nil})
		}
	}))
	defer ts.Close()

	client := New(Settings{Enabled: true, Domain: ts.URL, APIKey: "test-key"})

	// success
	b, err := client.GetBill(context.Background(), 101)
	if err != nil {
		t.Fatalf("GetBill(101): %v", err)
	}
	if b.Amount != 280 {
		t.Errorf("amount = %v, want 280", b.Amount)
	}
	if b.URL != ts.URL+"/bills/101" {
		t.Errorf("URL = %q, want %q", b.URL, ts.URL+"/bills/101")
	}
	if b.Category != "演出" {
		t.Errorf("category = %q, want 演出", b.Category)
	}

	// amount returned as string is parsed
	b2, err := client.GetBill(context.Background(), 102)
	if err != nil {
		t.Fatalf("GetBill(102): %v", err)
	}
	if b2.Amount != 300 {
		t.Errorf("amount (string) = %v, want 300", b2.Amount)
	}

	// not found
	if _, err := client.GetBill(context.Background(), 999); err != ErrNotFound {
		t.Errorf("GetBill(999) err = %v, want ErrNotFound", err)
	}

	// unauthorized: use an id not cached by the good client (cache is keyed by
	// domain+id, and the good client already cached 101/102 on this domain).
	bad := New(Settings{Enabled: true, Domain: ts.URL, APIKey: "wrong"})
	if _, err := bad.GetBill(context.Background(), 777); err != ErrUnauthorized {
		t.Errorf("GetBill wrong key err = %v, want ErrUnauthorized", err)
	}

	// not configured
	nc := New(Settings{Enabled: false, Domain: ts.URL, APIKey: "test-key"})
	if _, err := nc.GetBill(context.Background(), 101); err != ErrNotConfigured {
		t.Errorf("GetBill not configured err = %v, want ErrNotConfigured", err)
	}

	// invalid id
	if _, err := client.GetBill(context.Background(), 0); err == nil {
		t.Errorf("GetBill(0) should error")
	}
}

func TestClientGetBillsOrderAndErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idStr := strings.TrimPrefix(r.URL.Path, "/api/public/bills/")
		if idStr == "1" {
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "message": "ok", "data": map[string]interface{}{"id": 1, "amount": 10, "currency": "CNY"}})
		} else {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "message": "ok", "data": nil})
		}
	}))
	defer ts.Close()

	client := New(Settings{Enabled: true, Domain: ts.URL, APIKey: "k"})

	// Order preserved; only id 1 resolves, ids 2/3 -> errors.
	bills, errs := client.GetBills(context.Background(), []int64{1, 2, 3})
	if len(bills) != 1 || bills[0].ID != 1 {
		t.Fatalf("bills = %+v, want single id 1", bills)
	}
	if len(errs) != 2 {
		t.Fatalf("errs = %v, want 2 (ids 2 and 3)", errs)
	}
	if _, ok := errs[2]; !ok {
		t.Errorf("errs missing id 2: %v", errs)
	}
	if _, ok := errs[3]; !ok {
		t.Errorf("errs missing id 3: %v", errs)
	}

	// Duplicate ids are skipped and recorded as an error (cache keyed by domain+id).
	dupBills, dupErrs := client.GetBills(context.Background(), []int64{1, 1})
	if len(dupBills) != 1 || dupBills[0].ID != 1 {
		t.Fatalf("dup bills = %+v, want single id 1", dupBills)
	}
	if len(dupErrs) != 1 {
		t.Fatalf("dup errs = %v, want 1 (duplicate id 1)", dupErrs)
	}
}
