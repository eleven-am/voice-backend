package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSServer struct {
	auth   *Authenticator
	bridge *Bridge
	logger *slog.Logger
}

func NewWSServer(auth *Authenticator, bridge *Bridge, logger *slog.Logger) *WSServer {
	return &WSServer{
		auth:   auth,
		bridge: bridge,
		logger: logger.With("component", "ws_server"),
	}
}

func (s *WSServer) HandleConnection(c echo.Context) error {
	apiKey := c.QueryParam("api_key")
	if apiKey == "" {
		apiKey = c.Request().Header.Get("X-API-Key")
	}

	if apiKey == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "missing api key")
	}

	key, err := s.auth.ValidateAPIKey(c.Request().Context(), apiKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	}

	agentID := c.QueryParam("agent_id")
	if agentID == "" {
		if key.OwnerType == apikey.OwnerTypeAgent {
			agentID = key.OwnerID
		} else {
			return echo.NewHTTPError(http.StatusBadRequest, "missing agent_id")
		}
	}

	if err := s.auth.ValidateAgentAccess(key, agentID); err != nil {
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	}

	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		s.logger.Error("websocket upgrade failed", "error", err)
		return err
	}

	conn := newWSAgentConnection(ws, agentID, key.OwnerID, s.logger)
	s.bridge.RegisterAgent(conn)

	go conn.writePump()
	conn.readPump(s.bridge)

	return nil
}

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
		online:   true,
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
	defer c.mu.Unlock()
	c.online = online
}

func (c *wsAgentConnection) IsOnline() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.online
}

func (c *wsAgentConnection) Send(ctx context.Context, msg *GatewayMessage) error {
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
	c.online = false
	close(c.done)
	c.mu.Unlock()

	return c.ws.Close()
}

func (c *wsAgentConnection) readPump(bridge *Bridge) {
	defer func() {
		c.Close()
		bridge.UnregisterAgent(c.agentID)
	}()

	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("read error", "error", err)
			}
			return
		}

		var msg GatewayMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			c.logger.Error("unmarshal error", "error", err)
			continue
		}

		msg.AgentID = c.agentID
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now()
		}

		if msg.Type == MessageTypeResponse {
			if err := bridge.PublishResponse(context.Background(), &msg); err != nil {
				c.logger.Error("publish response failed", "error", err)
			}
		}
	}
}

func (c *wsAgentConnection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			c.ws.WriteMessage(websocket.CloseMessage, []byte{})
			return
		case msg := <-c.send:
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))

			data, err := json.Marshal(msg)
			if err != nil {
				c.logger.Error("marshal error", "error", err)
				continue
			}

			if err := c.ws.WriteMessage(websocket.TextMessage, data); err != nil {
				c.logger.Error("write error", "error", err)
				return
			}

		case <-ticker.C:
			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
