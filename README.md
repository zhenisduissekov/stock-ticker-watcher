# Stock Ticker Watcher

[![CI](https://github.com/zhenisduissekov/stock-ticker-watcher/actions/workflows/ci.yml/badge.svg)](https://github.com/zhenisduissekov/stock-ticker-watcher/actions/workflows/ci.yml)

A real-time stock watchlist: add tickers, and their prices stream to the browser
over WebSockets as they change. The backend is a layered Go service; the
frontend is React + Vite. Data lives in SQLite. Everything runs locally with one
command or via Docker Compose.

## What this is & why it exists

This is a **portfolio project**, built to demonstrate production-minded backend
engineering in Go without hiding behind a framework or a pile of infrastructure.
The problem — a live watchlist fed by a price stream — is deliberately small, so
the interesting part is *how* it's built: clean layering, correct concurrency,
graceful shutdown, a subscription-aware WebSocket hub, and tests that exercise
the real paths. It favors correctness and clarity over feature count.

## Architecture

Requests flow inward through four layers, each depending only on the layer below
via an interface. Two background flows (the price simulator and the WebSocket
hub) feed the same central price path that the webhook uses.

```
                 ┌───────────────────────────────────────────────┐
   Browser ◄──── │  Frontend (React + Vite)                       │
      │   WS     │  add/remove tickers · live price display       │
      │  price   └───────────────────────────────────────────────┘
      │ updates            │ REST /api            ▲ ws://…/ws
      ▼                    ▼                      │
┌──────────────────────────────────────────────────────────────────────┐
│  GO BACKEND  (:8080)                                                   │
│                                                                        │
│   HTTP API        internal/api      handlers, routing, middleware      │
│      │                                                                 │
│      ▼                                                                 │
│   Service         internal/service  watchlist logic · price cache      │
│      │                    ▲                    │ Broadcast(ticker)      │
│      ▼                    │ GetAllTickers      ▼                        │
│   Store           internal/store     SQLite (WAL) ── Store interface    │
│                           ▲                                            │
│                           │ reads watched tickers                      │
│   Simulator       internal/simulator ── random-walks watched prices    │
│                                              │ UpdatePrice (central)    │
│                                              ▼                          │
│   WebSocket Hub   internal/websocket  ticker → subscribed clients       │
└──────────────────────────────────────────────────────────────────────┘
                              ▲
                              │ POST /api/webhooks/prices
                       Third-party price provider (or curl)
```

**Layers**

| Layer | Package | Responsibility |
|-------|---------|----------------|
| HTTP API | `internal/api` | Request decoding, validation-error mapping, JSON responses, CORS + request logging middleware |
| Service | `internal/service` | Watchlist business logic; thread-safe in-memory price cache; the single `UpdatePrice` write path |
| Store | `internal/store` | `Store` interface + SQLite implementation (WAL, busy-timeout, foreign keys) |
| WebSocket | `internal/websocket` | `Hub` (subscription map + event loop) and per-connection `Client` read/write pumps |
| Simulator | `internal/simulator` | Background price generator driven by the current watchlist |

Composition happens in [`cmd/server/main.go`](cmd/server/main.go): it wires the
store, services, hub, simulator, and HTTP server, then handles graceful shutdown.

## Request flow (REST)

```
Client → CORS + logging middleware → API handler → service → store → SQLite
                                          │            │
                                     decode/validate  business rules
Response ← JSON ← handler ← (item | typed error) ←────┘
```

- `GET /api/watchlist` — fetch the user's tickers, merged with current cached prices (seeded deterministically if no live price yet).
- `POST /api/watchlist` — validate (uppercase, alphanumeric, ≤10 chars), insert, seed a starting price. `409` on duplicate, `400` on invalid input.
- `DELETE /api/watchlist/{ticker}` — remove a ticker. `404` if absent.

Errors are **typed sentinel errors** (`store.ErrTickerExists`,
`service.ErrTickerEmpty`, …) that handlers classify with `errors.Is` — not
string matching — so status-code mapping stays robust.

## WebSocket flow

```
Client connects → hub registers client → client sends {"action":"subscribe","ticker":"AAPL"}
      │                                          │
   WritePump (server→client, + pings)      hub adds client to ticker's subscriber set
   ReadPump  (client→server, + pongs)            │  and pushes the current price immediately
      │                                          ▼
      └──────────────  price update  ← hub.Broadcast(ticker) delivers ONLY to subscribers
```

- **Subscription-aware:** the hub maps `ticker → set of clients`; a price update is delivered only to clients subscribed to that ticker, not broadcast to everyone.
- **Keepalive:** each connection uses ping/pong with read/write deadlines, so half-open connections are detected and cleaned up.
- **Backpressure:** each client has a buffered send channel; if a slow client's buffer is full, the update is dropped rather than blocking the hub.

Client messages: `{"action":"subscribe"|"unsubscribe","ticker":"AAPL"}`
Server messages: `{"ticker":"AAPL","price":175.50}`

## Simulator flow

The simulator stands in for a third-party price feed. **It is driven by the
watchlist, not a hardcoded ticker list:**

```
every SIMULATE_INTERVAL seconds:
  tickers ← store.GetAllTickers()          # exactly what users are watching
  for each ticker:
    base ← last known price (or a deterministic seed if none yet)
    next ← base ± up to 1%                  # smooth random walk
    priceService.UpdatePrice(ticker, next)  # same central path as the webhook
```

So a ticker you add starts moving within one interval, and the simulator never
emits prices for symbols nobody is watching. Both the simulator and the webhook
funnel through `PriceService.UpdatePrice`, which updates the cache and broadcasts
to the hub — one write path, one source of truth.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `DATABASE_PATH` | `./stocks.db` | SQLite database file path |
| `FRONTEND_ORIGIN` | `*` | CORS allowed origin |
| `DEMO_USER_ID` | `1` | Demo user ID (single-user MVP) |
| `SIMULATE_PRICES` | `true` | Enable the background price simulator |
| `SIMULATE_INTERVAL` | `2` | Simulator tick interval (seconds) |
| `STATIC_DIR` | (empty) | Optional: serve a built frontend from this dir (single-binary mode). Empty = API/WebSocket only |

## Running locally

**Backend** (API + WebSocket on `:8080`):
```bash
go run ./cmd/server
```

**Frontend** (Vite dev server on `:5173`):
```bash
cd frontend
npm install
npm run dev
```

Open http://localhost:5173. In dev, Vite serves the UI and the app talks to the
backend on `:8080`.

**Send a manual price update** (bypasses the simulator):
```bash
curl -X POST http://localhost:8080/api/webhooks/prices \
  -H "Content-Type: application/json" \
  -d '{"ticker":"AAPL","price":180.00}'
```

## Running with Docker Compose

```bash
docker-compose up --build
```

Starts:
- **backend** — API + WebSocket on `:8080`, SQLite persisted to a mounted `./data` volume.
- **frontend** — nginx on `:5173` serving the built assets and reverse-proxying `/api` and `/ws` to the backend (the canonical deployment path).

Open http://localhost:5173.

## Running tests

```bash
go test ./...        # all backend tests
go vet ./...         # static checks
gofmt -l .           # formatting (no output = clean)
```

Coverage includes:
- **Service** — ticker validation, duplicate/empty handling, watchlist + price merging.
- **WebSocket hub** — subscribe/unsubscribe, per-ticker delivery isolation, multi-client fan-out, shutdown.
- **Simulator** — a newly added ticker receives updates; updates are driven by the watchlist (not a fixed list); prices walk from the last known value.
- **API (integration)** — real HTTP + WebSocket end-to-end: subscribe → update → deliver, health/readiness probes, and live `/stats` counters.

CI runs all of the above plus `go build ./...` and the frontend `tsc && vite build` on every push and PR — see [`.github/workflows/ci.yml`](.github/workflows/ci.yml).

## Production-minded features already included

- **Graceful shutdown** — SIGINT/SIGTERM cancels the simulator, drains the hub, and shuts down the HTTP server with a timeout.
- **Health & readiness probes** — `/healthz` (liveness) and `/readyz` (pings the DB); wired into the Docker healthcheck.
- **Structured logging** — `log/slog` JSON logs, plus per-request logging middleware capturing method, path, status, and duration.
- **Observability endpoint** — `/stats` exposes active clients, active subscriptions, and total price updates processed.
- **SQLite reliability** — WAL journal mode, `busy_timeout`, enforced foreign keys, and a tuned connection pool.
- **Correct concurrency** — single-goroutine hub event loop, `RWMutex`-guarded price cache, atomic counters, buffered sends with drop-on-full backpressure.
- **Typed errors** — sentinel errors classified with `errors.Is`, keeping HTTP status mapping robust to message changes.
- **Reproducible builds** — multi-stage Docker builds; `go.sum` committed; CI enforces formatting, vet, tests, and build.

## Interview value — skills demonstrated

| Skill | Where to look |
|-------|---------------|
| **Idiomatic Go** | interface-driven packages, sentinel errors, context propagation, table-driven tests |
| **Clean architecture** | `api → service → store` layering; dependencies point inward through interfaces (`Store`, `Hub`, `PriceCache`, `WatchlistSource`) |
| **WebSockets** | subscription-aware hub, ping/pong keepalive, read/write pumps ([`internal/websocket`](internal/websocket)) |
| **Concurrency** | hub event loop, mutex-protected cache, atomics, non-blocking fan-out — race-free under `-race` |
| **Graceful shutdown** | ordered teardown of goroutines + server in [`cmd/server/main.go`](cmd/server/main.go) |
| **SQLite reliability** | WAL/busy-timeout/FK pragmas and pool tuning in [`internal/store/sqlite.go`](internal/store/sqlite.go) |
| **Testing** | unit + integration tests, real HTTP/WS in tests, deterministic white-box simulator tests |
| **Docker** | multi-stage backend & frontend images, Compose with nginx reverse proxy and a persistent volume |
| **Observability** | structured logs, request logging middleware, `/stats`, health/readiness probes |
| **CI** | GitHub Actions enforcing fmt/vet/test/build for Go and typecheck/build for the frontend |

## Current limitations

These are conscious scope choices for a focused portfolio project, not oversights:

- **Single demo user, no auth** — one hardcoded user; no authentication/authorization or multi-tenancy.
- **In-memory price cache** — prices reset on restart (the watchlist itself persists in SQLite).
- **Single node** — all WebSocket state is in-process; scaling out would need a shared pub/sub layer, intentionally not added here.
- **SQLite** — great for this scale; a write-heavy multi-node deployment would move to PostgreSQL.
- **Open CORS & unauthenticated webhook** — fine for a local demo; production would lock down origins and sign webhook payloads.
- **Unbounded random walk** — the simulator has no mean-reversion, so long runs can drift; cosmetic only.

## License

MIT
