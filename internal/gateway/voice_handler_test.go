package gateway

import (
	"io"
	"log/slog"
	"testing"

	"github.com/eleven-am/voice-backend/internal/voicesession"
)

func TestNewVoiceStarter(t *testing.T) {
	cfg := VoiceStarterConfig{
		SessionManager: nil,
		AgentStore:     nil,
		DefaultConfig:  voicesession.Config{},
		Log:            nil,
	}

	starter := NewVoiceStarter(cfg)
	if starter == nil {
		t.Fatal("NewVoiceStarter should not return nil")
	}
	if starter.agentStore != nil {
		t.Error("agentStore should be nil when not provided")
	}
}

func TestNewVoiceStarter_WithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := VoiceStarterConfig{
		SessionManager: nil,
		AgentStore:     nil,
		DefaultConfig:  voicesession.Config{},
		Log:            logger,
	}

	starter := NewVoiceStarter(cfg)
	if starter == nil {
		t.Fatal("NewVoiceStarter should not return nil")
	}
	if starter.log == nil {
		t.Error("logger should not be nil")
	}
}
