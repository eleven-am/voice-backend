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
		DefaultAgentID: "default_agent",
		DefaultConfig:  voicesession.Config{},
		Log:            nil,
	}

	starter := NewVoiceStarter(cfg)
	if starter == nil {
		t.Fatal("NewVoiceStarter should not return nil")
	}
	if starter.defaultAgentID != "default_agent" {
		t.Errorf("expected default_agent, got %s", starter.defaultAgentID)
	}
}

func TestNewVoiceStarter_WithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := VoiceStarterConfig{
		SessionManager: nil,
		DefaultAgentID: "test_agent",
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
