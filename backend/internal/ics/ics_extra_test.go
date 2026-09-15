package ics

import (
	"mujian/internal/models"
	"strings"
	"testing"
	"time"
)

func TestFilterByStatus(t *testing.T) {
	recs := []models.Record{
		{ID: "r1", ActiveStatus: models.StatusNormal},
		{ID: "r2", ActiveStatus: models.StatusWantWatch},
		{ID: "r3", ActiveStatus: models.StatusNormal},
		{ID: "r4", ActiveStatus: models.StatusCancelled},
	}

	// Nil allowed → no filter
	result := FilterByStatus(recs, nil)
	if len(result) != 4 {
		t.Errorf("nil allowed should return all, got %d", len(result))
	}

	// Empty allowed → no filter
	result = FilterByStatus(recs, []int{})
	if len(result) != 4 {
		t.Errorf("empty allowed should return all, got %d", len(result))
	}

	// Filter for StatusNormal only
	result = FilterByStatus(recs, []int{models.StatusNormal})
	if len(result) != 2 {
		t.Errorf("StatusNormal filter = %d, want 2", len(result))
	}
	for _, r := range result {
		if r.ActiveStatus != models.StatusNormal {
			t.Errorf("expected StatusNormal, got %d", r.ActiveStatus)
		}
	}

	// Filter for multiple statuses
	result = FilterByStatus(recs, []int{models.StatusNormal, models.StatusCancelled})
	if len(result) != 3 {
		t.Errorf("multi-status filter = %d, want 3", len(result))
	}

	// Filter for status that no record has
	result = FilterByStatus(recs, []int{999})
	if len(result) != 0 {
		t.Errorf("no-match filter = %d, want 0", len(result))
	}
}

func TestStatusAllowed(t *testing.T) {
	if !StatusAllowed(models.StatusNormal, []int{models.StatusNormal, models.StatusWantWatch}) {
		t.Error("StatusNormal should be allowed")
	}
	if StatusAllowed(models.StatusCancelled, []int{models.StatusNormal}) {
		t.Error("StatusCancelled should not be allowed")
	}
	if StatusAllowed(0, nil) {
		t.Error("nil allowed should return false")
	}
	if StatusAllowed(0, []int{}) {
		t.Error("empty allowed should return false")
	}
}

func TestGenerateTodos(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	rec := models.Record{
		ID:   "rec-todo",
		Name: "提醒演出",
		Date: time.Date(2026, 9, 11, 19, 30, 0, 0, loc).Unix(),
	}

	out := GenerateTodos([]models.Record{rec}, loc, nil, nil)
	if !strings.Contains(out, "BEGIN:VCALENDAR") {
		t.Error("GenerateTodos should produce a VCALENDAR")
	}
	if strings.Count(out, "BEGIN:VTODO") != 1 {
		t.Errorf("expected 1 VTODO, got %d", strings.Count(out, "BEGIN:VTODO"))
	}
	if !strings.Contains(out, "UID:rec-todo@mujian") {
		t.Error("UID should contain record id")
	}
	if !strings.Contains(out, "SUMMARY:提醒演出") {
		t.Error("SUMMARY should contain record name")
	}
}

func TestGenerateTodosEmpty(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	out := GenerateTodos(nil, loc, nil, nil)
	if !strings.HasPrefix(out, "BEGIN:VCALENDAR") {
		t.Error("empty GenerateTodos should produce a VCALENDAR header")
	}
	if strings.Contains(out, "BEGIN:VTODO") {
		t.Error("empty GenerateTodos should not contain VTODO")
	}
}
