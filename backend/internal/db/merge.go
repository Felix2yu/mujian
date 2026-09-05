package db

import (
	"database/sql"
	"fmt"
	"mujian/internal/models"
	"strings"
)

// MergeArtistsResult reports what MergeArtists did.
type MergeArtistsResult struct {
	SourceID         string   `json:"source_id"`
	TargetID         string   `json:"target_id"`
	RecordsRepointed int64    `json:"records_repointed"` // 演出从 source 改挂到 target
	RecordsDeduped   int64    `json:"records_deduped"`   // 已同时挂 source 与 target，仅移除重复链接
	AliasesAdded     []string `json:"aliases_added"`     // 从 source 并入 target 的别名
	BioTakenOver     bool     `json:"bio_taken_over"`
	RemarkTakenOver  bool     `json:"remark_taken_over"`
	CoverTakenOver   bool     `json:"cover_taken_over"`
}

// MergeArtists merges artist source into target: every record link moves to
// target (links that already point at target are deduped), source's name and
// aliases fold into target's alias list, empty target fields (bio/remark/cover)
// are filled from source, and the source artist row is deleted.
func (db *DB) MergeArtists(sourceID, targetID string) (*MergeArtistsResult, error) {
	if sourceID == "" || targetID == "" {
		return nil, fmt.Errorf("需要提供 source 与 target")
	}
	if sourceID == targetID {
		return nil, fmt.Errorf("source 与 target 是同一个演员，无需合并")
	}

	source, err := db.GetArtist(sourceID)
	if err != nil {
		return nil, fmt.Errorf("source 演员不存在: %w", err)
	}
	target, err := db.GetArtist(targetID)
	if err != nil {
		return nil, fmt.Errorf("target 演员不存在: %w", err)
	}

	res := &MergeArtistsResult{SourceID: sourceID, TargetID: targetID, AliasesAdded: []string{}}

	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Repoint record links. UPDATE OR IGNORE skips rows that would collide
	// with an existing (record_id, target) link; the DELETE afterwards drops
	// those leftover source links. The two affected-row counts distinguish
	// repointed records from deduped ones.
	r1, err := tx.Exec("UPDATE OR IGNORE record_artists SET artist_id = ? WHERE artist_id = ?", targetID, sourceID)
	if err != nil {
		return nil, err
	}
	repointed, _ := r1.RowsAffected()
	r2, err := tx.Exec("DELETE FROM record_artists WHERE artist_id = ?", sourceID)
	if err != nil {
		return nil, err
	}
	deduped, _ := r2.RowsAffected()
	res.RecordsRepointed = repointed
	res.RecordsDeduped = deduped

	// Fold source's name and aliases into target's aliases. target.SortOrder
	// is preserved by only updating content columns here.
	aliases := target.Aliases
	if aliases == nil {
		aliases = []string{}
	}
	seen := map[string]bool{strings.TrimSpace(target.Name): true}
	for _, a := range aliases {
		seen[strings.TrimSpace(a)] = true
	}
	for _, cand := range append([]string{source.Name}, source.Aliases...) {
		cand = strings.TrimSpace(cand)
		if cand == "" || seen[cand] {
			continue
		}
		seen[cand] = true
		aliases = append(aliases, cand)
		res.AliasesAdded = append(res.AliasesAdded, cand)
	}

	bio, remark := target.Bio, target.Remark
	cover, coverFile, coverThumb := target.Cover, target.CoverFile, target.CoverThumb
	if strings.TrimSpace(bio) == "" && strings.TrimSpace(source.Bio) != "" {
		bio = source.Bio
		res.BioTakenOver = true
	}
	if strings.TrimSpace(remark) == "" && strings.TrimSpace(source.Remark) != "" {
		remark = source.Remark
		res.RemarkTakenOver = true
	}
	if coverFile == "" && source.CoverFile != "" {
		cover, coverFile, coverThumb = source.Cover, source.CoverFile, source.CoverThumb
		res.CoverTakenOver = true
	}

	if _, err := tx.Exec(`UPDATE artists SET aliases = ?, bio = ?, remark = ?, cover = ?, cover_file = ?, cover_thumb = ? WHERE id = ?`,
		marshalJSON(aliases), bio, remark, cover, coverFile, coverThumb, targetID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec("DELETE FROM artists WHERE id = ?", sourceID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

// MergeDramasResult reports what MergeDramas did.
type MergeDramasResult struct {
	SourceID         string   `json:"source_id"`
	TargetID         string   `json:"target_id"`
	RecordsRepointed int64    `json:"records_repointed"`
	RecordsDeduped   int64    `json:"records_deduped"`
	ZhezisMoved      int      `json:"zhezis_moved"`       // 折子改挂到 target
	ZhezisDeduped    int      `json:"zhezis_deduped"`     // 与 target 同名折子去重（链接改挂后删除）
	AliasesAdded     []string `json:"aliases_added"`
	RemarkTakenOver  bool     `json:"remark_taken_over"`
}

// MergeDramas merges drama source into target: record links move to target,
// source's zhezis are moved (same-name zhezis have their record links
// repointed to the target's zhezi and the duplicate deleted), name and aliases
// fold into target's aliases, an empty target remark is filled from source,
// and the source drama row is deleted.
func (db *DB) MergeDramas(sourceID, targetID string) (*MergeDramasResult, error) {
	if sourceID == "" || targetID == "" {
		return nil, fmt.Errorf("需要提供 source 与 target")
	}
	if sourceID == targetID {
		return nil, fmt.Errorf("source 与 target 是同一个剧目，无需合并")
	}

	source, err := db.GetDrama(sourceID)
	if err != nil {
		return nil, fmt.Errorf("source 剧目不存在: %w", err)
	}
	target, err := db.GetDrama(targetID)
	if err != nil {
		return nil, fmt.Errorf("target 剧目不存在: %w", err)
	}

	res := &MergeDramasResult{SourceID: sourceID, TargetID: targetID, AliasesAdded: []string{}}

	tx, err := db.conn.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	r1, err := tx.Exec("UPDATE OR IGNORE record_dramas SET drama_id = ? WHERE drama_id = ?", targetID, sourceID)
	if err != nil {
		return nil, err
	}
	repointed, _ := r1.RowsAffected()
	r2, err := tx.Exec("DELETE FROM record_dramas WHERE drama_id = ?", sourceID)
	if err != nil {
		return nil, err
	}
	deduped, _ := r2.RowsAffected()
	res.RecordsRepointed = repointed
	res.RecordsDeduped = deduped

	// Move source zhezis; same-name zhezis on target absorb the record links
	// and are deleted so target never ends up with duplicate zhezi names.
	targetZhezis, err := db.listZhezisByDramaTx(tx, targetID)
	if err != nil {
		return nil, err
	}
	byName := map[string]string{}
	for _, z := range targetZhezis {
		byName[z.Name] = z.ID
	}
	srcZhezis, err := db.listZhezisByDramaTx(tx, sourceID)
	if err != nil {
		return nil, err
	}
	for _, sz := range srcZhezis {
		if dupID, ok := byName[sz.Name]; ok {
			if _, err := tx.Exec("UPDATE OR IGNORE record_zhezis SET zhezi_id = ? WHERE zhezi_id = ?", dupID, sz.ID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec("DELETE FROM record_zhezis WHERE zhezi_id = ?", sz.ID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec("DELETE FROM zhezis WHERE id = ?", sz.ID); err != nil {
				return nil, err
			}
			res.ZhezisDeduped++
			continue
		}
		if _, err := tx.Exec("UPDATE zhezis SET drama_id = ? WHERE id = ?", targetID, sz.ID); err != nil {
			return nil, err
		}
		byName[sz.Name] = sz.ID
		res.ZhezisMoved++
	}

	aliases := target.Aliases
	if aliases == nil {
		aliases = []string{}
	}
	seen := map[string]bool{strings.TrimSpace(target.Name): true}
	for _, a := range aliases {
		seen[strings.TrimSpace(a)] = true
	}
	for _, cand := range append([]string{source.Name}, source.Aliases...) {
		cand = strings.TrimSpace(cand)
		if cand == "" || seen[cand] {
			continue
		}
		seen[cand] = true
		aliases = append(aliases, cand)
		res.AliasesAdded = append(res.AliasesAdded, cand)
	}

	remark := target.Remark
	if strings.TrimSpace(remark) == "" && strings.TrimSpace(source.Remark) != "" {
		remark = source.Remark
		res.RemarkTakenOver = true
	}

	if _, err := tx.Exec("UPDATE dramas SET aliases = ?, remark = ? WHERE id = ?",
		marshalJSON(aliases), remark, targetID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec("DELETE FROM dramas WHERE id = ?", sourceID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

// listZhezisByDramaTx is ListZhezisByDrama bound to a transaction so merge
// steps observe a consistent snapshot.
func (db *DB) listZhezisByDramaTx(tx *sql.Tx, dramaID string) ([]models.Zhezi, error) {
	rows, err := tx.Query("SELECT id, name, aliases, sort_order, remark FROM zhezis WHERE drama_id = ? ORDER BY sort_order, name", dramaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Zhezi{}
	for rows.Next() {
		var z models.Zhezi
		var rawAliases string
		if err := rows.Scan(&z.ID, &z.Name, &rawAliases, &z.SortOrder, &z.Remark); err != nil {
			return nil, err
		}
		z.Aliases = unmarshalStrings(rawAliases)
		out = append(out, z)
	}
	return out, rows.Err()
}
