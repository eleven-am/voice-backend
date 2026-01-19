package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/redis/go-redis/v9"
)

const (
	agentRequestChannel   = "agent:%s:requests"
	sessionResponsePrefix = "session:%s:responses"
)

type Bridge struct {
	redis           *redis.Client
	logger          *slog.Logger
	agentConns      map[string]AgentConnection
	agentCancels    map[string]context.CancelFunc
	sessionSubs     map[string]context.CancelFunc
	mu              sync.RWMutex
	responseHandler func(sessionID string, msg *transport.AgentMessage)
}

func NewBridge(redisClient *redis.Client, logger *slog.Logger) *Bridge {
	return &Bridge{
		redis:        redisClient,
		logger:       logger.With("component", "bridge"),
		agentConns:   make(map[string]AgentConnection),
		agentCancels: make(map[string]context.CancelFunc),
		sessionSubs:  make(map[string]context.CancelFunc),
	}
}

func (b *Bridge) SetResponseHandler(handler func(sessionID string, msg *transport.AgentMessage)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.responseHandler = handler
}

func (b *Bridge) RegisterAgent(conn AgentConnection) {
	b.mu.Lock()
	defer b.mu.Unlock()

	agentID := conn.AgentID()

	if cancel, exists := b.agentCancels[agentID]; exists {
		cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	b.agentConns[agentID] = conn
	b.agentCancels[agentID] = cancel
	b.logger.Info("agent registered", "agent_id", agentID)

	go b.subscribeToAgentRequests(ctx, conn)
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
	return conn, ok
}

func (b *Bridge) GetOnlineAgents() []AgentInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var agents []AgentInfo
	for _, conn := range b.agentConns {
		if conn.IsOnline() {
			agents = append(agents, AgentInfo{
				ID:     conn.AgentID(),
				Online: true,
			})
		}
	}
	return agents
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

func (b *Bridge) SubscribeToSession(sessionID string) {
	b.mu.Lock()
	if _, exists := b.sessionSubs[sessionID]; exists {
		b.mu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	b.sessionSubs[sessionID] = cancel
	b.mu.Unlock()

	go b.subscribeToSessionResponses(ctx, sessionID)
}

func (b *Bridge) UnsubscribeFromSession(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if cancel, ok := b.sessionSubs[sessionID]; ok {
		cancel()
		delete(b.sessionSubs, sessionID)
		b.logger.Debug("unsubscribed from session", "session_id", sessionID)
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

func (b *Bridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for agentID, cancel := range b.agentCancels {
		cancel()
		delete(b.agentCancels, agentID)
	}

	for agentID := range b.agentConns {
		delete(b.agentConns, agentID)
	}

	for sessionID, cancel := range b.sessionSubs {
		cancel()
		delete(b.sessionSubs, sessionID)
	}

	return nil
}
