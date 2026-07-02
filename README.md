# Stock Ticker Watcher

A production-grade real-time stock ticker watchlist application with a layered Go backend and React frontend.

## Architecture Overview

This project demonstrates clean architecture principles with clear separation of concerns, following idiomatic Go patterns suitable for senior backend engineering interviews.

### System Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           THIRD-PARTY PROVIDER                                │
│                        (Simulated via Background Ticker)                       │
│                                                                              │
│  Every 2 seconds → Random price updates for: AAPL, NVDA, IBM, GOOGL, etc.  │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    │ HTTP Webhook
                                    │ POST /api/webhooks/prices
                                    │ {"ticker": "AAPL", "price": 175.50}
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              GO BACKEND                                      │
│                          Port: 8080                                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────────────────────────────────────────────────────────┐ │
│  │                     HTTP API Layer                                     │ │
│  │  internal/api/                                                        │ │
│  │  ├── handlers.go  - Request handling, validation, response formatting │ │
│  │  └── routes.go    - Route registration, middleware                    │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
│                                    │                                        │
│  ┌──────────────────────────────────────────────────────────────────────┐ │
│  │                     Service Layer                                      │ │
│  │  internal/service/                                                     │ │
│  │  ├── watchlist.go - Watchlist business logic                          │ │
│  │  └── prices.go    - Price caching and broadcasting                     │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
│                                    │                                        │
│  ┌──────────────────────────────────────────────────────────────────────┐ │
│  │                     WebSocket Hub                                      │ │
│  │  internal/websocket/                                                   │ │
│  │  ├── hub.go       - Subscription-aware broadcasting                  │ │
│  │  └── client.go    - Client connection management                      │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
│                                    │                                        │
│  ┌──────────────────────────────────────────────────────────────────────┐ │
│  │                     Data Layer                                         │ │
│  │  internal/store/                                                        │ │
│  │  ├── interface.go  - Store interface definition                       │ │
│  │  └── sqlite.go     - SQLite implementation                            │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
│                                    │                                        │
│  ┌──────────────────────────────────────────────────────────────────────┐ │
│  │                     Background Simulator                               │ │
│  │  internal/simulator/simulator.go                                     │ │
│  └──────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │ WebSocket Connection          │
                    │ ws://localhost:8080/ws        │
                    │                               │
                    ▼                               ▼
┌─────────────────────────────────────┐   ┌─────────────────────────────────────┐
│        BROWSER CLIENT 1             │   │        BROWSER CLIENT 2             │
│   http://localhost:5173             │   │   http://localhost:5173             │
├─────────────────────────────────────┤   ├─────────────────────────────────────┤
│                                     │   │                                     │
│  ┌───────────────────────────────┐  │   │  ┌───────────────────────────────┐  │
│  │      React + Vite Frontend   │  │   │  │      React + Vite Frontend   │  │
│  ├──────────────────────────────┤  │   │  ├──────────────────────────────┤  │
│  │  • Add ticker input           │  │   │  │  • Add ticker input           │  │
│  │  • Watchlist display          │  │   │  │  • Watchlist display          │  │
│  │  • Remove buttons            │  │   │  │  • Remove buttons            │  │
│  │  • Connection status         │  │   │  │  • Connection status         │  │
│  │  • WebSocket client          │  │   │  │  • WebSocket client          │  │
│  └──────────────────────────────┘  │   │  └──────────────────────────────┘  │
│                                     │   │                                     │
│  Receives: {"ticker":"AAPL",        │   │  Receives: {"ticker":"AAPL",        │
│            "price":175.50}          │   │            "price":175.50}          │
│                                     │   │                                     │
│  Updates UI in real-time            │   │  Updates UI in real-time            │
└─────────────────────────────────────┘   └─────────────────────────────────────┘
```

### Project Layout

```
.
├── cmd/
│   └── server/
│       └── main.go              # Application entry point, graceful shutdown
├── internal/
│   ├── api/
│   │   ├── handlers.go          # HTTP request handlers
│   │   └── routes.go            # Route registration
│   ├── config/
│   │   └── config.go            # Environment-based configuration
│   ├── models/
│   │   └── models.go            # Data structures (DTOs, entities)
│   ├── service/
│   │   ├── watchlist.go         # Watchlist business logic
│   │   ├── prices.go            # Price service with caching
│   │   └── watchlist_test.go    # Unit tests
│   ├── store/
│   │   ├── interface.go         # Store interface definition
│   │   └── sqlite.go            # SQLite implementation
│   ├── simulator/
│   │   └── simulator.go         # Background price simulation
│   └── websocket/
│       ├── hub.go               # Subscription-aware WebSocket hub
│       └── client.go            # WebSocket client management
├── pkg/                         # Public packages (if any)
├── frontend/
│   ├── src/
│   │   ├── App.tsx              # Main React component
│   │   ├── App.css              # Component styles
│   │   ├── main.tsx             # React entry point
│   │   └── index.css            # Global styles
│   ├── index.html               # HTML template
│   ├── package.json             # Node dependencies
│   ├── vite.config.ts           # Vite configuration
│   ├── tsconfig.json            # TypeScript config
│   ├── Dockerfile               # Frontend Docker build
│   └── nginx.conf               # Nginx configuration
├── Dockerfile                   # Backend Docker build
├── docker-compose.yml           # Multi-container orchestration
├── go.mod                       # Go module definition
├── go.sum                       # Go dependency checksums
└── README.md                    # This file
```

### Design Principles

**Layered Architecture**: Clear separation between API, service, and data layers. Each layer has a single responsibility and depends only on the layer below it.

**Interface-Based Design**: The store layer is defined as an interface (`store.Store`), enabling easy swapping of implementations (e.g., SQLite → PostgreSQL) without affecting service logic.

**Dependency Injection**: Services receive dependencies through constructors, making testing straightforward and enabling mock implementations.

**Context Propagation**: All handlers and service methods accept `context.Context` for request cancellation, timeouts, and tracing.

**Structured Logging**: Uses `log/slog` for structured, JSON-formatted logs suitable for production environments.

**Graceful Shutdown**: Proper cleanup of HTTP server, WebSocket connections, and background goroutines on SIGTERM/SIGINT.

## Request Flows

### Watchlist Management Flow

```
Client → HTTP Request → API Handler → Service Layer → Store Layer → SQLite
        ↓                    ↓              ↓              ↓
   CORS Middleware    Validation    Business Logic    SQL Query
        ↓                    ↓              ↓              ↓
   Response ← JSON Format ← Watchlist ← Tickers ← Result Set
```

1. **GET /api/watchlist**
   - Handler validates request context
   - Service calls store to fetch tickers for user
   - Service merges with price cache
   - Returns watchlist with current prices

2. **POST /api/watchlist**
   - Handler validates JSON body
   - Service validates ticker format (uppercase, alphanumeric, max 10 chars)
   - Service checks for duplicates
   - Store inserts into database
   - Service initializes price in cache if new
   - Returns created watchlist item

3. **DELETE /api/watchlist/{ticker}**
   - Handler extracts ticker from URL
   - Service validates ticker format
   - Store removes from database
   - Returns success message

### Price Update Flow (Webhook)

```
Third-Party → POST /api/webhooks/prices → API Handler → Price Service
                                                   ↓
                                            Update Cache
                                                   ↓
                                            Broadcast to Hub
                                                   ↓
                                            Send to Subscribers
                                                   ↓
                                            WebSocket Clients
```

1. **POST /api/webhooks/prices**
   - Handler validates JSON body (ticker, price)
   - Price service validates (price > 0)
   - Updates in-memory price cache (thread-safe)
   - Triggers broadcast to WebSocket hub
   - Returns success response

### WebSocket Flow

```
Client → WebSocket Upgrade → Hub Registration → Send Current Prices
              ↓                              ↓
         Client ID Generation          Subscribe to Tickers
              ↓                              ↓
         Read Pump (Client Messages)   Write Pump (Server Messages)
              ↓                              ↓
    Handle Subscribe/Unsubscribe    Send Price Updates
              ↓                              ↓
         Update Subscriptions      Only to Subscribed Clients
```

**Subscription-Aware Broadcasting**:
- Each client maintains a list of subscribed tickers
- Hub maintains mapping: `ticker → set of clients`
- When price updates, only clients subscribed to that ticker receive the message
- Reduces bandwidth and improves scalability vs. broadcast-to-all

**Client Messages**:
```json
// Subscribe to ticker
{"action": "subscribe", "ticker": "AAPL"}

// Unsubscribe from ticker
{"action": "unsubscribe", "ticker": "AAPL"}
```

**Server Messages**:
```json
// Price update
{"ticker": "AAPL", "price": 175.50}
```

## Configuration

Environment variables control application behavior:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | HTTP server port |
| `DATABASE_PATH` | ./stocks.db | SQLite database file path |
| `FRONTEND_ORIGIN` | * | CORS allowed origin |
| `DEMO_USER_ID` | 1 | Demo user ID for MVP |
| `SIMULATE_PRICES` | true | Enable background price simulation |
| `SIMULATE_INTERVAL` | 2 | Simulation interval in seconds |

## Running the Application

### Local Development

**Backend**:
```bash
go run ./cmd/server
```

**Frontend**:
```bash
cd frontend
npm install
npm run dev
```

Access at http://localhost:5173

### Docker

**Build and run with docker-compose**:
```bash
docker-compose up --build
```

This starts:
- Backend on port 8080
- Frontend on port 5173
- Persistent data volume for SQLite database

### Manual Webhook Test

```bash
curl -X POST http://localhost:8080/api/webhooks/prices \
  -H "Content-Type: application/json" \
  -d '{"ticker": "AAPL", "price": 180.00}'
```

## Testing

Run unit tests:
```bash
go test ./internal/service/...
```

Tests cover:
- Ticker validation (format, length, characters)
- Duplicate detection
- Empty ticker handling
- Watchlist retrieval with price merging

## Scaling Discussion

### Current Limitations

1. **Single Node**: All WebSocket connections handled by one process
2. **In-Memory Cache**: Prices lost on restart, no persistence
3. **SQLite**: Not suitable for high-concurrency writes
4. **No Authentication**: Single demo user

### Migration Path to Production Scale

**Phase 1: Redis Pub/Sub**
- Replace in-memory price cache with Redis
- Use Redis Pub/Sub for cross-node price broadcasting
- Benefits: Persistence, distributed caching, horizontal scaling

**Phase 2: PostgreSQL**
- Migrate from SQLite to PostgreSQL
- Better concurrency, replication support
- Use connection pooling (pgxpool)

**Phase 3: Message Queue (Kafka/NATS)**
- Replace webhook with message queue consumer
- Decouple price ingestion from broadcasting
- Enable replay of price updates

**Phase 4: Multiple WebSocket Nodes**
- Deploy multiple backend instances behind load balancer
- Use Redis Pub/Sub or message queue for cross-node coordination
- Sticky sessions or connection migration for WebSocket

**Phase 5: Authentication & Multi-Tenancy**
- Add JWT-based authentication
- User-specific watchlists
- Row-level security in database
- Rate limiting per user

### Architecture Evolution

```
Current:
[Client] → [Single Go Server] → [SQLite] → [In-Memory Cache]

Scaled:
[Client] → [Load Balancer] → [Go Server 1] → [PostgreSQL]
              ↓                    [Go Server 2] → [Redis Pub/Sub]
              ↓                    [Go Server 3] → [Kafka]
              ↓
         [NATS/Kafka] ← [Price Provider]
```

## Future Improvements

1. **Historical Data**: Store price history in time-series database (TimescaleDB)
2. **Charts**: Integrate charting library (Recharts, Chart.js)
3. **Alerts**: Price threshold notifications via WebSocket
4. **Persistence**: Redis for price cache with TTL
5. **Metrics**: Prometheus metrics for monitoring
6. **Tracing**: OpenTelemetry integration
7. **Rate Limiting**: Per-user API rate limiting
8. **Webhook Authentication**: HMAC signature verification
9. **GraphQL**: Alternative to REST API
10. **gRPC**: High-performance inter-service communication

## MVP Notes

- Single demo user (ID: 1) for simplicity
- No authentication or authorization
- No historical data persistence
- In-memory price cache (resets on restart)
- Simulated third-party provider via background ticker
- CORS enabled for all origins (configure for production)
- SQLite for simplicity (migrate to PostgreSQL for production)

## License

MIT
