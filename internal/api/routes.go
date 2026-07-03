package api

import (
	"net/http"

	"github.com/gorilla/mux"
)

// RegisterRoutes registers all API routes
func (h *Handlers) RegisterRoutes(r *mux.Router) {
	// Structured request logging (outermost) then CORS, applied to all routes.
	r.Use(h.loggingMiddleware)
	r.Use(h.corsMiddleware)

	// Operational endpoints
	r.HandleFunc("/healthz", h.Healthz).Methods("GET")
	r.HandleFunc("/readyz", h.Readyz).Methods("GET")
	r.HandleFunc("/stats", h.Stats).Methods("GET")

	// API routes
	api := r.PathPrefix("/api").Subrouter()

	// OPTIONS is included on mutating routes so the CORS preflight matches a
	// route and runs through corsMiddleware (which short-circuits OPTIONS with
	// the CORS headers). Without it, mux 404s the preflight and browsers block
	// the real cross-origin POST/DELETE.
	api.HandleFunc("/watchlist", h.GetWatchlist).Methods("GET")
	api.HandleFunc("/watchlist", h.AddTicker).Methods("POST", "OPTIONS")
	api.HandleFunc("/watchlist/{ticker}", h.RemoveTicker).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/webhooks/prices", h.WebhookPriceUpdate).Methods("POST", "OPTIONS")

	// WebSocket endpoint
	r.HandleFunc("/ws", h.WebSocket)
}

// RegisterStaticRoutes registers static file serving
func (h *Handlers) RegisterStaticRoutes(r *mux.Router, staticDir string) {
	r.PathPrefix("/").Handler(http.FileServer(http.Dir(staticDir)))
}
