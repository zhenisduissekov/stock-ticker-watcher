package websocket

import (
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"stock-ticker-watcher/internal/models"
)

// MockPriceCache is a mock implementation of PriceCache
type MockPriceCache struct {
	prices map[string]float64
	mu     sync.RWMutex
}

func NewMockPriceCache() *MockPriceCache {
	return &MockPriceCache{
		prices: make(map[string]float64),
	}
}

func (m *MockPriceCache) GetPrice(ticker string) (float64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	price, exists := m.prices[ticker]
	return price, exists
}

func (m *MockPriceCache) SetPrice(ticker string, price float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prices[ticker] = price
}

// TestClient wraps a real Client with message tracking for testing
type TestClient struct {
	*Client
	messages []models.PriceUpdate
	mu       sync.Mutex
}

func NewTestClient(id string, hub *Hub, logger *slog.Logger) *TestClient {
	sendChan := make(chan []byte, 256)
	client := &Client{
		ID:     id,
		Send:   sendChan,
		Hub:    hub,
		Logger: logger,
	}

	tc := &TestClient{
		Client:   client,
		messages: make([]models.PriceUpdate, 0),
	}

	// Start a goroutine to capture messages
	go tc.captureMessages()

	return tc
}

func (tc *TestClient) captureMessages() {
	for data := range tc.Client.Send {
		var update models.PriceUpdate
		if err := json.Unmarshal(data, &update); err == nil {
			tc.mu.Lock()
			tc.messages = append(tc.messages, update)
			tc.mu.Unlock()
		}
	}
}

func (tc *TestClient) GetMessages() []models.PriceUpdate {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	result := make([]models.PriceUpdate, len(tc.messages))
	copy(result, tc.messages)
	return result
}

func (tc *TestClient) Close() {
	close(tc.Client.Send)
}

func TestSubscribe_ReceivesUpdates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	priceCache := NewMockPriceCache()
	priceCache.SetPrice("AAPL", 175.50)

	hub := NewHub(logger, priceCache)
	go hub.Run()
	defer hub.Shutdown()

	client := NewTestClient("client1", hub, logger)
	hub.Subscribe(client.Client, "AAPL")

	// Give time for the subscription to process
	time.Sleep(10 * time.Millisecond)

	// Broadcast an update
	hub.Broadcast("AAPL", 180.00)

	// Give time for the broadcast to process
	time.Sleep(10 * time.Millisecond)

	messages := client.GetMessages()
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages (initial sync + broadcast), got %d", len(messages))
	}

	// First message should be the initial sync
	if messages[0].Ticker != "AAPL" || messages[0].Price != 175.50 {
		t.Errorf("Initial sync message wrong: got %v", messages[0])
	}

	// Second message should be the broadcast
	if messages[1].Ticker != "AAPL" || messages[1].Price != 180.00 {
		t.Errorf("Broadcast message wrong: got %v", messages[1])
	}

	client.Close()
}

func TestUnsubscribe_StopsUpdates(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	priceCache := NewMockPriceCache()
	priceCache.SetPrice("AAPL", 175.50)

	hub := NewHub(logger, priceCache)
	go hub.Run()
	defer hub.Shutdown()

	client := NewTestClient("client1", hub, logger)
	hub.Subscribe(client.Client, "AAPL")

	// Give time for the subscription to process
	time.Sleep(10 * time.Millisecond)

	// Broadcast an update while subscribed
	hub.Broadcast("AAPL", 180.00)
	time.Sleep(10 * time.Millisecond)

	// Unsubscribe
	hub.Unsubscribe(client.Client, "AAPL")
	time.Sleep(10 * time.Millisecond)

	// Broadcast another update after unsubscribe
	hub.Broadcast("AAPL", 185.00)
	time.Sleep(10 * time.Millisecond)

	messages := client.GetMessages()
	// Should have initial sync + 1 broadcast (before unsubscribe)
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages (initial sync + 1 broadcast), got %d", len(messages))
	}

	// Last message should be the first broadcast
	if messages[1].Price != 180.00 {
		t.Errorf("Last message should be 180.00, got %f", messages[1].Price)
	}

	client.Close()
}

func TestSubscribe_OnlySubscribedTickers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	priceCache := NewMockPriceCache()
	priceCache.SetPrice("AAPL", 175.50)
	priceCache.SetPrice("NVDA", 450.00)

	hub := NewHub(logger, priceCache)
	go hub.Run()
	defer hub.Shutdown()

	client := NewTestClient("client1", hub, logger)
	// Subscribe only to AAPL
	hub.Subscribe(client.Client, "AAPL")

	time.Sleep(10 * time.Millisecond)

	// Broadcast updates for both tickers
	hub.Broadcast("AAPL", 180.00)
	hub.Broadcast("NVDA", 460.00)
	time.Sleep(10 * time.Millisecond)

	messages := client.GetMessages()
	// Should have initial sync for AAPL + AAPL broadcast only
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages (AAPL only), got %d", len(messages))
	}

	for _, msg := range messages {
		if msg.Ticker != "AAPL" {
			t.Errorf("Received message for unsubscribed ticker: %v", msg)
		}
	}

	client.Close()
}

func TestMultipleClients_Subscription(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	priceCache := NewMockPriceCache()
	priceCache.SetPrice("AAPL", 175.50)

	hub := NewHub(logger, priceCache)
	go hub.Run()
	defer hub.Shutdown()

	client1 := NewTestClient("client1", hub, logger)
	client2 := NewTestClient("client2", hub, logger)

	// Both clients subscribe to AAPL
	hub.Subscribe(client1.Client, "AAPL")
	hub.Subscribe(client2.Client, "AAPL")

	time.Sleep(10 * time.Millisecond)

	// Broadcast an update
	hub.Broadcast("AAPL", 180.00)
	time.Sleep(10 * time.Millisecond)

	// Both clients should receive the update
	messages1 := client1.GetMessages()
	messages2 := client2.GetMessages()

	if len(messages1) != 2 {
		t.Errorf("Client1 expected 2 messages, got %d", len(messages1))
	}
	if len(messages2) != 2 {
		t.Errorf("Client2 expected 2 messages, got %d", len(messages2))
	}

	client1.Close()
	client2.Close()
}

func TestHub_Shutdown(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	priceCache := NewMockPriceCache()

	hub := NewHub(logger, priceCache)
	go hub.Run()

	// Wait a bit for hub to start
	time.Sleep(10 * time.Millisecond)

	// Shutdown should not panic
	hub.Shutdown()

	// Give time for shutdown to complete
	time.Sleep(10 * time.Millisecond)
}

// TestIntegration_PriceUpdateFlow tests the full flow: subscribe -> price update -> receive
func TestIntegration_PriceUpdateFlow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	priceCache := NewMockPriceCache()

	// Set initial prices
	priceCache.SetPrice("AAPL", 175.50)
	priceCache.SetPrice("NVDA", 450.00)

	hub := NewHub(logger, priceCache)
	go hub.Run()
	defer hub.Shutdown()

	// Create and connect client
	client := NewTestClient("client1", hub, logger)

	// Subscribe to AAPL only
	hub.Subscribe(client.Client, "AAPL")
	time.Sleep(10 * time.Millisecond)

	// Send price update through the same path as real app (via hub.Broadcast)
	// This simulates what happens when PriceService.UpdatePrice is called
	hub.Broadcast("AAPL", 180.00)
	hub.Broadcast("NVDA", 460.00) // Unsubscribed ticker
	time.Sleep(10 * time.Millisecond)

	messages := client.GetMessages()

	// Should receive: initial sync for AAPL + AAPL broadcast
	if len(messages) != 2 {
		t.Errorf("Expected 2 messages (initial AAPL sync + AAPL broadcast), got %d", len(messages))
	}

	// Verify messages are for AAPL only
	for i, msg := range messages {
		if msg.Ticker != "AAPL" {
			t.Errorf("Message %d should be for AAPL, got %s", i, msg.Ticker)
		}
	}

	// Verify initial sync price
	if messages[0].Price != 175.50 {
		t.Errorf("Initial sync price should be 175.50, got %f", messages[0].Price)
	}

	// Verify broadcast price
	if messages[1].Price != 180.00 {
		t.Errorf("Broadcast price should be 180.00, got %f", messages[1].Price)
	}

	client.Close()
}
