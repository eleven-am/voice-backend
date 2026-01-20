package synthesis

import (
	"testing"
	"time"

	"github.com/eleven-am/voice-backend/internal/shared"
)

func TestNormalizeBackoff(t *testing.T) {
	tests := []struct {
		name  string
		input shared.BackoffConfig
		want  shared.BackoffConfig
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

func TestCallbacks_Fields(t *testing.T) {
	readyCalled := false
	audioCalled := false
	transcriptDeltaCalled := false
	transcriptDoneCalled := false
	doneCalled := false
	errorCalled := false

	cb := Callbacks{
		OnReady:           func(sampleRate uint32, voiceID string) { readyCalled = true },
		OnAudio:           func(data []byte, format string, sampleRate uint32) { audioCalled = true },
		OnTranscriptDelta: func(text string) { transcriptDeltaCalled = true },
		OnTranscriptDone:  func(text string) { transcriptDoneCalled = true },
		OnDone:            func(audioDurationMs, processingDurationMs, textLength uint64) { doneCalled = true },
		OnError:           func(err error) { errorCalled = true },
	}

	if cb.OnReady != nil {
		cb.OnReady(48000, "voice_123")
	}
	if cb.OnAudio != nil {
		cb.OnAudio([]byte{0x01}, "opus", 48000)
	}
	if cb.OnTranscriptDelta != nil {
		cb.OnTranscriptDelta("hello")
	}
	if cb.OnTranscriptDone != nil {
		cb.OnTranscriptDone("hello world")
	}
	if cb.OnDone != nil {
		cb.OnDone(1000, 50, 11)
	}
	if cb.OnError != nil {
		cb.OnError(nil)
	}

	if !readyCalled {
		t.Error("OnReady was not called")
	}
	if !audioCalled {
		t.Error("OnAudio was not called")
	}
	if !transcriptDeltaCalled {
		t.Error("OnTranscriptDelta was not called")
	}
	if !transcriptDoneCalled {
		t.Error("OnTranscriptDone was not called")
	}
	if !doneCalled {
		t.Error("OnDone was not called")
	}
	if !errorCalled {
		t.Error("OnError was not called")
	}
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		Address: "localhost:50053",
		Token:   "test-token",
		Backoff: shared.BackoffConfig{
			Initial:     100 * time.Millisecond,
			MaxAttempts: 5,
		},
	}

	if cfg.Address != "localhost:50053" {
		t.Error("Address not set")
	}
	if cfg.Token != "test-token" {
		t.Error("Token not set")
	}
	if cfg.Backoff.MaxAttempts != 5 {
		t.Error("Backoff.MaxAttempts not set")
	}
}

func TestRequest_Fields(t *testing.T) {
	cancel := make(chan struct{})
	req := Request{
		Text:     "hello world",
		VoiceID:  "af_heart",
		ModelID:  "kokoro-v0.19",
		Language: "en",
		Speed:    1.0,
		Format:   "opus",
		Cancel:   cancel,
	}

	if req.Text != "hello world" {
		t.Error("Text not set")
	}
	if req.VoiceID != "af_heart" {
		t.Error("VoiceID not set")
	}
	if req.ModelID != "kokoro-v0.19" {
		t.Error("ModelID not set")
	}
	if req.Language != "en" {
		t.Error("Language not set")
	}
	if req.Speed != 1.0 {
		t.Error("Speed not set")
	}
	if req.Format != "opus" {
		t.Error("Format not set")
	}
	if req.Cancel != cancel {
		t.Error("Cancel not set")
	}
}

