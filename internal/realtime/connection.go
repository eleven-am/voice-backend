package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/eleven-am/voice-backend/internal/transport"
	"github.com/pion/webrtc/v4"
)

const (
	SampleRate    = 48000
	Channels      = 1
	FrameDuration = 20
	FrameSize     = SampleRate * FrameDuration / 1000
)

type Conn struct {
	cfg         Config
	peer        *Peer
	dataChannel *webrtc.DataChannel
	output      *OutputWorker
	log         *slog.Logger

	messages  chan transport.ClientEnvelope
	audioIn   chan []byte
	done      chan struct{}
	closeOnce sync.Once

	mu         sync.RWMutex
	connected  bool
	paused     bool
	ttsStopped bool

	bpCb    transport.BackpressureCallback
	onVideo func([]byte, string)
}

func NewConn(peer *Peer, cfg Config, log *slog.Logger) (*Conn, error) {
	if log == nil {
		log = slog.Default()
	}

	bufSize := cfg.BufferSizes.AudioFrames
	if bufSize <= 0 {
		bufSize = 128
	}

	eventBufSize := cfg.BufferSizes.Events
	if eventBufSize <= 0 {
		eventBufSize = 64
	}

	c := &Conn{
		cfg:      cfg,
		peer:     peer,
		log:      log,
		messages: make(chan transport.ClientEnvelope, eventBufSize),
		audioIn:  make(chan []byte, bufSize),
		done:     make(chan struct{}),
	}

	c.output = NewOutputWorker(peer, bufSize)

	peer.OnAudio(func(opusData []byte) {
		select {
		case c.audioIn <- opusData:
		case <-c.done:
		default:
			c.mu.RLock()
			cb := c.bpCb
			c.mu.RUnlock()
			if cb != nil {
				cb(1)
			}
		}
	})

	peer.OnConnected(func() {
		c.mu.Lock()
		c.connected = true
		c.mu.Unlock()
	})

	peer.OnFailed(func() {
		c.Close()
	})

	peer.OnVideo(func(payload []byte, mimeType string) {
		c.mu.RLock()
		cb := c.onVideo
		c.mu.RUnlock()
		if cb != nil {
			cb(payload, mimeType)
		}
	})

	return c, nil
}

func (c *Conn) SetupDataChannel(dc *webrtc.DataChannel) {
	c.dataChannel = dc

	dc.OnOpen(func() {
		c.mu.Lock()
		c.connected = true
		c.mu.Unlock()
		c.output.Start()
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString {
			c.handleMessage(msg.Data)
		}
	})

	dc.OnClose(func() {
		c.Close()
	})
}

func (c *Conn) handleMessage(data []byte) {
	var base struct {
		Type    string `json:"type"`
		EventID string `json:"event_id,omitempty"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return
	}

	if base.Type == "ice.candidate" {
		c.handleICECandidate(data)
		return
	}

	env := transport.ClientEnvelope{
		Type:    base.Type,
		Payload: json.RawMessage(data),
	}

	select {
	case c.messages <- env:
	case <-c.done:
	}
}

func (c *Conn) handleICECandidate(data []byte) {
	var msg struct {
		Candidate webrtc.ICECandidateInit `json:"candidate"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	if err := c.peer.AddICECandidate(msg.Candidate); err != nil {
		c.log.Debug("failed to add ICE candidate", "error", err)
	}
}

func (c *Conn) SendICECandidate(candidate webrtc.ICECandidateInit) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil
	}
	dc := c.dataChannel
	c.mu.RUnlock()

	if dc == nil {
		return nil
	}

	msg := map[string]any{
		"type":      "ice.candidate",
		"candidate": candidate,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return dc.SendText(string(data))
}

func (c *Conn) Send(ctx context.Context, event transport.ServerEvent) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil
	}
	dc := c.dataChannel
	c.mu.RUnlock()

	if dc == nil {
		return nil
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return dc.SendText(string(data))
}

func (c *Conn) SendAudio(ctx context.Context, chunk transport.AudioChunk) error {
	c.mu.RLock()
	if !c.connected || c.paused || c.ttsStopped {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	if len(chunk.Data) == 0 {
		return nil
	}

	if chunk.Format != "opus" {
		c.log.Warn("received non-opus audio, expected opus", "format", chunk.Format)
		return nil
	}

	return c.output.Enqueue(chunk.Data, FrameSize)
}

func (c *Conn) Messages() <-chan transport.ClientEnvelope {
	return c.messages
}

func (c *Conn) AudioIn() <-chan []byte {
	return c.audioIn
}

func (c *Conn) AudioFormat() transport.AudioFormat {
	return transport.AudioFormatOpus
}

func (c *Conn) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()

		close(c.done)

		c.output.Stop()

		close(c.messages)
		close(c.audioIn)

		err = c.peer.Close()
	})
	return err
}

func (c *Conn) FlushAudioQueue() int {
	return c.output.Flush()
}

func (c *Conn) SetBackpressureCallback(cb transport.BackpressureCallback) {
	c.mu.Lock()
	c.bpCb = cb
	c.mu.Unlock()
	c.output.SetBackpressureCallback(cb)
}

func (c *Conn) PauseOutput() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = true
	c.output.Pause()
}

func (c *Conn) ResumeOutput() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = false
	c.ttsStopped = false
	c.output.Resume()
}

func (c *Conn) StopTTS() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttsStopped = true
}

func (c *Conn) OnVideo(fn func([]byte, string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onVideo = fn
}

func (c *Conn) HasVideo() bool {
	return c.peer.HasVideoTrack()
}
