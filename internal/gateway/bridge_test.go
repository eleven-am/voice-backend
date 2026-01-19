package gateway

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/redis/go-redis/v9"
)

type mockAgentConnection struct {
	agentID   string
	sessionID string
	userID    string
	online    bool
	messages  chan *GatewayMessage
	sendErr   error
	mu        sync.Mutex
}

func newMockAgentConnection(agentID string) *mockAgentConnection {
	return &mockAgentConnection{
		agentID:  agentID,
		online:   true,
		messages: make(chan *GatewayMessage, 10),
	}
}

func (m *mockAgentConnection) Send(ctx context.Context, msg *GatewayMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	select {
	case m.messages <- msg:
	default:
	}
	return nil
}

func (m *mockAgentConnection) Messages() <-chan *GatewayMessage {
	return m.messages
}

func (m *mockAgentConnection) SessionID() string {
	return m.sessionID
}

func (m *mockAgentConnection) UserID() string {
	return m.userID
}

func (m *mockAgentConnection) Close() error {
	close(m.messages)
	return nil
}

func (m *mockAgentConnection) AgentID() string {
	return m.agentID
}

func (m *mockAgentConnection) SetOnline(online bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.online = online
}

func (m *mockAgentConnection) IsOnline() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.online
}

func newTestBridge(t *testing.T) (*Bridge, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bridge := NewBridge(redisClient, logger)

	return bridge, mr
}

func TestNewBridge(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	if bridge == nil {
		t.Fatal("bridge should not be nil")
	}
	if bridge.agentConns == nil {
		t.Error("agentConns should be initialized")
	}
	if bridge.sessionSubs == nil {
		t.Error("sessionSubs should be initialized")
	}
}

func TestBridge_RegisterAgent(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	conn := newMockAgentConnection("agent_123")
	err := bridge.RegisterAgent(conn)
	if err != nil {
		t.Fatalf("RegisterAgent error: %v", err)
	}

	if !bridge.IsOnline("agent_123") {
		t.Error("agent should be online after registration")
	}
}

func TestBridge_RegisterAgent_AlreadyConnected(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	conn1 := newMockAgentConnection("agent_123")
	conn2 := newMockAgentConnection("agent_123")

	bridge.RegisterAgent(conn1)

	err := bridge.RegisterAgent(conn2)
	if err != ErrAgentAlreadyConnected {
		t.Errorf("expected ErrAgentAlreadyConnected, got %v", err)
	}
}

func TestBridge_UnregisterAgent(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	conn := newMockAgentConnection("agent_123")
	bridge.RegisterAgent(conn)

	if !bridge.IsOnline("agent_123") {
		t.Error("agent should be online")
	}

	bridge.UnregisterAgent("agent_123")

	if bridge.IsOnline("agent_123") {
		t.Error("agent should be offline after unregistration")
	}
}

func TestBridge_UnregisterAgent_NotExists(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	bridge.UnregisterAgent("nonexistent")
}

func TestBridge_GetAgent(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	conn := newMockAgentConnection("agent_123")
	bridge.RegisterAgent(conn)

	retrieved, ok := bridge.GetAgent("agent_123")
	if !ok {
		t.Error("agent should be found")
	}
	if retrieved.AgentID() != "agent_123" {
		t.Errorf("expected agent_123, got %s", retrieved.AgentID())
	}
}

func TestBridge_GetAgent_NotFound(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	_, ok := bridge.GetAgent("nonexistent")
	if ok {
		t.Error("agent should not be found")
	}
}

func TestBridge_GetAgent_Offline(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	conn := newMockAgentConnection("agent_123")
	bridge.RegisterAgent(conn)
	conn.SetOnline(false)

	_, ok := bridge.GetAgent("agent_123")
	if ok {
		t.Error("offline agent should not be found")
	}
}

func TestBridge_IsOnline(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	if bridge.IsOnline("nonexistent") {
		t.Error("nonexistent agent should not be online")
	}

	conn := newMockAgentConnection("agent_123")
	bridge.RegisterAgent(conn)

	if !bridge.IsOnline("agent_123") {
		t.Error("registered agent should be online")
	}
}

func TestBridge_ListAgents(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	agents := bridge.ListAgents()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}

	conn1 := newMockAgentConnection("agent_1")
	conn2 := newMockAgentConnection("agent_2")
	bridge.RegisterAgent(conn1)
	bridge.RegisterAgent(conn2)

	agents = bridge.ListAgents()
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}

	conn1.SetOnline(false)
	agents = bridge.ListAgents()
	if len(agents) != 1 {
		t.Errorf("expected 1 agent after offline, got %d", len(agents))
	}
}

func TestBridge_SetResponseHandler(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	handler := func(sessionID string, msg *transport.AgentMessage) {
		_ = sessionID
		_ = msg
	}

	bridge.SetResponseHandler(handler)

	bridge.mu.RLock()
	if bridge.responseHandler == nil {
		t.Error("response handler should be set")
	}
	bridge.mu.RUnlock()
}

func TestBridge_SubscribeToSession(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	err := bridge.SubscribeToSession("session_123")
	if err != nil {
		t.Fatalf("SubscribeToSession error: %v", err)
	}

	if bridge.SessionSubCount() != 1 {
		t.Errorf("expected 1 session sub, got %d", bridge.SessionSubCount())
	}

	err = bridge.SubscribeToSession("session_123")
	if err != nil {
		t.Fatalf("duplicate subscribe should not error: %v", err)
	}

	if bridge.SessionSubCount() != 1 {
		t.Errorf("expected still 1 session sub, got %d", bridge.SessionSubCount())
	}
}

func TestBridge_UnsubscribeFromSession(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	bridge.SubscribeToSession("session_123")
	if bridge.SessionSubCount() != 1 {
		t.Fatal("expected 1 session sub")
	}

	bridge.UnsubscribeFromSession("session_123")
	time.Sleep(10 * time.Millisecond)

	if bridge.SessionSubCount() != 0 {
		t.Errorf("expected 0 session subs, got %d", bridge.SessionSubCount())
	}
}

func TestBridge_RefreshSessionSubscription(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	bridge.SubscribeToSession("session_123")
	time.Sleep(10 * time.Millisecond)

	bridge.RefreshSessionSubscription("session_123")

	bridge.mu.RLock()
	sub := bridge.sessionSubs["session_123"]
	bridge.mu.RUnlock()

	if sub == nil {
		t.Fatal("subscription should exist")
	}
	if time.Since(sub.createdAt) > time.Second {
		t.Error("createdAt should be refreshed")
	}
}

func TestBridge_RefreshSessionSubscription_NotExists(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	bridge.RefreshSessionSubscription("nonexistent")
}

func TestBridge_PublishUtterance(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	ctx := context.Background()
	msg := &transport.AgentMessage{
		Type:      transport.MessageTypeUtterance,
		AgentID:   "agent_123",
		SessionID: "session_456",
		RequestID: "req_789",
		Payload:   map[string]string{"text": "hello"},
	}

	err := bridge.PublishUtterance(ctx, msg)
	if err != nil {
		t.Fatalf("PublishUtterance error: %v", err)
	}
}

func TestBridge_PublishResponse(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	ctx := context.Background()
	msg := &transport.AgentMessage{
		Type:      transport.MessageTypeResponse,
		AgentID:   "agent_123",
		SessionID: "session_456",
		RequestID: "req_789",
		Payload:   map[string]string{"text": "hi there"},
	}

	err := bridge.PublishResponse(ctx, msg)
	if err != nil {
		t.Fatalf("PublishResponse error: %v", err)
	}
}

func TestBridge_PublishToAgents(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	ctx := context.Background()
	msg := &transport.AgentMessage{
		Type:      transport.MessageTypeUtterance,
		SessionID: "session_456",
		RequestID: "req_789",
	}

	err := bridge.PublishToAgents(ctx, []string{"agent_1", "agent_2"}, msg)
	if err != nil {
		t.Fatalf("PublishToAgents error: %v", err)
	}
}

func TestBridge_PublishCancellation(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	ctx := context.Background()
	err := bridge.PublishCancellation(ctx, "agent_123", "session_456", "user_interrupt")
	if err != nil {
		t.Fatalf("PublishCancellation error: %v", err)
	}
}

func TestBridge_SessionSubCount(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	if bridge.SessionSubCount() != 0 {
		t.Errorf("expected 0, got %d", bridge.SessionSubCount())
	}

	bridge.SubscribeToSession("session_1")
	bridge.SubscribeToSession("session_2")

	if bridge.SessionSubCount() != 2 {
		t.Errorf("expected 2, got %d", bridge.SessionSubCount())
	}
}

func TestBridge_Close(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()

	conn := newMockAgentConnection("agent_123")
	bridge.RegisterAgent(conn)
	bridge.SubscribeToSession("session_123")

	err := bridge.Close()
	if err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if bridge.SessionSubCount() != 0 {
		t.Error("session subs should be cleared")
	}
}

func TestBridge_RegisterAgent_ReplacesOffline(t *testing.T) {
	bridge, mr := newTestBridge(t)
	defer mr.Close()
	defer bridge.Close()

	conn1 := newMockAgentConnection("agent_123")
	bridge.RegisterAgent(conn1)

	conn1.SetOnline(false)
	bridge.UnregisterAgent("agent_123")

	conn2 := newMockAgentConnection("agent_123")
	err := bridge.RegisterAgent(conn2)
	if err != nil {
		t.Fatalf("should be able to register after unregistration: %v", err)
	}
}
