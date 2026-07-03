package store

import (
	"context"
	"errors"
)

// Sentinel errors returned by store implementations so callers can classify
// outcomes with errors.Is rather than matching on error strings.
var (
	// ErrTickerExists is returned when adding a ticker that is already in the
	// user's watchlist.
	ErrTickerExists = errors.New("ticker already in watchlist")
	// ErrTickerNotFound is returned when removing a ticker that is not in the
	// user's watchlist.
	ErrTickerNotFound = errors.New("ticker not found in watchlist")
)

// Store defines the interface for database operations
type Store interface {
	GetWatchlist(ctx context.Context, userID int) ([]string, error)
	GetAllTickers(ctx context.Context) ([]string, error)
	AddTicker(ctx context.Context, userID int, ticker string) error
	RemoveTicker(ctx context.Context, userID int, ticker string) error
	Ping(ctx context.Context) error
	Close() error
}
