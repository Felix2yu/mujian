package models

import "encoding/json"

// Coordinate mirrors the export's nested `coordinate` object.
type Coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// Record is the canonical data model. Its JSON tags match recordlive_export/
// data.json exactly so records can be imported/exported with zero mapping.
// Arrays (artist_names, guest, play, tagIds) and the nested coordinate are
// stored as JSON text columns in the database.
type Record struct {
	ID                string      `json:"id"`
	Name              string      `json:"name"`
	Channel           string      `json:"channel"`
	City              string      `json:"city"`
	Address           string      `json:"address"`
	Coordinate        *Coordinate `json:"coordinate"`
	Cover             string      `json:"cover"`
	CoverFile         string      `json:"coverFile"`
	CoverThumb        string      `json:"coverThumb"`
	CustomCategoryID  string      `json:"customCategoryId"`
	CategoryName      string      `json:"categoryName"`   // 主剧种 = CategoryNames[0]，kept in sync for export compatibility
	CategoryNames     []string    `json:"categoryNames"` // 一场演出可涉及多个剧种
	ArtistNames       []string    `json:"artist_names"`
	Guest             []string    `json:"guest"`
	Play              []string    `json:"play"`
	DramaIDs          []string    `json:"drama_ids"`
	ZheziIDs          []string    `json:"zhezi_ids"`
	ArtistIDs         []string    `json:"artist_ids"`
	TagIDs            []string    `json:"tagIds"`
	Date              int64       `json:"date"` // unix seconds
	DateText          string      `json:"dateText"`
	Rating            int         `json:"rating"`
	Seat              string      `json:"seat"`
	Friends           string      `json:"friends"`
	Company           string      `json:"company"`
	Remark            string      `json:"remark"`
	ActiveStatus      int         `json:"active_status"`
	Price             float64     `json:"price"`
	PriceCurrency     string      `json:"price_currency"`
	PayPrice          float64     `json:"pay_price"`
	PayPriceCurrency  string      `json:"pay_price_currency"`
	OtherCost         float64     `json:"other_cost"`
	OtherCostCurrency string      `json:"other_cost_currency"`
}

// Category mirrors the export's top-level `categories` array.
type Category struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	ActiveIDs   []string `json:"activeIds"`
	RecordCount int      `json:"recordCount"`
	SortOrder   int      `json:"sortOrder"` // manual ordering, 0 = alphabetical
}

// Drama is a 剧目 (a play/drama), a first-class entity that owns an ordered
// list of 折子 (zhezi / sub-scenes).
type Drama struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	CategoryName  string   `json:"categoryName"`  // 主剧种 = CategoryNames[0]，kept in sync for compatibility
	CategoryNames []string `json:"categoryNames"` // 剧目可跨多个剧种, e.g. [昆剧 苏剧]
	Remark        string   `json:"remark"`
	SortOrder     int      `json:"sortOrder"` // manual ordering, 0 = alphabetical
	ZheziCount    int      `json:"zheziCount"`
	RecordCount   int      `json:"recordCount"` // performances referencing this drama
}

// Zhezi is a 折子 (a sub-scene) belonging to exactly one drama. Because
// different 剧种/剧团 name the same 折子 differently, each zhezi keeps an
// ordered list of allowed aliases. SortOrder controls manual ordering within
// the parent drama.
type Zhezi struct {
	ID        string   `json:"id"`
	DramaID   string   `json:"dramaId"`
	Name      string   `json:"name"`
	Aliases   []string `json:"aliases"`
	SortOrder int      `json:"sortOrder"`
	Remark    string   `json:"remark"`
}

// DramaDetail is Drama plus its ordered zhezis and the performances that
// reference the drama.
type DramaDetail struct {
	Drama
	Zhezis  []Zhezi  `json:"zhezis"`
	Records []Record `json:"records"`
}

// DramaTree is a lightweight drama + its zhezis, used by pickers that need the
// full drama/zhezi structure in one request without performance records.
type DramaTree struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	CategoryName  string   `json:"categoryName"`
	CategoryNames []string `json:"categoryNames"`
	Zhezis        []Zhezi  `json:"zhezis"`
}

// Artist is a 演员 (performer), a first-class entity mirroring dramas. It owns
// a cover, bio and aliases so it can power a dedicated actor home page. The
// reverse relationship (which performances feature this actor) is resolved via
// the record_artists relation table.
type Artist struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	Remark      string   `json:"remark"`
	Cover       string   `json:"cover"`
	CoverFile   string   `json:"coverFile"`
	CoverThumb  string   `json:"coverThumb"`
	Bio         string   `json:"bio"`
	SortOrder   int      `json:"sortOrder"`   // manual ordering, 0 = alphabetical
	RecordCount int      `json:"recordCount"` // performances referencing this artist
}

// ArtistDetail is Artist plus the performances that feature the actor.
type ArtistDetail struct {
	Artist
	Records []Record `json:"records"`
}

// ArtistTree is a lightweight artist used by pickers (name + id only).
type ArtistTree struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Meta mirrors the export's `meta` object (song / tags / webdav_config).
// Each value is kept as raw JSON so unknown/empty shapes are preserved
// faithfully.
type Meta struct {
	Song         json.RawMessage `json:"song"`
	Tags         json.RawMessage `json:"tags"`
	WebdavConfig json.RawMessage `json:"webdav_config"`
}

// ExportData is the exact top-level shape of recordlive_export/data.json.
// This is both the import format and the export/backup format, guaranteeing
// the database fields stay in lock-step with the source file.
type ExportData struct {
	Source       string     `json:"source"`
	ExportedAt   string     `json:"exportedAt"`
	RecordCount  int        `json:"recordCount"`
	CoverMissing int        `json:"coverMissing"`
	CoverDir     string     `json:"coverDir"`
	CoverNote    string     `json:"coverNote"`
	Meta         Meta       `json:"meta"`
	Records      []Record   `json:"records"`
	Categories   []Category `json:"categories"`
}

// RecordRequest is the editable payload accepted by create/update endpoints.
// It intentionally uses the same field names as Record.
type RecordRequest struct {
	Name              string      `json:"name"`
	Channel           string      `json:"channel"`
	City              string      `json:"city"`
	Address           string      `json:"address"`
	Coordinate        *Coordinate `json:"coordinate"`
	Cover             string      `json:"cover"`
	CoverFile         string      `json:"coverFile"`
	CoverThumb        string      `json:"coverThumb"`
	CustomCategoryID  string      `json:"customCategoryId"`
	CategoryName      string      `json:"categoryName"`
	CategoryNames     []string    `json:"categoryNames"` // 多剧种；优先于 CategoryName
	ArtistIDs         []string    `json:"artist_ids"`
	ArtistNames       []string    `json:"artist_names"`
	Guest             []string    `json:"guest"`
	Play              []string    `json:"play"`
	DramaIDs          []string    `json:"drama_ids"`
	ZheziIDs          []string    `json:"zhezi_ids"`
	TagIDs            []string    `json:"tagIds"`
	Date              int64       `json:"date"`
	DateText          string      `json:"dateText"`
	Rating            int         `json:"rating"`
	Seat              string      `json:"seat"`
	Friends           string      `json:"friends"`
	Company           string      `json:"company"`
	Remark            string      `json:"remark"`
	ActiveStatus      int         `json:"active_status"`
	Price             float64     `json:"price"`
	PriceCurrency     string      `json:"price_currency"`
	PayPrice          float64     `json:"pay_price"`
	PayPriceCurrency  string      `json:"pay_price_currency"`
	OtherCost         float64     `json:"other_cost"`
	OtherCostCurrency string      `json:"other_cost_currency"`
}

type CalendarEvent struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Date           int64    `json:"date"`
	City           string   `json:"city"`
	Address        string   `json:"address"`
	CoverFile      string   `json:"coverFile"`
	CoverThumb     string   `json:"coverThumb"`
	Rating         int      `json:"rating"`
	ActiveStatus   int      `json:"active_status"`
	CategoryName   string   `json:"categoryName"`
	CategoryNames  []string `json:"categoryNames"`
}

type Stats struct {
	TotalRecords int     `json:"total_records"`
	TotalCost    float64 `json:"total_cost"`
	AvgRating    float64 `json:"avg_rating"`
	TotalCities  int     `json:"total_cities"`
}

type MonthStat struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

type CategoryStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type CityStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type CostStat struct {
	Month string  `json:"month"`
	Cost  float64 `json:"cost"`
}

type DashboardStats struct {
	TotalRecords  int            `json:"total_records"`
	TotalCost     float64        `json:"total_cost"`
	AvgRating     float64        `json:"avg_rating"`
	TotalCities   int            `json:"total_cities"`
	ByMonth       []MonthStat    `json:"by_month"`
	ByCategory    []CategoryStat `json:"by_category"`
	ByCity        []CityStat     `json:"by_city"`
	CostByMonth   []CostStat     `json:"cost_by_month"`
	TopRated      []Record       `json:"top_rated"`
	RecentRecords []Record       `json:"recent_records"`
}

// ---------- Analytics ----------

// AnalyticsOverview holds headline KPIs plus period-over-period deltas
// (last 365 days vs the preceding 365 days) to surface comparison differences.
type AnalyticsOverview struct {
	TotalRecords    int     `json:"total_records"`
	TotalCost       float64 `json:"total_cost"`
	AvgRating       float64 `json:"avg_rating"`
	TotalCities     int     `json:"total_cities"`
	TotalArtists    int     `json:"total_artists"`
	TotalDramas     int     `json:"total_dramas"`
	RecordsDeltaPct float64 `json:"records_delta_pct"` // % change vs previous 365d
	CostDeltaPct    float64 `json:"cost_delta_pct"`
	RatingDelta     float64 `json:"rating_delta"` // absolute avg rating change
}

// TrendPoint is one month in the trend series.
type TrendPoint struct {
	Period    string  `json:"period"` // YYYY-MM
	Count     int     `json:"count"`
	Cost      float64 `json:"cost"`
	AvgRating float64 `json:"avg_rating"`
}

// DistItem is a single slice of a proportion/ distribution.
type DistItem struct {
	Name  string  `json:"name"`
	Count int     `json:"count"`
	Pct   float64 `json:"pct"` // share within its own distribution (0-100)
}

// ComparePoint contrasts the same calendar month across two years.
type ComparePoint struct {
	Period   string  `json:"period"` // YYYY-MM
	Current  float64 `json:"current"`
	Previous float64 `json:"previous"`
	DeltaPct float64 `json:"delta_pct"`
}

// Anomaly flags a month whose count deviates markedly from the series mean.
type Anomaly struct {
	Period   string  `json:"period"`
	Count    int     `json:"count"`
	Expected float64 `json:"expected"` // series mean
	ZScore   float64 `json:"zscore"`
	Type     string  `json:"type"` // "spike" | "drop"
}

// CorrPair is a Pearson correlation between two numeric fields.
type CorrPair struct {
	X string  `json:"x"` // human label
	Y string  `json:"y"` // human label
	R float64 `json:"r"` // -1..1
	N int     `json:"n"` // sample size
}

// ScatterPoint is one (x, y) pair for the correlation scatter plot.
type ScatterPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// RankItem is one entry in a top-N ranking.
type RankItem struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// WeekdayItem is the show count for a given weekday.
type WeekdayItem struct {
	Weekday int    `json:"weekday"` // 0=周日 .. 6=周六
	Name    string `json:"name"`
	Count   int    `json:"count"`
}

// RewatchStats measures how often dramas / artists are seen more than once.
type RewatchStats struct {
	TotalDramas      int     `json:"total_dramas"`
	RewatchedDramas  int     `json:"rewatched_dramas"`
	DramaRate        float64 `json:"drama_rate"` // % of dramas seen >=2 times
	TotalArtists     int     `json:"total_artists"`
	RewatchedArtists int     `json:"rewatched_artists"`
	ArtistRate       float64 `json:"artist_rate"` // % of artists seen >=2 times
}

// DiscoverPoint is one month's count of first-time-seen artists / dramas.
type DiscoverPoint struct {
	Period     string `json:"period"` // YYYY-MM
	NewArtists int    `json:"new_artists"`
	NewDramas  int    `json:"new_dramas"`
}

// DiversityIndex captures Shannon entropy / evenness of category, artist and
// drama distributions — a high evenness means viewing is well balanced rather
// than concentrated on a few favourites.
type DiversityIndex struct {
	CategoryEntropy  float64 `json:"category_entropy"`
	CategoryEvenness float64 `json:"category_evenness"` // 0..1
	ArtistEntropy    float64 `json:"artist_entropy"`
	ArtistEvenness   float64 `json:"artist_evenness"`
	DramaEntropy     float64 `json:"drama_entropy"`
	DramaEvenness    float64 `json:"drama_evenness"`
}

// IntervalStats summarises gaps (in days) between consecutive performances.
type IntervalStats struct {
	Avg     float64    `json:"avg"`     // 平均间隔(天)
	Median  float64    `json:"median"`  // 中位间隔(天)
	Max     float64    `json:"max"`     // 最长间隔(天)
	Buckets []DistItem `json:"buckets"` // 间隔分桶
}

// AnalyticsData is the full payload returned by /api/analytics, covering
// trend, distribution, comparison, anomaly, correlation and ranking views.
type AnalyticsData struct {
	GeneratedAt    int64             `json:"generated_at"`
	Overview       AnalyticsOverview `json:"overview"`
	Trends         []TrendPoint      `json:"trends"`          // last 24 months
	CategoryDist   []DistItem        `json:"category_dist"`   // multi-category expansion
	ChannelDist    []DistItem        `json:"channel_dist"`    // 渠道占比
	CompanyDist    []DistItem        `json:"company_dist"`    // 剧团占比
	CityDist       []DistItem        `json:"city_dist"`       // 城市占比
	RatingDist     []DistItem        `json:"rating_dist"`     // 评分分布 1..5
	YearDist       []DistItem        `json:"year_dist"`       // 按年分布
	CompareMonthly []ComparePoint    `json:"compare_monthly"` // 近12月同比
	Anomalies      []Anomaly         `json:"anomalies"`       // 异常波动
	CorrPairs      []CorrPair        `json:"corr_pairs"`      // 相关性
	Scatter        []ScatterPoint    `json:"scatter"`         // 票价 vs 评分 散点
	TopArtists     []RankItem        `json:"top_artists"`
	TopDramas      []RankItem        `json:"top_dramas"`
	TopVenues      []RankItem        `json:"top_venues"`

	// 行为与经济的扩展维度
	PriceBuckets      []DistItem        `json:"price_buckets"`       // 实付票价分桶
	OtherCostBuckets  []DistItem        `json:"other_cost_buckets"`  // 其他花费分桶
	TopZhezis     []RankItem        `json:"top_zhezis"`     // 常看折子
	Rewatch       *RewatchStats     `json:"rewatch"`        // 复看率
	Discovery     []DiscoverPoint   `json:"discovery"`      // 每月新发现
	Diversity     *DiversityIndex   `json:"diversity"`      // 多样性指数
	Intervals     *IntervalStats    `json:"intervals"`      // 观演间隔
	WeekdayDist   []WeekdayItem     `json:"weekday_dist"`   // 周几分布
}

// Settings / request structures are reused from config.
type Settings struct {
	Theme       string `json:"theme"`
	StorageType string `json:"storage_type"`
	S3Endpoint  string `json:"s3_endpoint"`
	S3Bucket    string `json:"s3_bucket"`
	S3Region    string `json:"s3_region"`
	S3AccessKey string `json:"s3_access_key"`
	S3SecretKey string `json:"s3_secret_key"`
	S3PublicURL string `json:"s3_public_url"`
}

type SettingsRequest struct {
	Theme       *string `json:"theme"`
	StorageType *string `json:"storage_type"`
	S3Endpoint  *string `json:"s3_endpoint"`
	S3Bucket    *string `json:"s3_bucket"`
	S3Region    *string `json:"s3_region"`
	S3AccessKey *string `json:"s3_access_key"`
	S3SecretKey *string `json:"s3_secret_key"`
	S3PublicURL *string `json:"s3_public_url"`
}

// ---------- Cover management ----------

// Cover is a row in the covers metadata table (content hash -> file).
type Cover struct {
	Hash      string `json:"hash"`
	FileName  string `json:"file_name"`
	Ext       string `json:"ext"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

// CoverRef is one distinct cover for the reuse picker.
type CoverRef struct {
	FileName   string `json:"file_name"`
	Ext        string `json:"ext"`
	Size       int64  `json:"size"`
	RefCount   int    `json:"ref_count"`
	SampleName string `json:"sample_name"`
	Category   string `json:"category"`
}

// DupRecord is one record inside a duplicate group.
type DupRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CoverFile string `json:"cover_file"`
}

// DupGroup is a set of records whose covers share the same content hash.
type DupGroup struct {
	Hash      string      `json:"hash"`
	Ext       string      `json:"ext"`
	Size      int64       `json:"size"`
	Count     int         `json:"count"`
	Canonical string      `json:"canonical"`
	Records   []DupRecord `json:"records"`
}

// OrphanItem is an unreferenced cover file.
type OrphanItem struct {
	FileName string `json:"file_name"`
	Size     int64  `json:"size"`
}

// ---------- Batch update ----------

// BatchArrayOp specifies how to modify a JSON-array column.
// Op: "set" (replace), "append" (union), "remove" (difference).
type BatchArrayOp struct {
	Op    string   `json:"op"`    // set | append | remove
	Value []string `json:"value"` // ids or names
}

// BatchUpdateParams holds all possible fields for batch update.
type BatchUpdateParams struct {
	IDs               []string
	Name              *string
	CategoryName      *string
	CategoryNames     *BatchArrayOp // 多剧种 set/append/remove（覆盖 CategoryName）
	Rating            *int
	ActiveStatus      *int
	City              *string
	Address           *string
	Channel           *string
	Company           *string
	Friends           *string
	Remark            *string
	Seat              *string
	DateText          *string             // 演出时间文本，解析后联动 date 列；空串清空
	Coordinate        *Coordinate         // 直接覆写坐标 JSON
	Price             *float64
	PriceCurrency     *string
	PayPrice          *float64
	PayPriceCurrency  *string
	OtherCost         *float64
	OtherCostCurrency *string
	DramaIDs          *BatchArrayOp
	ZheziIDs          *BatchArrayOp
	Play              *BatchArrayOp
	Guest             *BatchArrayOp
	ArtistNames       *BatchArrayOp
	TagIDs            *BatchArrayOp
}
