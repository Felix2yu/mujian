package db

import (
	"testing"

	"mujian/internal/models"
)

// 节假日表的三个写入口径：内置种子只补缺、外部刷新整年替换、按年前缀查询。
func TestHolidaySeedAndReplace(t *testing.T) {
	db := newTestDB(t)

	seed := []models.HolidayDay{
		{Date: "2026-10-01", Name: "国庆节", IsOffDay: true},
		{Date: "2026-09-20", Name: "国庆节", IsOffDay: false},
	}
	if err := db.SeedHolidayDays(seed, "builtin"); err != nil {
		t.Fatalf("SeedHolidayDays: %v", err)
	}
	// 幂等 + 不覆盖已有行（已有行可能来自更新的外部数据）
	if err := db.SeedHolidayDays([]models.HolidayDay{
		{Date: "2026-10-01", Name: "旧名", IsOffDay: true},
		{Date: "2027-01-01", Name: "元旦", IsOffDay: true},
	}, "builtin"); err != nil {
		t.Fatalf("SeedHolidayDays twice: %v", err)
	}

	days, err := db.ListHolidayDays(2026)
	if err != nil {
		t.Fatalf("ListHolidayDays: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("2026 应有 2 条，实际 %d 条", len(days))
	}
	if days[0].Date != "2026-09-20" || days[1].Date != "2026-10-01" {
		t.Errorf("应按日期升序返回，实际 %+v", days)
	}
	if days[1].Name != "国庆节" {
		t.Errorf("种子不应覆盖已有行，实际 name=%q", days[1].Name)
	}
	if days[0].IsOffDay {
		t.Errorf("补班日 isOffDay 应为 false")
	}
	other, _ := db.ListHolidayDays(2027)
	if len(other) != 1 || other[0].Source != "builtin" {
		t.Errorf("2027 种子应有 1 条 source=builtin，实际 %+v", other)
	}

	// 外部刷新：整年替换 —— 该年不再存在的日期要被清掉
	if err := db.ReplaceHolidayYear(2026, []models.HolidayDay{
		{Date: "2026-10-01", Name: "国庆节", IsOffDay: true},
		{Date: "2026-10-02", Name: "国庆节", IsOffDay: true},
	}, "remote"); err != nil {
		t.Fatalf("ReplaceHolidayYear: %v", err)
	}
	days, err = db.ListHolidayDays(2026)
	if err != nil {
		t.Fatalf("ListHolidayDays: %v", err)
	}
	if len(days) != 2 {
		t.Fatalf("刷新后 2026 应有 2 条，实际 %d 条", len(days))
	}
	if days[0].Date != "2026-10-01" || days[0].Source != "remote" {
		t.Errorf("刷新后应只剩 10-01/10-02 且 source=remote，实际 %+v", days)
	}
	// 替换只影响目标年份
	if other, _ := db.ListHolidayDays(2027); len(other) != 1 {
		t.Errorf("2027 的数据不应被 2026 的刷新影响，实际 %+v", other)
	}
}

// 外部刷新节流时间戳的读写（跨进程重启保持）。
func TestHolidayRefreshedAt(t *testing.T) {
	db := newTestDB(t)
	if got := db.HolidayRefreshedAt(); got != 0 {
		t.Errorf("初始应为 0，实际 %d", got)
	}
	if err := db.SetHolidayRefreshedAt(1770000000); err != nil {
		t.Fatalf("SetHolidayRefreshedAt: %v", err)
	}
	if got := db.HolidayRefreshedAt(); got != 1770000000 {
		t.Errorf("读回应为 1770000000，实际 %d", got)
	}
}
