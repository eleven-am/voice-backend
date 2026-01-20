package voicesession

import (
	"context"
	"log/slog"
	"sync"

	"github.com/eleven-am/voice-backend/internal/synthesis"
	"github.com/eleven-am/voice-backend/internal/transport"
)

type doneNotifier struct {
	once   sync.Once
	bridge *TTSBridge
	onDone func()
}

func (d *doneNotifier) notify() {
	d.once.Do(func() {
		d.bridge.mu.Lock()
		d.bridge.inFlight = false
		d.bridge.mu.Unlock()
		if d.onDone != nil {
			d.onDone()
		}
	})
}

type TTSBridge struct {
	synth   synthesis.Synthesizer
	voiceID string
	speed   float32
	format  string
	conn    transport.Connection
	log     *slog.Logger

	mu       sync.Mutex
	inFlight bool
	cancel   context.CancelFunc
}

type TTSBridgeConfig struct {
	Synth   synthesis.Synthesizer
	VoiceID string
	Speed   float32
	Format  string
	Conn    transport.Connection
	Log     *slog.Logger
}

func NewTTSBridge(cfg TTSBridgeConfig) *TTSBridge {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	return &TTSBridge{
		synth:   cfg.Synth,
		voiceID: cfg.VoiceID,
		speed:   cfg.Speed,
		format:  cfg.Format,
		conn:    cfg.Conn,
		log:     log,
	}
}

func (b *TTSBridge) StartStream(ctx context.Context, text string, onDone func()) {
	b.mu.Lock()
	if b.inFlight && b.cancel != nil {
		b.cancel()
	}
	sCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	b.inFlight = true
	b.mu.Unlock()

	notifier := &doneNotifier{bridge: b, onDone: onDone}

	req := synthesis.Request{
		Text:    text,
		VoiceID: b.voiceID,
		Speed:   b.speed,
		Format:  b.format,
		Cancel:  sCtx.Done(),
	}

	cb := synthesis.Callbacks{
		OnAudio: func(data []byte, format string, sampleRate uint32) {
			chunk := transport.AudioChunk{
				Data:       data,
				Format:     format,
				SampleRate: sampleRate,
			}
			b.log.Debug("TTS chunk", "bytes", len(data), "format", format, "sample_rate", sampleRate)
			if err := b.conn.SendAudio(sCtx, chunk); err != nil {
				b.log.Error("failed to send audio", "error", err)
			}
		},
		OnDone: func(audioDurationMs, processingDurationMs, textLength uint64) {
			if ctrl, ok := b.conn.(transport.OutputController); ok {
				ctrl.WaitForAudioDrain()
			}
			notifier.notify()
		},
		OnError: func(err error) {
			b.log.Error("TTS error", "error", err)
			notifier.notify()
		},
	}

	go func() {
		err := b.synth.Synthesize(sCtx, req, cb)
		if err != nil {
			b.log.Error("TTS synthesize failed", "error", err)
		}
		notifier.notify()
	}()
}

func (b *TTSBridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	b.inFlight = false
}

func (b *TTSBridge) IsActive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.inFlight
}
