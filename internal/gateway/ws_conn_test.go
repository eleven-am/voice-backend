package gateway

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

func newTestWSConnection(t *testing.T, ws *websocket.Conn) *WSAgentConnection {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewWSAgentConnection(ws, "agent_123", "owner_456", logger)
}

func TestNewWSAgentConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer ws.Close()

	conn := newTestWSConnection(t, ws)

	if conn.AgentID() != "agent_123" {
		t.Errorf("expected agent_123, got %s", conn.AgentID())
	}
	if conn.UserID() != "owner_456" {
		t.Errorf("expected owner_456, got %s", conn.UserID())
	}
	if conn.SessionID() != "" {
		t.Errorf("expected empty session ID, got %s", conn.SessionID())
	}
}

func TestWSAgentConnection_SetOnline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer ws.Close()

	conn := newTestWSConnection(t, ws)

	if conn.IsOnline() {
		t.Error("new connection should not be online by default")
	}

	conn.SetOnline(true)
	if !conn.IsOnline() {
		t.Error("connection should be online after SetOnline(true)")
	}

	conn.SetOnline(false)
	if conn.IsOnline() {
		t.Error("connection should be offline after SetOnline(false)")
	}
}

func TestWSAgentConnection_Messages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer ws.Close()

	conn := newTestWSConnection(t, ws)

	messages := conn.Messages()
	if messages == nil {
		t.Error("Messages() should not return nil")
	}
}

func TestWSAgentConnection_Send(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer ws.Close()

	conn := newTestWSConnection(t, ws)

	msg := &GatewayMessage{
		Type:    MessageTypeResponse,
		Payload: map[string]string{"text": "hello"},
	}

	ctx := context.Background()
	err = conn.Send(ctx, msg)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	select {
	case sent := <-conn.send:
		if sent.Type != MessageTypeResponse {
			t.Errorf("expected MessageTypeResponse, got %v", sent.Type)
		}
	case <-time.After(time.Second):
		t.Error("message should be in send channel")
	}
}

func TestWSAgentConnection_Send_Closed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	conn := newTestWSConnection(t, ws)
	conn.Close()

	msg := &GatewayMessage{Type: MessageTypeResponse}
	err = conn.Send(context.Background(), msg)
	if err != nil {
		t.Errorf("Send on closed connection should return nil, got %v", err)
	}
}

func TestWSAgentConnection_Close(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	conn := newTestWSConnection(t, ws)
	conn.SetOnline(true)

	err = conn.Close()
	if err != nil {
		t.Fatalf("Close error: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Fatalf("second Close should not error: %v", err)
	}
}

func TestWSAgentConnection_Send_BufferFull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	defer ws.Close()

	conn := newTestWSConnection(t, ws)

	for i := 0; i < 128; i++ {
		conn.Send(context.Background(), &GatewayMessage{Type: MessageTypeResponse})
	}

	err = conn.Send(context.Background(), &GatewayMessage{Type: MessageTypeResponse})
	if err != nil {
		t.Errorf("Send with full buffer should return nil (drop), got %v", err)
	}
}

func TestWSAgentConnection_ReadWritePump(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bridge := NewBridge(redisClient, logger)
	defer bridge.Close()

	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		conn := NewWSAgentConnection(ws, "agent_test", "owner_test", logger)
		ctx, cancel := context.WithCancel(context.Background())

		go conn.writePump(ctx)
		go func() {
			conn.readPump(ctx, bridge)
			close(serverDone)
		}()

		time.Sleep(50 * time.Millisecond)

		msg := &GatewayMessage{
			Type:    MessageTypeResponse,
			Payload: map[string]string{"test": "data"},
		}
		conn.Send(ctx, msg)

		time.Sleep(100 * time.Millisecond)
		cancel()
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:]
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}

	ws.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err = ws.ReadMessage()
	if err != nil {
		if !websocket.IsCloseError(err, websocket.CloseAbnormalClosure, websocket.CloseGoingAway) {
			_ = err
		}
	}

	ws.Close()

	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Error("server goroutines should have completed")
	}
}
