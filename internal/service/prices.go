package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// PriceService handles price updates and caching
type PriceService struct {
	priceCache map[string]float64
	cacheMutex sync.RWMutex
	hub        Hub
	logger     *slog.Logger
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
		return fmt.Errorf("ticker cannot be empty")
	}
	if price <= 0 {
		return fmt.Errorf("price must be positive")
	}

	// Update cache
	s.cacheMutex.Lock()
	s.priceCache[ticker] = price
	s.cacheMutex.Unlock()

	// Broadcast to WebSocket subscribers
	s.hub.Broadcast(ticker, price)

	s.logger.Info("Price updated", "ticker", ticker, "price", price)
	return nil
}

// GetPrice retrieves the current price for a ticker
func (s *PriceService) GetPrice(ticker string) (float64, bool) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	price, exists := s.priceCache[ticker]
	return price, exists
}

// GetAllPrices returns all cached prices
func (s *PriceService) GetAllPrices() map[string]float64 {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	// Return a copy to avoid race conditions
	prices := make(map[string]float64, len(s.priceCache))
	for k, v := range s.priceCache {
		prices[k] = v
	}
	return prices
}

// GetPriceCache returns the price cache for watchlist service
func (s *PriceService) GetPriceCache() map[string]float64 {
	return s.GetAllPrices()
}
