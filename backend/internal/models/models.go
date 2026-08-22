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
	CategoryName      string      `json:"categoryName"`
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
	ID           string `json:"id"`
	Name         string `json:"name"`
	CategoryName string `json:"categoryName"` // 剧种, e.g. 昆曲 / 越剧
	Remark       string `json:"remark"`
	SortOrder    int    `json:"sortOrder"` // manual ordering, 0 = alphabetical
	ZheziCount   int    `json:"zheziCount"`
	RecordCount  int    `json:"recordCount"` // performances referencing this drama
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
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	CategoryName string  `json:"categoryName"`
	Zhezis       []Zhezi `json:"zhezis"`
}

// Artist is a 演员 (performer), a first-class entity mirroring dramas. It owns
// a cover, bio and aliases so it can power a dedicated actor home page. The
// reverse relationship (which performances feature this actor) is resolved via
// the record_artists relation table.
type Artist struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Aliases     []string `json:"aliases"`
	Remark      string `json:"remark"`
	Cover       string `json:"cover"`
	CoverFile   string `json:"coverFile"`
	CoverThumb  string `json:"coverThumb"`
	Bio         string `json:"bio"`
	SortOrder   int    `json:"sortOrder"` // manual ordering, 0 = alphabetical
	RecordCount int    `json:"recordCount"` // performances referencing this artist
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
	RecordCount   int        `json:"recordCount"`
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
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Date         int64   `json:"date"`
	City         string  `json:"city"`
	Address      string  `json:"address"`
	CoverFile    string  `json:"coverFile"`
	Rating       int     `json:"rating"`
	CategoryName string  `json:"categoryName"`
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

// Settings / request structures are reused from config.
type Settings struct {
	Theme              string `json:"theme"`
	StorageType        string `json:"storage_type"`
	S3Endpoint         string `json:"s3_endpoint"`
	S3Bucket           string `json:"s3_bucket"`
	S3Region           string `json:"s3_region"`
	S3AccessKey        string `json:"s3_access_key"`
	S3SecretKey        string `json:"s3_secret_key"`
	S3PublicURL        string `json:"s3_public_url"`
}

type SettingsRequest struct {
	Theme          *string `json:"theme"`
	StorageType    *string `json:"storage_type"`
	S3Endpoint     *string `json:"s3_endpoint"`
	S3Bucket       *string `json:"s3_bucket"`
	S3Region       *string `json:"s3_region"`
	S3AccessKey    *string `json:"s3_access_key"`
	S3SecretKey    *string `json:"s3_secret_key"`
	S3PublicURL    *string `json:"s3_public_url"`
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
	CategoryName      *string
	Rating            *int
	ActiveStatus      *int
	City              *string
	Address           *string
	Channel           *string
	Company           *string
	Friends           *string
	Remark            *string
	Seat              *string
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
