package config

import (
	"strings"
	"testing"
)

func TestGetCostEnabledFields(t *testing.T) {
	// Empty config falls back to default ["price"].
	c := &Config{}
	fields := c.GetCostEnabledFields()
	if len(fields) != 1 || fields[0] != "price" {
		t.Errorf("default cost fields = %v, want [price]", fields)
	}

	// Explicit valid fields.
	c2 := &Config{CostEnabledFields: "price,pay_price,other_cost"}
	fields2 := c2.GetCostEnabledFields()
	if len(fields2) != 3 {
		t.Errorf("cost fields = %v, want 3 fields", fields2)
	}

	// Mixed valid and invalid — invalid filtered out.
	c3 := &Config{CostEnabledFields: "price,bogus,other_cost"}
	fields3 := c3.GetCostEnabledFields()
	if len(fields3) != 2 {
		t.Errorf("mixed cost fields = %v, want 2 valid", fields3)
	}

	// All invalid falls back to default.
	c4 := &Config{CostEnabledFields: "bogus1,bogus2"}
	fields4 := c4.GetCostEnabledFields()
	if len(fields4) != 1 || fields4[0] != "price" {
		t.Errorf("all-invalid cost fields = %v, want [price]", fields4)
	}

	// Whitespace handling.
	c5 := &Config{CostEnabledFields: " price , pay_price "}
	fields5 := c5.GetCostEnabledFields()
	if len(fields5) != 2 {
		t.Errorf("whitespace cost fields = %v, want 2", fields5)
	}
}

func TestGetHuozhiSettings(t *testing.T) {
	c := &Config{
		HuozhiEnabled:          true,
		HuozhiDomain:           "https://example.com",
		HuozhiAPIKey:           "key123",
		HuozhiBillPath:         "/bills/{id}",
		HuozhiTicketCategories: "京剧,昆曲",
	}
	s := c.GetHuozhiSettings()
	if !s.Enabled || s.Domain != "https://example.com" || s.APIKey != "key123" {
		t.Errorf("GetHuozhiSettings: %+v", s)
	}
	if s.TicketCategories != "京剧,昆曲" {
		t.Errorf("TicketCategories = %q", s.TicketCategories)
	}

	// Empty config
	s2 := (&Config{}).GetHuozhiSettings()
	if s2.Enabled || s2.Domain != "" {
		t.Errorf("empty HuozhiSettings: %+v", s2)
	}
}

func TestGetReminderConfig(t *testing.T) {
	c := &Config{
		ReminderMode:       "same_day",
		ReminderBeforeHours: 3,
		ReminderDailyHour:   10,
		ReminderDailyMinute: 30,
	}
	rc := c.GetReminderConfig()
	if rc.Mode != "same_day" || rc.BeforeHours != 3 || rc.DailyHour != 10 || rc.DailyMinute != 30 {
		t.Errorf("GetReminderConfig: %+v", rc)
	}

	// Empty config returns zero values
	rc2 := (&Config{}).GetReminderConfig()
	if rc2.Mode != "" || rc2.BeforeHours != 0 {
		t.Errorf("empty ReminderConfig: %+v", rc2)
	}
}

func TestNormalizeHuozhiDomain(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"https://example.com", "https://example.com"},
		{"example.com", "https://example.com"},
		{"  example.com  ", "https://example.com"},
		{"https://example.com/", "https://example.com"},
		{"http://x.com", "http://x.com"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := normalizeHuozhiDomain(tc.in); got != tc.want {
			t.Errorf("normalizeHuozhiDomain(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeBillPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/bills/{id}", "/bills/{id}"},
		{"bills/{id}", "/bills/{id}"},
		{"bills/", "/bills/{id}"},
		{"", "/bills/{id}"},
		{"bills/{id}/extra", "/bills/{id}/extra"},
	}
	for _, tc := range tests {
		if got := normalizeBillPath(tc.in); got != tc.want {
			t.Errorf("normalizeBillPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestKnownCostFields(t *testing.T) {
	if !knownCostFields["price"] {
		t.Error("price should be a known cost field")
	}
	if !knownCostFields["pay_price"] {
		t.Error("pay_price should be a known cost field")
	}
	if knownCostFields["bogus"] {
		t.Error("bogus should not be a known cost field")
	}
	if strings.Join([]string{"a", "b"}, ",") != "a,b" {
		t.Error("placeholder")
	}
}
