package db

import (
	"testing"
	"time"

	"mujian/internal/models"
)

// FindTimeOverlaps 的重叠口径：既有记录时段与目标时段必须真正相交，
// 首尾相接不算冲突；未填时长按 1 分钟兜底；已取消 / 软删除不计入。
func TestFindTimeOverlaps(t *testing.T) {
	db := newTestDB(t)
	base := time.Date(2026, 10, 5, 19, 30, 0, 0, time.Local)

	must := func(r models.Record) {
		t.Helper()
		if err := db.UpsertRecord(r); err != nil {
			t.Fatalf("UpsertRecord %s: %v", r.ID, err)
		}
	}
	// 19:30-21:30 的正常场次
	must(models.Record{ID: "c1", Name: "牡丹亭", Date: base.Unix(), Duration: 120, ActiveStatus: 0})
	// 已取消：不再占用购票时间
	must(models.Record{ID: "c2", Name: "取消场", Date: base.Add(30 * time.Minute).Unix(), Duration: 60, ActiveStatus: 2})
	// 想看：属于计划内，计入冲突
	must(models.Record{ID: "c3", Name: "想看场", Date: base.AddDate(0, 0, 1).Unix(), Duration: 90, ActiveStatus: 1})
	// 未填时长的历史记录
	must(models.Record{ID: "c4", Name: "零时长场", Date: base.Add(5 * time.Hour).Unix(), Duration: 0, ActiveStatus: 0})

	ids := func(cs []models.TimeConflict) map[string]bool {
		out := map[string]bool{}
		for _, c := range cs {
			out[c.ID] = true
		}
		return out
	}

	t.Run("时段相交", func(t *testing.T) {
		// 20:00-20:30 落在 c1 的 19:30-21:30 内
		got, err := db.FindTimeOverlaps(base.Add(30*time.Minute).Unix(), 30, "", 20)
		if err != nil {
			t.Fatalf("FindTimeOverlaps: %v", err)
		}
		m := ids(got)
		if !m["c1"] {
			t.Errorf("期望命中 c1，实际 %+v", got)
		}
		if m["c2"] {
			t.Errorf("已取消的 c2 不应计入冲突")
		}
	})

	t.Run("首尾相接不算冲突", func(t *testing.T) {
		// 目标 18:00-19:30 与 c1 在 19:30 正好衔接；目标 21:30-22:30 同理
		for _, start := range []int64{
			base.Add(-90 * time.Minute).Unix(),
			base.Add(120 * time.Minute).Unix(),
		} {
			got, err := db.FindTimeOverlaps(start, 90, "", 20)
			if err != nil {
				t.Fatalf("FindTimeOverlaps: %v", err)
			}
			if ids(got)["c1"] {
				t.Errorf("start=%d 与 c1 仅相接，不应算冲突", start)
			}
		}
	})

	t.Run("零时长按默认120分钟估算", func(t *testing.T) {
		// 目标未填时长：按 120 分钟估算区间，与 c1 开场同一时刻自然命中
		got, err := db.FindTimeOverlaps(base.Unix(), 0, "", 20)
		if err != nil {
			t.Fatalf("FindTimeOverlaps: %v", err)
		}
		if !ids(got)["c1"] {
			t.Errorf("同一开场时刻应命中 c1，实际 %+v", got)
		}
		// 目标提前 1 小时（18:30）且未填时长：按 120 估算为 18:30-20:30，
		// 与 c1 的 19:30-21:30 相交 —— 旧口径（1 分钟）在这里会漏报
		got, err = db.FindTimeOverlaps(base.Add(-60*time.Minute).Unix(), 0, "", 20)
		if err != nil {
			t.Fatalf("FindTimeOverlaps: %v", err)
		}
		if !ids(got)["c1"] {
			t.Errorf("留空时长应按 120 分钟估算并命中 c1，实际 %+v", got)
		}
		// 既有记录 c4 未填时长（同样按 120 估算为 00:30-02:30）
		got, err = db.FindTimeOverlaps(base.Add(5*time.Hour).Unix(), 5, "", 20)
		if err != nil {
			t.Fatalf("FindTimeOverlaps: %v", err)
		}
		if !ids(got)["c4"] {
			t.Errorf("应命中零时长的 c4，实际 %+v", got)
		}
	})

	t.Run("编辑时排除自身", func(t *testing.T) {
		got, err := db.FindTimeOverlaps(base.Unix(), 60, "c1", 20)
		if err != nil {
			t.Fatalf("FindTimeOverlaps: %v", err)
		}
		if ids(got)["c1"] {
			t.Errorf("exclude=c1 不应再返回自身")
		}
	})

	t.Run("软删除不计入", func(t *testing.T) {
		if err := db.DeleteRecord("c1"); err != nil {
			t.Fatalf("DeleteRecord: %v", err)
		}
		got, err := db.FindTimeOverlaps(base.Unix(), 60, "", 20)
		if err != nil {
			t.Fatalf("FindTimeOverlaps: %v", err)
		}
		if ids(got)["c1"] {
			t.Errorf("已删除的 c1 不应计入冲突")
		}
	})

	t.Run("次日想看场", func(t *testing.T) {
		next := base.AddDate(0, 0, 1)
		got, err := db.FindTimeOverlaps(next.Add(30*time.Minute).Unix(), 30, "", 20)
		if err != nil {
			t.Fatalf("FindTimeOverlaps: %v", err)
		}
		if !ids(got)["c3"] {
			t.Errorf("应命中次日的 c3，实际 %+v", got)
		}
	})

	// 典型场景：既有 19:00-21:00（时长 120），新建 18:00 开场。
	t.Run("18点开场与19-21场次", func(t *testing.T) {
		eve := time.Date(2026, 11, 8, 19, 0, 0, 0, time.Local)
		if err := db.UpsertRecord(models.Record{ID: "c5", Name: "晚场", Date: eve.Unix(), Duration: 120, ActiveStatus: 0}); err != nil {
			t.Fatalf("UpsertRecord c5: %v", err)
		}
		start18 := time.Date(2026, 11, 8, 18, 0, 0, 0, time.Local).Unix()

		// 时长留空 → 按默认 120 分钟估算为 18:00-20:00，与 19:00-21:00 相交，触发
		got, err := db.FindTimeOverlaps(start18, 0, "", 20)
		if err != nil {
			t.Fatalf("FindTimeOverlaps: %v", err)
		}
		if !ids(got)["c5"] {
			t.Errorf("时长留空应按 120 分钟估算并命中 19:00 场次，实际 %+v", got)
		}

		// 时长 60 分钟 → 18:00-19:00 正好在 19:00 衔接，仍不算冲突
		got, err = db.FindTimeOverlaps(start18, 60, "", 20)
		if err != nil {
			t.Fatalf("FindTimeOverlaps: %v", err)
		}
		if ids(got)["c5"] {
			t.Errorf("18:00-19:00 与 19:00 仅相接，不应冲突，实际 %+v", got)
		}

		// 时长 90 分钟 → 18:00-19:30 与 19:00-21:00 相交，触发
		got, err = db.FindTimeOverlaps(start18, 90, "", 20)
		if err != nil {
			t.Fatalf("FindTimeOverlaps: %v", err)
		}
		if !ids(got)["c5"] {
			t.Errorf("18:00-19:30 应与 19:00-21:00 冲突，实际 %+v", got)
		}
	})
}
