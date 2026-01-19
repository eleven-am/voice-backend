package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/transport"
)

const sseKeepAliveInterval = 30 * time.Second

type SSEAgentConn struct {
	writer    http.ResponseWriter
	flusher   http.Flusher
	agentID   string
	ownerID   string
	send      chan *transport.AgentMessage
	done      chan struct{}
	closeOnce sync.Once

	mu        sync.RWMutex
	connected bool
}

func NewSSEAgentConn(w http.ResponseWriter, agentID, ownerID string) (*SSEAgentConn, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, http.ErrNotSupported
	}

	return &SSEAgentConn{
		writer:    w,
		flusher:   flusher,
		agentID:   agentID,
		ownerID:   ownerID,
		send:      make(chan *transport.AgentMessage, 128),
		done:      make(chan struct{}),
		connected: true,
	}, nil
}

func (c *SSEAgentConn) AgentID() string {
	return c.agentID
}

func (c *SSEAgentConn) SessionID() string {
	return ""
}

func (c *SSEAgentConn) UserID() string {
	return c.ownerID
}

func (c *SSEAgentConn) IsOnline() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *SSEAgentConn) SetOnline(online bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connected = online
}

func (c *SSEAgentConn) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()

		close(c.done)
		close(c.send)
	})
	return nil
}

func (c *SSEAgentConn) Send(ctx context.Context, msg *GatewayMessage) error {
	select {
	case c.send <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return http.ErrServerClosed
	}
}

func (c *SSEAgentConn) Messages() <-chan *GatewayMessage {
	return nil
}

func (c *SSEAgentConn) Run(ctx context.Context) error {
	ticker := time.NewTicker(sseKeepAliveInterval)
	defer ticker.Stop()
	defer func() { _ = c.Close() }()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return nil
			}
			if err := c.writeMessage(msg); err != nil {
				return err
			}
		case <-ticker.C:
			if err := c.writeKeepAlive(); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return nil
		}
	}
}

func (c *SSEAgentConn) writeMessage(msg *transport.AgentMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = c.writer.Write([]byte("data: "))
	if err != nil {
		return err
	}

	_, err = c.writer.Write(data)
	if err != nil {
		return err
	}

	_, err = c.writer.Write([]byte("\n\n"))
	if err != nil {
		return err
	}

	c.flusher.Flush()
	return nil
}

func (c *SSEAgentConn) writeKeepAlive() error {
	_, err := c.writer.Write([]byte(":keepalive\n\n"))
	if err != nil {
		return err
	}
	c.flusher.Flush()
	return nil
}
