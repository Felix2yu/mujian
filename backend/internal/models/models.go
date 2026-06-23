package models

import "time"

type ShowStatus string

const (
	StatusPlanned  ShowStatus = "planned"
	StatusWatched  ShowStatus = "watched"
	StatusCancelled ShowStatus = "cancelled"
)

type Show struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Venue       string     `json:"venue"`
	Date        time.Time  `json:"date"`
	Duration    int        `json:"duration"` // minutes
	Status      ShowStatus `json:"status"`
	CategoryID  *int64     `json:"category_id"`
	PosterURL   string     `json:"poster_url"`
	Setlist     string     `json:"setlist"`     // newline separated
	Cast        string     `json:"cast"`        // newline separated
	Company     string     `json:"company"`
	Friends     string     `json:"friends"`     // comma separated
	Rating      *int       `json:"rating"`      // 1-5
	Seat        string     `json:"seat"`
	Notes       string     `json:"notes"`
	Review      string     `json:"review"`
	TicketCost  *float64   `json:"ticket_cost"`
	OtherCost   *float64   `json:"other_cost"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CategoryName string    `json:"category_name,omitempty"`
}

type Category struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type ShowRequest struct {
	Name       string  `json:"name"`
	Venue      string  `json:"venue"`
	Date       string  `json:"date"`
	Duration   int     `json:"duration"`
	Status     string  `json:"status"`
	CategoryID *int64  `json:"category_id"`
	PosterURL  string  `json:"poster_url"`
	Setlist    string  `json:"setlist"`
	Cast       string  `json:"cast"`
	Company    string  `json:"company"`
	Friends    string  `json:"friends"`
	Rating     *int    `json:"rating"`
	Seat       string  `json:"seat"`
	Notes      string  `json:"notes"`
	Review     string  `json:"review"`
	TicketCost *float64 `json:"ticket_cost"`
	OtherCost  *float64 `json:"other_cost"`
}

type CalendarEvent struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Venue    string `json:"venue"`
	Date     string `json:"date"`
	Duration int    `json:"duration"`
	Status   string `json:"status"`
	Color    string `json:"color"`
}

type Stats struct {
	TotalShows   int     `json:"total_shows"`
	TotalCost    float64 `json:"total_cost"`
	AvgRating    float64 `json:"avg_rating"`
	TotalVenues  int     `json:"total_venues"`
	TotalHours   float64 `json:"total_hours"`
}
