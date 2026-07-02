package api

import (
	"net/http"

	"github.com/gorilla/mux"
)

// RegisterRoutes registers all API routes
func (h *Handlers) RegisterRoutes(r *mux.Router) {
	// Apply CORS middleware to all routes
	r.Use(h.corsMiddleware)

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
