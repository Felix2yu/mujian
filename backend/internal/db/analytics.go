package db

import (
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"mujian/internal/models"
)

// VenueGroup is an aggregated view of records sharing the same venue address.
// Venues have no dedicated entity table — records.address acts as the implicit
// identifier — so grouping by address with counts is how "the same place"
// (including near-duplicate spellings) is surfaced for cleanup workflows.
type VenueGroup struct {
	Address     string             `json:"address"`
	Cities      []string           `json:"cities"`
	RecordCount int                `json:"record_count"`
	HasCoord    bool               `json:"has_coord"`
	Coordinate  *models.Coordinate `json:"coordinate,omitempty"`
}

// ListVenueGroups returns address groups, optionally filtered by a
// case-insensitive substring match on the address. Empty query returns all
// groups. Groups are ordered by record count descending so busy venues come
// first.
func (db *DB) ListVenueGroups(query string) ([]VenueGroup, error) {
	rows, err := db.conn.Query(`
		SELECT address,
		       COUNT(*)          AS cnt,
		       MAX(coordinate != '' AND coordinate != 'null') AS has_coord,
		       (SELECT coordinate FROM records r2
		        WHERE r2.address = records.address
		        AND r2.deleted_at = 0
		        AND r2.coordinate != ''
		        AND r2.coordinate != 'null'
		        LIMIT 1) AS coord_json
		FROM records
		WHERE deleted_at = 0 AND address != ''
		GROUP BY address
		ORDER BY cnt DESC, address`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	q := strings.ToLower(strings.TrimSpace(query))
	out := []VenueGroup{}
	for rows.Next() {
		var g VenueGroup
		var coordJSON sql.NullString
		if err := rows.Scan(&g.Address, &g.RecordCount, &g.HasCoord, &coordJSON); err != nil {
			return nil, err
		}
		if q != "" && !strings.Contains(strings.ToLower(g.Address), q) {
			continue
		}
		if g.HasCoord && coordJSON.Valid && coordJSON.String != "" {
			g.Coordinate = unmarshalCoordinate(coordJSON.String)
		}
		g.Cities = []string{}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Attach distinct non-empty cities per group in a second pass to keep the
	// main aggregation simple.
	for i := range out {
		crows, err := db.conn.Query(
			`SELECT DISTINCT city FROM records WHERE deleted_at = 0 AND address = ? AND city != '' ORDER BY city LIMIT 5`,
			out[i].Address)
		if err != nil {
			return nil, err
		}
		for crows.Next() {
			var c string
			if err := crows.Scan(&c); err == nil {
				out[i].Cities = append(out[i].Cities, c)
			}
		}
		crows.Close()
	}
	return out, nil
}

// ValueCount is a field-value frequency pair used for analysing which free-text
// values (company names, cities, channels…) exist and how often each occurs.
type ValueCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// allowedCountFields guards GetValueCounts against arbitrary column access;
// only these scalar text fields make sense to group by.
var allowedCountFields = map[string]bool{
	"company":       true,
	"city":          true,
	"channel":       true,
	"category_name": true,
}

// GetValueCounts groups records by the given field and returns each distinct
// value with its usage count, ordered by count descending. Empty values are
// skipped. Used e.g. to spot near-duplicate company spellings before merging.
func (db *DB) GetValueCounts(field string) ([]ValueCount, error) {
	if !allowedCountFields[field] {
		return nil, fmt.Errorf("unsupported count field: %s", field)
	}
	// field comes from the whitelist above, never from user input.
	rows, err := db.conn.Query(fmt.Sprintf(
		`SELECT %s, COUNT(*) FROM records WHERE deleted_at = 0 AND %s != '' GROUP BY %s ORDER BY COUNT(*) DESC, %s`,
		field, field, field, field))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ValueCount{}
	for rows.Next() {
		var v ValueCount
		if err := rows.Scan(&v.Value, &v.Count); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetAnalytics computes a comprehensive analytics payload covering trend,
// distribution, comparison, anomaly and correlation views. All queries run
// against the records table (and the artist/drama relation tables), so a
// single call powers the whole analysis page.
func (db *DB) GetAnalytics() (*models.AnalyticsData, error) {
	out := &models.AnalyticsData{GeneratedAt: time.Now().Unix()}
	out.Trends = []models.TrendPoint{}
	out.CategoryDist = []models.DistItem{}
	out.ChannelDist = []models.DistItem{}
	out.CompanyDist = []models.DistItem{}
	out.CityDist = []models.DistItem{}
	out.RatingDist = []models.DistItem{}
	out.YearDist = []models.DistItem{}
	out.CompareMonthly = []models.ComparePoint{}
	out.Anomalies = []models.Anomaly{}
	out.CorrPairs = []models.CorrPair{}
	out.Scatter = []models.ScatterPoint{}
	out.TopArtists = []models.RankItem{}
	out.TopDramas = []models.RankItem{}
	out.TopVenues = []models.RankItem{}
	out.PriceBuckets = []models.DistItem{}
	out.OtherCostBuckets = []models.DistItem{}
	out.TopZhezis = []models.RankItem{}
	out.Discovery = []models.DiscoverPoint{}
	out.WeekdayDist = []models.WeekdayItem{}

	now := time.Now()
	windowStart := startOfMonth(now.AddDate(0, -23, 0)) // 24 months inclusive

	// ---- Overview KPIs ----
	// Unlike the sub-sections below (which degrade to empty lists), a failed
	// KPI scan means the DB is unreachable — surface it instead of rendering
	// an all-zero page.
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM records WHERE deleted_at = 0").Scan(&out.Overview.TotalRecords); err != nil {
		return nil, fmt.Errorf("analytics overview: %w", err)
	}
	if err := db.conn.QueryRow("SELECT COALESCE(SUM(CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END + COALESCE(other_cost, 0)), 0) FROM records WHERE deleted_at = 0").Scan(&out.Overview.TotalCost); err != nil {
		return nil, fmt.Errorf("analytics overview: %w", err)
	}
	if err := db.conn.QueryRow("SELECT COALESCE(AVG(CAST(rating AS REAL)), 0) FROM records WHERE deleted_at = 0 AND rating IS NOT NULL AND rating != 0").Scan(&out.Overview.AvgRating); err != nil {
		return nil, fmt.Errorf("analytics overview: %w", err)
	}
	if err := db.conn.QueryRow("SELECT COUNT(DISTINCT city) FROM records WHERE deleted_at = 0 AND city != ''").Scan(&out.Overview.TotalCities); err != nil {
		return nil, fmt.Errorf("analytics overview: %w", err)
	}
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM artists").Scan(&out.Overview.TotalArtists); err != nil {
		return nil, fmt.Errorf("analytics overview: %w", err)
	}
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM dramas").Scan(&out.Overview.TotalDramas); err != nil {
		return nil, fmt.Errorf("analytics overview: %w", err)
	}

	// Period-over-period: last 365 days vs the preceding 365 days.
	curStart := now.AddDate(0, 0, -365).Unix()
	prevStart := now.AddDate(0, 0, -730).Unix()
	var curCount, prevCount int
	var curCost, prevCost float64
	var curRating, prevRating float64
	if err := db.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END + COALESCE(other_cost, 0)), 0), COALESCE(AVG(CAST(rating AS REAL)), 0) FROM records WHERE deleted_at = 0 AND date >= ?", curStart).Scan(&curCount, &curCost, &curRating); err != nil {
		return nil, fmt.Errorf("analytics overview: %w", err)
	}
	if err := db.conn.QueryRow("SELECT COUNT(*), COALESCE(SUM(CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END + COALESCE(other_cost, 0)), 0), COALESCE(AVG(CAST(rating AS REAL)), 0) FROM records WHERE deleted_at = 0 AND date >= ? AND date < ?", prevStart, curStart).Scan(&prevCount, &prevCost, &prevRating); err != nil {
		return nil, fmt.Errorf("analytics overview: %w", err)
	}
	out.Overview.RecordsDeltaPct = pctChange(float64(prevCount), float64(curCount))
	out.Overview.CostDeltaPct = pctChange(prevCost, curCost)
	out.Overview.RatingDelta = round1(curRating - prevRating)

	// ---- Trends (last 24 months) ----
	type monthAgg struct {
		count     int
		cost      float64
		avgRating float64
	}
	byMonth := map[string]monthAgg{}
	rows, err := db.conn.Query(`
		SELECT strftime('%Y-%m', datetime(date, 'unixepoch')) AS m,
		       COUNT(*),
		       COALESCE(SUM(CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END + COALESCE(other_cost, 0)), 0),
		       COALESCE(AVG(CAST(rating AS REAL)), 0)
		FROM records
		WHERE deleted_at = 0 AND date >= ?
		GROUP BY m`, windowStart.Unix())
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var m string
			var a monthAgg
			if err := rows.Scan(&m, &a.count, &a.cost, &a.avgRating); err == nil {
				byMonth[m] = a
			}
		}
	}
	// Build a continuous 24-month series (fill gaps with zeros).
	for i := 0; i < 24; i++ {
		m := startOfMonth(now).AddDate(0, -23+i, 0).Format("2006-01")
		a := byMonth[m]
		out.Trends = append(out.Trends, models.TrendPoint{
			Period:    m,
			Count:     a.count,
			Cost:      a.cost,
			AvgRating: a.avgRating,
		})
	}

	// ---- Distributions ----
	out.CategoryDist = db.distFromQuery(`
		SELECT je.value AS name, COUNT(*) AS cnt FROM records,
			json_each(records.category_names) je
		WHERE records.deleted_at = 0 AND je.value != '' GROUP BY je.value ORDER BY cnt DESC`)
	out.ChannelDist = db.distFromQuery(`
		SELECT channel AS name, COUNT(*) AS cnt FROM records
		WHERE deleted_at = 0 AND channel != '' GROUP BY channel ORDER BY cnt DESC`)
	out.CompanyDist = db.distFromQuery(`
		SELECT company AS name, COUNT(*) AS cnt FROM records
		WHERE deleted_at = 0 AND company != '' GROUP BY company ORDER BY cnt DESC`)
	out.CityDist = db.distFromQuery(`
		SELECT city AS name, COUNT(*) AS cnt FROM records
		WHERE deleted_at = 0 AND city != '' GROUP BY city ORDER BY cnt DESC`)
	out.YearDist = db.distFromQuery(`
		SELECT strftime('%Y', datetime(date, 'unixepoch')) AS name, COUNT(*) AS cnt
		FROM records WHERE deleted_at = 0 AND date > 0 GROUP BY 1 ORDER BY 1`)

	// Rating distribution: counts for stars 1..5.
	ratedTotal := 0
	ratedByStar := map[int]int{}
	rrows, rerr := db.conn.Query("SELECT rating, COUNT(*) FROM records WHERE deleted_at = 0 AND rating > 0 GROUP BY rating")
	if rerr == nil {
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

	// ---- Comparison: last 12 months YoY (derived from the 24-month trend) ----
	if len(out.Trends) == 24 {
		for i := 12; i < 24; i++ {
			cur := out.Trends[i].Count
			prev := out.Trends[i-12].Count
			out.CompareMonthly = append(out.CompareMonthly, models.ComparePoint{
				Period:   out.Trends[i].Period,
				Current:  float64(cur),
				Previous: float64(prev),
				DeltaPct: pctChange(float64(prev), float64(cur)),
			})
		}
	}

	// ---- Anomaly detection (z-score over the active window only) ----
	// Only the "active window" (first non-empty month → last non-empty month)
	// is scored. Leading/trailing all-zero months mean "not yet recording" /
	// "stopped recording" and must not be flagged as a drop nor inflate the mean.
	counts := make([]float64, len(out.Trends))
	for i, t := range out.Trends {
		counts[i] = float64(t.Count)
	}
	firstIdx, lastIdx := -1, -1
	for i, c := range counts {
		if c > 0 {
			if firstIdx == -1 {
				firstIdx = i
			}
			lastIdx = i
		}
	}
	if firstIdx != -1 {
		active := counts[firstIdx : lastIdx+1]
		mean, std := meanStd(active)
		if std > 0 {
			for i := firstIdx; i <= lastIdx; i++ {
				z := (counts[i] - mean) / std
				if math.Abs(z) > 1.5 {
					out.Anomalies = append(out.Anomalies, models.Anomaly{
						Period:   out.Trends[i].Period,
						Count:    out.Trends[i].Count,
						Expected: round1(mean),
						ZScore:   round2(z),
						Type:     ternary(z > 0, "spike", "drop"),
					})
				}
			}
		}
	}
	sort.Slice(out.Anomalies, func(i, j int) bool {
		return out.Anomalies[i].Period > out.Anomalies[j].Period
	})

	// ---- Correlations (Pearson r on numeric pairs) ----
	out.CorrPairs = append(out.CorrPairs, db.pearsonPair("有效票价", "评分",
		"SELECT CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END AS effective_price, rating FROM records WHERE deleted_at = 0 AND (pay_price > 0 OR price > 0) AND rating > 0"))
	out.CorrPairs = append(out.CorrPairs, db.pearsonPair("其他花费", "评分",
		"SELECT other_cost, rating FROM records WHERE deleted_at = 0 AND other_cost > 0 AND rating > 0"))
	out.CorrPairs = append(out.CorrPairs, db.pearsonPair("标价", "有效票价",
		"SELECT price, CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END AS effective_price FROM records WHERE deleted_at = 0 AND price > 0"))
	out.CorrPairs = append(out.CorrPairs, db.pearsonPair("有效票价", "其他花费",
		"SELECT CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END AS effective_price, other_cost FROM records WHERE deleted_at = 0 AND (pay_price > 0 OR price > 0) AND other_cost > 0"))

	// Scatter sample: effective price vs rating (most recent 150 with both values).
	srows, serr := db.conn.Query(`
		SELECT CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END AS effective_price, rating FROM records
		WHERE deleted_at = 0 AND (pay_price > 0 OR price > 0) AND rating > 0
		ORDER BY date DESC LIMIT 150`)
	if serr == nil {
		defer srows.Close()
		for srows.Next() {
			var x, y float64
			if srows.Scan(&x, &y) == nil {
				out.Scatter = append(out.Scatter, models.ScatterPoint{X: x, Y: y})
			}
		}
	}

	// ---- Rankings ----
	out.TopArtists = db.rankFromQuery(`
		SELECT a.id, a.name, COUNT(*) AS cnt FROM record_artists ra
		JOIN records rc ON rc.id = ra.record_id AND rc.deleted_at = 0
		JOIN artists a ON a.id = ra.artist_id
		GROUP BY a.id ORDER BY cnt DESC LIMIT 10`)
	out.TopDramas = db.rankFromQuery(`
		SELECT d.id, d.name, COUNT(*) AS cnt FROM record_dramas rd
		JOIN records rc ON rc.id = rd.record_id AND rc.deleted_at = 0
		JOIN dramas d ON d.id = rd.drama_id
		GROUP BY d.id ORDER BY cnt DESC LIMIT 10`)
	out.TopVenues = db.rankFromQuery(`
		SELECT '' AS id, address AS name, COUNT(*) AS cnt FROM records
		WHERE deleted_at = 0 AND address != '' GROUP BY address ORDER BY cnt DESC LIMIT 10`)

	// ---- New behavioural / economic dimensions ----
	out.PriceBuckets = db.priceBucketDist()
	out.OtherCostBuckets = db.otherCostBucketDist()
	out.TopZhezis = db.topZhezis()
	out.Rewatch = db.rewatchStats()
	out.Discovery = db.discoverySeries()
	out.Diversity = db.diversityIndex()
	out.Intervals = db.intervalStats()
	out.WeekdayDist = db.weekdayDist()

	return out, nil
}

// distFromQuery runs a 2-column (name, count) query and appends DistItems with
// percentage shares computed against the sum of the returned counts.
func (db *DB) distFromQuery(query string, args ...interface{}) []models.DistItem {
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		slog.Warn("analytics dist query", "err", err)
		return []models.DistItem{}
	}
	defer rows.Close()
	items := []models.DistItem{}
	total := 0
	for rows.Next() {
		var name string
		var c int
		if err := rows.Scan(&name, &c); err != nil {
			continue
		}
		items = append(items, models.DistItem{Name: name, Count: c})
		total += c
	}
	for i := range items {
		items[i].Pct = pctOf(float64(total), float64(items[i].Count))
	}
	return items
}

// rankFromQuery runs a 3-column (id, name, count) query for top-N listings.
func (db *DB) rankFromQuery(query string, args ...interface{}) []models.RankItem {
	rows, err := db.conn.Query(query, args...)
	if err != nil {
		slog.Warn("analytics rank query", "err", err)
		return []models.RankItem{}
	}
	defer rows.Close()
	items := []models.RankItem{}
	for rows.Next() {
		var id, name string
		var c int
		if err := rows.Scan(&id, &name, &c); err != nil {
			continue
		}
		items = append(items, models.RankItem{ID: id, Name: name, Count: c})
	}
	return items
}

// pearsonPair computes Pearson's r for the two numeric columns returned by query.
func (db *DB) pearsonPair(xLabel, yLabel, query string) models.CorrPair {
	rows, err := db.conn.Query(query)
	if err != nil {
		return models.CorrPair{X: xLabel, Y: yLabel, R: 0, N: 0}
	}
	defer rows.Close()
	var xs, ys []float64
	for rows.Next() {
		var x, y float64
		if rows.Scan(&x, &y) != nil {
			continue
		}
		xs = append(xs, x)
		ys = append(ys, y)
	}
	return models.CorrPair{X: xLabel, Y: yLabel, R: pearson(xs, ys), N: len(xs)}
}

// ---------- analytics helpers ----------

func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

func pctChange(prev, cur float64) float64 {
	if prev <= 0 {
		if cur > 0 {
			return 100
		}
		return 0
	}
	return round1((cur - prev) / prev * 100)
}

func pctOf(total, part float64) float64 {
	if total <= 0 {
		return 0
	}
	return round1(part / total * 100)
}

func meanStd(xs []float64) (float64, float64) {
	n := len(xs)
	if n == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(n)
	variance := 0.0
	for _, x := range xs {
		d := x - mean
		variance += d * d
	}
	variance /= float64(n)
	return mean, math.Sqrt(variance)
}

// pearson returns the Pearson correlation coefficient, or 0 when undefined.
func pearson(xs, ys []float64) float64 {
	n := len(xs)
	if n != len(ys) || n < 3 {
		return 0
	}
	sx, sy, sxx, syy, sxy := 0.0, 0.0, 0.0, 0.0, 0.0
	for i := 0; i < n; i++ {
		x, y := xs[i], ys[i]
		sx += x
		sy += y
		sxx += x * x
		syy += y * y
		sxy += x * y
	}
	num := float64(n)*sxy - sx*sy
	den := math.Sqrt((float64(n)*sxx - sx*sx) * (float64(n)*syy - sy*sy))
	if den == 0 {
		return 0
	}
	return round2(num / den)
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

// ---------- new behavioural / economic analytics ----------

// priceBucketDist buckets effective price (pay_price if > 0, else price) into
// standard ranges. Only records with a positive effective price are considered;
// an empty slice is returned when no price data exists.
func (db *DB) priceBucketDist() []models.DistItem {
	order := []string{"0–99", "100–199", "200–399", "400–799", "800+"}
	rows, err := db.conn.Query(`
		SELECT
			CASE
				WHEN effective_price < 100 THEN '0–99'
				WHEN effective_price < 200 THEN '100–199'
				WHEN effective_price < 400 THEN '200–399'
				WHEN effective_price < 800 THEN '400–799'
				ELSE '800+'
			END AS bucket,
			COUNT(*) AS cnt
		FROM (SELECT CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END AS effective_price FROM records WHERE deleted_at = 0)
		WHERE effective_price > 0
		GROUP BY bucket`)
	if err != nil {
		return []models.DistItem{}
	}
	defer rows.Close()
	m := map[string]int{}
	total := 0
	for rows.Next() {
		var b string
		var c int
		if rows.Scan(&b, &c) != nil {
			continue
		}
		m[b] = c
		total += c
	}
	if total == 0 {
		return []models.DistItem{}
	}
	out := []models.DistItem{}
	for _, b := range order {
		if c, ok := m[b]; ok {
			out = append(out, models.DistItem{Name: b, Count: c, Pct: pctOf(float64(total), float64(c))})
		}
	}
	return out
}

// otherCostBucketDist buckets other_cost into the same standard ranges as
// priceBucketDist, so the two economic dimensions are visually comparable.
func (db *DB) otherCostBucketDist() []models.DistItem {
	order := []string{"0–99", "100–199", "200–399", "400–799", "800+"}
	rows, err := db.conn.Query(`
		SELECT
			CASE
				WHEN other_cost < 100 THEN '0–99'
				WHEN other_cost < 200 THEN '100–199'
				WHEN other_cost < 400 THEN '200–399'
				WHEN other_cost < 800 THEN '400–799'
				ELSE '800+'
			END AS bucket,
			COUNT(*) AS cnt
		FROM records
		WHERE deleted_at = 0 AND other_cost > 0
		GROUP BY bucket`)
	if err != nil {
		return []models.DistItem{}
	}
	defer rows.Close()
	m := map[string]int{}
	total := 0
	for rows.Next() {
		var b string
		var c int
		if rows.Scan(&b, &c) != nil {
			continue
		}
		m[b] = c
		total += c
	}
	if total == 0 {
		return []models.DistItem{}
	}
	out := []models.DistItem{}
	for _, b := range order {
		if c, ok := m[b]; ok {
			out = append(out, models.DistItem{Name: b, Count: c, Pct: pctOf(float64(total), float64(c))})
		}
	}
	return out
}

// topZhezis ranks the most-watched 折子 (sub-scenes) by performance count.
func (db *DB) topZhezis() []models.RankItem {
	return db.rankFromQuery(`
		SELECT z.id, z.name, COUNT(*) AS cnt
		FROM record_zhezis rz
		JOIN records rc ON rc.id = rz.record_id AND rc.deleted_at = 0
		JOIN zhezis z ON z.id = rz.zhezi_id
		GROUP BY z.id
		ORDER BY cnt DESC
		LIMIT 10`)
}

// rewatchStats measures how many dramas / artists have been seen at least twice.
func (db *DB) rewatchStats() *models.RewatchStats {
	r := &models.RewatchStats{}
	db.conn.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN c >= 2 THEN 1 ELSE 0 END), 0)
		FROM (SELECT rd.drama_id, COUNT(*) AS c FROM record_dramas rd JOIN records rc ON rc.id = rd.record_id AND rc.deleted_at = 0 GROUP BY rd.drama_id)`).
		Scan(&r.TotalDramas, &r.RewatchedDramas)
	db.conn.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN c >= 2 THEN 1 ELSE 0 END), 0)
		FROM (SELECT ra.artist_id, COUNT(*) AS c FROM record_artists ra JOIN records rc ON rc.id = ra.record_id AND rc.deleted_at = 0 GROUP BY ra.artist_id)`).
		Scan(&r.TotalArtists, &r.RewatchedArtists)
	r.DramaRate = pctOf(float64(r.TotalDramas), float64(r.RewatchedDramas))
	r.ArtistRate = pctOf(float64(r.TotalArtists), float64(r.RewatchedArtists))
	return r
}

// discoverySeries returns, for each month, how many artists / dramas were seen
// for the first time (their earliest linked performance falls in that month).
func (db *DB) discoverySeries() []models.DiscoverPoint {
	artistFirst := map[string]int{}
	if rows, err := db.conn.Query(`
		SELECT strftime('%Y-%m', datetime(MIN(r.date), 'unixepoch')) AS m
		FROM record_artists ra
		JOIN records r ON r.id = ra.record_id
		WHERE r.date > 0 AND r.deleted_at = 0
		GROUP BY ra.artist_id`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var m string
			if rows.Scan(&m) == nil {
				artistFirst[m]++
			}
		}
	}
	dramaFirst := map[string]int{}
	if rows, err := db.conn.Query(`
		SELECT strftime('%Y-%m', datetime(MIN(r.date), 'unixepoch')) AS m
		FROM record_dramas rd
		JOIN records r ON r.id = rd.record_id
		WHERE r.date > 0 AND r.deleted_at = 0
		GROUP BY rd.drama_id`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var m string
			if rows.Scan(&m) == nil {
				dramaFirst[m]++
			}
		}
	}
	months := map[string]bool{}
	for m := range artistFirst {
		months[m] = true
	}
	for m := range dramaFirst {
		months[m] = true
	}
	out := []models.DiscoverPoint{}
	for m := range months {
		out = append(out, models.DiscoverPoint{
			Period:     m,
			NewArtists: artistFirst[m],
			NewDramas:  dramaFirst[m],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Period < out[j].Period })
	return out
}

// diversityIndex computes Shannon entropy and normalised evenness for the
// category / artist / drama distributions.
func (db *DB) diversityIndex() *models.DiversityIndex {
	d := &models.DiversityIndex{}
	catCounts := db.groupCounts(`SELECT COUNT(*) AS c FROM records, json_each(records.category_names) je WHERE records.deleted_at = 0 AND je.value != '' GROUP BY je.value`)
	d.CategoryEntropy, d.CategoryEvenness = shannon(catCounts)
	artCounts := db.groupCounts(`SELECT COUNT(*) AS c FROM record_artists ra JOIN records rc ON rc.id = ra.record_id AND rc.deleted_at = 0 GROUP BY ra.artist_id`)
	d.ArtistEntropy, d.ArtistEvenness = shannon(artCounts)
	draCounts := db.groupCounts(`SELECT COUNT(*) AS c FROM record_dramas rd JOIN records rc ON rc.id = rd.record_id AND rc.deleted_at = 0 GROUP BY rd.drama_id`)
	d.DramaEntropy, d.DramaEvenness = shannon(draCounts)
	return d
}

// groupCounts runs a single-column count-per-group query and returns the list
// of per-group counts.
func (db *DB) groupCounts(query string) []int {
	rows, err := db.conn.Query(query)
	if err != nil {
		slog.Warn("analytics group counts", "err", err)
		return []int{}
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var c int
		if rows.Scan(&c) == nil {
			out = append(out, c)
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("analytics group counts", "err", err)
	}
	return out
}

// shannon returns the Shannon entropy and its normalised evenness (0..1).
// Evenness is entropy divided by ln(k) where k is the number of groups.
func shannon(counts []int) (float64, float64) {
	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 || len(counts) <= 1 {
		return 0, 0
	}
	h := 0.0
	for _, c := range counts {
		if c <= 0 {
			continue
		}
		p := float64(c) / float64(total)
		h -= p * math.Log(p)
	}
	evenness := 0.0
	if len(counts) > 1 {
		evenness = h / math.Log(float64(len(counts)))
	}
	return round2(h), round2(evenness)
}

// intervalStats summarises the gaps (in days) between consecutive performances.
func (db *DB) intervalStats() *models.IntervalStats {
	rows, err := db.conn.Query(`SELECT date FROM records WHERE deleted_at = 0 AND date > 0 ORDER BY date`)
	if err != nil {
		return &models.IntervalStats{Buckets: []models.DistItem{}}
	}
	defer rows.Close()
	var dates []int64
	for rows.Next() {
		var d int64
		if rows.Scan(&d) == nil {
			dates = append(dates, d)
		}
	}
	if len(dates) < 2 {
		return &models.IntervalStats{Buckets: []models.DistItem{}}
	}
	gaps := []float64{}
	for i := 1; i < len(dates); i++ {
		gap := float64(dates[i]-dates[i-1]) / 86400.0
		gaps = append(gaps, gap)
	}
	sum := 0.0
	for _, g := range gaps {
		sum += g
	}
	avg := sum / float64(len(gaps))
	sorted := append([]float64{}, gaps...)
	sort.Float64s(sorted)
	median := sorted[len(sorted)/2]
	if len(sorted)%2 == 0 {
		median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}
	maxv := sorted[len(sorted)-1]
	type bucket struct {
		label string
		upper float64
	}
	defs := []bucket{
		{"1天", 1}, {"2天", 2}, {"3天", 3}, {"4天", 4}, {"5天", 5}, {"6天", 6}, {"7天", 7},
		{"8–14天", 14}, {"15–30天", 30}, {"31–60天", 60}, {"61–90天", 90},
		{"91–180天", 180}, {"181–365天", 365}, {">365天", math.MaxFloat64},
	}
	bc := make([]int, len(defs))
	for _, g := range gaps {
		for i, d := range defs {
			if g <= d.upper {
				bc[i]++
				break
			}
		}
	}
	total := len(gaps)
	buckets := []models.DistItem{}
	for i, d := range defs {
		if bc[i] == 0 {
			continue
		}
		buckets = append(buckets, models.DistItem{Name: d.label, Count: bc[i], Pct: pctOf(float64(total), float64(bc[i]))})
	}
	return &models.IntervalStats{
		Avg:     round1(avg),
		Median:  round1(median),
		Max:     round1(maxv),
		Buckets: buckets,
	}
}

// weekdayDist returns show counts per weekday, ordered Monday → Sunday.
func (db *DB) weekdayDist() []models.WeekdayItem {
	names := []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
	order := []int{1, 2, 3, 4, 5, 6, 0}
	counts := map[int]int{}
	if rows, err := db.conn.Query(`SELECT CAST(strftime('%w', datetime(date, 'unixepoch')) AS INTEGER) AS wd, COUNT(*) FROM records WHERE deleted_at = 0 AND date > 0 GROUP BY wd`); err == nil {
		defer rows.Close()
		for rows.Next() {
			var wd, c int
			if rows.Scan(&wd, &c) == nil {
				counts[wd] = c
			}
		}
	}
	out := []models.WeekdayItem{}
	for _, wd := range order {
		out = append(out, models.WeekdayItem{Weekday: wd, Name: names[wd], Count: counts[wd]})
	}
	return out
}
