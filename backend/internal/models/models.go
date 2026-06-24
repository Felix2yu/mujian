package models

import "time"

type ShowStatus string

const (
	StatusNormal         ShowStatus = "normal"
	StatusCancelled      ShowStatus = "cancelled"
	StatusPendingTickets ShowStatus = "pending_tickets"
	StatusNoShow         ShowStatus = "no_show"
)

type Show struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Venue        string     `json:"venue"`
	Date         time.Time  `json:"date"`
	Duration     int        `json:"duration"`
	Status       ShowStatus `json:"status"`
	CategoryID   *int64     `json:"category_id"`
	PosterURL    string     `json:"poster_url"`
	Setlist      string     `json:"setlist"`
	Cast         string     `json:"cast"`
	Company      string     `json:"company"`
	Friends      string     `json:"friends"`
	Rating       *int       `json:"rating"`
	Seat         string     `json:"seat"`
	Notes        string     `json:"notes"`
	Review       string     `json:"review"`
	TicketCost   *float64   `json:"ticket_cost"`
	OtherCost    *float64   `json:"other_cost"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CategoryName string     `json:"category_name,omitempty"`
}

type Category struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
	ShowCount int    `json:"show_count"`
}

type ShowRequest struct {
	Name       string   `json:"name"`
	Venue      string   `json:"venue"`
	Date       string   `json:"date"`
	Duration   int      `json:"duration"`
	Status     string   `json:"status"`
	CategoryID *int64   `json:"category_id"`
	PosterURL  string   `json:"poster_url"`
	Setlist    string   `json:"setlist"`
	Cast       string   `json:"cast"`
	Company    string   `json:"company"`
	Friends    string   `json:"friends"`
	Rating     *int     `json:"rating"`
	Seat       string   `json:"seat"`
	Notes      string   `json:"notes"`
	Review     string   `json:"review"`
	TicketCost *float64 `json:"ticket_cost"`
	OtherCost  *float64 `json:"other_cost"`
}

type CalendarEvent struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Venue     string `json:"venue"`
	Date      string `json:"date"`
	Duration  int    `json:"duration"`
	Status    string `json:"status"`
	Color     string `json:"color"`
	PosterURL string `json:"poster_url"`
}

type Stats struct {
	TotalShows  int     `json:"total_shows"`
	TotalCost   float64 `json:"total_cost"`
	AvgRating   float64 `json:"avg_rating"`
	TotalVenues int     `json:"total_venues"`
	TotalHours  float64 `json:"total_hours"`
}

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

type SceneSort struct {
	ID        int64  `json:"id"`
	Play      string `json:"play"`
	Scenes    string `json:"scenes"`
	UpdatedAt string `json:"updated_at"`
}
