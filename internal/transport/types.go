package transport

import (
	"context"
	"net/http"
	"time"
)

type MessageType string

const (
	MessageTypeUtterance     MessageType = "utterance"
	MessageTypeResponse      MessageType = "response"
	MessageTypeSessionStart  MessageType = "session_start"
	MessageTypeSessionEnd    MessageType = "session_end"
	MessageTypeAgentStatus   MessageType = "agent_status"
	MessageTypeError         MessageType = "error"
	MessageTypeVoiceStart    MessageType = "voice_start"
	MessageTypeVoiceEnd      MessageType = "voice_end"
	MessageTypeAudioFrame    MessageType = "audio_frame"
	MessageTypeSpeechStart   MessageType = "speech_start"
	MessageTypeSpeechEnd     MessageType = "speech_end"
	MessageTypeTranscript    MessageType = "transcript"
	MessageTypeTTSStart      MessageType = "tts_start"
	MessageTypeTTSEnd        MessageType = "tts_end"
	MessageTypeInterrupt     MessageType = "interrupt"
	MessageTypeFrameRequest  MessageType = "frame_request"
	MessageTypeFrameResponse MessageType = "frame_response"
)

type AgentMessage struct {
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
	Text    string         `json:"text"`
	IsFinal bool           `json:"is_final"`
	User    *UserInfo      `json:"user,omitempty"`
	Vision  *VisionContext `json:"vision,omitempty"`
}

type UserInfo struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	IP    string `json:"ip,omitempty"`
}

type VisionContext struct {
	Description string `json:"description,omitempty"`
	Timestamp   int64  `json:"timestamp,omitempty"`
	Available   bool   `json:"available"`
}

type ResponsePayload struct {
	Text      string `json:"text"`
	FromAgent string `json:"from_agent"`
}

type Bridge interface {
	PublishUtterance(ctx context.Context, msg *AgentMessage) error
	PublishToAgents(ctx context.Context, agentIDs []string, msg *AgentMessage) error
	PublishCancellation(ctx context.Context, agentID, sessionID, reason string) error
	PublishResponse(ctx context.Context, msg *AgentMessage) error
	SubscribeToSession(sessionID string) error
	UnsubscribeFromSession(sessionID string)
	SetResponseHandler(handler func(sessionID string, msg *AgentMessage))
}

type AudioFormat int

const (
	AudioFormatOpus AudioFormat = iota
	AudioFormatPCM
)

type AudioChunk struct {
	EventID      string
	ResponseID   string
	ItemID       string
	OutputIndex  int
	ContentIndex int
	Data         []byte
	Format       string
	SampleRate   uint32
}

type BackpressureCallback func(droppedCount int)

type ServerEvent struct {
	Type    string
	Payload any
}

type ClientEnvelope struct {
	Type    string
	Payload any
}

type UserContext struct {
	UserID string
	IP     string
	Name   string
	Email  string
}

type StartRequest struct {
	Conn        Connection
	UserContext *UserContext
	Config      *SessionConfig
}

type SessionConfig struct {
	Voice                     string                      `json:"voice,omitempty"`
	InputAudioTranscription   *InputAudioTranscription    `json:"input_audio_transcription,omitempty"`
	TurnDetection             *TurnDetectionConfig        `json:"turn_detection,omitempty"`
	OutputAudioFormat         string                      `json:"output_audio_format,omitempty"`
	Speed                     float32                     `json:"speed,omitempty"`
}

type InputAudioTranscription struct {
	Model       string  `json:"model,omitempty"`
	Language    string  `json:"language,omitempty"`
	Prompt      string  `json:"prompt,omitempty"`
	Temperature float32 `json:"temperature,omitempty"`
}

type TurnDetectionConfig struct {
	Type              string   `json:"type,omitempty"`
	Threshold         float32  `json:"threshold,omitempty"`
	PrefixPaddingMs   int      `json:"prefix_padding_ms,omitempty"`
	SilenceDurationMs int      `json:"silence_duration_ms,omitempty"`
	CreateResponse    *bool    `json:"create_response,omitempty"`
}

type SessionStarter interface {
	Start(req StartRequest) error
}

type UserProfile struct {
	UserID string
	Name   string
	Email  string
}

type AuthFunc func(r *http.Request) (*UserProfile, error)

type PartialTranscriptEvent struct {
	Text      string
	IsFinal   bool
	Timestamp time.Time
}

type AgentCancelledEvent struct {
	AgentID string
	Reason  string
}

type FrameRequestPayload struct {
	StartTime int64 `json:"start_time"`
	EndTime   int64 `json:"end_time"`
	Limit     int   `json:"limit"`
	RawBase64 bool  `json:"raw_base64"`
}

type FrameResponsePayload struct {
	Frames       []FrameData `json:"frames,omitempty"`
	Descriptions []string    `json:"descriptions,omitempty"`
	Error        string      `json:"error,omitempty"`
}

type FrameData struct {
	Timestamp int64  `json:"timestamp"`
	Base64    string `json:"base64,omitempty"`
}
