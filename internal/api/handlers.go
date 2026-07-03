package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"stock-ticker-watcher/internal/config"
	"stock-ticker-watcher/internal/models"
	"stock-ticker-watcher/internal/service"
	"stock-ticker-watcher/internal/store"
	"stock-ticker-watcher/internal/websocket"

	"github.com/gorilla/mux"
	gorillaws "github.com/gorilla/websocket"
)

// Pinger verifies a downstream dependency (the database) is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Handlers holds HTTP handlers
type Handlers struct {
	watchlistService *service.WatchlistService
	priceService     *service.PriceService
	hub              *websocket.Hub
	db               Pinger
	config           *config.Config
	logger           *slog.Logger
}

// NewHandlers creates a new handlers instance
func NewHandlers(
	watchlistService *service.WatchlistService,
	priceService *service.PriceService,
	hub *websocket.Hub,
	db Pinger,
	config *config.Config,
	logger *slog.Logger,
) *Handlers {
	return &Handlers{
		watchlistService: watchlistService,
		priceService:     priceService,
		hub:              hub,
		db:               db,
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
		switch {
		case errors.Is(err, store.ErrTickerExists):
			h.respondError(w, http.StatusConflict, "Ticker already in watchlist")
		case errors.Is(err, service.ErrTickerEmpty),
			errors.Is(err, service.ErrTickerTooLong),
			errors.Is(err, service.ErrTickerInvalid):
			h.respondError(w, http.StatusBadRequest, err.Error())
		default:
			h.respondError(w, http.StatusInternalServerError, "Failed to add ticker")
			h.logger.Error("AddTicker failed", "error", err)
		}
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
		switch {
		case errors.Is(err, store.ErrTickerNotFound), errors.Is(err, service.ErrTickerEmpty):
			h.respondError(w, http.StatusNotFound, "Ticker not found in watchlist")
		default:
			h.respondError(w, http.StatusInternalServerError, "Failed to remove ticker")
			h.logger.Error("RemoveTicker failed", "error", err)
		}
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

// Healthz is a liveness probe: 200 if the process is running and serving.
func (h *Handlers) Healthz(w http.ResponseWriter, r *http.Request) {
	h.respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Readyz is a readiness probe: 200 only if the database responds to a ping.
func (h *Handlers) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		h.logger.Error("Readiness check failed", "error", err)
		h.respondJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
			"error":  "database not ready",
		})
		return
	}

	h.respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// Stats exposes simple runtime counters as JSON (not Prometheus).
func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"active_clients":          h.hub.ClientCount(),
		"active_subscriptions":    h.hub.SubscriptionCount(),
		"price_updates_processed": h.priceService.UpdatesProcessed(),
	})
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

// responseRecorder wraps http.ResponseWriter to capture the status code for
// request logging. It forwards Hijack so the WebSocket upgrade still works.
type responseRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
	rr.written = true
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if !rr.written {
		rr.status = http.StatusOK
		rr.written = true
	}
	return rr.ResponseWriter.Write(b)
}

func (rr *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := rr.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hj.Hijack()
}

// loggingMiddleware emits a structured log line per request with method, path,
// status, duration, and remote address.
func (h *Handlers) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rr := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rr, r)

		h.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rr.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
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
