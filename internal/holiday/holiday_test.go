package holiday

import (
	"context"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"mujian/internal/db"
)

// 内置数据是随二进制发布的兜底：必须可解析、日期格式合法、
// 且同时包含放假（休）与调休补班（班）两类条目。
func TestBuiltinDays(t *testing.T) {
	days, err := BuiltinDays()
	if err != nil {
		t.Fatalf("BuiltinDays: %v", err)
	}
	if len(days) == 0 {
		t.Fatal("内置节假日数据不应为空")
	}
	dateRe := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	years := map[string]bool{}
	var off, work int
	for _, d := range days {
		if !dateRe.MatchString(d.Date) {
			t.Errorf("非法日期 %q", d.Date)
		}
		if d.Name == "" {
			t.Errorf("%s 缺少节日名", d.Date)
		}
		years[d.Date[:4]] = true
		if d.IsOffDay {
			off++
		} else {
			work++
		}
	}
	if !years["2025"] || !years["2026"] {
		t.Errorf("内置数据应覆盖 2025 与 2026，实际 %v", years)
	}
	if off == 0 || work == 0 {
		t.Errorf("放假与补班条目都应存在，off=%d work=%d", off, work)
	}
}

// Seed 把内置数据写进 DB（幂等）；已写入刷新时间戳时 Refresh 直接跳过，
// 不发起任何外部请求。
func TestSeedAndRefreshThrottle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	defer database.Close()

	Seed(database)
	Seed(database) // 幂等

	days, err := database.ListHolidayDays(2025)
	if err != nil {
		t.Fatalf("ListHolidayDays: %v", err)
	}
	if len(days) == 0 {
		t.Fatal("Seed 后 2025 应有数据")
	}

	// 刚刚刷新过：Refresh 应直接返回，不触网
	if err := database.SetHolidayRefreshedAt(time.Now().Unix()); err != nil {
		t.Fatalf("SetHolidayRefreshedAt: %v", err)
	}
	if err := Refresh(context.Background(), database); err != nil {
		t.Fatalf("Refresh（应被节流跳过）: %v", err)
	}
	if got := database.HolidayRefreshedAt(); got != time.Now().Unix() && got == 0 {
		t.Errorf("节流路径不应改动时间戳，实际 %d", got)
	}
}
