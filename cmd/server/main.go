package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"stock-ticker-watcher/internal/api"
	"stock-ticker-watcher/internal/config"
	"stock-ticker-watcher/internal/service"
	"stock-ticker-watcher/internal/simulator"
	"stock-ticker-watcher/internal/store"
	"stock-ticker-watcher/internal/websocket"

	"github.com/gorilla/mux"
)

func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg := config.Load()
	logger.Info("Starting stock ticker watcher", "port", cfg.Port, "database", cfg.DatabasePath)

	// Initialize database
	dbStore, err := store.New(cfg.DatabasePath, logger)
	if err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer dbStore.Close()

	// Initialize services
	watchlistService := service.NewWatchlistService(dbStore, logger)

	// Initialize WebSocket hub (without price cache initially)
	hub := websocket.NewHub(logger, nil)
	go hub.Run()

	// Initialize price service with hub reference
	priceService := service.NewPriceService(hub, logger)

	// Set price cache on hub to resolve circular dependency
	hub.SetPriceCache(priceService)

	// Initialize simulator
	var sim *simulator.Simulator
	if cfg.SimulatePrices {
		sim = simulator.New(priceService, logger, cfg.SimulateInterval)
	}

	// Initialize handlers
	handlers := api.NewHandlers(watchlistService, priceService, hub, cfg, logger)

	// Setup router
	r := mux.NewRouter()
	handlers.RegisterRoutes(r)

	// Optional single-binary frontend serving. Disabled by default: in Docker
	// Compose nginx serves the built frontend and proxies /api and /ws here,
	// and in local dev Vite serves it. Set STATIC_DIR to enable.
	if cfg.StaticDir != "" {
		handlers.RegisterStaticRoutes(r, cfg.StaticDir)
		logger.Info("Serving static frontend", "dir", cfg.StaticDir)
	}

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start simulator in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if sim != nil {
		go sim.Start(ctx)
	}

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server listening", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Block until signal received
	select {
	case <-sigChan:
		logger.Info("Shutdown signal received")
	case err := <-serverErr:
		logger.Error("Server error", "error", err)
		os.Exit(1)
	}

	// Graceful shutdown
	logger.Info("Starting graceful shutdown")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Cancel simulator
	cancel()

	// Shutdown WebSocket hub
	hub.Shutdown()

	// Shutdown HTTP server
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", "error", err)
	}

	logger.Info("Shutdown complete")
}
