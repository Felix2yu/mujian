package ics

import (
	"fmt"
	"mujian/internal/models"
	"strings"
	"time"
)

// GenerateCalendar renders all records into an RFC 5545 VCALENDAR string.
// zheziNames maps a 折子 id to its display name; pass nil if names are not
// available (折子 will then be omitted from DESCRIPTION).
func GenerateCalendar(records []models.Record, loc *time.Location, zheziNames map[string]string) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Mujian//Record Tracker//CN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	b.WriteString("METHOD:PUBLISH\r\n")
	b.WriteString("X-WR-CALNAME:幕间\r\n")
	b.WriteString(fmt.Sprintf("X-WR-TIMEZONE:%s\r\n", loc.String()))
	b.WriteString("BEGIN:VTIMEZONE\r\n")
	b.WriteString(fmt.Sprintf("TZID:%s\r\n", loc.String()))
	b.WriteString("BEGIN:STANDARD\r\n")
	b.WriteString("DTSTART:19700101T000000\r\n")
	_, offsetSec := time.Now().In(loc).Zone()
	// RFC 5545 requires an explicit + or - sign; naive %+02d formatting on a
	// negative offset would emit an invalid "+-0500" for e.g. New York.
	sign := "+"
	if offsetSec < 0 {
		sign = "-"
		offsetSec = -offsetSec
	}
	offsetH := offsetSec / 3600
	offsetM := (offsetSec % 3600) / 60
	b.WriteString(fmt.Sprintf("TZOFFSETFROM:%s%02d%02d\r\n", sign, offsetH, offsetM))
	b.WriteString(fmt.Sprintf("TZOFFSETTO:%s%02d%02d\r\n", sign, offsetH, offsetM))
	b.WriteString("END:STANDARD\r\n")
	b.WriteString("END:VTIMEZONE\r\n")

	for _, rec := range records {
		writeEvent(&b, rec, loc, zheziNames)
	}

	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func writeEvent(b *strings.Builder, rec models.Record, loc *time.Location, zheziNames map[string]string) {
	start := time.Unix(rec.Date, 0).In(loc)
	// End time reflects the real performance duration when recorded; fall back
	// to a 2-hour default for records without a duration (keeps legacy events
	// from collapsing to zero-length).
	end := start.Add(2 * time.Hour)
	if rec.Duration > 0 {
		end = start.Add(time.Duration(rec.Duration) * time.Minute)
	}
	end = end.In(loc)
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
	// GEO carries the lat/lng separately from the textual LOCATION so calendar
	// clients can drop a map pin. RFC 5545 format is "LAT;LON".
	if rec.Coordinate != nil {
		b.WriteString(fmt.Sprintf("GEO:%f;%f\r\n", rec.Coordinate.Latitude, rec.Coordinate.Longitude))
	}

	var desc []string
	if len(rec.Play) > 0 {
		desc = append(desc, "剧目: "+strings.Join(rec.Play, ", "))
	}
	if names := zheziNameList(rec.ZheziIDs, zheziNames); len(names) > 0 {
		desc = append(desc, "折子: "+strings.Join(names, ", "))
	}
	if len(rec.ArtistNames) > 0 {
		desc = append(desc, "演员: "+strings.Join(rec.ArtistNames, ", "))
	}
	if rec.Company != "" {
		desc = append(desc, "剧团: "+rec.Company)
	}

	if len(desc) > 0 {
		// Join with a real newline and let escapeICS turn it into the ICS "\n"
		// escape. Escaping the joined string (rather than pre-escaping items)
		// would double-escape the separator as "\\n".
		b.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", escapeICS(strings.Join(desc, "\n"))))
	}

	b.WriteString(fmt.Sprintf("CATEGORIES:%s\r\n", escapeICS(rec.CategoryName)))
	b.WriteString("END:VEVENT\r\n")
}

// zheziNameList resolves a record's 折子 ids to display names using the
// provided map. Returns nil when there are no ids or the map is nil, so the
// caller can treat a nil result as "no 折子 line".
func zheziNameList(ids []string, names map[string]string) []string {
	if len(ids) == 0 || names == nil {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if n, ok := names[id]; ok && n != "" {
			out = append(out, n)
		}
	}
	return out
}

func escapeICS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\r\n", "\\n")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\n")
	return s
}
