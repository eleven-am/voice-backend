package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type WSAgentConnection struct {
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

func NewWSAgentConnection(ws *websocket.Conn, agentID, ownerID string, logger *slog.Logger) *WSAgentConnection {
	return &WSAgentConnection{
		ws:       ws,
		agentID:  agentID,
		ownerID:  ownerID,
		logger:   logger.With("agent_id", agentID),
		send:     make(chan *GatewayMessage, 128),
		messages: make(chan *GatewayMessage, 128),
		done:     make(chan struct{}),
	}
}

func (c *WSAgentConnection) AgentID() string {
	return c.agentID
}

func (c *WSAgentConnection) SessionID() string {
	return ""
}

func (c *WSAgentConnection) UserID() string {
	return c.ownerID
}

func (c *WSAgentConnection) SetOnline(online bool) {
	c.mu.Lock()
	c.online = online
	c.mu.Unlock()
}

func (c *WSAgentConnection) IsOnline() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.online
}

func (c *WSAgentConnection) Send(_ context.Context, msg *GatewayMessage) error {
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

func (c *WSAgentConnection) Messages() <-chan *GatewayMessage {
	return c.messages
}

func (c *WSAgentConnection) Close() error {
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

func (c *WSAgentConnection) readPump(ctx context.Context, bridge *Bridge) {
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
		select {
		case <-ctx.Done():
			return
		case <-c.done:
			return
		default:
		}

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
			if err := bridge.PublishResponse(ctx, &msg); err != nil {
				c.logger.Error("failed to publish response", "error", err)
			}
		}

		select {
		case c.messages <- &msg:
		case <-ctx.Done():
			return
		default:
			c.logger.Warn("message buffer full, dropping message")
		}
	}
}

func (c *WSAgentConnection) writePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
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

func (h *AgentHandler) handleWebSocket(c echo.Context) error {
	key := GetAPIKey(c)
	agentID := key.OwnerID

	ws, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", "error", err)
		return err
	}

	conn := NewWSAgentConnection(ws, agentID, key.OwnerID, h.logger)

	if err := h.bridge.RegisterAgent(conn); err != nil {
		h.logger.Error("failed to register agent", "error", err)
		_ = ws.Close()
		return nil
	}

	h.logger.Info("agent connected (WebSocket)", "agent_id", agentID, "owner_id", key.OwnerID)

	ctx := c.Request().Context()
	go conn.writePump(ctx)
	conn.readPump(ctx, h.bridge)

	h.bridge.UnregisterAgent(agentID)

	h.logger.Info("agent disconnected (WebSocket)", "agent_id", agentID)
	return nil
}
