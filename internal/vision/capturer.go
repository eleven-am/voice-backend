package vision

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"log/slog"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

type FrameCapturer struct {
	store       *Store
	sessionID   string
	logger      *slog.Logger
	captureRate time.Duration
	decoder     VideoDecoder

	mu            sync.Mutex
	sampleBuilder *samplebuilder.SampleBuilder
	lastCapture   time.Time
	mimeType      string
	stopped       bool
}

type VideoDecoder interface {
	Decode(data []byte, mimeType string) (image.Image, error)
	Close() error
}

type CapturerConfig struct {
	SessionID   string
	Store       *Store
	Decoder     VideoDecoder
	CaptureRate time.Duration
	Logger      *slog.Logger
}

func NewFrameCapturer(cfg CapturerConfig) *FrameCapturer {
	if cfg.CaptureRate == 0 {
		cfg.CaptureRate = 2 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &FrameCapturer{
		store:       cfg.Store,
		sessionID:   cfg.SessionID,
		logger:      cfg.Logger.With("component", "frame-capturer", "session_id", cfg.SessionID),
		captureRate: cfg.CaptureRate,
		decoder:     cfg.Decoder,
	}
}

func (c *FrameCapturer) HandleRTPPacket(rawPacket []byte, mimeType string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped {
		return
	}

	if c.sampleBuilder == nil || c.mimeType != mimeType {
		c.mimeType = mimeType
		c.sampleBuilder = c.createSampleBuilder(mimeType)
		if c.sampleBuilder == nil {
			c.logger.Debug("VISION DEBUG capturer: failed to create sample builder", "mimeType", mimeType)
			return
		}
		c.logger.Debug("VISION DEBUG capturer: created sample builder", "mimeType", mimeType)
	}

	pkt := &rtp.Packet{}
	if err := pkt.Unmarshal(rawPacket); err != nil {
		c.logger.Debug("VISION DEBUG capturer: failed to unmarshal RTP packet", "error", err)
		return
	}

	c.sampleBuilder.Push(pkt)

	for {
		sample := c.sampleBuilder.Pop()
		if sample == nil {
			break
		}

		isKeyframe := len(sample.Data) > 0 && (sample.Data[0]&0x01) == 0
		if !isKeyframe {
			continue
		}

		now := time.Now()
		if now.Sub(c.lastCapture) < c.captureRate {
			continue
		}

		c.lastCapture = now
		c.logger.Debug("VISION DEBUG capturer: processing keyframe", "sampleSize", len(sample.Data))
		go c.processFrame(sample.Data, mimeType, now.UnixMilli())
	}
}

func (c *FrameCapturer) createSampleBuilder(mimeType string) *samplebuilder.SampleBuilder {
	switch mimeType {
	case "video/VP8":
		return samplebuilder.New(64, &codecs.VP8Packet{}, 90000)
	case "video/VP9":
		return samplebuilder.New(64, &codecs.VP9Packet{}, 90000)
	case "video/H264":
		return samplebuilder.New(64, &codecs.H264Packet{}, 90000)
	default:
		c.logger.Warn("unsupported video codec", "mime_type", mimeType)
		return nil
	}
}

func (c *FrameCapturer) processFrame(data []byte, mimeType string, timestamp int64) {
	if c.decoder == nil {
		c.storeRawFrame(data, timestamp)
		return
	}

	img, err := c.decoder.Decode(data, mimeType)
	if err != nil {
		c.logger.Debug("frame decode failed", "error", err)
		return
	}

	if c.isBlackFrame(img) {
		c.logger.Debug("skipping black frame")
		return
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		c.logger.Debug("jpeg encode failed", "error", err)
		return
	}

	frame := &Frame{
		SessionID: c.sessionID,
		Timestamp: timestamp,
		Data:      buf.Bytes(),
		Width:     img.Bounds().Dx(),
		Height:    img.Bounds().Dy(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	c.logger.Debug("storing frame", "size", len(frame.Data), "width", frame.Width, "height", frame.Height, "timestamp", frame.Timestamp)
	if err := c.store.StoreFrame(ctx, frame); err != nil {
		c.logger.Error("store frame failed", "error", err)
	}
}

func (c *FrameCapturer) isBlackFrame(img image.Image) bool {
	bounds := img.Bounds()
	samples := 0
	bright := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y += 10 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 10 {
			r, g, b, _ := img.At(x, y).RGBA()
			samples++
			if r > 0x1000 || g > 0x1000 || b > 0x1000 {
				bright++
			}
		}
	}

	return samples == 0 || float64(bright)/float64(samples) < 0.05
}

func (c *FrameCapturer) storeRawFrame(data []byte, timestamp int64) {
	frame := &Frame{
		SessionID: c.sessionID,
		Timestamp: timestamp,
		Data:      data,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := c.store.StoreFrame(ctx, frame); err != nil {
		c.logger.Error("store raw frame failed", "error", err)
	}
}

func (c *FrameCapturer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	if c.decoder != nil {
		c.decoder.Close()
	}
}
