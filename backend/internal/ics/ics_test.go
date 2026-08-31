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

	out := GenerateCalendar([]models.Record{rec}, loc, nil)
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
		"DESCRIPTION:剧目: 惊梦\\n演员: 张军\\, 单雯\\n剧团: 上海昆剧团\r\n",
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
	out := GenerateCalendar([]models.Record{rec}, loc, nil)
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

// A record with a coordinate must emit a RFC 5545 GEO property (LAT;LON) while
// keeping the textual LOCATION as the venue.
func TestGenerateCalendarGeo(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	rec := models.Record{
		ID:        "rec-geo",
		Name:      "地理测试",
		Address:   "上海大剧院",
		Coordinate: &models.Coordinate{Latitude: 31.2304, Longitude: 121.4737},
		Date:      time.Date(2026, 9, 1, 19, 30, 0, 0, loc).Unix(),
	}
	out := GenerateCalendar([]models.Record{rec}, loc, nil)
	if !strings.Contains(out, "LOCATION:上海大剧院\r\n") {
		t.Errorf("LOCATION should carry the venue:\n%s", out)
	}
	if !strings.Contains(out, "GEO:31.230400;121.473700\r\n") {
		t.Errorf("GEO should carry lat;lon:\n%s", out)
	}
}

// Records without a coordinate must not emit a GEO property.
func TestGenerateCalendarNoGeo(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	rec := models.Record{ID: "rec-nogeo", Name: "无坐标", Date: 0}
	out := GenerateCalendar([]models.Record{rec}, loc, nil)
	if strings.Contains(out, "GEO:") {
		t.Errorf("no GEO expected when coordinate is nil:\n%s", out)
	}
}

// Duration drives DTEND (a 150-minute / 2h30m show at 19:30 ends at 22:00).
// It is intentionally not echoed as a DESCRIPTION line (per the trimmed schema).
func TestGenerateCalendarDuration(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	rec := models.Record{
		ID:       "rec-d",
		Name:     "长生殿",
		Duration: 150,
		Date:     time.Date(2026, 9, 1, 19, 30, 0, 0, loc).Unix(),
	}
	out := GenerateCalendar([]models.Record{rec}, loc, nil)
	if !strings.Contains(out, "DTEND;TZID=Asia/Shanghai:20260901T220000\r\n") {
		t.Errorf("DTEND should reflect Duration (150m):\n%s", out)
	}
	if strings.Contains(out, "时长:") {
		t.Errorf("duration should not appear as a DESCRIPTION line:\n%s", out)
	}
}

// 折子 names are resolved from the id->name map and emitted (in 剧目/演员/剧团
// order) only when present. Missing ids are skipped.
func TestGenerateCalendarZhezi(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	rec := models.Record{
		ID:          "rec-z",
		Name:        "牡丹亭",
		Play:        []string{"游园惊梦"},
		ArtistNames: []string{"单雯"},
		Company:     "上海昆剧团",
		ZheziIDs:    []string{"z1", "z2", "missing"},
		Date:        time.Date(2026, 9, 1, 19, 30, 0, 0, loc).Unix(),
	}
	names := map[string]string{"z1": "游园", "z2": "惊梦"}
	out := GenerateCalendar([]models.Record{rec}, loc, names)
	want := "DESCRIPTION:剧目: 游园惊梦\\n折子: 游园\\, 惊梦\\n演员: 单雯\\n剧团: 上海昆剧团\r\n"
	if !strings.Contains(out, want) {
		t.Errorf("DESCRIPTION with 折子 should be %q, got:\n%s", want, out)
	}
}

// A record with ZheziIDs but no name map (or names unresolved) emits no 折子
// line.
func TestGenerateCalendarZheziNoNames(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	rec := models.Record{
		ID:          "rec-zn",
		Name:        "牡丹亭",
		Play:        []string{"游园惊梦"},
		ArtistNames: []string{"单雯"},
		Company:     "上海昆剧团",
		ZheziIDs:    []string{"z1"},
		Date:        time.Date(2026, 9, 1, 19, 30, 0, 0, loc).Unix(),
	}
	out := GenerateCalendar([]models.Record{rec}, loc, nil)
	if strings.Contains(out, "折子:") {
		t.Errorf("no 折子 line expected when name map is nil:\n%s", out)
	}
	if !strings.Contains(out, "DESCRIPTION:剧目: 游园惊梦\\n演员: 单雯\\n剧团: 上海昆剧团\r\n") {
		t.Errorf("DESCRIPTION should omit 折子 but keep the rest:\n%s", out)
	}
}

// Records without a duration keep the 2-hour default end time and emit no
// duration line (regression guard for legacy data).
func TestGenerateCalendarNoDuration(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	rec := models.Record{
		ID:   "rec-nod",
		Name: "未填时长",
		Date: time.Date(2026, 9, 1, 19, 30, 0, 0, loc).Unix(),
	}
	out := GenerateCalendar([]models.Record{rec}, loc, nil)
	if !strings.Contains(out, "DTEND;TZID=Asia/Shanghai:20260901T213000\r\n") {
		t.Errorf("DTEND should default to +2h when Duration is 0:\n%s", out)
	}
	if strings.Contains(out, "时长:") {
		t.Errorf("no duration line expected when Duration is 0:\n%s", out)
	}
}

func TestGenerateCalendarEmpty(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	out := GenerateCalendar(nil, loc, nil)
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
	// Bare CR and CRLF are folded to the literal backslash-n escape (regression:
	// CR used to pass through raw and could split a content line).
	for _, in := range []string{"a\rb", "a\r\nb"} {
		if got := escapeICS(in); got != "a\\nb" {
			t.Errorf("escapeICS(%q) = %q, want %q", in, got, "a\\nb")
		}
	}
}

// Western-hemisphere timezones must produce a negative TZOFFSET (RFC 5545),
// not the invalid "+-0500" emitted before the sign fix.
func TestGenerateCalendarNegativeOffset(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	out := GenerateCalendar(nil, loc, nil)
	if !strings.Contains(out, "TZOFFSETFROM:-") || !strings.Contains(out, "TZOFFSETTO:-") {
		t.Errorf("negative offset should be signed with '-':\n%s", out)
	}
	if strings.Contains(out, "+-") {
		t.Errorf("invalid '+-HHMM' offset found:\n%s", out)
	}
}

// Category names and carriage returns are user-controlled: they must be
// escaped so a record cannot break the ICS structure (calendar injection).
func TestGenerateCalendarEscapesCategoriesAndCR(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	rec := models.Record{
		ID:           "rec-x",
		Name:         "注入测试",
		CategoryName: "昆\r曲,折子\r\n戏",
		Remark:       "第一行\r\n第二行",
		Date:         1755000000,
	}
	out := GenerateCalendar([]models.Record{rec}, loc, nil)
	for _, bad := range []string{"CATEGORIES:昆\r", "昆\n曲", ",折子\r"} {
		if strings.Contains(out, bad) {
			t.Errorf("unescaped ICS structure break %q in:\n%s", bad, out)
		}
	}
	if !strings.Contains(out, "CATEGORIES:昆\\n曲\\,折子\\n戏\r\n") {
		t.Errorf("CATEGORIES should escape CR/LF/comma (CRLF folds to one \\n):\n%s", out)
	}
}
