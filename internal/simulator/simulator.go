package simulator

import (
	"context"
	"log/slog"
	"math/rand"
	"time"
)

// Simulator generates random price updates to simulate third-party provider
type Simulator struct {
	priceService PriceService
	tickers      []string
	basePrices   map[string]float64
	interval     time.Duration
	logger       *slog.Logger
}

// PriceService interface for updating prices
type PriceService interface {
	UpdatePrice(ctx context.Context, ticker string, price float64) error
}

// New creates a new simulator
func New(priceService PriceService, logger *slog.Logger, interval int) *Simulator {
	tickers := []string{"AAPL", "NVDA", "IBM", "GOOGL", "MSFT", "TSLA", "AMZN", "META"}
	basePrices := map[string]float64{
		"AAPL": 175,
		"NVDA": 450,
		"IBM":  160,
		"GOOGL": 140,
		"MSFT": 380,
		"TSLA": 180,
		"AMZN": 145,
		"META": 300,
	}

	return &Simulator{
		priceService: priceService,
		tickers:      tickers,
		basePrices:   basePrices,
		interval:     time.Duration(interval) * time.Second,
		logger:       logger,
	}
}

// Start begins the simulation in a goroutine
func (s *Simulator) Start(ctx context.Context) {
	s.logger.Info("Starting price simulator", "interval", s.interval.Seconds(), "tickers", len(s.tickers))

	// Send initial prices
	s.sendInitialPrices(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Simulator stopped")
			return
		case <-ticker.C:
			s.sendRandomUpdate(ctx)
		}
	}
}

// sendInitialPrices sends initial prices for all tickers
func (s *Simulator) sendInitialPrices(ctx context.Context) {
	for _, ticker := range s.tickers {
		base := s.basePrices[ticker]
		price := base + (rand.Float64()-0.5)*10
		if err := s.priceService.UpdatePrice(ctx, ticker, price); err != nil {
			s.logger.Error("Failed to send initial price", "ticker", ticker, "error", err)
		}
	}
}

// sendRandomUpdate sends a random price update for a random ticker
func (s *Simulator) sendRandomUpdate(ctx context.Context) {
	randomTicker := s.tickers[rand.Intn(len(s.tickers))]
	base := s.basePrices[randomTicker]
	change := (rand.Float64() - 0.5) * 10
	newPrice := base + change

	if err := s.priceService.UpdatePrice(ctx, randomTicker, newPrice); err != nil {
		s.logger.Error("Failed to send price update", "ticker", randomTicker, "error", err)
	} else {
		s.logger.Debug("Simulated price update", "ticker", randomTicker, "price", newPrice)
	}
}
