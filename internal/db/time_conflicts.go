package db

import (
	"mujian/internal/models"
)

// defaultDurationMin 是时长未填写（0）时参与冲突估算的默认分钟数，
// 与前端新建表单的默认时长保持一致（RecordForm emptyForm duration: 120）。
const defaultDurationMin = 120

// FindTimeOverlaps 返回与目标时段 [start, start+duration) 重叠的既有演出。
//
// 口径：
//   - 重叠判定：other.date < startEnd && other.end > start；
//   - 时长未填写（0）时按默认 120 分钟估算区间（新建表单的默认时长）：
//     提醒场景宁可多报 —— 你的库里大多数记录没填时长，未知不估算会导致
//     大量真实冲突漏报，而误报的代价只是多看一眼提示（不阻止保存）；
//   - 只统计 正常(0) / 想看(1) 的记录：已取消 / 未赴约不会再占用购票时间；
//   - excludeID 用于编辑场景排除自身；软删除记录天然排除。
//
// 结果按时间升序、上限 limit 条（表单提示场景不需要全量）。
func (db *DB) FindTimeOverlaps(start int64, durationMin int, excludeID string, limit int) ([]models.TimeConflict, error) {
	if limit <= 0 {
		limit = 20
	}
	if durationMin <= 0 {
		durationMin = defaultDurationMin
	}
	end := start + int64(durationMin)*60
	rows, err := db.conn.Query(`
		SELECT id, name, date, date_text, duration, city, address, active_status, category_name
		FROM records
		WHERE deleted_at = 0
			AND active_status IN (0, 1)
			AND date < ?
			AND date + (CASE WHEN duration > 0 THEN duration ELSE ? END) * 60 > ?
			AND (? = '' OR id <> ?)
		ORDER BY date ASC
		LIMIT ?
	`, end, defaultDurationMin, start, excludeID, excludeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.TimeConflict{}
	for rows.Next() {
		var c models.TimeConflict
		if err := rows.Scan(&c.ID, &c.Name, &c.Date, &c.DateText, &c.Duration,
			&c.City, &c.Address, &c.ActiveStatus, &c.CategoryName); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
