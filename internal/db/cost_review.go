package db

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"mujian/internal/models"
)

// 费用补全（cost review）数据层。
//
// 背景与语义模型详见 models.CostPendingRecord 上方的注释。核心不变量：
//
//  1. 列值 > 0 的字段天然「已填写」，永远不会被本模块读写；
//  2. 待确认集合 = (列值 IS NULL OR 列值 = 0) AND 无 cost_reviews 行；
//  3. 所有写操作都带 `字段 IS NULL OR 字段 = 0` 守卫，即便前端传入了
//     过期的 id 列表，也不可能覆盖一个已被填成真实金额的有效值。

// costFieldColumn 把逻辑字段名映射到物理列名。这是全项目唯一把列名拼进
// SQL 的地方，其余位置一律用该白名单的返回值，避免 SQL 注入。
func costFieldColumn(field string) (string, bool) {
	switch field {
	case models.CostFieldPrice:
		return "price", true
	case models.CostFieldPayPrice:
		return "pay_price", true
	case models.CostFieldOtherCost:
		return "other_cost", true
	}
	return "", false
}

// costFields 返回被费用补全管理字段的规范顺序。
func costFields() []string {
	return []string{models.CostFieldPrice, models.CostFieldPayPrice, models.CostFieldOtherCost}
}

// costPlaceholders 生成 n 个逗号分隔的 "?" 占位符。
func costPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// costStringArgs 把 id 切片转成 []interface{}，供 Exec 变参使用。
func costStringArgs(ids []string) []interface{} {
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

// totalCostExpr 是 total_cost 的权威算式，与迁移期的回填 SQL 及
// UpsertRecord 内的 Go 版本保持一致（pay_price 优先，否则 price，
// 再加 other_cost；NULL 视为 0）。
const totalCostExpr = `(CASE WHEN pay_price > 0 THEN pay_price ELSE COALESCE(price, 0) END) + COALESCE(other_cost, 0)`

// ListCostPending 返回所有「在 enabled 字段中至少有一个待确认」的在库记录，
// 并为每条记录标出具体哪些字段仍待确认。enabled 为空表示不纳入任何字段，
// 直接返回空列表（调用方应传入 GetCostEnabledFields 的结果）。
//
// limit 保护性地兜底（<=0 或 >5000 时取 5000）；概览计数请用 CostSummary，
// 它不受该上限影响。
func (db *DB) ListCostPending(ctx context.Context, limit int, enabled []string) ([]models.CostPendingRecord, error) {
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	// 仅纳入白名单字段，避免把任意字符串拼进 SQL（costFieldColumn 已做二次校验）。
	fields := make([]string, 0, len(enabled))
	for _, f := range enabled {
		if _, ok := costFieldColumn(f); ok {
			fields = append(fields, f)
		}
	}
	if len(fields) == 0 {
		return []models.CostPendingRecord{}, nil
	}

	// LIST 与 WHERE 语义不同：WHERE 是「任一字段待确认」，CASE 是「逐个字段
	// 是否待确认」。按 enabled 动态生成这两组片段，字段名经白名单校验后安全。
	baseCols := []string{
		"r.id", "r.name", "r.date", "r.city", "r.address",
		"r.price", "r.pay_price", "r.other_cost", "r.total_cost",
	}
	caseExprs := make([]string, 0, len(fields))
	whereParts := make([]string, 0, len(fields))
	for _, f := range fields {
		col, _ := costFieldColumn(f)
		caseExprs = append(caseExprs,
			fmt.Sprintf("CASE WHEN (r.%s IS NULL OR r.%s = 0) AND NOT EXISTS (SELECT 1 FROM cost_reviews c WHERE c.record_id = r.id AND c.field = '%s') THEN 1 ELSE 0 END", col, col, f))
		whereParts = append(whereParts,
			fmt.Sprintf("((r.%s IS NULL OR r.%s = 0) AND NOT EXISTS (SELECT 1 FROM cost_reviews c WHERE c.record_id = r.id AND c.field = '%s'))", col, col, f))
	}
	q := fmt.Sprintf(`
SELECT %s, %s
FROM records r
WHERE r.deleted_at = 0 AND (%s)
ORDER BY r.date DESC, r.id DESC
LIMIT ?`, strings.Join(baseCols, ", "), strings.Join(caseExprs, ", "), strings.Join(whereParts, " OR "))

	rows, err := db.conn.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list cost pending: %w", err)
	}
	defer rows.Close()

	out := make([]models.CostPendingRecord, 0, 64)
	for rows.Next() {
		var (
			rec               models.CostPendingRecord
			price, pay, other sql.NullFloat64
			pend              = make([]int, len(fields))
		)
		scanArgs := []interface{}{
			&rec.ID, &rec.Name, &rec.Date, &rec.City, &rec.Address,
			&price, &pay, &other, &rec.TotalCost,
		}
		for i := range fields {
			scanArgs = append(scanArgs, &pend[i])
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, fmt.Errorf("scan cost pending: %w", err)
		}
		rec.Price = nullFloatPtr(price)
		rec.PayPrice = nullFloatPtr(pay)
		rec.OtherCost = nullFloatPtr(other)
		rec.PendingFields = make([]string, 0, len(fields))
		for i, f := range fields {
			if pend[i] == 1 {
				rec.PendingFields = append(rec.PendingFields, f)
			}
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cost pending: %w", err)
	}
	return out, nil
}

// nullFloatPtr 把 sql.NullFloat64 转成裸指针：NULL 保持 nil（未填写），
// 有效值（含 0）返回其指针。
func nullFloatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	f := v.Float64
	return &f
}

// CostSummary 返回费用补全模块的概览计数（全量，不受列表 limit 影响）。
// enabled 为空表示不纳入任何字段：概览的待确认/已标记都为 0，待确认记录数也为 0。
func (db *DB) CostSummary(enabled []string) (*models.CostSummary, error) {
	s := &models.CostSummary{
		Pending:  make(map[string]int, 3),
		Reviewed: make(map[string]int, 3),
	}
	if err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM records WHERE deleted_at = 0`).Scan(&s.TotalRecords); err != nil {
		return nil, fmt.Errorf("count records: %w", err)
	}

	// 仅统计白名单字段，字段名经 costFieldColumn 二次校验后安全拼入 SQL。
	fields := make([]string, 0, len(enabled))
	for _, f := range enabled {
		if _, ok := costFieldColumn(f); ok {
			fields = append(fields, f)
		}
	}

	for _, field := range fields {
		col, _ := costFieldColumn(field)

		var pending int
		pq := fmt.Sprintf(`SELECT COUNT(*) FROM records r
			WHERE r.deleted_at = 0 AND (r.%s IS NULL OR r.%s = 0)
			  AND NOT EXISTS (SELECT 1 FROM cost_reviews c WHERE c.record_id = r.id AND c.field = ?)`, col, col)
		if err := db.conn.QueryRow(pq, field).Scan(&pending); err != nil {
			return nil, fmt.Errorf("count pending %s: %w", field, err)
		}
		s.Pending[field] = pending

		// 只统计仍在库（未软删）的记录，避免回收站里的确认记录抬高计数。
		var reviewed int
		if err := db.conn.QueryRow(
			`SELECT COUNT(*) FROM cost_reviews c JOIN records r ON r.id = c.record_id
			 WHERE r.deleted_at = 0 AND c.field = ?`, field).Scan(&reviewed); err != nil {
			return nil, fmt.Errorf("count reviewed %s: %w", field, err)
		}
		s.Reviewed[field] = reviewed
		s.ReviewedTotal += reviewed
	}

	if len(fields) == 0 {
		s.PendingRecords = 0
		return s, nil
	}
	whereParts := make([]string, 0, len(fields))
	for _, f := range fields {
		col, _ := costFieldColumn(f)
		whereParts = append(whereParts,
			fmt.Sprintf("((r.%s IS NULL OR r.%s = 0) AND NOT EXISTS (SELECT 1 FROM cost_reviews c WHERE c.record_id = r.id AND c.field = '%s'))", col, col, f))
	}
	pendingRecordsQ := fmt.Sprintf(`
SELECT COUNT(*) FROM records r
WHERE r.deleted_at = 0 AND (%s)`, strings.Join(whereParts, " OR "))
	if err := db.conn.QueryRow(pendingRecordsQ).Scan(&s.PendingRecords); err != nil {
		return nil, fmt.Errorf("count pending records: %w", err)
	}
	return s, nil
}

// recomputeTotalCostTx 用权威算式刷新给定记录的 total_cost 缓存列。
// 必须在写入费用列之后单独执行 —— SQL 的 SET 表达式一律基于行更新前的旧值，
// 把 total_cost 与费用列写在同一条 UPDATE 里会算错。
func recomputeTotalCostTx(tx *sql.Tx, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	q := fmt.Sprintf(`UPDATE records SET total_cost = %s
		WHERE deleted_at = 0 AND id IN (%s)`, totalCostExpr, costPlaceholders(len(ids)))
	if _, err := tx.Exec(q, costStringArgs(ids)...); err != nil {
		return fmt.Errorf("recompute total_cost: %w", err)
	}
	return nil
}

// upsertCostReviewTx 写入 / 更新一条确认记录。仅当该字段当前仍为 NULL/0
// （即确实处于待确认态）时才落库，返回实际写入的行数。
func upsertCostReviewTx(tx *sql.Tx, ids []string, field, state string) (int64, error) {
	col, ok := costFieldColumn(field)
	if !ok {
		return 0, fmt.Errorf("unknown cost field %q", field)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	q := fmt.Sprintf(`INSERT INTO cost_reviews (record_id, field, state, updated_at)
		SELECT r.id, ?, ?, ? FROM records r
		WHERE r.deleted_at = 0 AND (r.%s IS NULL OR r.%s = 0) AND r.id IN (%s)
		ON CONFLICT(record_id, field) DO UPDATE SET state = excluded.state, updated_at = excluded.updated_at`,
		col, col, costPlaceholders(len(ids)))
	args := append([]interface{}{field, state, time.Now().Unix()}, costStringArgs(ids)...)
	res, err := tx.Exec(q, args...)
	if err != nil {
		return 0, fmt.Errorf("upsert cost review %s: %w", field, err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// MarkCostZero 把给定记录的某个费用字段确认为「真实 0 元」（免费 / 无支出）：
// 列值落为 0（与 NULL 的「未填写」明确区分开），并写入确认记录使其离开
// 待确认列表。
//
// 返回值列的原始值被改写（含 NULL→0）的记录数。已填成 >0 的记录不会被触及。
func (db *DB) MarkCostZero(ids []string, field string) (int64, error) {
	col, ok := costFieldColumn(field)
	if !ok {
		return 0, fmt.Errorf("unknown cost field %q", field)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// NULL → 0 才是真正的改写；本来就是 0 的行 RowsAffected 也会 +1，
	// 这里不区分二者（调用方只关心「有哪些记录被确认」）。
	q := fmt.Sprintf(`UPDATE records SET %s = 0
		WHERE deleted_at = 0 AND (%s IS NULL OR %s = 0) AND id IN (%s)`,
		col, col, col, costPlaceholders(len(ids)))
	res, err := tx.Exec(q, costStringArgs(ids)...)
	if err != nil {
		return 0, fmt.Errorf("mark cost zero %s: %w", field, err)
	}
	touched, _ := res.RowsAffected()

	if err := recomputeTotalCostTx(tx, ids); err != nil {
		return 0, err
	}
	if _, err := upsertCostReviewTx(tx, ids, field, models.CostStateZero); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return touched, nil
}

// MarkCostSkip 把给定记录的某个费用字段确认为「暂不处理 / 无法确定」：
// 列值原样保留，只写确认记录，使其离开待确认列表（可在「已标记」区撤销）。
func (db *DB) MarkCostSkip(ids []string, field string) (int64, error) {
	if _, ok := costFieldColumn(field); !ok {
		return 0, fmt.Errorf("unknown cost field %q", field)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	n, err := upsertCostReviewTx(tx, ids, field, models.CostStateSkip)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// SetCostAmount 为给定记录的某个费用字段填入明确金额。amount <= 0 时退化为
// MarkCostZero（0 元即「免费 / 无支出」）；amount > 0 时写入金额并删除该字段
// 上可能存在的历史确认记录 —— 有了真实金额，确认记录已无意义。
//
// 同样只作用于当前为 NULL/0 的字段，绝不覆盖已填写的有效金额。
func (db *DB) SetCostAmount(ids []string, field string, amount float64) (int64, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, fmt.Errorf("invalid amount")
	}
	if amount <= 0 {
		return db.MarkCostZero(ids, field)
	}
	col, ok := costFieldColumn(field)
	if !ok {
		return 0, fmt.Errorf("unknown cost field %q", field)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	q := fmt.Sprintf(`UPDATE records SET %s = ?
		WHERE deleted_at = 0 AND (%s IS NULL OR %s = 0) AND id IN (%s)`,
		col, col, col, costPlaceholders(len(ids)))
	args := append([]interface{}{amount}, costStringArgs(ids)...)
	res, err := tx.Exec(q, args...)
	if err != nil {
		return 0, fmt.Errorf("set cost amount %s: %w", field, err)
	}
	touched, _ := res.RowsAffected()

	if err := recomputeTotalCostTx(tx, ids); err != nil {
		return 0, err
	}
	delQ := fmt.Sprintf(`DELETE FROM cost_reviews WHERE field = ? AND record_id IN (%s)`,
		costPlaceholders(len(ids)))
	if _, err := tx.Exec(delQ, append([]interface{}{field}, costStringArgs(ids)...)...); err != nil {
		return 0, fmt.Errorf("clear cost review %s: %w", field, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return touched, nil
}

// UnreviewCost 撤销指定记录的费用确认，使其重新回到待确认列表（列值不变）。
// field 为空表示撤销这些记录的全部三个字段。
func (db *DB) UnreviewCost(ids []string, field string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if field != "" {
		if _, ok := costFieldColumn(field); !ok {
			return 0, fmt.Errorf("unknown cost field %q", field)
		}
	}
	args := make([]interface{}, 0, len(ids)+1)
	q := fmt.Sprintf(`DELETE FROM cost_reviews WHERE record_id IN (%s)`, costPlaceholders(len(ids)))
	args = append(args, costStringArgs(ids)...)
	if field != "" {
		q += ` AND field = ?`
		args = append(args, field)
	}
	res, err := db.conn.Exec(q, args...)
	if err != nil {
		return 0, fmt.Errorf("unreview cost: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// UnreviewCostAll 撤销某个字段（field 为空表示全部字段）的所有费用确认。
// 用于「撤销该分组的全部标记」。
func (db *DB) UnreviewCostAll(field string) (int64, error) {
	var (
		res sql.Result
		err error
	)
	if field == "" {
		res, err = db.conn.Exec(`DELETE FROM cost_reviews`)
	} else {
		if _, ok := costFieldColumn(field); !ok {
			return 0, fmt.Errorf("unknown cost field %q", field)
		}
		res, err = db.conn.Exec(`DELETE FROM cost_reviews WHERE field = ?`, field)
	}
	if err != nil {
		return 0, fmt.Errorf("unreview cost all: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// clearCostReviewsForFilled 在记录被整体保存（新建 / 编辑 / 导入）后，清掉
// 「已填成真实金额」字段上陈旧的确认标记：有了明确金额，之前的「0 元确认」
// 只会误导统计。对仍为 NULL/0 的字段保留标记，避免用户对一条记录其他字段的
// 编辑把已完成的费用确认弄丢。
func clearCostReviewsForFilled(exec sqlExecutor, recordID string, r models.Record) error {
	fields := make([]string, 0, 3)
	if r.Price != nil && *r.Price > 0 {
		fields = append(fields, models.CostFieldPrice)
	}
	if r.PayPrice != nil && *r.PayPrice > 0 {
		fields = append(fields, models.CostFieldPayPrice)
	}
	if r.OtherCost != nil && *r.OtherCost > 0 {
		fields = append(fields, models.CostFieldOtherCost)
	}
	for _, f := range fields {
		if _, err := exec.Exec(
			`DELETE FROM cost_reviews WHERE record_id = ? AND field = ?`, recordID, f); err != nil {
			return fmt.Errorf("clear cost review %s/%s: %w", recordID, f, err)
		}
	}
	return nil
}
