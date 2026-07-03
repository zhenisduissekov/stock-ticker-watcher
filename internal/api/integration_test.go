package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stock-ticker-watcher/internal/config"
	"stock-ticker-watcher/internal/models"
	"stock-ticker-watcher/internal/service"
	"stock-ticker-watcher/internal/store"
	"stock-ticker-watcher/internal/websocket"

	"github.com/gorilla/mux"
	gorillaws "github.com/gorilla/websocket"
)

// testEnv wires the real HTTP router, WebSocket hub, price service, and a
// file-backed SQLite store for end-to-end integration tests.
type testEnv struct {
	server   *httptest.Server
	priceSvc *service.PriceService
	hub      *websocket.Hub
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	dbStore, err := store.New(dbPath, logger)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	watchlistSvc := service.NewWatchlistService(dbStore, logger)
	hub := websocket.NewHub(logger, nil)
	go hub.Run()
	priceSvc := service.NewPriceService(hub, logger)
	hub.SetPriceCache(priceSvc)

	cfg := &config.Config{DemoUserID: 1, FrontendOrigin: "*"}
	handlers := NewHandlers(watchlistSvc, priceSvc, hub, dbStore, cfg, logger)

	r := mux.NewRouter()
	handlers.RegisterRoutes(r)
	server := httptest.NewServer(r)

	t.Cleanup(func() {
		server.Close()
		hub.Shutdown()
		dbStore.Close()
	})

	return &testEnv{server: server, priceSvc: priceSvc, hub: hub}
}

// waitFor polls cond until it returns true or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

func dialWS(t *testing.T, serverURL string) *gorillaws.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws"
	conn, _, err := gorillaws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial WebSocket: %v", err)
	}
	return conn
}

// TestWebSocketPriceDelivery proves the full subscribe -> update -> deliver
// path, and that a ticker the client did NOT subscribe to is not delivered.
func TestWebSocketPriceDelivery(t *testing.T) {
	env := newTestEnv(t)

	// 1. Client connects.
	conn := dialWS(t, env.server.URL)
	defer conn.Close()

	// 2. Client subscribes to AAPL.
	if err := conn.WriteJSON(map[string]string{"action": "subscribe", "ticker": "AAPL"}); err != nil {
		t.Fatalf("failed to send subscribe: %v", err)
	}

	// Wait until the hub has registered the subscription so the subsequent
	// broadcast is guaranteed to have a subscriber (avoids a send/subscribe race).
	waitFor(t, 2*time.Second, func() bool { return env.hub.SubscriptionCount() >= 1 })

	// 3. Trigger the price update path: one subscribed ticker, one unrelated.
	ctx := context.Background()
	if err := env.priceSvc.UpdatePrice(ctx, "SPY", 999.99); err != nil { // no subscribers
		t.Fatalf("UpdatePrice(SPY) failed: %v", err)
	}
	if err := env.priceSvc.UpdatePrice(ctx, "AAPL", 150.25); err != nil {
		t.Fatalf("UpdatePrice(AAPL) failed: %v", err)
	}

	// 4. Read everything delivered within the window.
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	gotAAPL := false
	for {
		var upd models.PriceUpdate
		if err := conn.ReadJSON(&upd); err != nil {
			break // read deadline reached: no more messages
		}
		switch upd.Ticker {
		case "SPY":
			// 5. Unrelated ticker must never be delivered to this client.
			t.Fatalf("received update for unsubscribed ticker SPY: %+v", upd)
		case "AAPL":
			gotAAPL = true
			if upd.Price != 150.25 {
				t.Errorf("AAPL price = %v, want 150.25", upd.Price)
			}
		default:
			t.Fatalf("received unexpected ticker: %+v", upd)
		}
	}

	if !gotAAPL {
		t.Fatal("did not receive the AAPL price update")
	}
}

// TestHealthAndReadiness covers the operational probes.
func TestHealthAndReadiness(t *testing.T) {
	env := newTestEnv(t)

	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := http.Get(env.server.URL + path)
		if err != nil {
			t.Fatalf("GET %s failed: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, resp.StatusCode)
		}
	}
}

// TestStatsReflectActivity verifies the runtime counters move with real activity.
func TestStatsReflectActivity(t *testing.T) {
	env := newTestEnv(t)

	conn := dialWS(t, env.server.URL)
	defer conn.Close()

	if err := conn.WriteJSON(map[string]string{"action": "subscribe", "ticker": "AAPL"}); err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	waitFor(t, 2*time.Second, func() bool { return env.hub.SubscriptionCount() >= 1 })

	if err := env.priceSvc.UpdatePrice(context.Background(), "AAPL", 150.25); err != nil {
		t.Fatalf("UpdatePrice failed: %v", err)
	}

	resp, err := http.Get(env.server.URL + "/stats")
	if err != nil {
		t.Fatalf("GET /stats failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /stats status = %d, want 200", resp.StatusCode)
	}

	var stats struct {
		ActiveClients         int   `json:"active_clients"`
		ActiveSubscriptions   int   `json:"active_subscriptions"`
		PriceUpdatesProcessed int64 `json:"price_updates_processed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode /stats: %v", err)
	}

	if stats.ActiveClients < 1 {
		t.Errorf("active_clients = %d, want >= 1", stats.ActiveClients)
	}
	if stats.ActiveSubscriptions < 1 {
		t.Errorf("active_subscriptions = %d, want >= 1", stats.ActiveSubscriptions)
	}
	if stats.PriceUpdatesProcessed < 1 {
		t.Errorf("price_updates_processed = %d, want >= 1", stats.PriceUpdatesProcessed)
	}
}
