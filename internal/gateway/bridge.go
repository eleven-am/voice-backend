package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/redis/go-redis/v9"
)

var ErrAgentAlreadyConnected = errors.New("agent already connected")

const (
	agentRequestChannel   = "agent:%s:requests"
	sessionResponsePrefix = "session:%s:responses"

	sessionSubTTL   = 30 * time.Minute
	cleanupInterval = 5 * time.Minute
	maxSessionSubs  = 10000
)

type sessionSub struct {
	cancel    context.CancelFunc
	createdAt time.Time
}

type Bridge struct {
	redis           *redis.Client
	logger          *slog.Logger
	agentConns      map[string]AgentConnection
	agentCancels    map[string]context.CancelFunc
	sessionSubs     map[string]*sessionSub
	mu              sync.RWMutex
	responseHandler func(sessionID string, msg *transport.AgentMessage)

	ctx       context.Context
	cancel    context.CancelFunc
	cleanupWg sync.WaitGroup
}

func NewBridge(redisClient *redis.Client, logger *slog.Logger) *Bridge {
	ctx, cancel := context.WithCancel(context.Background())

	b := &Bridge{
		redis:        redisClient,
		logger:       logger.With("component", "bridge"),
		agentConns:   make(map[string]AgentConnection),
		agentCancels: make(map[string]context.CancelFunc),
		sessionSubs:  make(map[string]*sessionSub),
		ctx:          ctx,
		cancel:       cancel,
	}

	b.cleanupWg.Add(1)
	go b.cleanupLoop()

	return b
}

func (b *Bridge) cleanupLoop() {
	defer b.cleanupWg.Done()

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.cleanupStaleSessions()
		}
	}
}

func (b *Bridge) cleanupStaleSessions() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	var stale []string

	for sessionID, sub := range b.sessionSubs {
		if now.Sub(sub.createdAt) > sessionSubTTL {
			stale = append(stale, sessionID)
		}
	}

	for _, sessionID := range stale {
		if sub, ok := b.sessionSubs[sessionID]; ok {
			sub.cancel()
			delete(b.sessionSubs, sessionID)
			b.logger.Debug("cleaned up stale session subscription", "session_id", sessionID)
		}
	}

	if len(stale) > 0 {
		b.logger.Info("cleaned up stale session subscriptions", "count", len(stale))
	}
}

func (b *Bridge) SetResponseHandler(handler func(sessionID string, msg *transport.AgentMessage)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.responseHandler = handler
}

func (b *Bridge) RegisterAgent(conn AgentConnection) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	agentID := conn.AgentID()

	if existing, exists := b.agentConns[agentID]; exists && existing.IsOnline() {
		return ErrAgentAlreadyConnected
	}

	if cancel, exists := b.agentCancels[agentID]; exists {
		cancel()
	}

	ctx, cancel := context.WithCancel(b.ctx)
	b.agentConns[agentID] = conn
	b.agentCancels[agentID] = cancel
	b.logger.Info("agent registered", "agent_id", agentID)

	go b.subscribeToAgentRequests(ctx, conn)
	return nil
}

func (b *Bridge) UnregisterAgent(agentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if cancel, ok := b.agentCancels[agentID]; ok {
		cancel()
		delete(b.agentCancels, agentID)
	}

	if _, ok := b.agentConns[agentID]; ok {
		delete(b.agentConns, agentID)
		b.logger.Info("agent unregistered", "agent_id", agentID)
	}
}

func (b *Bridge) GetAgent(agentID string) (AgentConnection, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	conn, ok := b.agentConns[agentID]
	if !ok || !conn.IsOnline() {
		return nil, false
	}
	return conn, ok
}

func (b *Bridge) IsOnline(agentID string) bool {
	_, ok := b.GetAgent(agentID)
	return ok
}

type ConnectedAgent struct {
	AgentID string `json:"agent_id"`
	OwnerID string `json:"owner_id"`
	Online  bool   `json:"online"`
}

func (b *Bridge) ListAgents() []AgentInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var agents []AgentInfo
	for agentID, conn := range b.agentConns {
		if conn.IsOnline() {
			agents = append(agents, AgentInfo{
				ID:     agentID,
				Online: true,
			})
		}
	}
	return agents
}

func (b *Bridge) ListConnectedAgents() []ConnectedAgent {
	b.mu.RLock()
	defer b.mu.RUnlock()

	agents := make([]ConnectedAgent, 0, len(b.agentConns))
	for agentID, conn := range b.agentConns {
		agents = append(agents, ConnectedAgent{
			AgentID: agentID,
			OwnerID: conn.UserID(),
			Online:  conn.IsOnline(),
		})
	}
	return agents
}

func (b *Bridge) AgentCount() (total, online int) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	total = len(b.agentConns)
	for _, conn := range b.agentConns {
		if conn.IsOnline() {
			online++
		}
	}
	return
}

func (b *Bridge) PublishUtterance(ctx context.Context, msg *transport.AgentMessage) error {
	channel := fmt.Sprintf(agentRequestChannel, msg.AgentID)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal utterance: %w", err)
	}

	if err := b.redis.Publish(ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("publish utterance: %w", err)
	}

	b.logger.Debug("published utterance",
		"agent_id", msg.AgentID,
		"session_id", msg.SessionID,
		"request_id", msg.RequestID)
	return nil
}

func (b *Bridge) PublishResponse(ctx context.Context, msg *transport.AgentMessage) error {
	channel := fmt.Sprintf(sessionResponsePrefix, msg.SessionID)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	if err := b.redis.Publish(ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("publish response: %w", err)
	}

	b.logger.Debug("published response",
		"session_id", msg.SessionID,
		"agent_id", msg.AgentID,
		"request_id", msg.RequestID)
	return nil
}

func (b *Bridge) SubscribeToSession(sessionID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.sessionSubs[sessionID]; exists {
		return nil
	}

	if len(b.sessionSubs) >= maxSessionSubs {
		return fmt.Errorf("max session subscriptions reached (%d)", maxSessionSubs)
	}

	ctx, cancel := context.WithCancel(b.ctx)
	b.sessionSubs[sessionID] = &sessionSub{
		cancel:    cancel,
		createdAt: time.Now(),
	}

	go b.subscribeToSessionResponses(ctx, sessionID)
	return nil
}

func (b *Bridge) UnsubscribeFromSession(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.unsubscribeFromSessionLocked(sessionID)
}

func (b *Bridge) unsubscribeFromSessionLocked(sessionID string) {
	if sub, ok := b.sessionSubs[sessionID]; ok {
		sub.cancel()
		delete(b.sessionSubs, sessionID)
		b.logger.Debug("unsubscribed from session", "session_id", sessionID)
	}
}

func (b *Bridge) RefreshSessionSubscription(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if sub, ok := b.sessionSubs[sessionID]; ok {
		sub.createdAt = time.Now()
	}
}

func (b *Bridge) subscribeToAgentRequests(ctx context.Context, conn AgentConnection) {
	agentID := conn.AgentID()
	channel := fmt.Sprintf(agentRequestChannel, agentID)

	pubsub := b.redis.Subscribe(ctx, channel)
	defer pubsub.Close()

	b.logger.Info("subscribed to agent requests", "agent_id", agentID, "channel", channel)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				b.logger.Error("receive agent request", "error", err, "agent_id", agentID)
				return
			}

			var agentMsg transport.AgentMessage
			if err := json.Unmarshal([]byte(msg.Payload), &agentMsg); err != nil {
				b.logger.Error("unmarshal agent request", "error", err, "agent_id", agentID)
				continue
			}

			gatewayMsg := &GatewayMessage{
				Type:      MessageType(agentMsg.Type),
				RequestID: agentMsg.RequestID,
				SessionID: agentMsg.SessionID,
				AgentID:   agentMsg.AgentID,
				UserID:    agentMsg.UserID,
				RoomID:    agentMsg.RoomID,
				Timestamp: agentMsg.Timestamp,
				Payload:   agentMsg.Payload,
			}

			if err := conn.Send(ctx, gatewayMsg); err != nil {
				b.logger.Error("send to agent", "error", err, "agent_id", agentID)
			}
		}
	}
}

func (b *Bridge) subscribeToSessionResponses(ctx context.Context, sessionID string) {
	channel := fmt.Sprintf(sessionResponsePrefix, sessionID)

	pubsub := b.redis.Subscribe(ctx, channel)
	defer pubsub.Close()

	defer func() {
		b.mu.Lock()
		delete(b.sessionSubs, sessionID)
		b.mu.Unlock()
	}()

	b.logger.Debug("subscribed to session responses", "session_id", sessionID, "channel", channel)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := pubsub.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				b.logger.Error("receive session response", "error", err, "session_id", sessionID)
				return
			}

			var agentMsg transport.AgentMessage
			if err := json.Unmarshal([]byte(msg.Payload), &agentMsg); err != nil {
				b.logger.Error("unmarshal session response", "error", err, "session_id", sessionID)
				continue
			}

			b.logger.Debug("received agent message",
				"session_id", sessionID,
				"agent_id", agentMsg.AgentID,
				"type", agentMsg.Type,
				"request_id", agentMsg.RequestID)

			b.mu.RLock()
			handler := b.responseHandler
			b.mu.RUnlock()

			if handler != nil {
				handler(sessionID, &agentMsg)
			}
		}
	}
}

func (b *Bridge) PublishToAgents(ctx context.Context, agentIDs []string, msg *transport.AgentMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	for _, agentID := range agentIDs {
		channel := fmt.Sprintf(agentRequestChannel, agentID)
		if err := b.redis.Publish(ctx, channel, data).Err(); err != nil {
			b.logger.Error("publish to agent failed",
				"agent_id", agentID,
				"error", err)
			continue
		}
		b.logger.Debug("published to agent",
			"agent_id", agentID,
			"session_id", msg.SessionID,
			"request_id", msg.RequestID)
	}
	return nil
}

func (b *Bridge) PublishCancellation(ctx context.Context, agentID, sessionID, reason string) error {
	msg := &transport.AgentMessage{
		Type:      transport.MessageTypeInterrupt,
		SessionID: sessionID,
		AgentID:   agentID,
		Payload: map[string]string{
			"reason": reason,
		},
	}

	channel := fmt.Sprintf(agentRequestChannel, agentID)
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal cancellation: %w", err)
	}

	if err := b.redis.Publish(ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("publish cancellation: %w", err)
	}

	b.logger.Debug("published cancellation",
		"agent_id", agentID,
		"session_id", sessionID,
		"reason", reason)
	return nil
}

func (b *Bridge) SessionSubCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.sessionSubs)
}

func (b *Bridge) Close() error {
	b.cancel()
	b.cleanupWg.Wait()

	b.mu.Lock()
	defer b.mu.Unlock()

	for agentID, cancel := range b.agentCancels {
		cancel()
		delete(b.agentCancels, agentID)
	}

	for agentID := range b.agentConns {
		delete(b.agentConns, agentID)
	}

	for sessionID := range b.sessionSubs {
		delete(b.sessionSubs, sessionID)
	}

	return nil
}
