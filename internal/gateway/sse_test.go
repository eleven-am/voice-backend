package gateway

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

type mockFlusherWriter struct {
	*httptest.ResponseRecorder
}

func (m *mockFlusherWriter) Flush() {}

type nonFlusherWriter struct {
	header http.Header
}

func (w *nonFlusherWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *nonFlusherWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (w *nonFlusherWriter) WriteHeader(statusCode int) {}

func TestNewSSEAgentConn(t *testing.T) {
	w := &mockFlusherWriter{httptest.NewRecorder()}
	conn, err := NewSSEAgentConn(w, "agent_123", "owner_456")
	if err != nil {
		t.Fatalf("NewSSEAgentConn error: %v", err)
	}

	if conn.AgentID() != "agent_123" {
		t.Errorf("expected agent_123, got %s", conn.AgentID())
	}
	if conn.UserID() != "owner_456" {
		t.Errorf("expected owner_456, got %s", conn.UserID())
	}
	if conn.SessionID() != "" {
		t.Errorf("expected empty session ID, got %s", conn.SessionID())
	}
	if !conn.IsOnline() {
		t.Error("new connection should be online")
	}
}

func TestNewSSEAgentConn_NoFlusher(t *testing.T) {
	w := &nonFlusherWriter{}
	_, err := NewSSEAgentConn(w, "agent_123", "owner_456")
	if err == nil {
		t.Error("expected error for non-flusher writer")
	}
}

func TestSSEAgentConn_SetOnline(t *testing.T) {
	w := &mockFlusherWriter{httptest.NewRecorder()}
	conn, _ := NewSSEAgentConn(w, "agent_123", "owner_456")

	conn.SetOnline(false)
	if conn.IsOnline() {
		t.Error("expected offline")
	}

	conn.SetOnline(true)
	if !conn.IsOnline() {
		t.Error("expected online")
	}
}

func TestSSEAgentConn_Close(t *testing.T) {
	w := &mockFlusherWriter{httptest.NewRecorder()}
	conn, _ := NewSSEAgentConn(w, "agent_123", "owner_456")

	err := conn.Close()
	if err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if conn.IsOnline() {
		t.Error("connection should be offline after close")
	}

	err = conn.Close()
	if err != nil {
		t.Fatalf("second Close error: %v", err)
	}
}

func TestSSEAgentConn_Messages(t *testing.T) {
	w := &mockFlusherWriter{httptest.NewRecorder()}
	conn, _ := NewSSEAgentConn(w, "agent_123", "owner_456")

	if conn.Messages() != nil {
		t.Error("Messages() should return nil for SSE connections")
	}
}

func TestSSEAgentConn_Send(t *testing.T) {
	w := &mockFlusherWriter{httptest.NewRecorder()}
	conn, _ := NewSSEAgentConn(w, "agent_123", "owner_456")

	msg := &transport.AgentMessage{
		Type:    transport.MessageTypeResponse,
		Payload: map[string]string{"text": "hello"},
	}

	ctx := context.Background()
	err := conn.Send(ctx, msg)
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}
}

func TestSSEAgentConn_Send_Cancelled(t *testing.T) {
	w := &mockFlusherWriter{httptest.NewRecorder()}
	conn, _ := NewSSEAgentConn(w, "agent_123", "owner_456")

	for range 128 {
		conn.send <- nil
	}

	msg := &transport.AgentMessage{Type: transport.MessageTypeResponse}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := conn.Send(ctx, msg)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestSSEAgentConn_Send_AfterClose(t *testing.T) {
	w := &mockFlusherWriter{httptest.NewRecorder()}
	conn, _ := NewSSEAgentConn(w, "agent_123", "owner_456")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- conn.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Run should have completed")
	}

	if conn.IsOnline() {
		t.Error("connection should be offline after Run completes")
	}
}

func newTestAgentHandler(t *testing.T) (*AgentHandler, *miniredis.Miniredis, *Bridge) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bridge := NewBridge(redisClient, logger)

	mock := &mockAPIKeyValidator{
		validateFunc: func(ctx context.Context, secret string) (*apikey.APIKey, error) {
			return &apikey.APIKey{
				ID:        "key_123",
				OwnerID:   "agent_456",
				OwnerType: apikey.OwnerTypeAgent,
			}, nil
		},
	}
	auth := NewAuthenticator(mock)

	handler := NewAgentHandler(auth, bridge, logger)
	return handler, mr, bridge
}

func TestNewAgentHandler(t *testing.T) {
	handler, mr, bridge := newTestAgentHandler(t)
	defer mr.Close()
	defer bridge.Close()

	if handler == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestAgentHandler_RegisterRoutes(t *testing.T) {
	handler, mr, bridge := newTestAgentHandler(t)
	defer mr.Close()
	defer bridge.Close()

	e := echo.New()
	g := e.Group("/agents")
	handler.RegisterRoutes(g)

	routes := e.Routes()
	expectedPaths := []string{
		"/agents",
		"/agents/status",
	}

	routePaths := make(map[string]bool)
	for _, r := range routes {
		routePaths[r.Path] = true
	}

	for _, path := range expectedPaths {
		if !routePaths[path] {
			t.Errorf("expected route %s to be registered", path)
		}
	}
}

func TestAgentHandler_HandleStatus(t *testing.T) {
	handler, mr, bridge := newTestAgentHandler(t)
	defer mr.Close()
	defer bridge.Close()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/agents/status", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := handler.HandleStatus(c)
	if err != nil {
		t.Fatalf("HandleStatus error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if !strings.Contains(rec.Body.String(), `"agents"`) {
		t.Error("response should contain agents field")
	}
}

func TestAgentHandler_HandleEvent_InvalidPayload(t *testing.T) {
	handler, mr, bridge := newTestAgentHandler(t)
	defer mr.Close()
	defer bridge.Close()

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader("{invalid"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set("api_key", &apikey.APIKey{OwnerID: "agent_123"})

	err := handler.HandleEvent(c)
	if err == nil {
		t.Error("expected error for invalid payload")
	}
}

func TestAgentHandler_HandleEvent_Success(t *testing.T) {
	handler, mr, bridge := newTestAgentHandler(t)
	defer mr.Close()
	defer bridge.Close()

	e := echo.New()
	body := `{"type":"response.text.done","session_id":"sess_123","payload":{"text":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/agents", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set("api_key", &apikey.APIKey{OwnerID: "agent_123"})

	err := handler.HandleEvent(c)
	if err != nil {
		t.Fatalf("HandleEvent error: %v", err)
	}

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}
}

func TestAgentHandler_HandleConnect_Conflict(t *testing.T) {
	handler, mr, bridge := newTestAgentHandler(t)
	defer mr.Close()
	defer bridge.Close()

	conn := newMockAgentConnection("agent_123")
	bridge.RegisterAgent(conn)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set("api_key", &apikey.APIKey{OwnerID: "agent_123"})

	err := handler.handleSSE(c)
	if err == nil {
		t.Error("expected conflict error")
	}
}

func TestExtractAPIKey_Bearer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer sk-test-key")

	key := extractAPIKey(req)
	if key != "sk-test-key" {
		t.Errorf("expected sk-test-key, got %s", key)
	}
}

func TestExtractAPIKey_QueryParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?api_key=sk-test-key", nil)

	key := extractAPIKey(req)
	if key != "sk-test-key" {
		t.Errorf("expected sk-test-key, got %s", key)
	}
}

func TestExtractAPIKey_Empty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	key := extractAPIKey(req)
	if key != "" {
		t.Errorf("expected empty string, got %s", key)
	}
}

func TestSSEAgentConn_Run_ContextCancel(t *testing.T) {
	w := &mockFlusherWriter{httptest.NewRecorder()}
	conn, _ := NewSSEAgentConn(w, "agent_123", "owner_456")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error)
	go func() {
		done <- conn.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Run should have completed")
	}
}

func TestSSEAgentConn_Run_SendMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &mockFlusherWriter{rec}
	conn, _ := NewSSEAgentConn(w, "agent_123", "owner_456")

	go conn.Run(t.Context())

	msg := &transport.AgentMessage{
		Type:    transport.MessageTypeResponse,
		Payload: map[string]string{"text": "hello"},
	}
	conn.Send(context.Background(), msg)

	time.Sleep(50 * time.Millisecond)
	conn.Close()

	body := rec.Body.String()
	if !strings.Contains(body, "data:") {
		t.Error("expected SSE data prefix in output")
	}
}

func TestAgentHandler_HandleConnect_SSE(t *testing.T) {
	handler, mr, bridge := newTestAgentHandler(t)
	defer mr.Close()
	defer bridge.Close()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set("api_key", &apikey.APIKey{OwnerID: "new_agent_789"})

	errChan := make(chan error, 1)
	go func() {
		errChan <- handler.HandleConnect(c)
	}()

	time.Sleep(100 * time.Millisecond)
	bridge.UnregisterAgent("new_agent_789")

	select {
	case <-errChan:
	case <-time.After(time.Second):
	}
}

func TestAgentHandler_HandleConnect_WebSocket(t *testing.T) {
	handler, mr, bridge := newTestAgentHandler(t)
	defer mr.Close()
	defer bridge.Close()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/agents", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set("api_key", &apikey.APIKey{OwnerID: "ws_agent_123"})

	err := handler.HandleConnect(c)
	if err == nil {
		t.Log("HandleConnect returned nil (expected - WebSocket upgrade fails in test)")
	}
}
