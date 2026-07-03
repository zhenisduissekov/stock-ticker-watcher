package simulator

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"stock-ticker-watcher/internal/service"
)

// maxStepPct is the maximum fraction a price can move in a single tick (±1%),
// keeping the simulated series a smooth random walk rather than a wild jump.
const maxStepPct = 0.01

// Simulator generates random price updates to simulate a third-party provider.
// It drives updates for exactly the tickers users are currently watching,
// walking each one from its last known price.
type Simulator struct {
	priceService PriceService
	watchlist    WatchlistSource
	interval     time.Duration
	logger       *slog.Logger
}

// PriceService is the central price update path. The simulator reads the last
// known price and writes the next one through the same method the webhook uses.
type PriceService interface {
	UpdatePrice(ctx context.Context, ticker string, price float64) error
	GetPrice(ticker string) (float64, bool)
}

// WatchlistSource supplies the set of tickers that should receive live updates.
type WatchlistSource interface {
	GetAllTickers(ctx context.Context) ([]string, error)
}

// New creates a new simulator.
func New(priceService PriceService, watchlist WatchlistSource, logger *slog.Logger, interval int) *Simulator {
	return &Simulator{
		priceService: priceService,
		watchlist:    watchlist,
		interval:     time.Duration(interval) * time.Second,
		logger:       logger,
	}
}

// Start begins the simulation, ticking until the context is cancelled.
func (s *Simulator) Start(ctx context.Context) {
	s.logger.Info("Starting price simulator", "interval", s.interval.Seconds())

	// Prime prices for anything already watched at startup.
	s.tick(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Simulator stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick updates every currently watched ticker once.
func (s *Simulator) tick(ctx context.Context) {
	tickers, err := s.watchlist.GetAllTickers(ctx)
	if err != nil {
		s.logger.Error("Simulator failed to load watched tickers", "error", err)
		return
	}

	for _, ticker := range tickers {
		s.updateOne(ctx, ticker)
	}
}

// updateOne applies a small random-walk step to a single ticker, using its
// last known price as the base and seeding a reasonable price if none exists.
func (s *Simulator) updateOne(ctx context.Context, ticker string) {
	base, ok := s.priceService.GetPrice(ticker)
	if !ok || base <= 0 {
		base = service.SeedPrice(ticker)
	}

	// Move up to ±maxStepPct of the current price.
	newPrice := base * (1 + (rand.Float64()*2-1)*maxStepPct)
	if newPrice <= 0 {
		newPrice = base
	}

	if err := s.priceService.UpdatePrice(ctx, ticker, newPrice); err != nil {
		s.logger.Error("Failed to send price update", "ticker", ticker, "error", err)
		return
	}
	s.logger.Debug("Simulated price update", "ticker", ticker, "price", newPrice)
}
