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
	writeHeader(&b, loc)

	for _, rec := range records {
		writeEvent(&b, rec, loc, zheziNames)
	}

	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

// EventCalendar renders a single record as a standalone VCALENDAR with one
// VEVENT. It is the object-resource representation served by the CalDAV
// backend, sharing the exact same event formatting as the subscription feed.
func EventCalendar(rec models.Record, loc *time.Location, zheziNames map[string]string) string {
	var b strings.Builder
	writeHeader(&b, loc)
	writeEvent(&b, rec, loc, zheziNames)
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func writeHeader(b *strings.Builder, loc *time.Location) {
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
	// foldICSLine enforces RFC 5545 §3.1 (max 75 octets per line); every
	// property below goes through it since DESCRIPTION and the Apple
	// structured-location lines routinely exceed the limit with CJK text.
	writeLine := func(s string) {
		b.WriteString(foldICSLine(s))
		b.WriteString("\r\n")
	}
	writeLine(fmt.Sprintf("UID:%s@mujian", rec.ID))
	// DTSTAMP must be deterministic (derived from the event time, not
	// time.Now) so the CalDAV ETag — a hash of this rendering — stays stable
	// across syncs instead of forcing clients to re-fetch everything.
	writeLine(fmt.Sprintf("DTSTAMP;TZID=%s:%s", loc.String(), startStr))
	writeLine(fmt.Sprintf("DTSTART;TZID=%s:%s", loc.String(), startStr))
	writeLine(fmt.Sprintf("DTEND;TZID=%s:%s", loc.String(), endStr))
	writeLine(fmt.Sprintf("SUMMARY:%s", escapeICS(rec.Name)))

	if rec.Address != "" {
		writeLine(fmt.Sprintf("LOCATION:%s", escapeICS(rec.Address)))
	}
	// GEO carries the lat/lng separately from the textual LOCATION so calendar
	// clients can drop a map pin. RFC 5545 format is "LAT;LON".
	if rec.Coordinate != nil {
		writeLine(fmt.Sprintf("GEO:%f;%f", rec.Coordinate.Latitude, rec.Coordinate.Longitude))
		// Apple's own export format: a URI-valued structured location whose
		// value is "geo:lat,lng". X-TITLE labels the pin with the venue name.
		// (The invented X-APPLE-MAPKIT-* parameters previously used here are
		// not part of any Apple export and are dropped.)
		// Note: Apple Calendar ignores these for *subscribed* feeds — maps
		// only render when the .ics is imported into a local/iCloud calendar.
		loc1 := fmt.Sprintf("X-APPLE-STRUCTURED-LOCATION;VALUE=URI;X-APPLE-RADIUS=100:geo:%.6f,%.6f", rec.Coordinate.Latitude, rec.Coordinate.Longitude)
		if rec.Address != "" {
			title := escapeICS(rec.Address)
			title = strings.ReplaceAll(title, "\"", "\\\"")
			loc1 = fmt.Sprintf("X-APPLE-STRUCTURED-LOCATION;VALUE=URI;X-APPLE-RADIUS=100;X-TITLE=\"%s\":geo:%.6f,%.6f", title, rec.Coordinate.Latitude, rec.Coordinate.Longitude)
		}
		writeLine(loc1)
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
		writeLine(fmt.Sprintf("DESCRIPTION:%s", escapeICS(strings.Join(desc, "\n"))))
	}

	writeLine(fmt.Sprintf("CATEGORIES:%s", escapeICS(rec.CategoryName)))
	writeLine("END:VEVENT")
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

// foldICSLine enforces RFC 5545 §3.1: content lines longer than 75 octets are
// split into continuations starting with a single space. Split points are
// backed off to UTF-8 rune boundaries so CJK text is never cut mid-character.
func foldICSLine(line string) string {
	const maxOctets = 75
	if len(line) <= maxOctets {
		return line
	}
	var out strings.Builder
	pos := 0
	for pos < len(line) {
		if pos > 0 {
			out.WriteString("\r\n ")
		}
		limit := maxOctets
		if pos > 0 {
			limit = maxOctets - 1 // one octet is consumed by the leading space
		}
		end := pos + limit
		if end > len(line) {
			end = len(line)
		} else if end < len(line) {
			// Back off to a rune boundary (continuation bytes are 0b10xxxxxx).
			for end > pos && (line[end]&0xC0) == 0x80 {
				end--
			}
		}
		out.WriteString(line[pos:end])
		pos = end
	}
	return out.String()
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
