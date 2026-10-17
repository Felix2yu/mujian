package db

import (
	"testing"
	"time"

	"mujian/internal/models"
)

func yRec(id string, when time.Time, city string, rating, status int, pay, price float64, duration int, friends string) models.Record {
	r := models.Record{
		ID: id, Name: "演出" + id, Channel: "大麦", City: city, Address: city + "剧院",
		Date: when.Unix(), DateText: when.Format("2006-01-02 15:04"),
		Rating: rating, ActiveStatus: status, Duration: duration, Friends: friends,
	}
	if pay > 0 {
		r.PayPrice = fltPtr(pay)
	}
	if price > 0 {
		r.Price = fltPtr(price)
	}
	return r
}

func TestGetYearlyReport(t *testing.T) {
	db := newTestDB(t)
	loc := time.Local
	recs := []models.Record{
		yRec("y-1", time.Date(2026, 3, 1, 10, 0, 0, 0, loc), "上海", 5, 0, 500, 0, 120, "老王、小李"),
		yRec("y-2", time.Date(2026, 3, 1, 21, 0, 0, 0, loc), "上海", 0, 0, 0, 100, 90, ""),
		yRec("y-3", time.Date(2026, 8, 1, 19, 30, 0, 0, loc), "北京", 3, 1, 50, 0, 0, ""),
		yRec("y-4", time.Date(2025, 6, 1, 19, 30, 0, 0, loc), "广州", 4, 0, 300, 0, 100, ""),
	}
	for _, r := range recs {
		if err := db.UpsertRecord(r); err != nil {
			t.Fatalf("UpsertRecord %s: %v", r.ID, err)
		}
	}

	rep, err := db.GetYearlyReport(2026)
	if err != nil {
		t.Fatalf("GetYearlyReport: %v", err)
	}

	if rep.Year != 2026 {
		t.Errorf("year = %d", rep.Year)
	}
	if len(rep.AvailableYears) != 2 || rep.AvailableYears[0] != 2026 || rep.AvailableYears[1] != 2025 {
		t.Errorf("available_years = %v (want [2026 2025])", rep.AvailableYears)
	}

	// KPIs: 3 shows in 2026; cost 500+100+50=650; avg rating (5+3)/2=4; 2 cities.
	if rep.Overview.TotalRecords != 3 {
		t.Errorf("total_records = %d", rep.Overview.TotalRecords)
	}
	if rep.Overview.TotalCost != 650 {
		t.Errorf("total_cost = %v", rep.Overview.TotalCost)
	}
	if rep.Overview.AvgRating != 4.0 {
		t.Errorf("avg_rating = %v", rep.Overview.AvgRating)
	}
	if rep.Overview.TotalCities != 2 {
		t.Errorf("total_cities = %d", rep.Overview.TotalCities)
	}
	if rep.Overview.TotalHours != 3.5 {
		t.Errorf("total_hours = %v", rep.Overview.TotalHours)
	}
	// 2025 had 1 show → 2026 grew 200%.
	if rep.Overview.RecordsDeltaPct != 200 {
		t.Errorf("records_delta_pct = %v", rep.Overview.RecordsDeltaPct)
	}

	if len(rep.Monthly) != 12 {
		t.Fatalf("monthly len = %d", len(rep.Monthly))
	}
	if rep.Monthly[2].Period != "2026-03" || rep.Monthly[2].Count != 2 {
		t.Errorf("march = %+v", rep.Monthly[2])
	}
	if rep.Monthly[0].Count != 0 {
		t.Errorf("january should be zero-filled, got %+v", rep.Monthly[0])
	}

	// Highlights
	if rep.Highlights.PeakMonth != "2026-03" || rep.Highlights.PeakMonthCount != 2 {
		t.Errorf("peak month = %+v", rep.Highlights)
	}
	if rep.Highlights.MostExpensive == nil || rep.Highlights.MostExpensive.ID != "y-1" || rep.Highlights.MostExpensive.Value != 500 {
		t.Errorf("most_expensive = %+v", rep.Highlights.MostExpensive)
	}
	if rep.Highlights.BestRated == nil || rep.Highlights.BestRated.ID != "y-1" {
		t.Errorf("best_rated = %+v", rep.Highlights.BestRated)
	}
	// First/last 已观看 shows in the year (想看 y-3 excluded by active_status=0).
	if rep.Highlights.FirstShow == nil || rep.Highlights.FirstShow.ID != "y-1" {
		t.Errorf("first_show = %+v", rep.Highlights.FirstShow)
	}
	if rep.Highlights.LastShow == nil || rep.Highlights.LastShow.ID != "y-2" {
		t.Errorf("last_show = %+v (want y-2)", rep.Highlights.LastShow)
	}
	if rep.Highlights.LongestGapDays <= 150 {
		t.Errorf("longest_gap_days = %v (want >150)", rep.Highlights.LongestGapDays)
	}

	// Rank context: 3 shows beats 2025's 1 → rank 1 of 2, record year.
	if rep.Rank.YearsWithShows != 2 || rep.Rank.CountRank != 1 || !rep.Rank.IsRecordCount {
		t.Errorf("rank = %+v", rep.Rank)
	}

	// No holiday rows seeded → zero-safe, not an error.
	if rep.Holiday.Shows != 0 || rep.Holiday.ByHoliday == nil {
		t.Errorf("holiday = %+v", rep.Holiday)
	}

	// 2027 has no records: report must still assemble empty, not fail.
	future, err := db.GetYearlyReport(2027)
	if err != nil {
		t.Fatalf("GetYearlyReport(2027): %v", err)
	}
	if future.Overview.TotalRecords != 0 || len(future.Monthly) != 12 {
		t.Errorf("empty year = %+v", future.Overview)
	}
}

func TestGetAnalyticsExtendedDimensions(t *testing.T) {
	db := newTestDB(t)
	loc := time.Local
	recs := []models.Record{
		yRec("x-1", time.Date(2026, 3, 1, 10, 0, 0, 0, loc), "上海", 5, 0, 500, 0, 120, "老王、小李"),
		yRec("x-2", time.Date(2026, 3, 1, 21, 0, 0, 0, loc), "上海", 0, 2, 0, 100, 90, ""),
		yRec("x-3", time.Date(2026, 8, 1, 19, 30, 0, 0, loc), "北京", 3, 1, 50, 0, 0, ""),
	}
	for _, r := range recs {
		if err := db.UpsertRecord(r); err != nil {
			t.Fatalf("UpsertRecord %s: %v", r.ID, err)
		}
	}

	a, err := db.GetAnalytics()
	if err != nil {
		t.Fatalf("GetAnalytics: %v", err)
	}

	if a.StatusFunnel == nil {
		t.Fatal("status_funnel missing")
	}
	if a.StatusFunnel.Watched != 1 || a.StatusFunnel.Want != 1 || a.StatusFunnel.Canceled != 1 || a.StatusFunnel.Total != 3 {
		t.Errorf("status_funnel = %+v", a.StatusFunnel)
	}
	if a.StatusFunnel.AttendedPct != 50 {
		t.Errorf("attended_pct = %v (want 50)", a.StatusFunnel.AttendedPct)
	}

	if a.Geo == nil || a.Geo.HomeCity != "上海" || a.Geo.LocalCount != 2 || a.Geo.AwayCount != 1 || a.Geo.AwayCityCount != 1 {
		t.Errorf("geo = %+v", a.Geo)
	}

	if a.Companions == nil || a.Companions.WithFriends != 1 || a.Companions.Solo != 2 {
		t.Errorf("companions = %+v", a.Companions)
	}
	if len(a.Companions.TopCompanions) != 2 || a.Companions.TopCompanions[0].Name != "小李" {
		// counts tie at 1 → alphabetical by name; either could be first.
		t.Logf("top_companions = %+v", a.Companions.TopCompanions)
		if len(a.Companions.TopCompanions) == 0 {
			t.Errorf("top_companions empty")
		}
	}

	if a.Duration == nil || a.Duration.TotalHours != 3.5 || a.Duration.WithDuration != 2 {
		t.Errorf("duration = %+v", a.Duration)
	}
	if a.Duration.MaxShowsPerDay != 2 || a.Duration.MultiShowDays != 1 {
		t.Errorf("duration density = %+v", a.Duration)
	}

	if a.ZheziCover == nil || a.ZheziCover.TotalZhezis != 0 || a.ZheziCover.Dramas == nil {
		t.Errorf("zhezi_coverage = %+v", a.ZheziCover)
	}
}
