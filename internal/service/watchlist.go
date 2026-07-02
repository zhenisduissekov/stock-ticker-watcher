package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"stock-ticker-watcher/internal/models"
	"stock-ticker-watcher/internal/store"
)

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
func (s *WatchlistService) GetWatchlist(ctx context.Context, userID int, priceCache map[string]float64) ([]models.WatchlistItem, error) {
	tickers, err := s.store.GetWatchlist(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get watchlist: %w", err)
	}

	watchlist := make([]models.WatchlistItem, 0, len(tickers))
	for _, ticker := range tickers {
		price, exists := priceCache[ticker]
		if !exists {
			// Initialize with random price if not in cache
			price = 100 + (float64(len(ticker)) * 10)
		}
		watchlist = append(watchlist, models.WatchlistItem{
			Ticker: ticker,
			Price:  price,
		})
	}

	return watchlist, nil
}

// AddTicker adds a ticker to the user's watchlist
func (s *WatchlistService) AddTicker(ctx context.Context, userID int, req models.AddTickerRequest, priceCache map[string]float64) (*models.WatchlistItem, error) {
	// Validate request
	ticker := strings.TrimSpace(strings.ToUpper(req.Ticker))
	if ticker == "" {
		return nil, fmt.Errorf("ticker cannot be empty")
	}

	if len(ticker) > 10 {
		return nil, fmt.Errorf("ticker too long (max 10 characters)")
	}

	// Check for invalid characters
	for _, r := range ticker {
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return nil, fmt.Errorf("ticker contains invalid characters")
		}
	}

	// Add to database
	if err := s.store.AddTicker(ctx, userID, ticker); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") || strings.Contains(err.Error(), "already exists") {
			return nil, fmt.Errorf("ticker already in watchlist")
		}
		return nil, fmt.Errorf("failed to add ticker: %w", err)
	}

	// Initialize price in cache if not exists
	if _, exists := priceCache[ticker]; !exists {
		priceCache[ticker] = 100 + (float64(len(ticker)) * 10)
	}

	s.logger.Info("Ticker added to watchlist", "user_id", userID, "ticker", ticker)

	return &models.WatchlistItem{
		Ticker: ticker,
		Price:  priceCache[ticker],
	}, nil
}

// RemoveTicker removes a ticker from the user's watchlist
func (s *WatchlistService) RemoveTicker(ctx context.Context, userID int, ticker string) error {
	ticker = strings.TrimSpace(strings.ToUpper(ticker))
	if ticker == "" {
		return fmt.Errorf("ticker cannot be empty")
	}

	if err := s.store.RemoveTicker(ctx, userID, ticker); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("ticker not found in watchlist")
		}
		return fmt.Errorf("failed to remove ticker: %w", err)
	}

	s.logger.Info("Ticker removed from watchlist", "user_id", userID, "ticker", ticker)
	return nil
}
