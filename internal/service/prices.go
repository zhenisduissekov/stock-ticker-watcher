package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
)

// ErrPriceInvalid is returned when a price update carries a non-positive price.
var ErrPriceInvalid = errors.New("price must be positive")

// PriceService handles price updates and caching
type PriceService struct {
	priceCache       map[string]float64
	cacheMutex       sync.RWMutex
	hub              Hub
	logger           *slog.Logger
	updatesProcessed atomic.Int64
}

// Hub interface for WebSocket broadcasting
type Hub interface {
	Broadcast(ticker string, price float64)
}

// NewPriceService creates a new price service
func NewPriceService(hub Hub, logger *slog.Logger) *PriceService {
	return &PriceService{
		priceCache: make(map[string]float64),
		hub:        hub,
		logger:     logger,
	}
}

// UpdatePrice updates the price for a ticker and broadcasts to subscribers
func (s *PriceService) UpdatePrice(ctx context.Context, ticker string, price float64) error {
	// Validate
	if ticker == "" {
		return ErrTickerEmpty
	}
	if price <= 0 {
		return ErrPriceInvalid
	}

	// Update cache
	s.cacheMutex.Lock()
	s.priceCache[ticker] = price
	s.cacheMutex.Unlock()

	// Broadcast to WebSocket subscribers
	s.hub.Broadcast(ticker, price)

	s.updatesProcessed.Add(1)
	s.logger.Info("Price updated", "ticker", ticker, "price", price)
	return nil
}

// UpdatesProcessed returns the total number of price updates processed since
// startup (webhook + simulator).
func (s *PriceService) UpdatesProcessed() int64 {
	return s.updatesProcessed.Load()
}

// InitPrice sets a seed price for a ticker only if one is not already cached.
// Returns the effective price (existing or newly seeded). Unlike a plain map
// write, this mutates the real internal cache under the lock.
func (s *PriceService) InitPrice(ticker string, price float64) float64 {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	if existing, ok := s.priceCache[ticker]; ok {
		return existing
	}
	s.priceCache[ticker] = price
	return price
}

// GetPrice retrieves the current price for a ticker
func (s *PriceService) GetPrice(ticker string) (float64, bool) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	price, exists := s.priceCache[ticker]
	return price, exists
}

// GetAllPrices returns a snapshot copy of all cached prices, safe for the
// caller to read without holding the lock.
func (s *PriceService) GetAllPrices() map[string]float64 {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	prices := make(map[string]float64, len(s.priceCache))
	for k, v := range s.priceCache {
		prices[k] = v
	}
	return prices
}
