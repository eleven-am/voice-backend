package synthesis

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/synthesis/ttspb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	addr    string
	conn    *grpc.ClientConn
	client  ttspb.TextToSpeechServiceClient
	mu      sync.RWMutex
	token   string
	creds   grpc.DialOption
	backoff shared.BackoffConfig
}

func New(cfg Config) (*Client, error) {
	var creds grpc.DialOption
	if cfg.TLSCreds != nil {
		creds = grpc.WithTransportCredentials(cfg.TLSCreds)
	} else {
		creds = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	conn, err := grpc.NewClient(cfg.Address, creds)
	if err != nil {
		return nil, fmt.Errorf("dial sidecar: %w", err)
	}

	return &Client{
		addr:    cfg.Address,
		conn:    conn,
		client:  ttspb.NewTextToSpeechServiceClient(conn),
		token:   cfg.Token,
		creds:   creds,
		backoff: normalizeBackoff(cfg.Backoff),
	}, nil
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

func (c *Client) synthStream(ctx context.Context) (ttspb.TextToSpeechService_SynthesizeClient, error) {
	md := metadata.MD{}
	if c.token != "" {
		md.Set("authorization", fmt.Sprintf("Bearer %s", c.token))
	}
	return c.client.Synthesize(metadata.NewOutgoingContext(ctx, md))
}

func (c *Client) Synthesize(ctx context.Context, req Request, cb Callbacks) error {
	stream, err := c.synthStream(ctx)
	if err != nil {
		return fmt.Errorf("synthesize: %w", err)
	}

	if err := stream.Send(&ttspb.TtsClientMessage{Msg: &ttspb.TtsClientMessage_Config{Config: &ttspb.TtsSessionConfig{
		VoiceId:        req.VoiceID,
		ModelId:        req.ModelID,
		Language:       req.Language,
		Speed:          req.Speed,
		ResponseFormat: req.Format,
	}}}); err != nil {
		return fmt.Errorf("send config: %w", err)
	}

	if err := stream.Send(&ttspb.TtsClientMessage{Msg: &ttspb.TtsClientMessage_Text{Text: &ttspb.TextChunk{Text: req.Text}}}); err != nil {
		return fmt.Errorf("send text: %w", err)
	}

	if err := stream.Send(&ttspb.TtsClientMessage{Msg: &ttspb.TtsClientMessage_End{End: &ttspb.EndOfText{}}}); err != nil {
		return fmt.Errorf("send end: %w", err)
	}

	doneCh := make(chan struct{})
	defer close(doneCh)

	recvErrCh := make(chan error, 1)
	go func() {
		defer close(recvErrCh)
		for {
			msg, err := stream.Recv()
			if err != nil {
				if err == io.EOF || ctx.Err() != nil {
					return
				}
				if cb.OnError != nil {
					cb.OnError(err)
				}
				recvErrCh <- err
				return
			}

			switch x := msg.Msg.(type) {
			case *ttspb.TtsServerMessage_Ready:
				if cb.OnReady != nil && x.Ready != nil {
					cb.OnReady(x.Ready.SampleRate, x.Ready.VoiceId)
				}
			case *ttspb.TtsServerMessage_Audio:
				if cb.OnAudio != nil && x.Audio != nil {
					cb.OnAudio(x.Audio.Data, x.Audio.GetFormat(), x.Audio.GetSampleRate())
				}
				if cb.OnTranscriptDelta != nil && x.Audio != nil && x.Audio.Transcript != "" {
					cb.OnTranscriptDelta(x.Audio.Transcript)
				}
			case *ttspb.TtsServerMessage_Done:
				if cb.OnDone != nil && x.Done != nil {
					cb.OnDone(x.Done.AudioDurationMs, x.Done.ProcessingDurationMs, x.Done.TextLength)
				}
				if cb.OnTranscriptDone != nil && x.Done != nil && x.Done.Transcript != "" {
					cb.OnTranscriptDone(x.Done.Transcript)
				}
				return
			case *ttspb.TtsServerMessage_Error:
				if cb.OnError != nil && x.Error != nil {
					cb.OnError(fmt.Errorf("sidecar error: %s", x.Error.GetMessage()))
				}
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-req.Cancel:
		_ = stream.CloseSend()
		return nil
	case err := <-recvErrCh:
		return err
	}
}

func (c *Client) ListVoices(ctx context.Context) ([]*ttspb.Voice, error) {
	resp, err := c.client.ListVoices(ctx, &ttspb.ListVoicesRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetVoices(), nil
}

func (c *Client) ListModels(ctx context.Context) ([]*ttspb.TTSModel, error) {
	resp, err := c.client.ListModels(ctx, &ttspb.ListModelsRequest{})
	if err != nil {
		return nil, err
	}
	return resp.GetModels(), nil
}

func (c *Client) CreateVoice(ctx context.Context, voiceID string, audioData []byte, name, language, gender, referenceText string) (*ttspb.Voice, error) {
	md := metadata.MD{}
	if c.token != "" {
		md.Set("authorization", fmt.Sprintf("Bearer %s", c.token))
	}

	resp, err := c.client.CreateVoice(metadata.NewOutgoingContext(ctx, md), &ttspb.CreateVoiceRequest{
		VoiceId:       voiceID,
		AudioData:     audioData,
		Name:          name,
		Language:      language,
		Gender:        gender,
		ReferenceText: referenceText,
	})
	if err != nil {
		return nil, fmt.Errorf("create voice: %w", err)
	}
	return resp.GetVoice(), nil
}

func (c *Client) DeleteVoice(ctx context.Context, voiceID string) (bool, error) {
	md := metadata.MD{}
	if c.token != "" {
		md.Set("authorization", fmt.Sprintf("Bearer %s", c.token))
	}

	resp, err := c.client.DeleteVoice(metadata.NewOutgoingContext(ctx, md), &ttspb.DeleteVoiceRequest{
		VoiceId: voiceID,
	})
	if err != nil {
		return false, fmt.Errorf("delete voice: %w", err)
	}
	return resp.GetSuccess(), nil
}

func (c *Client) Reconnect() error {
	cfg := c.backoffConfig()
	backoff := cfg.Initial
	for attempts := 0; attempts < cfg.MaxAttempts; attempts++ {
		conn, err := grpc.NewClient(c.addr, c.creds)
		if err != nil {
			time.Sleep(backoff)
			backoff = minDuration(backoff*2, cfg.MaxDelay)
			continue
		}
		conn.Connect()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if !conn.WaitForStateChange(ctx, connectivity.Idle) {
			cancel()
			conn.Close()
			time.Sleep(backoff)
			backoff = minDuration(backoff*2, cfg.MaxDelay)
			continue
		}
		cancel()
		c.mu.Lock()
		if c.conn != nil {
			c.conn.Close()
		}
		c.conn = conn
		c.client = ttspb.NewTextToSpeechServiceClient(conn)
		c.mu.Unlock()
		return nil
	}
	return fmt.Errorf("reconnect failed")
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
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
