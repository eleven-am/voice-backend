package gateway

import (
	"context"
	"time"

	pb "github.com/eleven-am/voice-backend/internal/gateway/proto"
)

type MessageType string

const (
	MessageTypeUtterance    MessageType = "utterance"
	MessageTypeResponse     MessageType = "response"
	MessageTypeSessionStart MessageType = "session_start"
	MessageTypeSessionEnd   MessageType = "session_end"
	MessageTypeAgentStatus  MessageType = "agent_status"
	MessageTypeError        MessageType = "error"
)

type GatewayMessage struct {
	Type      MessageType `json:"type"`
	RequestID string      `json:"request_id"`
	SessionID string      `json:"session_id"`
	AgentID   string      `json:"agent_id,omitempty"`
	UserID    string      `json:"user_id,omitempty"`
	RoomID    string      `json:"room_id,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   any         `json:"payload"`
}

type UtterancePayload struct {
	Text    string `json:"text"`
	IsFinal bool   `json:"is_final"`
}

type ResponsePayload struct {
	Text      string `json:"text"`
	FromAgent string `json:"from_agent"`
}

type AgentStatusPayload struct {
	AgentID string `json:"agent_id"`
	Online  bool   `json:"online"`
}

type ErrorPayload struct {
	Code    pb.ErrorCode      `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

type SessionStartPayload struct {
	Agents []AgentInfo `json:"agents"`
}

type AgentInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Online       bool     `json:"online"`
	Capabilities []string `json:"capabilities"`
}

type Connection interface {
	Send(ctx context.Context, msg *GatewayMessage) error
	Messages() <-chan *GatewayMessage
	SessionID() string
	UserID() string
	Close() error
}

type AgentConnection interface {
	Connection
	AgentID() string
	SetOnline(online bool)
	IsOnline() bool
}

type VoiceAgentConnection interface {
	Send(ctx context.Context, msg *pb.ServerMessage) error
	Messages() <-chan *pb.ClientMessage
	SessionID() string
	UserID() string
	RoomID() string
	Close() error
}
