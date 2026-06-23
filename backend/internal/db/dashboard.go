package db

import "mujian/internal/models"

type DashboardStats struct {
	TotalShows   int             `json:"total_shows"`
	TotalCost    float64         `json:"total_cost"`
	AvgRating    float64         `json:"avg_rating"`
	TotalVenues  int             `json:"total_venues"`
	TotalHours   float64         `json:"total_hours"`
	ByMonth      []MonthStat     `json:"by_month"`
	ByCategory   []CategoryStat  `json:"by_category"`
	ByVenue      []VenueStat     `json:"by_venue"`
	ByStatus     []StatusStat    `json:"by_status"`
	CostByMonth  []CostStat      `json:"cost_by_month"`
	TopRated     []models.Show   `json:"top_rated"`
	RecentWatched []models.Show  `json:"recent_watched"`
}

type MonthStat struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

type CategoryStat struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	Count int    `json:"count"`
}

type VenueStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type StatusStat struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type CostStat struct {
	Month string  `json:"month"`
	Cost  float64 `json:"cost"`
}

func (db *DB) GetDashboardStats() (*DashboardStats, error) {
	s := &DashboardStats{}

	db.conn.QueryRow("SELECT COUNT(*) FROM shows").Scan(&s.TotalShows)
	db.conn.QueryRow("SELECT COALESCE(SUM(COALESCE(ticket_cost,0) + COALESCE(other_cost,0)), 0) FROM shows").Scan(&s.TotalCost)
	db.conn.QueryRow("SELECT COALESCE(AVG(CAST(rating AS REAL)), 0) FROM shows WHERE rating IS NOT NULL").Scan(&s.AvgRating)
	db.conn.QueryRow("SELECT COUNT(DISTINCT venue) FROM shows WHERE venue != ''").Scan(&s.TotalVenues)
	db.conn.QueryRow("SELECT COALESCE(SUM(duration), 0) / 60.0 FROM shows").Scan(&s.TotalHours)

	// By month (last 12 months)
	rows, err := db.conn.Query(`
		SELECT strftime('%Y-%m', date) as month, COUNT(*) as cnt
		FROM shows
		WHERE date >= date('now', '-12 months')
		GROUP BY month
		ORDER BY month
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var m MonthStat
			rows.Scan(&m.Month, &m.Count)
			s.ByMonth = append(s.ByMonth, m)
		}
	}

	// By category
	rows2, err := db.conn.Query(`
		SELECT c.name, c.color, COUNT(s.id) as cnt
		FROM categories c
		LEFT JOIN shows s ON s.category_id = c.id
		GROUP BY c.id
		HAVING cnt > 0
		ORDER BY cnt DESC
	`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var c CategoryStat
			rows2.Scan(&c.Name, &c.Color, &c.Count)
			s.ByCategory = append(s.ByCategory, c)
		}
	}

	// By venue
	rows3, err := db.conn.Query(`
		SELECT venue, COUNT(*) as cnt
		FROM shows
		WHERE venue != ''
		GROUP BY venue
		ORDER BY cnt DESC
		LIMIT 10
	`)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var v VenueStat
			rows3.Scan(&v.Name, &v.Count)
			s.ByVenue = append(s.ByVenue, v)
		}
	}

	// By status
	rows4, err := db.conn.Query(`
		SELECT status, COUNT(*) as cnt
		FROM shows
		GROUP BY status
	`)
	if err == nil {
		defer rows4.Close()
		for rows4.Next() {
			var st StatusStat
			rows4.Scan(&st.Status, &st.Count)
			s.ByStatus = append(s.ByStatus, st)
		}
	}

	// Cost by month
	rows5, err := db.conn.Query(`
		SELECT strftime('%Y-%m', date) as month, SUM(COALESCE(ticket_cost,0) + COALESCE(other_cost,0)) as cost
		FROM shows
		WHERE date >= date('now', '-12 months')
		AND (ticket_cost IS NOT NULL OR other_cost IS NOT NULL)
		GROUP BY month
		ORDER BY month
	`)
	if err == nil {
		defer rows5.Close()
		for rows5.Next() {
			var c CostStat
			rows5.Scan(&c.Month, &c.Cost)
			s.CostByMonth = append(s.CostByMonth, c)
		}
	}

	// Top rated
	topRows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.rating IS NOT NULL
		ORDER BY s.rating DESC, s.date DESC
		LIMIT 5
	`)
	if err == nil {
		defer topRows.Close()
		s.TopRated, _ = scanShows(topRows)
	}

	// Recent watched
	recentRows, err := db.conn.Query(`
		SELECT s.id, s.name, s.venue, s.date, s.duration, s.status, s.category_id,
		       s.poster_url, s.setlist, s.cast, s.company, s.friends, s.rating,
		       s.seat, s.notes, s.review, s.ticket_cost, s.other_cost,
		       s.created_at, s.updated_at, COALESCE(c.name, '') as category_name
		FROM shows s
		LEFT JOIN categories c ON s.category_id = c.id
		WHERE s.status = 'watched'
		ORDER BY s.date DESC
		LIMIT 5
	`)
	if err == nil {
		defer recentRows.Close()
		s.RecentWatched, _ = scanShows(recentRows)
	}

	return s, nil
}
