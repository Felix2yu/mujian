package ics

import (
	"fmt"
	"mujian/internal/models"
	"strings"
	"time"
)

func GenerateCalendar(records []models.Record, loc *time.Location) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Mujian//Record Tracker//CN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	b.WriteString("METHOD:PUBLISH\r\n")
	b.WriteString("X-WR-CALNAME:现场记录\r\n")
	b.WriteString(fmt.Sprintf("X-WR-TIMEZONE:%s\r\n", loc.String()))
	b.WriteString("BEGIN:VTIMEZONE\r\n")
	b.WriteString(fmt.Sprintf("TZID:%s\r\n", loc.String()))
	b.WriteString("BEGIN:STANDARD\r\n")
	b.WriteString("DTSTART:19700101T000000\r\n")
	_, offsetSec := time.Now().In(loc).Zone()
	offsetH := offsetSec / 3600
	offsetM := (offsetSec % 3600) / 60
	b.WriteString(fmt.Sprintf("TZOFFSETFROM:+%02d%02d\r\n", offsetH, offsetM))
	b.WriteString(fmt.Sprintf("TZOFFSETTO:+%02d%02d\r\n", offsetH, offsetM))
	b.WriteString("END:STANDARD\r\n")
	b.WriteString("END:VTIMEZONE\r\n")

	for _, rec := range records {
		writeEvent(&b, rec, loc)
	}

	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func writeEvent(b *strings.Builder, rec models.Record, loc *time.Location) {
	start := time.Unix(rec.Date, 0).In(loc)
	end := start.Add(2 * time.Hour).In(loc)
	startStr := start.Format("20060102T150405")
	endStr := end.Format("20060102T150405")

	b.WriteString("BEGIN:VEVENT\r\n")
	b.WriteString(fmt.Sprintf("UID:%s@mujian\r\n", rec.ID))
	b.WriteString(fmt.Sprintf("DTSTART;TZID=%s:%s\r\n", loc.String(), startStr))
	b.WriteString(fmt.Sprintf("DTEND;TZID=%s:%s\r\n", loc.String(), endStr))
	b.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeICS(rec.Name)))

	if rec.Address != "" {
		b.WriteString(fmt.Sprintf("LOCATION:%s\r\n", escapeICS(rec.Address)))
	}

	var desc []string
	if rec.Channel != "" {
		desc = append(desc, "渠道: "+rec.Channel)
	}
	if rec.Company != "" {
		desc = append(desc, "剧团: "+rec.Company)
	}
	if len(rec.ArtistNames) > 0 {
		desc = append(desc, "演员: "+strings.Join(rec.ArtistNames, ", "))
	}
	if len(rec.Play) > 0 {
		desc = append(desc, "剧目: "+strings.Join(rec.Play, ", "))
	}
	if rec.Friends != "" {
		desc = append(desc, "同行: "+rec.Friends)
	}
	if rec.Seat != "" {
		desc = append(desc, "座位: "+rec.Seat)
	}
	if rec.Remark != "" {
		desc = append(desc, "备注: "+rec.Remark)
	}
	if rec.Rating != 0 {
		desc = append(desc, fmt.Sprintf("评分: %d", rec.Rating))
	}

	if len(desc) > 0 {
		// Join with a real newline and let escapeICS turn it into the ICS "\n"
		// escape. Escaping the joined string (rather than pre-escaping items)
		// would double-escape the separator as "\\n".
		b.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", escapeICS(strings.Join(desc, "\n"))))
	}

	b.WriteString(fmt.Sprintf("CATEGORIES:%s\r\n", rec.CategoryName))
	b.WriteString("END:VEVENT\r\n")
}

func escapeICS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
