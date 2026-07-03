# Demo Walkthrough (~2 minutes)

A guided tour of Stock Ticker Watcher: run it, add a ticker, watch prices stream
live, remove it, push a manual price, and check the operational endpoints.

> Prerequisites: Go 1.21+ and Node 18+. (Or skip to [Docker](#option-b-docker-compose).)

---

## 1. Run it locally

**Option A — Go + Vite (two terminals)**

Terminal 1 — backend (API + WebSocket on `:8080`):
```bash
go run ./cmd/server
```

Terminal 2 — frontend (Vite dev server on `:5173`):
```bash
cd frontend
npm install
npm run dev
```

Open **http://localhost:5173**.

<a name="option-b-docker-compose"></a>
**Option B — Docker Compose (one command)**
```bash
docker-compose up --build
```
Then open **http://localhost:5173** (nginx serves the UI and proxies `/api` + `/ws`).

---

## 2. Add a ticker and watch live updates

In the UI:
1. Type a symbol — e.g. `AAPL` — and click **Add**.
2. It appears in your watchlist with a starting price.
3. Within a couple of seconds the price begins to **move on its own** — the
   built-in simulator random-walks every watched ticker and pushes updates over
   the WebSocket. No refresh needed.

Try adding a few more (`NVDA`, `TSLA`, or anything alphanumeric up to 10 chars).
Each starts streaming as soon as it's added — the simulator is driven by *your*
watchlist, not a fixed list.

> Open a second browser tab to see updates arrive in both simultaneously —
> that's the subscription-aware hub fanning out to all subscribed clients.

---

## 3. Remove a ticker

Click **Remove** next to any ticker. It disappears from the watchlist and stops
receiving updates immediately.

---

## 4. Push a price manually (webhook)

The webhook is the same code path a real third-party feed would use. With a
ticker (say `AAPL`) in your watchlist and its tab open, run:

```bash
curl -s -X POST localhost:8080/api/webhooks/prices \
  -H 'Content-Type: application/json' \
  -d '{"ticker":"AAPL","price":123.45}'
# → {"message":"Price updated"}
```

Watch the UI: `AAPL` jumps to **123.45** instantly, then the simulator resumes
walking from there. This proves webhook and simulator share one central update
path (`PriceService.UpdatePrice` → cache → broadcast).

Validation is enforced:
```bash
curl -s -X POST localhost:8080/api/webhooks/prices \
  -H 'Content-Type: application/json' -d '{"ticker":"AAPL","price":-5}'
# → {"error":"price must be positive"}   (HTTP 400)
```

---

## 5. Inspect via the REST API

```bash
# Current watchlist with prices
curl -s localhost:8080/api/watchlist
# → [{"ticker":"AAPL","price":123.45}, ...]

# Add via API (instead of the UI)
curl -s -X POST localhost:8080/api/watchlist \
  -H 'Content-Type: application/json' -d '{"ticker":"MSFT"}'
# → {"ticker":"MSFT","price":140}          (HTTP 201)

# Adding a duplicate is rejected
curl -s -o /dev/null -w '%{http_code}\n' -X POST localhost:8080/api/watchlist \
  -H 'Content-Type: application/json' -d '{"ticker":"MSFT"}'
# → 409

# Remove via API
curl -s -X DELETE localhost:8080/api/watchlist/MSFT
# → {"message":"Ticker removed"}
```

---

## 6. Operational endpoints

```bash
# Liveness — is the process up?
curl -s localhost:8080/healthz
# → {"status":"ok"}

# Readiness — is the DB reachable? (used by the Docker healthcheck)
curl -s localhost:8080/readyz
# → {"status":"ready"}

# Runtime counters — moves as you connect clients and prices update
curl -s localhost:8080/stats
# → {"active_clients":1,"active_subscriptions":2,"price_updates_processed":57}
```

Load the UI in a browser first, then hit `/stats` again — `active_clients` and
`active_subscriptions` reflect the live WebSocket connections.

---

## 7. (Optional) Poke the WebSocket directly

Using [`websocat`](https://github.com/vi/websocat):
```bash
websocat ws://localhost:8080/ws
# then paste:
{"action":"subscribe","ticker":"AAPL"}
# → you'll immediately get the current price, then a stream of updates:
# {"ticker":"AAPL","price":175.42}
# {"ticker":"AAPL","price":176.01}
```

---

## What just happened

- The **watchlist** persists in SQLite; **prices** live in an in-memory cache.
- The **simulator** walks prices for exactly the tickers being watched and calls
  the same `UpdatePrice` path as the webhook.
- The **hub** delivers each update only to clients subscribed to that ticker.
- Everything shuts down cleanly on `Ctrl-C` (graceful shutdown of the simulator,
  hub, and HTTP server).

For architecture, request/WebSocket flows, and the skills-to-code map, see the
[README](README.md).
