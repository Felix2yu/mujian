package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mujian/internal/models"
)

// 未来演出接口的三条硬约束：只要未来的、按开演时间升序、已取消的不出现。
// 测试服时区固定为 UTC，所以这里的时间也按 UTC 取，避免本地时区错位导致误判。
func TestListUpcomingRecords(t *testing.T) {
	srv, _, database, _ := newTestServer(t, nil)
	defer srv.Close()

	now := time.Now().UTC()
	cases := []struct {
		name   string
		at     time.Time
		status int
	}{
		{"已开演", now.Add(-2 * time.Hour), models.StatusNormal},
		{"三小时后", now.Add(3 * time.Hour), models.StatusNormal},
		{"十天后", now.Add(10 * 24 * time.Hour), models.StatusNormal},
		{"已取消", now.Add(5 * time.Hour), models.StatusCancelled},
		{"未赴约", now.Add(6 * time.Hour), models.StatusNoShow},
	}
	for _, c := range cases {
		if _, err := database.CreateRecord(models.RecordRequest{
			Name:         c.name,
			Date:         c.at.Unix(),
			ActiveStatus: c.status,
		}); err != nil {
			t.Fatalf("CreateRecord(%s): %v", c.name, err)
		}
	}

	resp, err := http.Get(srv.URL + "/api/records/upcoming?days=14")
	if err != nil {
		t.Fatalf("GET upcoming: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var out struct {
		Items []upcomingRecord `json:"items"`
		Days  int              `json:"days"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Days != 14 {
		t.Errorf("days = %d, want 14", out.Days)
	}
	if len(out.Items) != 2 {
		t.Fatalf("items = %d, want 2 (三小时后 + 十天后)", len(out.Items))
	}
	if out.Items[0].Name != "三小时后" || out.Items[1].Name != "十天后" {
		t.Errorf("顺序或内容不对：%q, %q", out.Items[0].Name, out.Items[1].Name)
	}
	if out.Items[0].Date < now.Unix() {
		t.Errorf("返回了已经开演的场次：%d", out.Items[0].Date)
	}
}

// days 上限用于挡住被放大的扫描范围。
func TestListUpcomingRecordsClampsDays(t *testing.T) {
	srv, _, _, _ := newTestServer(t, nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/records/upcoming?days=99999")
	if err != nil {
		t.Fatalf("GET upcoming: %v", err)
	}
	defer resp.Body.Close()

	var out struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Days != upcomingMaxDays {
		t.Errorf("days = %d, want %d", out.Days, upcomingMaxDays)
	}
}
