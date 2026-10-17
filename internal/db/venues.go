package db

import (
	"database/sql"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Venue 是场馆的去重实体层：以 records.address 为唯一真源，venues 表只负责把
// 同一物理场馆的多种写法（「苏州开明大戏院」/「开明大戏院（观前街店）」）归并到
// 一个规范名，并在合并时把这些记录的 address 改写为规范名。records 不新增
// venue_id 列——保持 address 自包含，避免污染写入热路径与全站 address 展示逻辑。
type Venue struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	City        string   `json:"city"`
	Lat         float64  `json:"lat"`
	Lng         float64  `json:"lng"`
	Aliases     []string `json:"aliases"`
	SortOrder   int      `json:"sort_order"`
	// RecordCount 是派生值（非持久化的实时演出数）。
	RecordCount int `json:"record_count"`
	// DupGroup 由服务端按「同城 + 去后缀/去括号/去城市前缀 + 坐标<300m」聚合，
	// 仅用于前端把疑似重复场馆归组展示；同一组内的合并由用户显式确认。
	DupGroup int `json:"dup_group,omitempty"`
}

var (
	venueParenRe = regexp.MustCompile(`[（(][^（）()]*[）)]`)
	venueWSRe    = regexp.MustCompile(`\s+`)
	venueSuffixes = []string{"大戏院", "大剧院", "戏院", "剧院", "剧场", "艺术中心", "会堂", "中心", "大会堂", "体育馆", "博物馆", "文化馆"}
)

// normVenueName 归一化场馆名：去括号、压缩空白、去首尾空格。
func normVenueName(s string) string {
	s = venueParenRe.ReplaceAllString(s, "")
	s = venueWSRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// venueDupKey 计算疑似重复分组键：同城 + 去掉常见场馆后缀与城市前缀后的基名。
func venueDupKey(city, name string) string {
	base := normVenueName(name)
	for _, suf := range venueSuffixes {
		if strings.HasSuffix(base, suf) && len(base) > len(suf) {
			base = strings.TrimSpace(strings.TrimSuffix(base, suf))
		}
	}
	base = strings.TrimSpace(strings.TrimPrefix(base, strings.TrimSpace(city)))
	return strings.TrimSpace(city) + "\u0000" + base
}

func (db *DB) createVenuesTable() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS venues (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			city TEXT NOT NULL DEFAULT '',
			lat REAL NOT NULL DEFAULT 0,
			lng REAL NOT NULL DEFAULT 0,
			aliases TEXT NOT NULL DEFAULT '[]',
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_venues_name ON venues(name)`,
		`CREATE INDEX IF NOT EXISTS idx_venues_city ON venues(city)`,
	}
	for _, s := range stmts {
		if _, err := db.conn.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

// BackfillVenuesFromRecords 仅在所有记录都没有对应场馆时运行一次：
// 按去重后的 address 生成规范场馆，规范名取该地址出现最多的写法，坐标取均值。
func (db *DB) BackfillVenuesFromRecords() error {
	var n int
	if err := db.conn.QueryRow("SELECT COUNT(*) FROM venues").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	rows, err := db.conn.Query(`
		SELECT address, city, coordinate
		FROM records WHERE deleted_at = 0 AND TRIM(address) <> ''`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type acc struct {
		city    string
		spell   map[string]int // 不同写法计数
		latSum  float64
		lngSum  float64
		coordN  int
	}
	byNorm := map[string]*acc{}
	for rows.Next() {
		var address, city, coord string
		if err := rows.Scan(&address, &city, &coord); err != nil {
			return err
		}
		norm := normVenueName(address)
		if norm == "" {
			continue
		}
		a, ok := byNorm[norm]
		if !ok {
			a = &acc{city: strings.TrimSpace(city), spell: map[string]int{}}
			byNorm[norm] = a
		}
		a.spell[strings.TrimSpace(address)]++
		if lat, lng, ok := parseCoord(coord); ok {
			a.latSum += lat
			a.lngSum += lng
			a.coordN++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	keys := make([]string, 0, len(byNorm))
	for k := range byNorm {
		keys = append(keys, k)
	}
	sort.Strings(keys) // 稳定排序，便于测试

	for _, norm := range keys {
		a := byNorm[norm]
		canonical := ""
		best := 0
		for spell, c := range a.spell {
			if c > best || (c == best && spell < canonical) {
				best = c
				canonical = spell
			}
		}
		lat, lng := 0.0, 0.0
		if a.coordN > 0 {
			lat, lng = a.latSum/float64(a.coordN), a.lngSum/float64(a.coordN)
		}
		if _, err := db.conn.Exec(
			"INSERT INTO venues (id, name, city, lat, lng, sort_order) VALUES (?, ?, ?, ?, ?, (SELECT COALESCE(MAX(sort_order),0)+1 FROM venues))",
			newID(), canonical, a.city, lat, lng); err != nil {
			return err
		}
	}
	return nil
}

// RescanVenues 重新同步场馆层与当前记录：
//   - 为当前未代表的 address 新建场馆；
//   - 删除已无任何记录（record_count=0）的场馆（含被合并掉的源），避免僵尸行。
//
// 已合并的规范场馆因记录都改写到规范地址而仍有记录，故不会被清掉。
func (db *DB) RescanVenues() (added, removed int, err error) {
	// 当前每个规范名对应的场馆 id
	rows, err := db.conn.Query("SELECT id, name FROM venues")
	if err != nil {
		return 0, 0, err
	}
	venueByNorm := map[string]string{}
	var venueIDs []string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return 0, 0, err
		}
		venueByNorm[normVenueName(name)] = id
		venueIDs = append(venueIDs, id)
	}
	rows.Close()

	// 当前记录里每个规范名出现的最常用写法与坐标均值
	type acc struct {
		spell  map[string]int
		latSum float64
		lngSum float64
		coordN int
		city   string
	}
	byNorm := map[string]*acc{}
	r2, err := db.conn.Query("SELECT address, city, coordinate FROM records WHERE deleted_at = 0 AND TRIM(address) <> ''")
	if err != nil {
		return 0, 0, err
	}
	for r2.Next() {
		var address, city, coord string
		if err := r2.Scan(&address, &city, &coord); err != nil {
			r2.Close()
			return 0, 0, err
		}
		norm := normVenueName(address)
		if norm == "" {
			continue
		}
		a, ok := byNorm[norm]
		if !ok {
			a = &acc{city: strings.TrimSpace(city), spell: map[string]int{}}
			byNorm[norm] = a
		}
		a.spell[strings.TrimSpace(address)]++
		if lat, lng, ok := parseCoord(coord); ok {
			a.latSum += lat
			a.lngSum += lng
			a.coordN++
		}
	}
	r2.Close()

	// 新增缺失的场馆
	for norm, a := range byNorm {
		if _, exists := venueByNorm[norm]; exists {
			continue
		}
		canonical := ""
		best := 0
		for spell, c := range a.spell {
			if c > best || (c == best && spell < canonical) {
				best = c
				canonical = spell
			}
		}
		lat, lng := 0.0, 0.0
		if a.coordN > 0 {
			lat, lng = a.latSum/float64(a.coordN), a.lngSum/float64(a.coordN)
		}
		if _, err := db.conn.Exec(
			"INSERT INTO venues (id, name, city, lat, lng, sort_order) VALUES (?, ?, ?, ?, ?, (SELECT COALESCE(MAX(sort_order),0)+1 FROM venues))",
			newID(), canonical, a.city, lat, lng); err != nil {
			return added, removed, err
		}
		added++
	}

	// 删除无记录的场馆（含被合并掉的源）：应用层统计实时记录数。
	all, err := db.listVenuesRaw()
	if err != nil {
		return added, removed, err
	}
	for _, v := range all {
		cnt, err := db.countRecordsByVenueName(v.Name)
		if err != nil {
			return added, removed, err
		}
		if cnt == 0 {
			if _, err := db.conn.Exec("DELETE FROM venues WHERE id = ?", v.ID); err != nil {
				return added, removed, err
			}
			removed++
		}
	}
	return added, removed, nil
}

// countRecordsByVenueName 统计地址归一化后等于该规范名的非删除记录数。
func (db *DB) countRecordsByVenueName(name string) (int, error) {
	norm := normVenueName(name)
	var total int
	rows, err := db.conn.Query("SELECT address FROM records WHERE deleted_at = 0 AND TRIM(address) <> ''")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			return 0, err
		}
		if normVenueName(addr) == norm {
			total++
		}
	}
	return total, rows.Err()
}

func (db *DB) listVenuesRaw() ([]Venue, error) {
	rows, err := db.conn.Query("SELECT id, name, city, lat, lng, aliases, sort_order FROM venues ORDER BY sort_order, name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Venue{}
	for rows.Next() {
		var v Venue
		var rawAliases string
		if err := rows.Scan(&v.ID, &v.Name, &v.City, &v.Lat, &v.Lng, &rawAliases, &v.SortOrder); err != nil {
			return nil, err
		}
		v.Aliases = unmarshalStrings(rawAliases)
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListVenues 返回带实时记录数与疑似重复分组的场馆列表。
func (db *DB) ListVenues() ([]Venue, error) {
	raw, err := db.listVenuesRaw()
	if err != nil {
		return nil, err
	}
	// 计算每个场馆实时记录数
	for i := range raw {
		c, err := db.countRecordsByVenueName(raw[i].Name)
		if err != nil {
			return nil, err
		}
		raw[i].RecordCount = c
	}
	// 按 dup_key 分组；再按质心 <300m 合并组（union-find）。
	venueByID := make(map[string]Venue, len(raw))
	for _, v := range raw {
		venueByID[v.ID] = v
	}
	keyOf := map[string]int{} // "k:<dupkey>" -> group root
	nextGroup := 1
	for _, v := range raw {
		k := "k:" + venueDupKey(v.City, v.Name)
		if _, ok := keyOf[k]; !ok {
			keyOf[k] = nextGroup
			nextGroup++
		}
	}
	// 质心邻近合并
	parent := map[int]int{}
	for g := 1; g < nextGroup; g++ {
		parent[g] = g
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	ids := make([]string, 0, len(raw))
	cent := map[string][2]float64{}
	hasCoord := map[string]bool{}
	for _, v := range raw {
		ids = append(ids, v.ID)
		if v.Lat != 0 || v.Lng != 0 {
			cent[v.ID] = [2]float64{v.Lat, v.Lng}
			hasCoord[v.ID] = true
		}
	}
	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			ai, bi := ids[i], ids[j]
			if !hasCoord[ai] || !hasCoord[bi] {
				continue
			}
			d := haversine(cent[ai][0], cent[ai][1], cent[bi][0], cent[bi][1])
			if d < 0.3 { // 300m
				ra := find(keyOf["k:"+venueDupKey(venueByID[ai].City, venueByID[ai].Name)])
				rb := find(keyOf["k:"+venueDupKey(venueByID[bi].City, venueByID[bi].Name)])
				if ra != rb {
					parent[ra] = rb
				}
			}
		}
	}
	// 重新聚合根组
	rootOf := map[string]int{}
	for _, v := range raw {
		root := find(keyOf["k:"+venueDupKey(v.City, v.Name)])
		rootOf[v.ID] = root
	}
	// 仅当组内多于 1 个场馆时才打标（单场馆不打标，避免噪声）
	groupSize := map[int]int{}
	for _, v := range raw {
		groupSize[rootOf[v.ID]]++
	}
	for i := range raw {
		if groupSize[rootOf[raw[i].ID]] > 1 {
			raw[i].DupGroup = rootOf[raw[i].ID]
		}
	}
	return raw, nil
}

// PreviewMergeVenues 报告把 sources 并入 target 会改写多少条记录、并入多少别名。
func (db *DB) PreviewMergeVenues(targetID string, sourceIDs []string) (*MergePreview, error) {
	if targetID == "" || len(sourceIDs) == 0 {
		return nil, fmt.Errorf("需要提供 target 与至少一个 source")
	}
	target, err := db.GetVenue(targetID)
	if err != nil {
		return nil, fmt.Errorf("target 场馆不存在: %w", err)
	}
	p := &MergePreview{Kind: "venue", Target: MergeSide{ID: target.ID, Name: target.Name, RecordCount: target.RecordCount}}
	rewritten := 0
	aliasSet := map[string]bool{}
	for _, a := range target.Aliases {
		aliasSet[strings.TrimSpace(a)] = true
	}
	for _, sid := range sourceIDs {
		if sid == targetID {
			continue
		}
		src, err := db.GetVenue(sid)
		if err != nil {
			return nil, fmt.Errorf("source 场馆不存在: %w", err)
		}
		p.Source = MergeSide{ID: src.ID, Name: src.Name, RecordCount: src.RecordCount}
		p.Sources = append(p.Sources, MergeSide{ID: src.ID, Name: src.Name, RecordCount: src.RecordCount})
		// 该 source 规范名对应的记录数
		c, err := db.countRecordsByVenueName(src.Name)
		if err != nil {
			return nil, err
		}
		rewritten += c
		for _, a := range src.Aliases {
			a = strings.TrimSpace(a)
			if a != "" && !aliasSet[a] {
				aliasSet[a] = true
				p.AliasesAdded = append(p.AliasesAdded, a)
			}
		}
	}
	// 把 source 自己的规范名也作为别名并入（供日后去重识别）
	tn := strings.TrimSpace(target.Name)
	for _, sid := range sourceIDs {
		if sid == targetID {
			continue
		}
		src, err := db.GetVenue(sid)
		if err != nil {
			continue
		}
		sn := strings.TrimSpace(src.Name)
		if sn != "" && sn != tn && !aliasSet[sn] {
			aliasSet[sn] = true
			p.AliasesAdded = append(p.AliasesAdded, sn)
		}
	}
	p.RecordsRepoint = rewritten
	return p, nil
}

// MergeVenues 把若干 source 场馆并入 target：改写其规范名对应的记录 address 为
// target 规范名，合并别名，删除 source 行。
func (db *DB) MergeVenues(targetID string, sourceIDs []string) (rewritten int64, err error) {
	target, err := db.GetVenue(targetID)
	if err != nil {
		return 0, fmt.Errorf("target 场馆不存在: %w", err)
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	aliasSet := map[string]bool{}
	for _, a := range target.Aliases {
		aliasSet[strings.TrimSpace(a)] = true
	}
	aliasSet[strings.TrimSpace(target.Name)] = true

	// 应用层精确改写：逐条匹配归一化地址
	for _, sid := range sourceIDs {
		if sid == targetID {
			continue
		}
		src, err := db.GetVenue(sid)
		if err != nil {
			return 0, fmt.Errorf("source 场馆不存在: %w", err)
		}
		norm := normVenueName(src.Name)
		rows, err := tx.Query("SELECT id, address FROM records WHERE deleted_at = 0")
		if err != nil {
			return 0, err
		}
		var toUpdate []string
		for rows.Next() {
			var rid, addr string
			if err := rows.Scan(&rid, &addr); err != nil {
				rows.Close()
				return 0, err
			}
			if normVenueName(addr) == norm {
				toUpdate = append(toUpdate, rid)
			}
		}
		rows.Close()
		for _, rid := range toUpdate {
			if _, err := tx.Exec("UPDATE records SET address = ? WHERE id = ?", target.Name, rid); err != nil {
				return 0, err
			}
			rewritten++
		}
		// 合并别名
		for _, a := range src.Aliases {
			a = strings.TrimSpace(a)
			if a != "" && !aliasSet[a] {
				aliasSet[a] = true
			}
		}
		aliasSet[strings.TrimSpace(src.Name)] = true
		// 删除 source 场馆
		if _, err := tx.Exec("DELETE FROM venues WHERE id = ?", sid); err != nil {
			return 0, err
		}
	}

	// 写回合并后的别名
	aliases := []string{}
	for a := range aliasSet {
		if a != "" && a != strings.TrimSpace(target.Name) {
			aliases = append(aliases, a)
		}
	}
	if _, err := tx.Exec("UPDATE venues SET aliases = ? WHERE id = ?", marshalJSON(aliases), targetID); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return rewritten, nil
}

// GetVenue 返回单个场馆（含实时记录数）。
func (db *DB) GetVenue(id string) (*Venue, error) {
	var v Venue
	var rawAliases string
	err := db.conn.QueryRow("SELECT id, name, city, lat, lng, aliases, sort_order FROM venues WHERE id = ?", id).
		Scan(&v.ID, &v.Name, &v.City, &v.Lat, &v.Lng, &rawAliases, &v.SortOrder)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("venue not found")
	}
	if err != nil {
		return nil, err
	}
	v.Aliases = unmarshalStrings(rawAliases)
	c, err := db.countRecordsByVenueName(v.Name)
	if err != nil {
		return nil, err
	}
	v.RecordCount = c
	return &v, nil
}

func parseCoord(s string) (float64, float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	lat, err1 := parseFloat(parts[0])
	lng, err2 := parseFloat(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return lat, lng, true
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLng := toRad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
