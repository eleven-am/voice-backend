package transport

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMessageType_Constants(t *testing.T) {
	tests := []struct {
		msgType MessageType
		want    string
	}{
		{MessageTypeUtterance, "utterance"},
		{MessageTypeResponse, "response"},
		{MessageTypeSessionStart, "session_start"},
		{MessageTypeSessionEnd, "session_end"},
		{MessageTypeAgentStatus, "agent_status"},
		{MessageTypeError, "error"},
		{MessageTypeVoiceStart, "voice_start"},
		{MessageTypeVoiceEnd, "voice_end"},
		{MessageTypeAudioFrame, "audio_frame"},
		{MessageTypeSpeechStart, "speech_start"},
		{MessageTypeSpeechEnd, "speech_end"},
		{MessageTypeTranscript, "transcript"},
		{MessageTypeTTSStart, "tts_start"},
		{MessageTypeTTSEnd, "tts_end"},
		{MessageTypeInterrupt, "interrupt"},
	}

	for _, tt := range tests {
		if string(tt.msgType) != tt.want {
			t.Errorf("MessageType = %q, want %q", tt.msgType, tt.want)
		}
	}
}

func TestAudioFormat_Constants(t *testing.T) {
	if AudioFormatOpus != 0 {
		t.Errorf("AudioFormatOpus = %d, want 0", AudioFormatOpus)
	}
	if AudioFormatPCM != 1 {
		t.Errorf("AudioFormatPCM = %d, want 1", AudioFormatPCM)
	}
}

func TestAgentMessage_JSON(t *testing.T) {
	now := time.Now()
	msg := AgentMessage{
		Type:      MessageTypeUtterance,
		RequestID: "req_123",
		SessionID: "sess_456",
		AgentID:   "agent_789",
		UserID:    "user_abc",
		RoomID:    "room_xyz",
		Timestamp: now,
		Payload:   map[string]string{"text": "hello"},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded AgentMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, msg.Type)
	}
	if decoded.RequestID != msg.RequestID {
		t.Errorf("RequestID = %q, want %q", decoded.RequestID, msg.RequestID)
	}
	if decoded.SessionID != msg.SessionID {
		t.Errorf("SessionID = %q, want %q", decoded.SessionID, msg.SessionID)
	}
}

func TestUtterancePayload_JSON(t *testing.T) {
	payload := UtterancePayload{
		Text:    "hello world",
		IsFinal: true,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded UtterancePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Text != payload.Text {
		t.Errorf("Text = %q, want %q", decoded.Text, payload.Text)
	}
	if decoded.IsFinal != payload.IsFinal {
		t.Errorf("IsFinal = %v, want %v", decoded.IsFinal, payload.IsFinal)
	}
}

func TestResponsePayload_JSON(t *testing.T) {
	payload := ResponsePayload{
		Text:      "response text",
		FromAgent: "agent_123",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ResponsePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Text != payload.Text {
		t.Errorf("Text = %q, want %q", decoded.Text, payload.Text)
	}
	if decoded.FromAgent != payload.FromAgent {
		t.Errorf("FromAgent = %q, want %q", decoded.FromAgent, payload.FromAgent)
	}
}

func TestAudioChunk_Fields(t *testing.T) {
	chunk := AudioChunk{
		EventID:      "evt_123",
		ResponseID:   "resp_456",
		ItemID:       "item_789",
		OutputIndex:  0,
		ContentIndex: 1,
		Data:         []byte{0x01, 0x02, 0x03},
		Format:       "opus",
		SampleRate:   48000,
	}

	if chunk.EventID != "evt_123" {
		t.Error("EventID not set correctly")
	}
	if len(chunk.Data) != 3 {
		t.Error("Data not set correctly")
	}
	if chunk.SampleRate != 48000 {
		t.Error("SampleRate not set correctly")
	}
}

func TestServerEvent_Fields(t *testing.T) {
	event := ServerEvent{
		Type:    "test_event",
		Payload: map[string]int{"count": 5},
	}

	if event.Type != "test_event" {
		t.Error("Type not set")
	}
	if event.Payload == nil {
		t.Error("Payload not set")
	}
}

func TestClientEnvelope_Fields(t *testing.T) {
	envelope := ClientEnvelope{
		Type:    "client_message",
		Payload: "data",
	}

	if envelope.Type != "client_message" {
		t.Error("Type not set")
	}
}

func TestUserContext_Fields(t *testing.T) {
	ctx := UserContext{
		UserID: "user_123",
		IP:     "127.0.0.1",
		Name:   "Test User",
		Email:  "test@example.com",
	}

	if ctx.UserID != "user_123" {
		t.Error("UserID not set")
	}
	if ctx.Email != "test@example.com" {
		t.Error("Email not set")
	}
}

func TestStartRequest_Fields(t *testing.T) {
	userCtx := &UserContext{UserID: "user_1"}
	req := StartRequest{
		Conn:        nil,
		UserContext: userCtx,
		ScopeLookup: nil,
	}

	if req.UserContext != userCtx {
		t.Error("UserContext not set")
	}
}

func TestUserProfile_Fields(t *testing.T) {
	profile := UserProfile{
		UserID: "user_123",
		Name:   "Test User",
		Email:  "test@example.com",
	}

	if profile.UserID != "user_123" {
		t.Error("UserID not set")
	}
	if profile.Name != "Test User" {
		t.Error("Name not set")
	}
}

func TestPartialTranscriptEvent_Fields(t *testing.T) {
	now := time.Now()
	event := PartialTranscriptEvent{
		Text:      "partial text",
		IsFinal:   false,
		Timestamp: now,
	}

	if event.Text != "partial text" {
		t.Error("Text not set")
	}
	if event.IsFinal {
		t.Error("IsFinal should be false")
	}
	if event.Timestamp != now {
		t.Error("Timestamp not set")
	}
}

func TestAgentCancelledEvent_Fields(t *testing.T) {
	event := AgentCancelledEvent{
		AgentID: "agent_123",
		Reason:  "timeout",
	}

	if event.AgentID != "agent_123" {
		t.Error("AgentID not set")
	}
	if event.Reason != "timeout" {
		t.Error("Reason not set")
	}
}
