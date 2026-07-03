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

	api.HandleFunc("/watchlist", h.GetWatchlist).Methods("GET")
	api.HandleFunc("/watchlist", h.AddTicker).Methods("POST")
	api.HandleFunc("/watchlist/{ticker}", h.RemoveTicker).Methods("DELETE")
	api.HandleFunc("/webhooks/prices", h.WebhookPriceUpdate).Methods("POST")

	// WebSocket endpoint
	r.HandleFunc("/ws", h.WebSocket)
}

// RegisterStaticRoutes registers static file serving
func (h *Handlers) RegisterStaticRoutes(r *mux.Router, staticDir string) {
	r.PathPrefix("/").Handler(http.FileServer(http.Dir(staticDir)))
}
