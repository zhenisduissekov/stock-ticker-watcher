package models

import "time"

// WatchlistItem represents a stock in a user's watchlist
type WatchlistItem struct {
	Ticker string  `json:"ticker"`
	Price  float64 `json:"price"`
}

// AddTickerRequest represents a request to add a ticker to the watchlist
type AddTickerRequest struct {
	Ticker string `json:"ticker"`
}

// PriceUpdate represents a price update from a third-party provider
type PriceUpdate struct {
	Ticker string  `json:"ticker"`
	Price  float64 `json:"price"`
}

// User represents a user in the system
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// UserStock represents a user's stock in the database
type UserStock struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Ticker    string    `json:"ticker"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}
