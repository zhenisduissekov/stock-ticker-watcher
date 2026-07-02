package store

import "context"

// Store defines the interface for database operations
type Store interface {
	GetWatchlist(ctx context.Context, userID int) ([]string, error)
	AddTicker(ctx context.Context, userID int, ticker string) error
	RemoveTicker(ctx context.Context, userID int, ticker string) error
	Close() error
}
