package websocket

import (
	"encoding/json"
	"log/slog"
	"time"

	"stock-ticker-watcher/internal/models"

	"github.com/gorilla/websocket"
)

const (
	// writeWait is the time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// pongWait is how long we wait for a pong before considering the peer dead.
	pongWait = 60 * time.Second
	// pingPeriod is how often we send pings; must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// maxMessageSize bounds inbound client messages (subscribe/unsubscribe).
	maxMessageSize = 512
)

// Client represents a WebSocket client
type Client struct {
	ID     string
	Conn   *websocket.Conn
	Send   chan []byte
	Hub    *Hub
	Logger *slog.Logger
}

// NewClient creates a new WebSocket client
func NewClient(id string, conn *websocket.Conn, hub *Hub, logger *slog.Logger) *Client {
	return &Client{
		ID:     id,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		Hub:    hub,
		Logger: logger,
	}
}

// ReadPump reads messages from the WebSocket connection. It also drives
// keepalive: a read deadline plus a pong handler that extends the deadline
// on every pong, so a half-open connection eventually fails ReadMessage and
// the client is unregistered.
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.Logger.Error("WebSocket read error", "client_id", c.ID, "error", err)
			}
			break
		}

		// Handle client messages (e.g., subscribe/unsubscribe)
		c.handleMessage(message)
	}
}

// WritePump writes messages to the WebSocket connection and sends periodic
// pings to keep the connection alive and detect dead peers. When Send is
// closed by the hub (on unregister), it sends a close frame and exits.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel.
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				c.Logger.Error("WebSocket write error", "client_id", c.ID, "error", err)
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.Logger.Error("WebSocket ping error", "client_id", c.ID, "error", err)
				return
			}
		}
	}
}

// handleMessage processes incoming messages from the client
func (c *Client) handleMessage(message []byte) {
	var msg struct {
		Action string `json:"action"`
		Ticker string `json:"ticker"`
	}

	if err := json.Unmarshal(message, &msg); err != nil {
		c.Logger.Error("Failed to parse message", "client_id", c.ID, "error", err)
		return
	}

	switch msg.Action {
	case "subscribe":
		c.Hub.Subscribe(c, msg.Ticker)
	case "unsubscribe":
		c.Hub.Unsubscribe(c, msg.Ticker)
	default:
		c.Logger.Warn("Unknown action", "client_id", c.ID, "action", msg.Action)
	}
}

// SendPriceUpdate sends a price update to the client
func (c *Client) SendPriceUpdate(update models.PriceUpdate) error {
	data, err := json.Marshal(update)
	if err != nil {
		return err
	}

	select {
	case c.Send <- data:
		return nil
	default:
		return nil // Drop message if buffer is full
	}
}
