package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"stock-ticker-watcher/internal/api"
	"stock-ticker-watcher/internal/config"
	"stock-ticker-watcher/internal/service"
	"stock-ticker-watcher/internal/simulator"
	"stock-ticker-watcher/internal/store"
	"stock-ticker-watcher/internal/websocket"
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

	// Initialize WebSocket hub
	hub := websocket.NewHub(logger)
	go hub.Run()

	// Initialize services
	priceService := service.NewPriceService(hub, logger)
	watchlistService := service.NewWatchlistService(dbStore, logger)

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
	handlers.RegisterStaticRoutes(r, "../frontend/dist")

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

	// Shutdown HTTP server
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", "error", err)
	}

	logger.Info("Shutdown complete")
}
