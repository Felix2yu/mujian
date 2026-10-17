package db

import (
	"fmt"
	"mujian/internal/models"
	"time"
)

// 节假日数据的最近一次外部刷新时间存在 meta 表里（key 固定），
// 用于让「每天最多向外部源发起一次刷新」跨进程重启依然生效。
const holidayRefreshedAtKey = "holiday:refreshed_at"

// SeedHolidayDays 写入内置节假日数据作为兜底：只补 DB 中缺失的日期，
// 不覆盖已有行 —— 已有行可能来自更近一次外部刷新，内置数据反而更旧。
func (db *DB) SeedHolidayDays(days []models.HolidayDay, source string) error {
	if len(days) == 0 {
		return nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`
		INSERT INTO holidays (date, name, is_off, source, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(date) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().Unix()
	for _, d := range days {
		if d.Date == "" {
			continue
		}
		if _, err := stmt.Exec(d.Date, d.Name, boolToInt(d.IsOffDay), source, now); err != nil {
			return fmt.Errorf("seed holiday %s: %w", d.Date, err)
		}
	}
	return tx.Commit()
}

// ReplaceHolidayYear 用外部拉取到的某年数据整体替换该年的既有行。
// 先删后插保证「该年已不再放假/补班的日期」也能被同步清掉（数据源勘误）。
func (db *DB) ReplaceHolidayYear(year int, days []models.HolidayDay, source string) error {
	prefix := fmt.Sprintf("%d-", year)
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM holidays WHERE date LIKE ?", prefix+"%"); err != nil {
		return fmt.Errorf("clear holidays %d: %w", year, err)
	}
	stmt, err := tx.Prepare(`
		INSERT INTO holidays (date, name, is_off, source, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET name=excluded.name, is_off=excluded.is_off,
			source=excluded.source, updated_at=excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().Unix()
	for _, d := range days {
		if d.Date == "" || d.Date[:len(prefix)] != prefix {
			continue
		}
		if _, err := stmt.Exec(d.Date, d.Name, boolToInt(d.IsOffDay), source, now); err != nil {
			return fmt.Errorf("replace holiday %s: %w", d.Date, err)
		}
	}
	return tx.Commit()
}

// ListHolidayDays 返回某年的全部节假日 / 调休日，按日期升序。
func (db *DB) ListHolidayDays(year int) ([]models.HolidayDay, error) {
	prefix := fmt.Sprintf("%d-", year)
	rows, err := db.conn.Query(`
		SELECT date, name, is_off, source FROM holidays
		WHERE date LIKE ? ORDER BY date ASC
	`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.HolidayDay{}
	for rows.Next() {
		var d models.HolidayDay
		var off int
		if err := rows.Scan(&d.Date, &d.Name, &off, &d.Source); err != nil {
			return nil, err
		}
		d.IsOffDay = off != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// HolidayRefreshedAt 返回最近一次外部刷新成功的时间（unix 秒），无记录时为 0。
func (db *DB) HolidayRefreshedAt() int64 {
	var raw string
	err := db.conn.QueryRow("SELECT value FROM meta WHERE key = ?", holidayRefreshedAtKey).Scan(&raw)
	if err != nil || raw == "" {
		return 0
	}
	var ts int64
	if _, err := fmt.Sscan(raw, &ts); err != nil {
		return 0
	}
	return ts
}

// SetHolidayRefreshedAt 记录外部刷新成功的时间。
func (db *DB) SetHolidayRefreshedAt(ts int64) error {
	_, err := db.conn.Exec(`
		INSERT INTO meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value
	`, holidayRefreshedAtKey, fmt.Sprintf("%d", ts))
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
