package db

import (
	"testing"
	"time"
)

// GetCalendarEvents 覆盖点：月内过滤、月末边界、跨年隔离、字段映射、升序、空结果。
func TestGetCalendarEvents(t *testing.T) {
	g := newTestDB(t) // 默认时区 UTC

	mk := func(id string, y int, m time.Month, d int, status int, thumb string) {
		t.Helper()
		r := sampleRecord(id, time.Date(y, m, d, 19, 30, 0, 0, time.UTC).Unix())
		r.ActiveStatus = status
		r.CoverFile = "covers/" + id + ".avif"
		r.CoverThumb = thumb
		if err := g.UpsertRecord(r); err != nil {
			t.Fatalf("UpsertRecord %s: %v", id, err)
		}
	}

	mk("cal-jul-first", 2026, 7, 1, 1, "covers/t-a.avif")
	mk("cal-jul-last", 2026, 7, 31, 2, "") // 月末最后一天必须落在 7 月窗口内；无缩略图走 coverFile 兜底
	mk("cal-aug", 2026, 8, 1, 0, "covers/t-c.avif")
	mk("cal-dec", 2026, 12, 31, 0, "")
	mk("cal-next-year", 2027, 1, 1, 3, "")

	evs, err := g.GetCalendarEvents(2026, 7)
	if err != nil {
		t.Fatalf("GetCalendarEvents: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("july: want 2 events, got %d: %+v", len(evs), evs)
	}
	// 升序：月初记录在前。
	if evs[0].ID != "cal-jul-first" || evs[1].ID != "cal-jul-last" {
		t.Errorf("order wrong: %s, %s", evs[0].ID, evs[1].ID)
	}
	// 字段映射：coverThumb / active_status 必须与库中一致。
	if evs[0].CoverThumb != "covers/t-a.avif" {
		t.Errorf("coverThumb not mapped: %q", evs[0].CoverThumb)
	}
	if evs[0].ActiveStatus != 1 || evs[1].ActiveStatus != 2 {
		t.Errorf("activeStatus not mapped: %+v", evs)
	}
	if evs[1].CoverThumb != "" {
		t.Errorf("empty thumb should stay empty, got %q", evs[1].CoverThumb)
	}

	// 跨年隔离：12 月不含次年数据；次年 1 月不含上年数据。
	if evs, _ = g.GetCalendarEvents(2026, 12); len(evs) != 1 || evs[0].ID != "cal-dec" {
		t.Errorf("december: %+v", evs)
	}
	if evs, _ = g.GetCalendarEvents(2027, 1); len(evs) != 1 || evs[0].ID != "cal-next-year" {
		t.Errorf("january next year: %+v", evs)
	}

	// 空月份：返回空切片而非 nil，保证 JSON 序列化为 []。
	evs, err = g.GetCalendarEvents(2030, 5)
	if err != nil {
		t.Fatalf("empty month: %v", err)
	}
	if evs == nil || len(evs) != 0 {
		t.Errorf("want empty non-nil slice, got %#v", evs)
	}
}
