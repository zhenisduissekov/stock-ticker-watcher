package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
)

const (
	DemoUserID = 1
	Port       = "8080"
)

var (
	db          *sql.DB
	priceCache  = make(map[string]float64)
	cacheMutex  sync.RWMutex
	clients     = make(map[*websocket.Conn]bool)
	clientsLock sync.Mutex
	upgrader    = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for MVP
		},
	}
)

type WatchlistItem struct {
	Ticker string  `json:"ticker"`
	Price  float64 `json:"price"`
}

type PriceUpdate struct {
	Ticker string  `json:"ticker"`
	Price  float64 `json:"price"`
}

type AddTickerRequest struct {
	Ticker string `json:"ticker"`
}

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./stocks.db")
	if err != nil {
		log.Fatal(err)
	}

	// Create tables
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		);
		
		CREATE TABLE IF NOT EXISTS user_stocks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			ticker TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id),
			UNIQUE(user_id, ticker)
		);
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize demo user
	_, err = db.Exec("INSERT OR IGNORE INTO users (id, name) VALUES (?, ?)", DemoUserID, "Demo User")
	if err != nil {
		log.Fatal(err)
	}
}

func broadcastPriceUpdate(ticker string, price float64) {
	clientsLock.Lock()
	defer clientsLock.Unlock()

	message := PriceUpdate{Ticker: ticker, Price: price}
	for client := range clients {
		err := client.WriteJSON(message)
		if err != nil {
			client.Close()
			delete(clients, client)
		}
	}
}

func getCORSHeaders() map[string]string {
	return map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, DELETE, OPTIONS",
		"Access-Control-Allow-Headers": "Content-Type",
	}
}

func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for key, value := range getCORSHeaders() {
			w.Header().Set(key, value)
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func getWatchlist(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT ticker FROM user_stocks WHERE user_id = ?", DemoUserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var watchlist []WatchlistItem
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()

	for rows.Next() {
		var ticker string
		if err := rows.Scan(&ticker); err != nil {
			continue
		}
		price, exists := priceCache[ticker]
		if !exists {
			price = 100 + rand.Float64()*100
			priceCache[ticker] = price
		}
		watchlist = append(watchlist, WatchlistItem{Ticker: ticker, Price: price})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(watchlist)
}

func addTicker(w http.ResponseWriter, r *http.Request) {
	var req AddTickerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Ticker == "" {
		http.Error(w, "Ticker is required", http.StatusBadRequest)
		return
	}

	ticker := req.Ticker

	_, err := db.Exec("INSERT INTO user_stocks (user_id, ticker) VALUES (?, ?)", DemoUserID, ticker)
	if err != nil {
		http.Error(w, "Ticker already in watchlist", http.StatusConflict)
		return
	}

	cacheMutex.Lock()
	if _, exists := priceCache[ticker]; !exists {
		priceCache[ticker] = 100 + rand.Float64()*100
	}
	cacheMutex.Unlock()

	cacheMutex.RLock()
	price := priceCache[ticker]
	cacheMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(WatchlistItem{Ticker: ticker, Price: price})
}

func removeTicker(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	ticker := vars["ticker"]

	result, err := db.Exec("DELETE FROM user_stocks WHERE user_id = ? AND ticker = ?", DemoUserID, ticker)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Ticker not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Ticker removed"})
}

func webhookPriceUpdate(w http.ResponseWriter, r *http.Request) {
	var update PriceUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if update.Ticker == "" || update.Price <= 0 {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	cacheMutex.Lock()
	priceCache[update.Ticker] = update.Price
	cacheMutex.Unlock()

	broadcastPriceUpdate(update.Ticker, update.Price)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Price updated"})
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	clientsLock.Lock()
	clients[conn] = true
	clientsLock.Unlock()

	log.Println("Client connected via WebSocket")

	// Send current prices on connection
	cacheMutex.RLock()
	for ticker, price := range priceCache {
		conn.WriteJSON(PriceUpdate{Ticker: ticker, Price: price})
	}
	cacheMutex.RUnlock()

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}

	clientsLock.Lock()
	delete(clients, conn)
	clientsLock.Unlock()

	log.Println("Client disconnected")
}

func simulatePriceUpdates() {
	tickers := []string{"AAPL", "NVDA", "IBM", "GOOGL", "MSFT", "TSLA", "AMZN", "META"}
	basePrices := map[string]float64{
		"AAPL":  175,
		"NVDA":  450,
		"IBM":   160,
		"GOOGL": 140,
		"MSFT":  380,
		"TSLA":  180,
		"AMZN":  145,
		"META":  300,
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		randomTicker := tickers[rand.Intn(len(tickers))]
		base := basePrices[randomTicker]
		change := (rand.Float64() - 0.5) * 10
		newPrice := base + change

		cacheMutex.Lock()
		priceCache[randomTicker] = newPrice
		cacheMutex.Unlock()

		broadcastPriceUpdate(randomTicker, newPrice)
		log.Printf("Simulated price update: %s = $%.2f", randomTicker, newPrice)
	}
}

func main() {
	initDB()
	defer db.Close()

	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/watchlist", withCORS(getWatchlist)).Methods("GET", "OPTIONS")
	api.HandleFunc("/watchlist", withCORS(addTicker)).Methods("POST", "OPTIONS")
	api.HandleFunc("/watchlist/{ticker}", withCORS(removeTicker)).Methods("DELETE", "OPTIONS")
	api.HandleFunc("/webhooks/prices", withCORS(webhookPriceUpdate)).Methods("POST", "OPTIONS")

	// WebSocket endpoint
	r.HandleFunc("/ws", websocketHandler)

	// Start price simulation in background
	go simulatePriceUpdates()

	// Serve static files from frontend directory
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("../frontend/dist")))

	log.Printf("Server starting on port %s...", Port)
	log.Fatal(http.ListenAndServe(":"+Port, r))
}
