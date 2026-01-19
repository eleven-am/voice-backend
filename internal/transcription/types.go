package transcription

import (
	"time"

	"github.com/eleven-am/voice-backend/internal/transcription/sttpb"
	"google.golang.org/grpc/credentials"
)

type TranscriptEvent struct {
	Text                 string
	IsPartial            bool
	StartMs              uint64
	EndMs                uint64
	AudioDurationMs      uint64
	ProcessingDurationMs uint64
	Segments             []*sttpb.Segment
	Usage                *sttpb.Usage
	Model                string
}

type Callbacks struct {
	OnReady       func()
	OnSpeechStart func()
	OnSpeechEnd   func()
	OnTranscript  func(event TranscriptEvent)
	OnError       func(error)
}

type Config struct {
	Address  string
	Token    string
	TLSCreds credentials.TransportCredentials
	Backoff  BackoffConfig
}

type SessionOptions struct {
	Language         string
	ModelID          string
	Partials         bool
	PartialWindowMs  uint32
	PartialStrideMs  uint32
	IncludeWordTimes bool
	Hotwords         string
	InitialPrompt    string
	Task             string
	Temperature      float32
}

type BackoffConfig struct {
	Initial     time.Duration
	MaxAttempts int
	MaxDelay    time.Duration
}
