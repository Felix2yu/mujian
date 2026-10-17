package handlers

import (
	"mujian/internal/models"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) should be 1")
	}
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) should be 0")
	}
}

func TestDryRunDefaultTrue(t *testing.T) {
	// nil → true (default to dry run)
	if !dryRunDefaultTrue(nil) {
		t.Error("nil should default to true")
	}
	v := true
	if !dryRunDefaultTrue(&v) {
		t.Error("true should return true")
	}
	v2 := false
	if dryRunDefaultTrue(&v2) {
		t.Error("false should return false")
	}
}

func TestWantsDryRun(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"", false},
		{"dry_run=1", true},
		{"dry_run=true", true},
		{"dry_run=yes", true},
		{"dry_run=0", false},
		{"dry_run=false", false},
		{"dry_run=no", false},
	}
	for _, tc := range tests {
		r := httptest.NewRequest("GET", "/?"+tc.query, nil)
		if got := wantsDryRun(r); got != tc.want {
			t.Errorf("wantsDryRun(%q) = %v, want %v", tc.query, got, tc.want)
		}
	}
	// Also test with POST and different paths
	r := httptest.NewRequest("POST", "/api/records?dry_run=true", nil)
	if !wantsDryRun(r) {
		t.Error("POST with dry_run=true should return true")
	}
	r2 := httptest.NewRequest("DELETE", "/api/records/123?dry_run=1", nil)
	if !wantsDryRun(r2) {
		t.Error("DELETE with dry_run=1 should return true")
	}
	// Uppercase via URL parsing normalizes query keys, but value trimming+lowering works
	r3 := httptest.NewRequest("GET", "/?dry_run=Yes", nil)
	if !wantsDryRun(r3) {
		t.Error("dry_run=Yes should return true")
	}
}

func TestJSONResp(t *testing.T) {
	rec := httptest.NewRecorder()
	jsonResp(rec, 200, map[string]string{"ok": "true"})
	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestJSONErr(t *testing.T) {
	rec := httptest.NewRecorder()
	jsonErr(rec, 400, "bad request")
	if rec.Code != 400 {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "bad request") {
		t.Errorf("body should contain error message: %s", rec.Body.String())
	}
}

func TestNormalizeRecord(t *testing.T) {
	// normalizeRecord fills DateText from Date when DateText is empty
	req := &models.RecordRequest{Date: 1755000000}
	normalizeRecord(req)
	if req.DateText == "" {
		t.Error("DateText should be filled from Date")
	}
	if req.PriceCurrency != "CNY" {
		t.Errorf("PriceCurrency should default to CNY, got %q", req.PriceCurrency)
	}
	if req.PayPriceCurrency != "CNY" {
		t.Errorf("PayPriceCurrency should default to CNY, got %q", req.PayPriceCurrency)
	}
	if req.OtherCostCurrency != "CNY" {
		t.Errorf("OtherCostCurrency should default to CNY, got %q", req.OtherCostCurrency)
	}

	// DateText provided but Date is 0 → parse DateText into Date
	req2 := &models.RecordRequest{DateText: "2026-08-22 19:30"}
	normalizeRecord(req2)
	if req2.Date == 0 {
		t.Error("Date should be parsed from DateText")
	}

	// Both empty → no change
	req3 := &models.RecordRequest{}
	normalizeRecord(req3)
	if req3.Date != 0 {
		t.Error("Date should remain 0 when both are empty")
	}
}
