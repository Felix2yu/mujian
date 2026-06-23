package ics

import (
	"fmt"
	"mujian/internal/models"
	"strings"
	"time"
)

func GenerateCalendar(shows []models.Show) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Mujian//Performance Tracker//CN\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	b.WriteString("METHOD:PUBLISH\r\n")
	b.WriteString("X-WR-CALNAME:现场演出\r\n")
	b.WriteString("X-WR-TIMEZONE:Asia/Shanghai\r\n")
	b.WriteString("BEGIN:VTIMEZONE\r\n")
	b.WriteString("TZID:Asia/Shanghai\r\n")
	b.WriteString("BEGIN:STANDARD\r\n")
	b.WriteString("DTSTART:19700101T000000\r\n")
	b.WriteString("TZOFFSETFROM:+0800\r\n")
	b.WriteString("TZOFFSETTO:+0800\r\n")
	b.WriteString("END:STANDARD\r\n")
	b.WriteString("END:VTIMEZONE\r\n")

	for _, show := range shows {
		writeEvent(&b, show)
	}

	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

func writeEvent(b *strings.Builder, show models.Show) {
	start := show.Date.Format("20060102T150405")
	end := show.Date.Add(time.Duration(show.Duration) * time.Minute).Format("20060102T150405")

	b.WriteString("BEGIN:VEVENT\r\n")
	b.WriteString(fmt.Sprintf("UID:%d@mujian\r\n", show.ID))
	b.WriteString(fmt.Sprintf("DTSTART;TZID=Asia/Shanghai:%s\r\n", start))
	b.WriteString(fmt.Sprintf("DTEND;TZID=Asia/Shanghai:%s\r\n", end))
	b.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeICS(show.Name)))

	if show.Venue != "" {
		b.WriteString(fmt.Sprintf("LOCATION:%s\r\n", escapeICS(show.Venue)))
	}

	var desc []string
	if show.Company != "" {
		desc = append(desc, "剧团: "+show.Company)
	}
	if show.Cast != "" {
		desc = append(desc, "阵容: "+show.Cast)
	}
	if show.Friends != "" {
		desc = append(desc, "同行: "+show.Friends)
	}
	if show.Seat != "" {
		desc = append(desc, "座位: "+show.Seat)
	}
	if show.Review != "" {
		desc = append(desc, "剧评: "+show.Review)
	}
	if show.Notes != "" {
		desc = append(desc, "备注: "+show.Notes)
	}
	if show.Rating != nil {
		desc = append(desc, fmt.Sprintf("评分: %d/5", *show.Rating))
	}

	if len(desc) > 0 {
		b.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", escapeICS(strings.Join(desc, "\\n"))))
	}

	b.WriteString(fmt.Sprintf("CATEGORIES:%s\r\n", show.CategoryName))
	b.WriteString("END:VEVENT\r\n")
}

func escapeICS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
