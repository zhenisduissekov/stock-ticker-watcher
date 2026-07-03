package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"stock-ticker-watcher/internal/models"
	"stock-ticker-watcher/internal/store"
)

// Validation errors returned by the service layer. Handlers classify these
// with errors.Is; their messages are surfaced verbatim to API clients.
var (
	ErrTickerEmpty   = errors.New("ticker cannot be empty")
	ErrTickerTooLong = errors.New("ticker too long (max 10 characters)")
	ErrTickerInvalid = errors.New("ticker contains invalid characters")
)

// PriceStore provides read/seed access to the live price cache without exposing
// the underlying map (which must never be mutated via a copy).
type PriceStore interface {
	GetPrice(ticker string) (float64, bool)
	InitPrice(ticker string, price float64) float64
}

// SeedPrice returns a deterministic starting price for a ticker that has no
// live price yet, used until the provider/simulator sends a real update.
// Exported so the simulator can seed the same value the watchlist shows.
func SeedPrice(ticker string) float64 {
	return 100 + (float64(len(ticker)) * 10)
}

// WatchlistService handles watchlist business logic
type WatchlistService struct {
	store  store.Store
	logger *slog.Logger
}

// NewWatchlistService creates a new watchlist service
func NewWatchlistService(store store.Store, logger *slog.Logger) *WatchlistService {
	return &WatchlistService{
		store:  store,
		logger: logger,
	}
}

// GetWatchlist retrieves the user's watchlist with current prices
func (s *WatchlistService) GetWatchlist(ctx context.Context, userID int, prices PriceStore) ([]models.WatchlistItem, error) {
	tickers, err := s.store.GetWatchlist(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get watchlist: %w", err)
	}

	watchlist := make([]models.WatchlistItem, 0, len(tickers))
	for _, ticker := range tickers {
		price, exists := prices.GetPrice(ticker)
		if !exists {
			// Seed with a deterministic starting price until a real update arrives
			price = SeedPrice(ticker)
		}
		watchlist = append(watchlist, models.WatchlistItem{
			Ticker: ticker,
			Price:  price,
		})
	}

	return watchlist, nil
}

// AddTicker adds a ticker to the user's watchlist
func (s *WatchlistService) AddTicker(ctx context.Context, userID int, req models.AddTickerRequest, prices PriceStore) (*models.WatchlistItem, error) {
	// Validate request
	ticker := strings.TrimSpace(strings.ToUpper(req.Ticker))
	if ticker == "" {
		return nil, ErrTickerEmpty
	}

	if len(ticker) > 10 {
		return nil, ErrTickerTooLong
	}

	// Check for invalid characters
	for _, r := range ticker {
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return nil, ErrTickerInvalid
		}
	}

	// Add to database. The store returns store.ErrTickerExists on a duplicate,
	// which we propagate (wrapped) so the handler can classify it via errors.Is.
	if err := s.store.AddTicker(ctx, userID, ticker); err != nil {
		return nil, fmt.Errorf("failed to add ticker: %w", err)
	}

	// Seed a starting price in the live cache if one isn't already present.
	// InitPrice mutates the real cache under lock and returns the effective price.
	price := prices.InitPrice(ticker, SeedPrice(ticker))

	s.logger.Info("Ticker added to watchlist", "user_id", userID, "ticker", ticker)

	return &models.WatchlistItem{
		Ticker: ticker,
		Price:  price,
	}, nil
}

// RemoveTicker removes a ticker from the user's watchlist
func (s *WatchlistService) RemoveTicker(ctx context.Context, userID int, ticker string) error {
	ticker = strings.TrimSpace(strings.ToUpper(ticker))
	if ticker == "" {
		return ErrTickerEmpty
	}

	// The store returns store.ErrTickerNotFound when the ticker isn't present;
	// propagate it (wrapped) for the handler to classify via errors.Is.
	if err := s.store.RemoveTicker(ctx, userID, ticker); err != nil {
		return fmt.Errorf("failed to remove ticker: %w", err)
	}

	s.logger.Info("Ticker removed from watchlist", "user_id", userID, "ticker", ticker)
	return nil
}
