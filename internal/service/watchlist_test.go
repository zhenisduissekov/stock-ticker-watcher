package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"stock-ticker-watcher/internal/models"
)

// MockStore is a mock implementation of the store interface
type MockStore struct {
	watchlist     []string
	addError      error
	removeError   error
	getError      error
	addCalled     bool
	removeCalled  bool
	getCalled     bool
	addedTicker   string
	removedTicker string
}

func (m *MockStore) Close() error {
	return nil
}

func (m *MockStore) Ping(ctx context.Context) error {
	return nil
}

func (m *MockStore) GetWatchlist(ctx context.Context, userID int) ([]string, error) {
	m.getCalled = true
	if m.getError != nil {
		return nil, m.getError
	}
	return m.watchlist, nil
}

func (m *MockStore) GetAllTickers(ctx context.Context) ([]string, error) {
	if m.getError != nil {
		return nil, m.getError
	}
	return m.watchlist, nil
}

func (m *MockStore) AddTicker(ctx context.Context, userID int, ticker string) error {
	m.addCalled = true
	m.addedTicker = ticker
	if m.addError != nil {
		return m.addError
	}
	m.watchlist = append(m.watchlist, ticker)
	return nil
}

func (m *MockStore) RemoveTicker(ctx context.Context, userID int, ticker string) error {
	m.removeCalled = true
	m.removedTicker = ticker
	if m.removeError != nil {
		return m.removeError
	}
	for i, t := range m.watchlist {
		if t == ticker {
			m.watchlist = append(m.watchlist[:i], m.watchlist[i+1:]...)
			return nil
		}
	}
	return nil
}

// mockPriceStore is a minimal in-memory PriceStore for exercising the
// watchlist service without a real price/websocket stack.
type mockPriceStore struct {
	prices map[string]float64
}

func newMockPriceStore(prices map[string]float64) *mockPriceStore {
	if prices == nil {
		prices = make(map[string]float64)
	}
	return &mockPriceStore{prices: prices}
}

func (m *mockPriceStore) GetPrice(ticker string) (float64, bool) {
	p, ok := m.prices[ticker]
	return p, ok
}

func (m *mockPriceStore) InitPrice(ticker string, price float64) float64 {
	if existing, ok := m.prices[ticker]; ok {
		return existing
	}
	m.prices[ticker] = price
	return price
}

func TestAddTicker(t *testing.T) {
	tests := []struct {
		name        string
		request     models.AddTickerRequest
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid ticker",
			request: models.AddTickerRequest{Ticker: "AAPL"},
			wantErr: false,
		},
		{
			name:        "empty ticker",
			request:     models.AddTickerRequest{Ticker: ""},
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "ticker too long",
			request:     models.AddTickerRequest{Ticker: "TOOLONGTICKER"},
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:        "ticker with invalid characters",
			request:     models.AddTickerRequest{Ticker: "AAPL!"},
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:    "ticker with lowercase",
			request: models.AddTickerRequest{Ticker: "aapl"},
			wantErr: false,
		},
		{
			name:    "ticker with spaces",
			request: models.AddTickerRequest{Ticker: " AAPL "},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &MockStore{watchlist: []string{}}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			service := NewWatchlistService(mockStore, logger)
			prices := newMockPriceStore(nil)

			result, err := service.AddTicker(context.Background(), 1, tt.request, prices)

			if tt.wantErr {
				if err == nil {
					t.Errorf("AddTicker() expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("AddTicker() error = %v, expected to contain %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("AddTicker() unexpected error = %v", err)
				return
			}

			if result == nil {
				t.Error("AddTicker() returned nil result")
			}

			if !mockStore.addCalled {
				t.Error("AddTicker() did not call store.AddTicker")
			}

			// Service uppercases and trims tickers
			expectedTicker := strings.ToUpper(strings.TrimSpace(tt.request.Ticker))
			if mockStore.addedTicker != expectedTicker {
				t.Errorf("AddTicker() added ticker = %v, want %v", mockStore.addedTicker, expectedTicker)
			}
		})
	}
}

func TestAddTicker_Duplicate(t *testing.T) {
	mockStore := &MockStore{
		watchlist: []string{"AAPL"},
		addError:  nil,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewWatchlistService(mockStore, logger)
	prices := newMockPriceStore(nil)

	// First add should succeed
	_, err := service.AddTicker(context.Background(), 1, models.AddTickerRequest{Ticker: "NVDA"}, prices)
	if err != nil {
		t.Errorf("First AddTicker() failed = %v", err)
	}

	// Simulate duplicate error
	mockStore.addError = &duplicateError{}
	_, err = service.AddTicker(context.Background(), 1, models.AddTickerRequest{Ticker: "AAPL"}, prices)
	if err == nil {
		t.Error("AddTicker() expected duplicate error, got nil")
	}
}

func TestRemoveTicker(t *testing.T) {
	tests := []struct {
		name        string
		ticker      string
		watchlist   []string
		removeError error
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid removal",
			ticker:    "AAPL",
			watchlist: []string{"AAPL", "NVDA"},
			wantErr:   false,
		},
		{
			name:        "empty ticker",
			ticker:      "",
			watchlist:   []string{"AAPL"},
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "ticker not found",
			ticker:      "IBM",
			watchlist:   []string{"AAPL"},
			removeError: fmt.Errorf("ticker not found in watchlist"),
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &MockStore{watchlist: tt.watchlist, removeError: tt.removeError}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			service := NewWatchlistService(mockStore, logger)

			err := service.RemoveTicker(context.Background(), 1, tt.ticker)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RemoveTicker() expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("RemoveTicker() error = %v, expected to contain %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("RemoveTicker() unexpected error = %v", err)
			}

			if !mockStore.removeCalled {
				t.Error("RemoveTicker() did not call store.RemoveTicker")
			}

			if mockStore.removedTicker != tt.ticker {
				t.Errorf("RemoveTicker() removed ticker = %v, want %v", mockStore.removedTicker, tt.ticker)
			}
		})
	}
}

func TestGetWatchlist(t *testing.T) {
	mockStore := &MockStore{watchlist: []string{"AAPL", "NVDA"}}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	service := NewWatchlistService(mockStore, logger)
	prices := newMockPriceStore(map[string]float64{"AAPL": 175.50, "NVDA": 450.25})

	watchlist, err := service.GetWatchlist(context.Background(), 1, prices)

	if err != nil {
		t.Errorf("GetWatchlist() error = %v", err)
	}

	if len(watchlist) != 2 {
		t.Errorf("GetWatchlist() returned %d items, want 2", len(watchlist))
	}

	if !mockStore.getCalled {
		t.Error("GetWatchlist() did not call store.GetWatchlist")
	}
}

// Helper functions
type duplicateError struct{}

func (e *duplicateError) Error() string {
	return "UNIQUE constraint failed"
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
