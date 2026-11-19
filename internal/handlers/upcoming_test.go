package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mujian/internal/config"
	"mujian/internal/models"
)

// 未来演出接口的三条硬约束：只要未来的、按开演时间升序、已取消的不出现。
// 测试服时区固定为 UTC，所以这里的时间也按 UTC 取，避免本地时区错位导致误判。
func TestListUpcomingRecords(t *testing.T) {
	srv, _, database, _ := newTestServer(t, nil)
	defer srv.Close()

	now := time.Now().UTC()
	cases := []struct {
		name       string
		at         time.Time
		status     int
		coverThumb string
	}{
		{"已开演", now.Add(-2 * time.Hour), models.StatusNormal, ""},
		{"三小时后", now.Add(3 * time.Hour), models.StatusNormal, "covers/poster.thumb.400.webp"},
		{"十天后", now.Add(10 * 24 * time.Hour), models.StatusNormal, ""},
		{"已取消", now.Add(5 * time.Hour), models.StatusCancelled, ""},
		{"未赴约", now.Add(6 * time.Hour), models.StatusNoShow, ""},
	}
	for _, c := range cases {
		if _, err := database.CreateRecord(models.RecordRequest{
			Name:         c.name,
			Date:         c.at.Unix(),
			ActiveStatus: c.status,
			CoverThumb:   c.coverThumb,
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
	if out.Items[0].CoverURL != "/uploads/covers/poster.thumb.400.webp" {
		t.Errorf("coverUrl = %q, want /uploads/covers/poster.thumb.400.webp", out.Items[0].CoverURL)
	}
	if out.Items[1].CoverURL != "" {
		t.Errorf("没有封面的场次不该带 coverUrl：%q", out.Items[1].CoverURL)
	}
}

// 封面地址的三种来源：本地存储、S3 公网、以及外部 URL。
// 缩略图优先；data URI / 超长裸 base64 不返回，避免把整张图塞进 JSON。
func TestUpcomingCoverURL(t *testing.T) {
	local := &Handler{cfg: &config.Config{StorageType: "local"}}
	if got := local.upcomingCoverURL(models.Record{
		CoverFile:  "covers/full.avif",
		CoverThumb: "covers/海报 1.thumb.webp",
	}); got != "/uploads/covers/%E6%B5%B7%E6%8A%A5%201.thumb.webp" {
		t.Errorf("local coverUrl = %q", got)
	}
	// 没有缩略图时退到原图。
	if got := local.upcomingCoverURL(models.Record{CoverFile: "covers/full.avif"}); got != "/uploads/covers/full.avif" {
		t.Errorf("local fallback coverUrl = %q", got)
	}

	s3 := &Handler{cfg: &config.Config{
		StorageType: "s3",
		S3PublicURL: "https://cdn.example.com/",
	}}
	if got := s3.upcomingCoverURL(models.Record{CoverFile: "covers/full.avif"}); got != "https://cdn.example.com/covers/full.avif" {
		t.Errorf("s3 coverUrl = %q", got)
	}
	// S3 未配公网地址时宁可留空，也不能返回一个必然 404 的 /uploads/。
	if got := (&Handler{cfg: &config.Config{StorageType: "s3"}}).upcomingCoverURL(
		models.Record{CoverFile: "covers/full.avif"},
	); got != "" {
		t.Errorf("s3 without public URL should be empty, got %q", got)
	}

	external := local.upcomingCoverURL(models.Record{Cover: "https://img.example.com/poster.jpg"})
	if external != "https://img.example.com/poster.jpg" {
		t.Errorf("external coverUrl = %q", external)
	}

	if got := local.upcomingCoverURL(models.Record{Cover: "data:image/jpeg;base64,AAAA"}); got != "" {
		t.Errorf("data URI should be skipped, got %q", got)
	}
	if got := local.upcomingCoverURL(models.Record{Cover: "coveruuid"}); got != "" {
		t.Errorf("legacy uuid key should be skipped, got %q", got)
	}
	if got := local.upcomingCoverURL(models.Record{CoverFile: string(make([]byte, 600))}); got != "" {
		t.Errorf("oversized raw value should be skipped, got %q", got)
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
