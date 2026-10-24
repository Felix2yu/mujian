package db

import (
	"testing"
	"time"
)

// GetCalendarEvents 覆盖点：月内过滤、月末边界、跨年隔离、字段映射、升序、空结果。
func TestGetCalendarEvents(t *testing.T) {
	g := newTestDB(t)
	g.SetLocation(time.UTC) // 本测试按 UTC 日期构造记录，显式固定时区

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
	// 费用口径：sampleRecord 为 实付 180 + 其他 20 = 200，币种跟随实付。
	if evs[0].TotalCost != 200 || evs[0].CostCurrency != "CNY" {
		t.Errorf("cost not mapped: %v %q", evs[0].TotalCost, evs[0].CostCurrency)
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

// 日历页的月度开销依赖每条事件的 total_cost 与对应币种：实付优先，
// 没有实付时回落到票价；两者都未填时金额为 0、币种仍取票价币种。
func TestGetCalendarEventsCost(t *testing.T) {
	g := newTestDB(t)
	g.SetLocation(time.UTC)
	day := func(d int) int64 { return time.Date(2026, 8, d, 19, 30, 0, 0, time.UTC).Unix() }

	pay := 80.0
	other := 20.0
	paid := sampleRecord("cal-paid", day(1))
	paid.PayPrice, paid.PayPriceCurrency = &pay, "USD"
	paid.Price, paid.PriceCurrency = fltPtr(100), "CNY"
	paid.OtherCost, paid.OtherCostCurrency = &other, "USD"
	paid.HuozhiBillIDs = []int64{11, 11, 12} // 重复项由 handler 归一化，DB 原样保留
	if err := g.UpsertRecord(paid); err != nil {
		t.Fatalf("UpsertRecord paid: %v", err)
	}

	priced := sampleRecord("cal-priced", day(2))
	priced.PayPrice = nil
	priced.Price, priced.PriceCurrency = fltPtr(50), "JPY"
	priced.OtherCost = nil
	if err := g.UpsertRecord(priced); err != nil {
		t.Fatalf("UpsertRecord priced: %v", err)
	}

	unpriced := sampleRecord("cal-unpriced", day(3))
	unpriced.PayPrice, unpriced.Price, unpriced.OtherCost = nil, nil, nil
	unpriced.PriceCurrency = "HKD"
	if err := g.UpsertRecord(unpriced); err != nil {
		t.Fatalf("UpsertRecord unpriced: %v", err)
	}

	evs, err := g.GetCalendarEvents(2026, 8)
	if err != nil {
		t.Fatalf("GetCalendarEvents: %v", err)
	}
	byID := map[string]struct {
		cost float64
		cur  string
	}{}
	for _, e := range evs {
		byID[e.ID] = struct {
			cost float64
			cur  string
		}{e.TotalCost, e.CostCurrency}
	}
	want := map[string]struct {
		cost float64
		cur  string
	}{
		"cal-paid":     {100, "USD"},
		"cal-priced":   {50, "JPY"},
		"cal-unpriced": {0, "HKD"},
	}
	for id, w := range want {
		got, ok := byID[id]
		if !ok {
			t.Errorf("%s missing from calendar", id)
			continue
		}
		if got != w {
			t.Errorf("%s cost = %v %q, want %v %q", id, got.cost, got.cur, w.cost, w.cur)
		}
	}
	// 货殖账单 ID 必须随事件返回给 handler 解析（JSON 不下发）。
	for _, e := range evs {
		if e.ID != "cal-paid" {
			continue
		}
		if len(e.HuozhiBillIDs) != 3 || e.HuozhiBillIDs[0] != 11 ||
			e.HuozhiBillIDs[1] != 11 || e.HuozhiBillIDs[2] != 12 {
			t.Errorf("huozhi bill ids not mapped: %v", e.HuozhiBillIDs)
		}
	}
}

// 单行损坏不得拖垮整月数据：利用 SQLite 动态类型向 INTEGER 列
// 写入文本制造 Scan 失败，坏行应被跳过并记日志。
func TestGetCalendarEventsBadRowSkipped(t *testing.T) {
	g := newTestDB(t)
	r := sampleRecord("cal-good", time.Date(2026, 8, 10, 19, 0, 0, 0, time.UTC).Unix())
	if err := g.UpsertRecord(r); err != nil {
		t.Fatalf("UpsertRecord: %v", err)
	}
	if _, err := g.conn.Exec(`INSERT INTO records (id, name, rating) VALUES ('cal-corrupt', '脏数据', 'not-a-number')`); err != nil {
		t.Fatalf("insert corrupt row: %v", err)
	}

	evs, err := g.GetCalendarEvents(2026, 8)
	if err != nil {
		t.Fatalf("GetCalendarEvents: %v", err)
	}
	if len(evs) != 1 || evs[0].ID != "cal-good" {
		t.Errorf("corrupt row should be skipped, got %+v", evs)
	}
}

// 连接已关闭时查询必须报错而非静默返回空结果。
func TestGetCalendarEventsClosedDB(t *testing.T) {
	g := newTestDB(t)
	g.Close()
	if _, err := g.GetCalendarEvents(2026, 8); err == nil {
		t.Error("query on closed DB should error")
	}
}
