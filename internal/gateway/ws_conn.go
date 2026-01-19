package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024
)

type wsAgentConnection struct {
	ws       *websocket.Conn
	agentID  string
	ownerID  string
	logger   *slog.Logger
	send     chan *GatewayMessage
	messages chan *GatewayMessage
	online   bool
	mu       sync.RWMutex
	closed   bool
	done     chan struct{}
}

func newWSAgentConnection(ws *websocket.Conn, agentID, ownerID string, logger *slog.Logger) *wsAgentConnection {
	return &wsAgentConnection{
		ws:       ws,
		agentID:  agentID,
		ownerID:  ownerID,
		logger:   logger.With("agent_id", agentID),
		send:     make(chan *GatewayMessage, 256),
		messages: make(chan *GatewayMessage, 256),
		done:     make(chan struct{}),
	}
}

func (c *wsAgentConnection) AgentID() string {
	return c.agentID
}

func (c *wsAgentConnection) SessionID() string {
	return ""
}

func (c *wsAgentConnection) UserID() string {
	return c.ownerID
}

func (c *wsAgentConnection) SetOnline(online bool) {
	c.mu.Lock()
	c.online = online
	c.mu.Unlock()
}

func (c *wsAgentConnection) IsOnline() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.online
}

func (c *wsAgentConnection) Send(_ context.Context, msg *GatewayMessage) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	select {
	case c.send <- msg:
		return nil
	default:
		c.logger.Warn("send buffer full, dropping message")
		return nil
	}
}

func (c *wsAgentConnection) Messages() <-chan *GatewayMessage {
	return c.messages
}

func (c *wsAgentConnection) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	close(c.done)
	c.mu.Unlock()

	close(c.send)
	return c.ws.Close()
}

func (c *wsAgentConnection) readPump(bridge *Bridge) {
	defer func() {
		c.Close()
	}()

	c.ws.SetReadLimit(maxMessageSize)
	_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("websocket read error", "error", err)
			}
			return
		}

		var msg GatewayMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.logger.Error("failed to unmarshal message", "error", err)
			continue
		}

		msg.AgentID = c.agentID

		if msg.Type == MessageTypeResponse && msg.SessionID != "" {
			if err := bridge.PublishResponse(context.Background(), &msg); err != nil {
				c.logger.Error("failed to publish response", "error", err)
			}
		}

		select {
		case c.messages <- &msg:
		default:
			c.logger.Warn("message buffer full, dropping message")
		}
	}
}

func (c *wsAgentConnection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			data, err := json.Marshal(msg)
			if err != nil {
				c.logger.Error("failed to marshal message", "error", err)
				continue
			}

			if err := c.ws.WriteMessage(websocket.TextMessage, data); err != nil {
				c.logger.Error("websocket write error", "error", err)
				return
			}

		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.done:
			return
		}
	}
}
