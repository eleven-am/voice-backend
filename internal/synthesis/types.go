package synthesis

import (
	"github.com/eleven-am/voice-backend/internal/shared"
	"google.golang.org/grpc/credentials"
)

type Callbacks struct {
	OnReady           func(sampleRate uint32, voiceID string)
	OnAudio           func(data []byte, format string, sampleRate uint32)
	OnTranscriptDelta func(text string)
	OnTranscriptDone  func(text string)
	OnDone            func(audioDurationMs, processingDurationMs, textLength uint64)
	OnError           func(error)
}

type Config struct {
	Address  string
	Token    string
	TLSCreds credentials.TransportCredentials
	Backoff  shared.BackoffConfig
}

type Request struct {
	Text     string
	VoiceID  string
	ModelID  string
	Language string
	Speed    float32
	Format   string
	Cancel   <-chan struct{}
}
