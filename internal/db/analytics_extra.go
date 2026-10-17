package db

import (
	"sort"
	"strings"

	"mujian/internal/models"
)

// statusFunnel breaks records down by active_status: 0 正常（已观看）/ 1 想看 /
// 2 已取消 / 3 未赴约. AttendedPct excludes the still-pending 想看 pool from the
// denominator so it reads as "of everything that had a date, how it went".
func (db *DB) statusFunnel() *models.StatusFunnel {
	f := &models.StatusFunnel{Dist: []models.DistItem{}}
	counts := map[int]int{}
	rows, err := db.conn.Query(`SELECT active_status, COUNT(*) FROM records WHERE deleted_at = 0 GROUP BY active_status`)
	if err != nil {
		return f
	}
	defer rows.Close()
	for rows.Next() {
		var st, c int
		if rows.Scan(&st, &c) == nil {
			counts[st] = c
			f.Total += c
		}
	}
	f.Watched = counts[0]
	f.Want = counts[1]
	f.Canceled = counts[2]
	f.NoShow = counts[3]
	f.WantPending = counts[1]
	f.WatchedPct = pctOf(float64(f.Total), float64(f.Watched))
	f.AttendedPct = pctOf(float64(f.Watched+f.Canceled+f.NoShow), float64(f.Watched))
	labels := []struct {
		st    int
		label string
	}{
		{0, "已观看"}, {1, "想看"}, {2, "已取消"}, {3, "未赴约"},
	}
	for _, l := range labels {
		c := counts[l.st]
		if c == 0 {
			continue
		}
		f.Dist = append(f.Dist, models.DistItem{Name: l.label, Count: c, Pct: pctOf(float64(f.Total), float64(c))})
	}
	return f
}

// geoStats treats the most-attended city as the "home city" and splits
// viewing into local vs away. cityDist must already be count-desc ordered.
func (db *DB) geoStats(cityDist []models.DistItem) *models.GeoStats {
	g := &models.GeoStats{AwayCities: []models.DistItem{}}
	if len(cityDist) == 0 {
		return g
	}
	g.HomeCity = cityDist[0].Name
	g.HomeCount = cityDist[0].Count
	total := 0
	for _, c := range cityDist {
		total += c.Count
	}
	g.LocalCount = g.HomeCount
	g.AwayCount = total - g.HomeCount
	g.LocalPct = pctOf(float64(total), float64(g.LocalCount))
	g.AwayPct = pctOf(float64(total), float64(g.AwayCount))
	g.AwayCityCount = len(cityDist) - 1
	awayTotal := 0
	for _, c := range cityDist[1:] {
		awayTotal += c.Count
	}
	for i, c := range cityDist[1:] {
		if i >= 10 {
			break
		}
		g.AwayCities = append(g.AwayCities, models.DistItem{
			Name:  c.Name,
			Count: c.Count,
			Pct:   pctOf(float64(awayTotal), float64(c.Count)),
		})
	}
	return g
}

// companionSeparators are the characters users type between multiple companion
// names in the free-text friends field.
var companionSeparators = ",，、;；/| \t\n"

func splitFriendNames(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return strings.ContainsRune(companionSeparators, r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// companionStats contrasts solo outings (empty friends) with accompanied ones
// and ranks individual companion names across multi-person friend strings.
func (db *DB) companionStats() *models.CompanionStats {
	c := &models.CompanionStats{TopCompanions: []models.DistItem{}}
	if err := db.conn.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN friends != '' THEN 1 ELSE 0 END), 0)
		FROM records WHERE deleted_at = 0`).Scan(&c.Total, &c.WithFriends); err != nil {
		return c
	}
	c.Solo = c.Total - c.WithFriends
	c.SoloPct = pctOf(float64(c.Total), float64(c.Solo))
	c.WithPct = pctOf(float64(c.Total), float64(c.WithFriends))

	nameCount := map[string]int{}
	rows, err := db.conn.Query(`SELECT friends FROM records WHERE deleted_at = 0 AND friends != ''`)
	if err != nil {
		return c
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if rows.Scan(&s) != nil {
			continue
		}
		for _, name := range splitFriendNames(s) {
			nameCount[name]++
		}
	}
	names := make([]string, 0, len(nameCount))
	for n := range nameCount {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if nameCount[names[i]] != nameCount[names[j]] {
			return nameCount[names[i]] > nameCount[names[j]]
		}
		return names[i] < names[j]
	})
	totalMentions := 0
	for _, n := range names {
		totalMentions += nameCount[n]
	}
	for i, n := range names {
		if i >= 10 {
			break
		}
		c.TopCompanions = append(c.TopCompanions, models.DistItem{
			Name:  n,
			Count: nameCount[n],
			Pct:   pctOf(float64(totalMentions), float64(nameCount[n])),
		})
	}
	return c
}

// durationStats aggregates 演出时长 totals/buckets and day-level density
// (how often several shows are packed into one calendar day).
func (db *DB) durationStats() *models.DurationStats {
	d := &models.DurationStats{Buckets: []models.DistItem{}}
	var totalDuration, withDuration int
	db.conn.QueryRow(`
		SELECT COALESCE(SUM(duration), 0),
		       COALESCE(SUM(CASE WHEN duration > 0 THEN 1 ELSE 0 END), 0),
		       COUNT(*)
		FROM records WHERE deleted_at = 0`).Scan(&totalDuration, &withDuration, &d.TotalRecords)
	d.WithDuration = withDuration
	d.TotalHours = round1(float64(totalDuration) / 60.0)
	if withDuration > 0 {
		d.AvgMinutes = round1(float64(totalDuration) / float64(withDuration))
	}

	order := []struct {
		label string
		lo    int
		hi    int
	}{
		{"<60 分钟", 1, 59}, {"60–119 分钟", 60, 119}, {"120–149 分钟", 120, 149},
		{"150–179 分钟", 150, 179}, {"180 分钟以上", 180, 1 << 30},
	}
	bc := map[string]int{}
	if rows, err := db.conn.Query(`SELECT duration, COUNT(*) FROM records WHERE deleted_at = 0 AND duration > 0 GROUP BY duration`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var dur, c int
			if rows.Scan(&dur, &c) != nil {
				continue
			}
			for _, o := range order {
				if dur >= o.lo && dur <= o.hi {
					bc[o.label] += c
					break
				}
			}
		}
	}
	for _, o := range order {
		if c := bc[o.label]; c > 0 {
			d.Buckets = append(d.Buckets, models.DistItem{Name: o.label, Count: c, Pct: pctOf(float64(withDuration), float64(c))})
		}
	}

	mod := db.localTimeModifier()
	if rows, err := db.conn.Query(`
		SELECT per_day, COUNT(*) FROM (
			SELECT COUNT(*) AS per_day FROM records
			WHERE deleted_at = 0 AND date > 0
			GROUP BY strftime('%Y-%m-%d', date, 'unixepoch', ?)
		) GROUP BY per_day`, mod); err == nil {
		defer rows.Close()
		for rows.Next() {
			var perDay, days int
			if rows.Scan(&perDay, &days) != nil {
				continue
			}
			if perDay >= 2 {
				d.MultiShowDays += days
			}
			if perDay > d.MaxShowsPerDay {
				d.MaxShowsPerDay = perDay
			}
		}
	}
	return d
}

// zheziCoverage measures, overall and per drama, how many of the archived
// 折子 have actually been watched at least once.
func (db *DB) zheziCoverage() *models.ZheziCoverage {
	z := &models.ZheziCoverage{Dramas: []models.ZheziCoverageItem{}}
	totals := map[string]int{} // drama_id -> 折子总数
	if rows, err := db.conn.Query(`SELECT drama_id, COUNT(*) FROM zhezis GROUP BY drama_id`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var c int
			if rows.Scan(&id, &c) == nil {
				totals[id] = c
				z.TotalZhezis += c
			}
		}
	}
	seen := map[string]int{} // drama_id -> 已看过的不同折子数
	if rows, err := db.conn.Query(`
		SELECT zh.drama_id, COUNT(DISTINCT rz.zhezi_id)
		FROM record_zhezis rz
		JOIN zhezis zh ON zh.id = rz.zhezi_id
		JOIN records rc ON rc.id = rz.record_id AND rc.deleted_at = 0
		GROUP BY zh.drama_id`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var c int
			if rows.Scan(&id, &c) == nil {
				seen[id] = c
				z.CoveredZhezis += c
			}
		}
	}
	z.OverallPct = pctOf(float64(z.TotalZhezis), float64(z.CoveredZhezis))

	nameByID := map[string]string{}
	if rows, err := db.conn.Query(`SELECT id, name FROM dramas`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, name string
			if rows.Scan(&id, &name) == nil {
				nameByID[id] = name
			}
		}
	}
	items := []models.ZheziCoverageItem{}
	for id, total := range totals {
		s := seen[id]
		if s == 0 || total == 0 {
			continue
		}
		items = append(items, models.ZheziCoverageItem{
			DramaID: id, DramaName: nameByID[id],
			Seen: s, Total: total, Pct: pctOf(float64(total), float64(s)),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Pct != items[j].Pct {
			return items[i].Pct > items[j].Pct
		}
		return items[i].Seen > items[j].Seen
	})
	if len(items) > 10 {
		items = items[:10]
	}
	z.Dramas = items
	return z
}
