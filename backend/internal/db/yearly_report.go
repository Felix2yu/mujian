package db

import (
	"fmt"
	"sort"
	"time"

	"mujian/internal/models"
)

const effPriceSQL = "CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END"

// GetYearlyReport computes the viewing report for one calendar year: headline
// KPIs with YoY deltas, a 12-month series, distributions, rankings, plus the
// "annual report card" picks (peak month, priciest/best show, first
// discoveries, historical rank, holiday attendance).
func (db *DB) GetYearlyReport(year int) (*models.YearlyReport, error) {
	out := &models.YearlyReport{
		GeneratedAt:    time.Now().Unix(),
		Year:           year,
		AvailableYears: []int{},
		Monthly:        []models.TrendPoint{},
		CategoryDist:   []models.DistItem{},
		CityDist:       []models.DistItem{},
		RatingDist:     []models.DistItem{},
		PriceBuckets:   []models.DistItem{},
		TopArtists:     []models.RankItem{},
		TopDramas:      []models.RankItem{},
		TopVenues:      []models.RankItem{},
		TopZhezis:      []models.RankItem{},
	}
	out.Highlights.FirstArtists = []models.YearNameDate{}
	out.Highlights.FirstDramas = []models.YearNameDate{}
	out.Holiday.ByHoliday = []models.DistItem{}

	yearStart := time.Date(year, 1, 1, 0, 0, 0, 0, db.loc)
	s, e := yearStart.Unix(), yearStart.AddDate(1, 0, 0).Unix()
	prevS := yearStart.AddDate(-1, 0, 0).Unix()
	mod := db.localTimeModifier()

	// ---- available years for the selector ----
	if rows, err := db.conn.Query(`
		SELECT DISTINCT CAST(strftime('%Y', date, 'unixepoch', ?) AS INTEGER) AS y
		FROM records WHERE deleted_at = 0 AND date > 0 ORDER BY y`, mod); err == nil {
		defer rows.Close()
		for rows.Next() {
			var y int
			if rows.Scan(&y) == nil {
				out.AvailableYears = append(out.AvailableYears, y)
			}
		}
	}

	// ---- KPIs (year vs previous year) ----
	scanKpi := func(from, to int64) (int, float64, float64, float64, error) {
		var cnt, ratedSum, ratedN int
		var cost float64
		err := db.conn.QueryRow(`
			SELECT COUNT(*),
			       COALESCE(SUM(`+effPriceSQL+` + COALESCE(other_cost, 0)), 0),
			       COALESCE(SUM(CASE WHEN rating > 0 THEN rating ELSE 0 END), 0),
			       COUNT(CASE WHEN rating > 0 THEN 1 END)
			FROM records WHERE deleted_at = 0 AND date >= ? AND date < ?`, from, to).
			Scan(&cnt, &cost, &ratedSum, &ratedN)
		rating := 0.0
		if ratedN > 0 {
			rating = float64(ratedSum) / float64(ratedN)
		}
		return cnt, cost, rating, float64(ratedN), err
	}
	cnt, cost, avgRating, _, err := scanKpi(s, e)
	if err != nil {
		return nil, fmt.Errorf("yearly kpi: %w", err)
	}
	out.Overview.TotalRecords = cnt
	out.Overview.TotalCost = cost
	out.Overview.AvgRating = round1(avgRating)

	if err := db.conn.QueryRow(`SELECT COUNT(DISTINCT city) FROM records WHERE deleted_at = 0 AND city != '' AND date >= ? AND date < ?`, s, e).Scan(&out.Overview.TotalCities); err != nil {
		return nil, fmt.Errorf("yearly kpi: %w", err)
	}
	var durMin int
	if err := db.conn.QueryRow(`SELECT COALESCE(SUM(duration), 0) FROM records WHERE deleted_at = 0 AND date >= ? AND date < ?`, s, e).Scan(&durMin); err != nil {
		return nil, fmt.Errorf("yearly kpi: %w", err)
	}
	out.Overview.TotalHours = round1(float64(durMin) / 60.0)

	prevCnt, prevCost, prevRating, prevRatedN, err := scanKpi(prevS, s)
	if err != nil {
		return nil, fmt.Errorf("yearly kpi: %w", err)
	}
	out.Overview.RecordsDeltaPct = pctChange(float64(prevCnt), float64(cnt))
	out.Overview.CostDeltaPct = pctChange(prevCost, cost)
	if prevRatedN > 0 {
		out.Overview.RatingDelta = round1(avgRating - prevRating)
	}

	// ---- first discoveries this year (overall first show of an entity falls in the year) ----
	firstSeen := func(relTable, recordCol, entityCol, entTable string) (int, []models.YearNameDate) {
		count := 0
		list := []models.YearNameDate{}
		rows, err := db.conn.Query(`
			SELECT ent.name, strftime('%Y-%m-%d', datetime(MIN(r.date), 'unixepoch', ?)) AS first_date
			FROM `+relTable+` l
			JOIN records r ON r.id = l.`+recordCol+` AND r.deleted_at = 0
			JOIN `+entTable+` ent ON ent.id = l.`+entityCol+`
			WHERE r.date > 0
			GROUP BY l.`+entityCol+`
			HAVING MIN(r.date) >= ? AND MIN(r.date) < ?
			ORDER BY MIN(r.date) LIMIT 12`, mod, s, e)
		if err != nil {
			return 0, list
		}
		defer rows.Close()
		for rows.Next() {
			var name, date string
			if rows.Scan(&name, &date) == nil {
				list = append(list, models.YearNameDate{Name: name, DateText: date})
			}
		}
		count = len(list)
		// The list is capped at 12; get the true count separately.
		var total int
		q := `SELECT COUNT(*) FROM (
			SELECT l.` + entityCol + ` AS ent
			FROM ` + relTable + ` l
			JOIN records r ON r.id = l.` + recordCol + ` AND r.deleted_at = 0
			WHERE r.date > 0
			GROUP BY l.` + entityCol + `
			HAVING MIN(r.date) >= ? AND MIN(r.date) < ?)`
		if db.conn.QueryRow(q, s, e).Scan(&total) == nil {
			count = total
		}
		return count, list
	}
	out.Overview.NewArtists, out.Highlights.FirstArtists = firstSeen("record_artists", "record_id", "artist_id", "artists")
	out.Overview.NewDramas, out.Highlights.FirstDramas = firstSeen("record_dramas", "record_id", "drama_id", "dramas")

	// ---- 12-month series (zero-filled) ----
	type monthAgg struct {
		count     int
		cost      float64
		avgRating float64
	}
	byMonth := map[int]monthAgg{}
	rows, err := db.conn.Query(`
		SELECT CAST(strftime('%m', date, 'unixepoch', ?) AS INTEGER) AS m,
		       COUNT(*),
		       COALESCE(SUM(`+effPriceSQL+` + COALESCE(other_cost, 0)), 0),
		       COALESCE(AVG(CASE WHEN rating > 0 THEN rating END), 0)
		FROM records WHERE deleted_at = 0 AND date >= ? AND date < ?
		GROUP BY m`, mod, s, e)
	if err != nil {
		return nil, fmt.Errorf("yearly monthly: %w", err)
	}
	defer rows.Close()
	maxCount := 0
	peakMonthIdx := 0
	for rows.Next() {
		var m int
		var a monthAgg
		if rows.Scan(&m, &a.count, &a.cost, &a.avgRating) == nil {
			byMonth[m] = a
			if a.count > maxCount {
				maxCount = a.count
				peakMonthIdx = m
			}
		}
	}
	for m := 1; m <= 12; m++ {
		a := byMonth[m]
		out.Monthly = append(out.Monthly, models.TrendPoint{
			Period:    fmt.Sprintf("%04d-%02d", year, m),
			Count:     a.count,
			Cost:      a.cost,
			AvgRating: a.avgRating,
		})
	}
	if peakMonthIdx > 0 {
		out.Highlights.PeakMonth = fmt.Sprintf("%04d-%02d", year, peakMonthIdx)
		out.Highlights.PeakMonthCount = maxCount
	}

	// ---- distributions ----
	out.CategoryDist = db.distFromQuery(`
		SELECT je.value AS name, COUNT(*) AS cnt FROM records,
			json_each(records.category_names) je
		WHERE records.deleted_at = 0 AND je.value != '' AND records.date >= ? AND records.date < ?
		GROUP BY je.value ORDER BY cnt DESC`, s, e)
	out.CityDist = db.distFromQuery(`
		SELECT city AS name, COUNT(*) AS cnt FROM records
		WHERE deleted_at = 0 AND city != '' AND date >= ? AND date < ?
		GROUP BY city ORDER BY cnt DESC`, s, e)

	ratedByStar := map[int]int{}
	ratedTotal := 0
	if rrows, err := db.conn.Query(`SELECT rating, COUNT(*) FROM records WHERE deleted_at = 0 AND rating > 0 AND date >= ? AND date < ? GROUP BY rating`, s, e); err == nil {
		defer rrows.Close()
		for rrows.Next() {
			var star, c int
			if rrows.Scan(&star, &c) == nil {
				ratedByStar[star] = c
				ratedTotal += c
			}
		}
	}
	for star := 5; star >= 1; star-- {
		c := ratedByStar[star]
		out.RatingDist = append(out.RatingDist, models.DistItem{
			Name:  fmt.Sprintf("%d★", star),
			Count: c,
			Pct:   pctOf(float64(ratedTotal), float64(c)),
		})
	}
	if ratedTotal == 0 {
		out.RatingDist = []models.DistItem{}
	}

	out.WeekdayDist = db.weekdayDist("AND date >= ? AND date < ?", s, e)
	out.PriceBuckets = db.priceBucketDist("AND date >= ? AND date < ?", s, e)
	out.Intervals = db.intervalStats("AND date >= ? AND date < ?", s, e)

	// ---- rankings ----
	out.TopArtists = db.rankFromQuery(`
		SELECT a.id, a.name, COUNT(*) AS cnt FROM record_artists ra
		JOIN records rc ON rc.id = ra.record_id AND rc.deleted_at = 0 AND rc.date >= ? AND rc.date < ?
		JOIN artists a ON a.id = ra.artist_id
		GROUP BY a.id ORDER BY cnt DESC LIMIT 10`, s, e)
	out.TopDramas = db.rankFromQuery(`
		SELECT d.id, d.name, COUNT(*) AS cnt FROM record_dramas rd
		JOIN records rc ON rc.id = rd.record_id AND rc.deleted_at = 0 AND rc.date >= ? AND rc.date < ?
		JOIN dramas d ON d.id = rd.drama_id
		GROUP BY d.id ORDER BY cnt DESC LIMIT 10`, s, e)
	out.TopVenues = db.rankFromQuery(`
		SELECT '' AS id, address AS name, COUNT(*) AS cnt FROM records
		WHERE deleted_at = 0 AND address != '' AND date >= ? AND date < ?
		GROUP BY address ORDER BY cnt DESC LIMIT 10`, s, e)
	out.TopZhezis = db.rankFromQuery(`
		SELECT z.id, z.name, COUNT(*) AS cnt
		FROM record_zhezis rz
		JOIN records rc ON rc.id = rz.record_id AND rc.deleted_at = 0 AND rc.date >= ? AND rc.date < ?
		JOIN zhezis z ON z.id = rz.zhezi_id
		GROUP BY z.id ORDER BY cnt DESC LIMIT 10`, s, e)

	// ---- single-show highlights ----
	scanRef := func(query string, args ...interface{}) *models.YearShowRef {
		var ref models.YearShowRef
		err := db.conn.QueryRow(query, args...).Scan(&ref.ID, &ref.Name, &ref.DateText, &ref.City, &ref.Value)
		if err != nil || ref.Name == "" {
			return nil
		}
		return &ref
	}
	out.Highlights.MostExpensive = scanRef(`
		SELECT id, name, date_text, city, `+effPriceSQL+` AS v FROM records
		WHERE deleted_at = 0 AND date >= ? AND date < ? AND (pay_price > 0 OR price > 0)
		ORDER BY v DESC, date DESC LIMIT 1`, s, e)
	out.Highlights.BestRated = scanRef(`
		SELECT id, name, date_text, city, CAST(rating AS REAL) AS v FROM records
		WHERE deleted_at = 0 AND date >= ? AND date < ? AND rating > 0
		ORDER BY rating DESC, date DESC LIMIT 1`, s, e)
	out.Highlights.FirstShow = scanRef(`
		SELECT id, name, date_text, city, CAST(rating AS REAL) AS v FROM records
		WHERE deleted_at = 0 AND date >= ? AND date < ? AND active_status = 0
		ORDER BY date ASC LIMIT 1`, s, e)
	out.Highlights.LastShow = scanRef(`
		SELECT id, name, date_text, city, CAST(rating AS REAL) AS v FROM records
		WHERE deleted_at = 0 AND date >= ? AND date < ? AND active_status = 0
		ORDER BY date DESC LIMIT 1`, s, e)
	if out.Intervals != nil {
		out.Highlights.LongestGapDays = out.Intervals.Max
	}

	// ---- rank among all years ----
	type yearAgg struct {
		count int
		cost  float64
	}
	years := map[int]yearAgg{}
	if yrows, err := db.conn.Query(`
		SELECT CAST(strftime('%Y', date, 'unixepoch', ?) AS INTEGER) AS y,
		       COUNT(*),
		       COALESCE(SUM(`+effPriceSQL+` + COALESCE(other_cost, 0)), 0)
		FROM records WHERE deleted_at = 0 AND date > 0
		GROUP BY y`, mod); err == nil {
		defer yrows.Close()
		for yrows.Next() {
			var y int
			var a yearAgg
			if yrows.Scan(&y, &a.count, &a.cost) == nil {
				years[y] = a
			}
		}
	}
	if cur, ok := years[year]; ok {
		n := len(years)
		out.Rank.YearsWithShows = n
		countBetter, costBetter := 0, 0
		maxCount, maxCost := 0, float64(0)
		for y, a := range years {
			if y == year {
				continue
			}
			if a.count > cur.count {
				countBetter++
			}
			if a.cost > cur.cost {
				costBetter++
			}
			if a.count > maxCount {
				maxCount = a.count
			}
			if a.cost > maxCost {
				maxCost = a.cost
			}
		}
		out.Rank.CountRank = countBetter + 1
		out.Rank.CostRank = costBetter + 1
		out.Rank.IsRecordCount = cur.count >= maxCount && cur.count > 0
		out.Rank.IsRecordCost = cur.cost >= maxCost && cur.cost > 0
		if n > 1 {
			out.Rank.CountPercentile = round1(pctOf(float64(n), float64(n-countBetter)))
		} else {
			out.Rank.CountPercentile = 100
		}
	}

	// ---- holiday attendance (休 days from the holidays table) ----
	if hrows, err := db.conn.Query(`
		SELECT h.name, COUNT(*) AS cnt
		FROM records r
		JOIN holidays h ON h.date = strftime('%Y-%m-%d', r.date, 'unixepoch', ?) AND h.is_off = 1
		WHERE r.deleted_at = 0 AND r.date >= ? AND r.date < ?
		GROUP BY h.name ORDER BY cnt DESC`, mod, s, e); err == nil {
		defer hrows.Close()
		sum := 0
		for hrows.Next() {
			var name string
			var c int
			if hrows.Scan(&name, &c) == nil {
				out.Holiday.ByHoliday = append(out.Holiday.ByHoliday, models.DistItem{Name: name, Count: c})
				sum += c
			}
		}
		out.Holiday.Shows = sum
		out.Holiday.Pct = pctOf(float64(cnt), float64(sum))
		for i := range out.Holiday.ByHoliday {
			out.Holiday.ByHoliday[i].Pct = pctOf(float64(sum), float64(out.Holiday.ByHoliday[i].Count))
		}
	}

	sort.Slice(out.AvailableYears, func(i, j int) bool { return out.AvailableYears[i] > out.AvailableYears[j] })
	return out, nil
}
