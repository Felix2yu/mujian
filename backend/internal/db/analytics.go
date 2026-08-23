package db

import (
	"fmt"
	"strings"
)

// VenueGroup is an aggregated view of records sharing the same venue address.
// Venues have no dedicated entity table — records.address acts as the implicit
// identifier — so grouping by address with counts is how "the same place"
// (including near-duplicate spellings) is surfaced for cleanup workflows.
type VenueGroup struct {
	Address     string   `json:"address"`
	Cities      []string `json:"cities"`
	RecordCount int      `json:"record_count"`
	HasCoord    bool     `json:"has_coord"`
}

// ListVenueGroups returns address groups, optionally filtered by a
// case-insensitive substring match on the address. Empty query returns all
// groups. Groups are ordered by record count descending so busy venues come
// first.
func (db *DB) ListVenueGroups(query string) ([]VenueGroup, error) {
	rows, err := db.conn.Query(`
		SELECT address,
		       COUNT(*)          AS cnt,
		       MAX(coordinate != '' AND coordinate != 'null') AS has_coord
		FROM records
		WHERE address != ''
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
		if err := rows.Scan(&g.Address, &g.RecordCount, &g.HasCoord); err != nil {
			return nil, err
		}
		if q != "" && !strings.Contains(strings.ToLower(g.Address), q) {
			continue
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
			`SELECT DISTINCT city FROM records WHERE address = ? AND city != '' ORDER BY city LIMIT 5`,
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
		`SELECT %s, COUNT(*) FROM records WHERE %s != '' GROUP BY %s ORDER BY COUNT(*) DESC, %s`,
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
