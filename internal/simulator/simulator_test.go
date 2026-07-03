package simulator

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
)

// fakeWatchlist returns a fixed set of watched tickers.
type fakeWatchlist struct {
	tickers []string
}

func (f *fakeWatchlist) GetAllTickers(ctx context.Context) ([]string, error) {
	return f.tickers, nil
}

// fakePriceService records updates and acts as the last-known-price cache,
// mirroring service.PriceService's UpdatePrice/GetPrice contract.
type fakePriceService struct {
	mu      sync.Mutex
	prices  map[string]float64
	updated []string
}

func newFakePriceService(seed map[string]float64) *fakePriceService {
	prices := make(map[string]float64)
	for k, v := range seed {
		prices[k] = v
	}
	return &fakePriceService{prices: prices}
}

func (f *fakePriceService) UpdatePrice(ctx context.Context, ticker string, price float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.prices[ticker] = price
	f.updated = append(f.updated, ticker)
	return nil
}

func (f *fakePriceService) GetPrice(ticker string) (float64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.prices[ticker]
	return p, ok
}

func (f *fakePriceService) updatesFor(ticker string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, t := range f.updated {
		if t == ticker {
			n++
		}
	}
	return n
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestSimulator_UpdatesNewlyAddedTicker proves a ticker that is NOT in any
// hardcoded list — but is present in a watchlist — receives a live update with
// a valid positive price.
func TestSimulator_UpdatesNewlyAddedTicker(t *testing.T) {
	prices := newFakePriceService(nil) // no price yet: must be seeded
	watchlist := &fakeWatchlist{tickers: []string{"ZZZZ"}}
	sim := New(prices, watchlist, testLogger(), 1)

	sim.tick(context.Background())

	if got := prices.updatesFor("ZZZZ"); got != 1 {
		t.Fatalf("expected 1 update for ZZZZ, got %d", got)
	}
	if p, ok := prices.GetPrice("ZZZZ"); !ok || p <= 0 {
		t.Fatalf("expected a positive price for ZZZZ, got %v (ok=%v)", p, ok)
	}
}

// TestSimulator_DrivenByWatchlistNotHardcoded proves the simulator updates only
// the watched tickers and does not touch the previously hardcoded set.
func TestSimulator_DrivenByWatchlistNotHardcoded(t *testing.T) {
	prices := newFakePriceService(nil)
	watchlist := &fakeWatchlist{tickers: []string{"WATCHED"}}
	sim := New(prices, watchlist, testLogger(), 1)

	sim.tick(context.Background())

	if got := prices.updatesFor("WATCHED"); got != 1 {
		t.Fatalf("expected 1 update for WATCHED, got %d", got)
	}
	// None of the formerly hardcoded tickers should be touched.
	for _, t2 := range []string{"AAPL", "NVDA", "IBM", "GOOGL", "MSFT", "TSLA", "AMZN", "META"} {
		if got := prices.updatesFor(t2); got != 0 {
			t.Errorf("did not expect an update for unwatched ticker %s, got %d", t2, got)
		}
	}
}

// TestSimulator_WalksFromLastPrice proves the random walk uses the last known
// price as the base, moving no more than maxStepPct per tick.
func TestSimulator_WalksFromLastPrice(t *testing.T) {
	const base = 200.0
	prices := newFakePriceService(map[string]float64{"AAPL": base})
	watchlist := &fakeWatchlist{tickers: []string{"AAPL"}}
	sim := New(prices, watchlist, testLogger(), 1)

	sim.tick(context.Background())

	p, _ := prices.GetPrice("AAPL")
	lo, hi := base*(1-maxStepPct), base*(1+maxStepPct)
	if p < lo || p > hi {
		t.Fatalf("price %v out of random-walk bounds [%v, %v]", p, lo, hi)
	}
}
