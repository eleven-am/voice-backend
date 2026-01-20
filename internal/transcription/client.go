package transcription

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/transcription/sttpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type reconnectState int

const (
	reconnectIdle reconnectState = iota
	reconnectInProgress

	defaultMaxMessageSize = 512 * 1024 * 1024
)

type Client struct {
	addr           string
	conn           *grpc.ClientConn
	client         sttpb.TranscriptionServiceClient
	stream         sttpb.TranscriptionService_TranscribeClient
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	token          string
	creds          grpc.DialOption
	cb             Callbacks
	opts           SessionOptions
	readyCh        chan struct{}
	errCh          chan error
	backoff        shared.BackoffConfig
	reconnectState reconnectState
	reconnectCh    chan error
	maxMessageSize int
}

func New(cfg Config, opts SessionOptions, cb Callbacks) (*Client, error) {
	ctx, cancel := context.WithCancel(context.Background())

	var creds grpc.DialOption
	if cfg.TLSCreds != nil {
		creds = grpc.WithTransportCredentials(cfg.TLSCreds)
	} else {
		creds = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	maxMsgSize := cfg.MaxMessageSize
	if maxMsgSize <= 0 {
		maxMsgSize = defaultMaxMessageSize
	}

	c := &Client{
		addr:           cfg.Address,
		ctx:            ctx,
		cancel:         cancel,
		token:          cfg.Token,
		creds:          creds,
		cb:             cb,
		opts:           opts,
		readyCh:        make(chan struct{}),
		errCh:          make(chan error, 1),
		backoff:        normalizeBackoff(cfg.Backoff),
		reconnectState: reconnectIdle,
		reconnectCh:    make(chan error, 1),
		maxMessageSize: maxMsgSize,
	}

	if err := c.connectAndStart(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return false
	}
	s := conn.GetState()
	return s == connectivity.Ready || s == connectivity.Idle
}

func (c *Client) WaitReady(ctx context.Context) bool {
	select {
	case <-c.readyCh:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Client) connectAndStart() error {
	slog.Info("STT connecting to sidecar", "address", c.addr)
	conn, err := grpc.NewClient(c.addr, c.creds,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(c.maxMessageSize),
			grpc.MaxCallSendMsgSize(c.maxMessageSize),
		),
	)
	if err != nil {
		slog.Error("STT dial failed", "error", err)
		return fmt.Errorf("dial sidecar: %w", err)
	}
	slog.Info("STT connection established")

	stream, err := sttpb.NewTranscriptionServiceClient(conn).Transcribe(c.outgoingContext())
	if err != nil {
		slog.Error("STT open stream failed", "error", err)
		conn.Close()
		return fmt.Errorf("open stream: %w", err)
	}
	slog.Info("STT stream opened")

	c.mu.Lock()
	c.conn = conn
	c.client = sttpb.NewTranscriptionServiceClient(conn)
	c.stream = stream
	c.readyCh = make(chan struct{})
	c.errCh = make(chan error, 1)
	c.mu.Unlock()

	if err := c.sendConfig(); err != nil {
		slog.Error("STT send config failed", "error", err)
		return err
	}
	slog.Info("STT config sent")

	go c.readLoop()
	return nil
}

func (c *Client) outgoingContext() context.Context {
	md := metadata.MD{}
	if c.token != "" {
		md.Set("authorization", fmt.Sprintf("Bearer %s", c.token))
	}
	return metadata.NewOutgoingContext(c.ctx, md)
}

func (c *Client) sendConfig() error {
	c.mu.RLock()
	stream := c.stream
	c.mu.RUnlock()
	if stream == nil {
		return fmt.Errorf("stream not ready")
	}

	return stream.Send(&sttpb.ClientMessage{Msg: &sttpb.ClientMessage_Config{Config: &sttpb.SessionConfig{
		Language:              c.opts.Language,
		ModelId:               c.opts.ModelID,
		Partials:              c.opts.Partials,
		PartialWindowMs:       c.opts.PartialWindowMs,
		PartialStrideMs:       c.opts.PartialStrideMs,
		IncludeWordTimestamps: c.opts.IncludeWordTimes,
		Hotwords:              c.opts.Hotwords,
		InitialPrompt:         c.opts.InitialPrompt,
		Task:                  c.opts.Task,
		Temperature:           c.opts.Temperature,
		SampleRate:            16000,
	}}})
}

func (c *Client) SendAudio(pcm []byte) error {
	c.mu.RLock()
	stream := c.stream
	c.mu.RUnlock()
	if stream == nil {
		return fmt.Errorf("stream not ready")
	}
	return stream.Send(&sttpb.ClientMessage{Msg: &sttpb.ClientMessage_Audio{Audio: &sttpb.AudioFrame{
		SampleRate: 16000,
		Pcm16:      pcm,
	}}})
}

func (c *Client) SendEncodedAudio(format string, data []byte) error {
	c.mu.RLock()
	stream := c.stream
	c.mu.RUnlock()
	if stream == nil {
		return fmt.Errorf("stream not ready")
	}
	return stream.Send(&sttpb.ClientMessage{Msg: &sttpb.ClientMessage_EncodedAudio{EncodedAudio: &sttpb.EncodedAudio{
		Format: format,
		Data:   data,
	}}})
}

func (c *Client) SendOpusFrame(data []byte, sampleRate, channels uint32) error {
	c.mu.RLock()
	stream := c.stream
	c.mu.RUnlock()
	if stream == nil {
		return fmt.Errorf("stream not ready")
	}
	return stream.Send(&sttpb.ClientMessage{Msg: &sttpb.ClientMessage_OpusFrame{OpusFrame: &sttpb.OpusFrame{
		Data:       data,
		SampleRate: sampleRate,
		Channels:   channels,
	}}})
}

func (c *Client) readLoop() {
	slog.Info("STT readLoop started")
	for {
		msg, err := c.stream.Recv()
		if err != nil {
			slog.Error("STT readLoop recv error", "error", err)
			if err != io.EOF && c.cb.OnError != nil {
				c.cb.OnError(err)
			}
			c.errCh <- err
			return
		}

		switch m := msg.Msg.(type) {
		case *sttpb.ServerMessage_Ready:
			slog.Info("STT received Ready message")
			close(c.readyCh)
			if c.cb.OnReady != nil {
				c.cb.OnReady()
			}
		case *sttpb.ServerMessage_SpeechStarted:
			if c.cb.OnSpeechStart != nil {
				c.cb.OnSpeechStart()
			}
		case *sttpb.ServerMessage_SpeechStopped:
			if c.cb.OnSpeechEnd != nil {
				c.cb.OnSpeechEnd()
			}
		case *sttpb.ServerMessage_Transcript:
			slog.Info("STT received Transcript", "text", m.Transcript.GetText(), "partial", m.Transcript.GetIsPartial())
			if c.cb.OnTranscript != nil {
				t := m.Transcript
				evt := TranscriptEvent{
					Text:                 t.GetText(),
					IsPartial:            t.GetIsPartial(),
					StartMs:              t.GetStartMs(),
					EndMs:                t.GetEndMs(),
					AudioDurationMs:      t.GetAudioDurationMs(),
					ProcessingDurationMs: t.GetProcessingDurationMs(),
					Segments:             t.GetSegments(),
					Usage:                t.GetUsage(),
					Model:                t.GetModel(),
				}
				c.cb.OnTranscript(evt)
			}
		case *sttpb.ServerMessage_Error:
			if c.cb.OnError != nil {
				c.cb.OnError(fmt.Errorf("sidecar error: %s", m.Error.GetMessage()))
			}
		}
	}
}

func (c *Client) Reconnect() error {
	c.mu.Lock()
	if c.reconnectState == reconnectInProgress {
		c.mu.Unlock()
		return nil
	}
	c.reconnectState = reconnectInProgress
	c.reconnectCh = make(chan error, 1)
	c.mu.Unlock()

	go c.reconnectLoop()
	return nil
}

func (c *Client) ReconnectSync() error {
	if err := c.Reconnect(); err != nil {
		return err
	}
	return c.WaitReconnect(c.ctx)
}

func (c *Client) WaitReconnect(ctx context.Context) error {
	c.mu.RLock()
	ch := c.reconnectCh
	c.mu.RUnlock()

	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) IsReconnecting() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.reconnectState == reconnectInProgress
}

func (c *Client) reconnectLoop() {
	cfg := c.backoffConfig()
	backoff := cfg.Initial

	defer func() {
		c.mu.Lock()
		c.reconnectState = reconnectIdle
		c.mu.Unlock()
	}()

	for attempts := 0; attempts < cfg.MaxAttempts; attempts++ {
		select {
		case <-c.ctx.Done():
			c.notifyReconnect(c.ctx.Err())
			return
		default:
		}

		if err := c.connectAndStart(); err != nil {
			slog.Warn("STT reconnect attempt failed",
				"attempt", attempts+1,
				"max_attempts", cfg.MaxAttempts,
				"error", err)

			select {
			case <-c.ctx.Done():
				c.notifyReconnect(c.ctx.Err())
				return
			case <-time.After(backoff):
			}
			backoff = minDuration(backoff*2, cfg.MaxDelay)
			continue
		}

		slog.Info("STT reconnected successfully", "attempts", attempts+1)
		c.notifyReconnect(nil)
		return
	}

	err := fmt.Errorf("reconnect failed after %d attempts", cfg.MaxAttempts)
	slog.Error("STT reconnect failed", "error", err)
	c.notifyReconnect(err)
}

func (c *Client) notifyReconnect(err error) {
	c.mu.RLock()
	ch := c.reconnectCh
	c.mu.RUnlock()

	select {
	case ch <- err:
	default:
	}
}

func (c *Client) Close() error {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stream != nil {
		_ = c.stream.CloseSend()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) backoffConfig() shared.BackoffConfig {
	if c.backoff.Initial == 0 && c.backoff.MaxAttempts == 0 && c.backoff.MaxDelay == 0 {
		c.backoff = normalizeBackoff(shared.BackoffConfig{})
	}
	return c.backoff
}

func normalizeBackoff(cfg shared.BackoffConfig) shared.BackoffConfig {
	if cfg.Initial <= 0 {
		cfg.Initial = 100 * time.Millisecond
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 2 * time.Second
	}
	return cfg
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
