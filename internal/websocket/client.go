package websocket

import (
	"encoding/json"
	"log/slog"

	"github.com/gorilla/websocket"
	"stock-ticker-watcher/internal/models"
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

// ReadPump reads messages from the WebSocket connection
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()

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

// WritePump writes messages to the WebSocket connection
func (c *Client) WritePump() {
	defer c.Conn.Close()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				c.Logger.Error("WebSocket write error", "client_id", c.ID, "error", err)
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
