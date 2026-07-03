package websocket

import (
	"log/slog"
	"sync"

	"stock-ticker-watcher/internal/models"

	"github.com/google/uuid"
)

// Hub maintains the set of active clients and their subscriptions
type Hub struct {
	clients       map[*Client]bool
	subscriptions map[string]map[*Client]bool // ticker -> clients
	register      chan *Client
	unregister    chan *Client
	broadcast     chan models.PriceUpdate
	priceCache    PriceCache
	shutdown      chan struct{}
	mu            sync.RWMutex
	logger        *slog.Logger
}

// PriceCache interface for getting current prices
type PriceCache interface {
	GetPrice(ticker string) (float64, bool)
}

// NewHub creates a new WebSocket hub
func NewHub(logger *slog.Logger, priceCache PriceCache) *Hub {
	return &Hub{
		clients:       make(map[*Client]bool),
		subscriptions: make(map[string]map[*Client]bool),
		register:      make(chan *Client),
		unregister:    make(chan *Client),
		broadcast:     make(chan models.PriceUpdate, 256),
		priceCache:    priceCache,
		shutdown:      make(chan struct{}),
		logger:        logger,
	}
}

// Run starts the hub's event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case update := <-h.broadcast:
			h.broadcastUpdate(update)

		case <-h.shutdown:
			h.logger.Info("Hub shutting down")
			return
		}
	}
}

// registerClient adds a new client to the hub
func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[client] = true
	h.logger.Info("Client registered", "client_id", client.ID)
}

// unregisterClient removes a client from the hub
func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		// Remove from all subscriptions
		for ticker, clients := range h.subscriptions {
			if _, ok := clients[client]; ok {
				delete(clients, client)
				if len(clients) == 0 {
					delete(h.subscriptions, ticker)
				}
			}
		}
		close(client.Send)
		h.logger.Info("Client unregistered", "client_id", client.ID)
	}
}

// Subscribe adds a client to a ticker's subscription list
func (h *Hub) Subscribe(client *Client, ticker string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.subscriptions[ticker]; !ok {
		h.subscriptions[ticker] = make(map[*Client]bool)
	}
	h.subscriptions[ticker][client] = true
	h.logger.Info("Client subscribed", "client_id", client.ID, "ticker", ticker)

	// Send current price if available
	if h.priceCache != nil {
		if price, exists := h.priceCache.GetPrice(ticker); exists {
			client.SendPriceUpdate(models.PriceUpdate{
				Ticker: ticker,
				Price:  price,
			})
		}
	}
}

// Unsubscribe removes a client from a ticker's subscription list
func (h *Hub) Unsubscribe(client *Client, ticker string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if clients, ok := h.subscriptions[ticker]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.subscriptions, ticker)
		}
		h.logger.Info("Client unsubscribed", "client_id", client.ID, "ticker", ticker)
	}
}

// Broadcast sends a price update to all subscribed clients
func (h *Hub) Broadcast(ticker string, price float64) {
	h.broadcast <- models.PriceUpdate{
		Ticker: ticker,
		Price:  price,
	}
}

// broadcastUpdate sends an update to subscribed clients
func (h *Hub) broadcastUpdate(update models.PriceUpdate) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	clients, ok := h.subscriptions[update.Ticker]
	if !ok {
		return // No subscribers for this ticker
	}

	for client := range clients {
		if err := client.SendPriceUpdate(update); err != nil {
			h.logger.Error("Failed to send update", "client_id", client.ID, "ticker", update.Ticker, "error", err)
		}
	}
}

// Register adds a client to the hub (called from connection handler)
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister removes a client from the hub (called from connection handler)
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// SubscriptionCount returns the total number of active (ticker, client)
// subscriptions across all tickers.
func (h *Hub) SubscriptionCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, clients := range h.subscriptions {
		total += len(clients)
	}
	return total
}

// GenerateClientID generates a unique client ID
func GenerateClientID() string {
	return uuid.New().String()
}

// Shutdown gracefully stops the hub
func (h *Hub) Shutdown() {
	close(h.shutdown)
}

// SetPriceCache sets the price cache (used to resolve circular dependency)
func (h *Hub) SetPriceCache(priceCache PriceCache) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.priceCache = priceCache
}
