# Stock Ticker Watcher

A real-time stock ticker watchlist application with Go backend and React frontend.

## Architecture

### Backend (Go)
- **HTTP API**: RESTful endpoints for watchlist management
- **WebSocket**: Real-time price updates pushed to connected clients
- **SQLite**: Persistent storage for user watchlists
- **In-memory cache**: Latest prices stored in memory for fast access
- **Background ticker**: Simulates third-party price provider updates every 2 seconds

### Frontend (React + Vite)
- **React UI**: Watchlist display with add/remove functionality
- **WebSocket client**: Receives live price updates from backend
- **Vite**: Fast development server with hot reload

### Data Flow
1. Third-party provider → HTTP webhook to backend (`POST /api/webhooks/prices`)
2. Backend updates in-memory price cache
3. Backend broadcasts price update to all WebSocket clients
4. Frontend receives update via WebSocket and updates UI

Alternatively, the background ticker simulates this flow by generating random price updates.

## API Endpoints

- `GET /api/watchlist` - Get user's watchlist
- `POST /api/watchlist` - Add ticker to watchlist (body: `{"ticker": "AAPL"}`)
- `DELETE /api/watchlist/{ticker}` - Remove ticker from watchlist
- `POST /api/webhooks/prices` - Receive price updates from third-party (body: `{"ticker": "AAPL", "price": 175.50}`)
- `WS /ws` - WebSocket endpoint for real-time updates

## Run Instructions

### Prerequisites
- Go 1.21+
- Node.js 18+
- npm or yarn

### Backend Setup

1. Install Go dependencies:
```bash
cd /Users/zhenisduissekov/CascadeProjects/incident-analyzer/stock_ticker_watcher/CascadeProjects/2048
go mod download
```

2. Start the Go server:
```bash
go run main.go
```

The server will start on port 8080 and:
- Create `stocks.db` SQLite database
- Start background price simulation
- Serve frontend from `frontend/dist` (after build)

### Frontend Setup

1. Install Node dependencies:
```bash
cd frontend
npm install
```

2. For development (with hot reload):
```bash
npm run dev
```
This starts Vite dev server on port 5173 with API proxy to backend.

3. For production build:
```bash
npm run build
```
This creates optimized build in `frontend/dist` which the Go server can serve.

### Demo Flow

1. Start backend: `go run main.go`
2. Start frontend dev server: `cd frontend && npm run dev`
3. Open browser to `http://localhost:5173`
4. Add tickers (e.g., AAPL, NVDA, IBM)
5. Watch prices update in real-time every 2 seconds
6. Remove tickers using the Remove button

### Testing Webhook Endpoint

You can manually trigger price updates via curl:

```bash
curl -X POST http://localhost:8080/api/webhooks/prices \
  -H "Content-Type: application/json" \
  -d '{"ticker": "AAPL", "price": 180.00}'
```

This will update the price cache and broadcast to all connected clients.

## Project Structure

```
.
├── main.go              # Go backend server
├── go.mod              # Go dependencies
├── stocks.db           # SQLite database (created on first run)
├── frontend/
│   ├── package.json    # Node dependencies
│   ├── vite.config.ts  # Vite configuration
│   ├── tsconfig.json   # TypeScript config
│   ├── index.html      # HTML entry point
│   └── src/
│       ├── main.tsx    # React entry
│       ├── App.tsx     # Main component
│       ├── App.css     # Styles
│       └── index.css   # Global styles
└── README.md           # This file
```

## MVP Notes

- Single demo user (ID: 1)
- No authentication
- No historical data or charts
- In-memory price cache (resets on server restart)
- Simulated third-party provider via background ticker
- CORS enabled for all origins (for development)
