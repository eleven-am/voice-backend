package transcription

import (
	"testing"
	"time"

	"github.com/eleven-am/voice-backend/internal/shared"
)

func TestNormalizeBackoff(t *testing.T) {
	tests := []struct {
		name   string
		input  shared.BackoffConfig
		want   shared.BackoffConfig
	}{
		{
			name:  "empty config gets defaults",
			input: shared.BackoffConfig{},
			want: shared.BackoffConfig{
				Initial:     100 * time.Millisecond,
				MaxAttempts: 5,
				MaxDelay:    2 * time.Second,
			},
		},
		{
			name: "preserves non-zero values",
			input: shared.BackoffConfig{
				Initial:     200 * time.Millisecond,
				MaxAttempts: 10,
				MaxDelay:    5 * time.Second,
			},
			want: shared.BackoffConfig{
				Initial:     200 * time.Millisecond,
				MaxAttempts: 10,
				MaxDelay:    5 * time.Second,
			},
		},
		{
			name: "normalizes only zero values",
			input: shared.BackoffConfig{
				Initial:     0,
				MaxAttempts: 3,
				MaxDelay:    0,
			},
			want: shared.BackoffConfig{
				Initial:     100 * time.Millisecond,
				MaxAttempts: 3,
				MaxDelay:    2 * time.Second,
			},
		},
		{
			name: "negative values treated as zero",
			input: shared.BackoffConfig{
				Initial:     -100 * time.Millisecond,
				MaxAttempts: -5,
				MaxDelay:    -1 * time.Second,
			},
			want: shared.BackoffConfig{
				Initial:     100 * time.Millisecond,
				MaxAttempts: 5,
				MaxDelay:    2 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeBackoff(tt.input)
			if got.Initial != tt.want.Initial {
				t.Errorf("Initial = %v, want %v", got.Initial, tt.want.Initial)
			}
			if got.MaxAttempts != tt.want.MaxAttempts {
				t.Errorf("MaxAttempts = %d, want %d", got.MaxAttempts, tt.want.MaxAttempts)
			}
			if got.MaxDelay != tt.want.MaxDelay {
				t.Errorf("MaxDelay = %v, want %v", got.MaxDelay, tt.want.MaxDelay)
			}
		})
	}
}

func TestMinDuration(t *testing.T) {
	tests := []struct {
		a, b time.Duration
		want time.Duration
	}{
		{100 * time.Millisecond, 200 * time.Millisecond, 100 * time.Millisecond},
		{300 * time.Millisecond, 200 * time.Millisecond, 200 * time.Millisecond},
		{time.Second, time.Second, time.Second},
		{0, time.Second, 0},
		{time.Second, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.a.String()+"_vs_"+tt.b.String(), func(t *testing.T) {
			got := minDuration(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("minDuration(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestTranscriptEvent_Fields(t *testing.T) {
	event := TranscriptEvent{
		Text:                 "hello world",
		IsPartial:            false,
		StartMs:              100,
		EndMs:                500,
		AudioDurationMs:      400,
		ProcessingDurationMs: 50,
		Model:                "whisper-large-v3",
	}

	if event.Text != "hello world" {
		t.Error("Text not set")
	}
	if event.IsPartial {
		t.Error("IsPartial should be false")
	}
	if event.StartMs != 100 {
		t.Error("StartMs not set")
	}
	if event.EndMs != 500 {
		t.Error("EndMs not set")
	}
	if event.AudioDurationMs != 400 {
		t.Error("AudioDurationMs not set")
	}
	if event.Model != "whisper-large-v3" {
		t.Error("Model not set")
	}
}

func TestCallbacks_Fields(t *testing.T) {
	readyCalled := false
	speechStartCalled := false
	speechEndCalled := false
	transcriptCalled := false
	errorCalled := false

	cb := Callbacks{
		OnReady:       func() { readyCalled = true },
		OnSpeechStart: func() { speechStartCalled = true },
		OnSpeechEnd:   func() { speechEndCalled = true },
		OnTranscript:  func(event TranscriptEvent) { transcriptCalled = true },
		OnError:       func(err error) { errorCalled = true },
	}

	if cb.OnReady != nil {
		cb.OnReady()
	}
	if cb.OnSpeechStart != nil {
		cb.OnSpeechStart()
	}
	if cb.OnSpeechEnd != nil {
		cb.OnSpeechEnd()
	}
	if cb.OnTranscript != nil {
		cb.OnTranscript(TranscriptEvent{})
	}
	if cb.OnError != nil {
		cb.OnError(nil)
	}

	if !readyCalled {
		t.Error("OnReady was not called")
	}
	if !speechStartCalled {
		t.Error("OnSpeechStart was not called")
	}
	if !speechEndCalled {
		t.Error("OnSpeechEnd was not called")
	}
	if !transcriptCalled {
		t.Error("OnTranscript was not called")
	}
	if !errorCalled {
		t.Error("OnError was not called")
	}
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		Address: "localhost:50052",
		Token:   "test-token",
		Backoff: shared.BackoffConfig{
			Initial:     100 * time.Millisecond,
			MaxAttempts: 5,
		},
	}

	if cfg.Address != "localhost:50052" {
		t.Error("Address not set")
	}
	if cfg.Token != "test-token" {
		t.Error("Token not set")
	}
	if cfg.Backoff.MaxAttempts != 5 {
		t.Error("Backoff.MaxAttempts not set")
	}
}

func TestSessionOptions_Fields(t *testing.T) {
	opts := SessionOptions{
		Language:         "en",
		ModelID:          "whisper-large-v3",
		Partials:         true,
		PartialWindowMs:  2000,
		PartialStrideMs:  500,
		IncludeWordTimes: true,
		Hotwords:         "hello,world",
		InitialPrompt:    "Transcribe the audio",
		Task:             "transcribe",
		Temperature:      0.0,
	}

	if opts.Language != "en" {
		t.Error("Language not set")
	}
	if opts.ModelID != "whisper-large-v3" {
		t.Error("ModelID not set")
	}
	if !opts.Partials {
		t.Error("Partials should be true")
	}
	if opts.PartialWindowMs != 2000 {
		t.Error("PartialWindowMs not set")
	}
	if !opts.IncludeWordTimes {
		t.Error("IncludeWordTimes should be true")
	}
}

