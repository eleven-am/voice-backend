package gateway

import "github.com/eleven-am/voice-backend/internal/transport"

type AgentEvent struct {
	Type      transport.MessageType `json:"type"`
	SessionID string                `json:"session_id"`
	AgentID   string                `json:"agent_id,omitempty"`
	RequestID string                `json:"request_id,omitempty"`
	Payload   any                   `json:"payload,omitempty"`
}
