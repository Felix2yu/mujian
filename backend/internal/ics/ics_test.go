package ics

import (
	"mujian/internal/models"
	"strings"
	"testing"
	"time"
)

func TestGenerateCalendar(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	rec := models.Record{
		ID:           "rec-1",
		Name:         "牡丹亭, 游园",
		Channel:      "大麦",
		Company:      "上海昆剧团",
		City:         "上海",
		Address:      "上海大剧院; 主厅",
		ArtistNames:  []string{"张军", "单雯"},
		Play:         []string{"惊梦"},
		Friends:      "小王",
		Seat:         "A区1排2座",
		Remark:       "谢幕有返场\n很精彩",
		Rating:       5,
		CategoryName: "昆曲",
		Date:         time.Date(2026, 8, 22, 19, 30, 0, 0, loc).Unix(),
	}

	out := GenerateCalendar([]models.Record{rec}, loc)
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\n",
		"VERSION:2.0\r\n",
		"PRODID:-//Mujian//Record Tracker//CN\r\n",
		"X-WR-CALNAME:现场记录\r\n",
		"X-WR-TIMEZONE:Asia/Shanghai\r\n",
		"BEGIN:VTIMEZONE\r\n",
		"TZID:Asia/Shanghai\r\n",
		"BEGIN:VEVENT\r\n",
		"UID:rec-1@mujian\r\n",
		"DTSTART;TZID=Asia/Shanghai:20260822T193000\r\n",
		"DTEND;TZID=Asia/Shanghai:20260822T213000\r\n",
		"SUMMARY:牡丹亭\\, 游园\r\n",
		"LOCATION:上海大剧院\\; 主厅\r\n",
		"DESCRIPTION:渠道: 大麦\\n剧团: 上海昆剧团\\n演员: 张军\\, 单雯\\n剧目: 惊梦\\n同行: 小王\\n座位: A区1排2座\\n备注: 谢幕有返场\\n很精彩\\n评分: 5\r\n",
		"CATEGORIES:昆曲\r\n",
		"END:VEVENT\r\n",
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("calendar missing %q\n---\n%s", want, out)
		}
	}
}

func TestGenerateCalendarMinimalRecord(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	rec := models.Record{ID: "r2", Name: "无名", Date: 0}
	out := GenerateCalendar([]models.Record{rec}, loc)
	if !strings.Contains(out, "BEGIN:VEVENT") || !strings.Contains(out, "END:VEVENT") {
		t.Fatal("minimal record should produce a VEVENT")
	}
	if strings.Contains(out, "LOCATION:") {
		t.Error("empty address should not emit LOCATION")
	}
	if strings.Contains(out, "DESCRIPTION:") {
		t.Error("record without detail fields should not emit DESCRIPTION")
	}
	if strings.Contains(out, "评分: 0") {
		t.Error("rating 0 should not be emitted")
	}
	if !strings.Contains(out, "SUMMARY:无名\r\n") {
		t.Error("summary missing")
	}
}

func TestGenerateCalendarEmpty(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	out := GenerateCalendar(nil, loc)
	if !strings.HasPrefix(out, "BEGIN:VCALENDAR\r\n") || !strings.HasSuffix(out, "END:VCALENDAR\r\n") {
		t.Errorf("empty calendar malformed: %q", out)
	}
	if strings.Contains(out, "VEVENT") {
		t.Error("empty calendar should not contain events")
	}
}

func TestEscapeICS(t *testing.T) {
	in := `a\b;c,d` + "\n" + "e"
	want := `a\\b\;c\,d\n` + "e"
	if got := escapeICS(in); got != want {
		t.Errorf("escapeICS(%q) = %q, want %q", in, got, want)
	}
	if got := escapeICS("plain"); got != "plain" {
		t.Errorf("plain string should be unchanged: %q", got)
	}
}
