package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"stock-ticker-watcher/internal/config"
	"stock-ticker-watcher/internal/models"
	"stock-ticker-watcher/internal/service"
	"stock-ticker-watcher/internal/websocket"

	"github.com/gorilla/mux"
	gorillaws "github.com/gorilla/websocket"
)

// Handlers holds HTTP handlers
type Handlers struct {
	watchlistService *service.WatchlistService
	priceService     *service.PriceService
	hub              *websocket.Hub
	config           *config.Config
	logger           *slog.Logger
}

// NewHandlers creates a new handlers instance
func NewHandlers(
	watchlistService *service.WatchlistService,
	priceService *service.PriceService,
	hub *websocket.Hub,
	config *config.Config,
	logger *slog.Logger,
) *Handlers {
	return &Handlers{
		watchlistService: watchlistService,
		priceService:     priceService,
		hub:              hub,
		config:           config,
		logger:           logger,
	}
}

// GetWatchlist handles GET /api/watchlist
func (h *Handlers) GetWatchlist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	watchlist, err := h.watchlistService.GetWatchlist(ctx, h.config.DemoUserID, h.priceService)
	if err != nil {
		h.respondError(w, http.StatusInternalServerError, "Failed to get watchlist")
		h.logger.Error("GetWatchlist failed", "error", err)
		return
	}

	h.respondJSON(w, http.StatusOK, watchlist)
}

// AddTicker handles POST /api/watchlist
func (h *Handlers) AddTicker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req models.AddTickerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	item, err := h.watchlistService.AddTicker(ctx, h.config.DemoUserID, req, h.priceService)
	if err != nil {
		if err.Error() == "ticker already in watchlist" {
			h.respondError(w, http.StatusConflict, "Ticker already in watchlist")
			return
		}
		if err.Error() == "ticker cannot be empty" || err.Error() == "ticker too long (max 10 characters)" || err.Error() == "ticker contains invalid characters" {
			h.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.respondError(w, http.StatusInternalServerError, "Failed to add ticker")
		h.logger.Error("AddTicker failed", "error", err)
		return
	}

	h.respondJSON(w, http.StatusCreated, item)
}

// RemoveTicker handles DELETE /api/watchlist/{ticker}
func (h *Handlers) RemoveTicker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	ticker := vars["ticker"]

	if ticker == "" {
		h.respondError(w, http.StatusBadRequest, "Ticker is required")
		return
	}

	if err := h.watchlistService.RemoveTicker(ctx, h.config.DemoUserID, ticker); err != nil {
		if err.Error() == "ticker not found in watchlist" || err.Error() == "ticker cannot be empty" {
			h.respondError(w, http.StatusNotFound, "Ticker not found in watchlist")
			return
		}
		h.respondError(w, http.StatusInternalServerError, "Failed to remove ticker")
		h.logger.Error("RemoveTicker failed", "error", err)
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]string{"message": "Ticker removed"})
}

// WebhookPriceUpdate handles POST /api/webhooks/prices
func (h *Handlers) WebhookPriceUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var update models.PriceUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := h.priceService.UpdatePrice(ctx, update.Ticker, update.Price); err != nil {
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]string{"message": "Price updated"})
}

// WebSocket handles WebSocket connections
func (h *Handlers) WebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := gorillaws.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for MVP
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("WebSocket upgrade failed", "error", err)
		return
	}

	clientID := websocket.GenerateClientID()
	client := websocket.NewClient(clientID, conn, h.hub, h.logger)

	h.hub.Register(client)

	// Start pumps
	go client.WritePump()
	go client.ReadPump()

	h.logger.Info("WebSocket connection established", "client_id", clientID)
}

// respondJSON writes a JSON response
func (h *Handlers) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes an error response
func (h *Handlers) respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(models.ErrorResponse{Error: message})
}

// corsMiddleware adds CORS headers
func (h *Handlers) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", h.config.FrontendOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
