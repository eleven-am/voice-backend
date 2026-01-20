package gateway

import (
	"context"

	"github.com/eleven-am/voice-backend/internal/transport"
)

type MessageType = transport.MessageType

const (
	MessageTypeUtterance      = transport.MessageTypeUtterance
	MessageTypeResponse       = transport.MessageTypeResponse
	MessageTypeResponseDelta  = transport.MessageTypeResponseDelta
	MessageTypeResponseDone   = transport.MessageTypeResponseDone
	MessageTypeSessionStart   = transport.MessageTypeSessionStart
	MessageTypeSessionEnd     = transport.MessageTypeSessionEnd
	MessageTypeAgentStatus    = transport.MessageTypeAgentStatus
	MessageTypeError          = transport.MessageTypeError
	MessageTypeVoiceStart     = transport.MessageTypeVoiceStart
	MessageTypeVoiceEnd       = transport.MessageTypeVoiceEnd
	MessageTypeAudioFrame     = transport.MessageTypeAudioFrame
	MessageTypeSpeechStart    = transport.MessageTypeSpeechStart
	MessageTypeSpeechEnd      = transport.MessageTypeSpeechEnd
	MessageTypeTranscript     = transport.MessageTypeTranscript
	MessageTypeTTSStart       = transport.MessageTypeTTSStart
	MessageTypeTTSEnd         = transport.MessageTypeTTSEnd
	MessageTypeInterrupt      = transport.MessageTypeInterrupt
	MessageTypeFrameRequest   = transport.MessageTypeFrameRequest
	MessageTypeFrameResponse  = transport.MessageTypeFrameResponse
)

type GatewayMessage = transport.AgentMessage

type UtterancePayload = transport.UtterancePayload

type ResponsePayload = transport.ResponsePayload

type AgentStatusPayload struct {
	AgentID string `json:"agent_id"`
	Online  bool   `json:"online"`
}

type ErrorPayload struct {
	Code    string            `json:"code"`
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
