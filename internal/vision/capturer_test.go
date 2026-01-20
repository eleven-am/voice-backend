package vision

import (
	"context"
	"image"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type mockDecoder struct {
	decodeFunc func(data []byte, mimeType string) (image.Image, error)
	closed     bool
}

func (m *mockDecoder) Decode(data []byte, mimeType string) (image.Image, error) {
	if m.decodeFunc != nil {
		return m.decodeFunc(data, mimeType)
	}
	return image.NewRGBA(image.Rect(0, 0, 100, 100)), nil
}

func (m *mockDecoder) Close() error {
	m.closed = true
	return nil
}

func TestNewFrameCapturer_Defaults(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
	})
	if capturer == nil {
		t.Fatal("NewFrameCapturer should not return nil")
	}
	if capturer.sessionID != "session-123" {
		t.Errorf("expected sessionID 'session-123', got %s", capturer.sessionID)
	}
	if capturer.captureRate != 2*time.Second {
		t.Errorf("expected default captureRate 2s, got %v", capturer.captureRate)
	}
	if capturer.logger == nil {
		t.Error("logger should not be nil (default)")
	}
}

func TestNewFrameCapturer_CustomRate(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID:   "session-123",
		CaptureRate: 500 * time.Millisecond,
	})
	if capturer.captureRate != 500*time.Millisecond {
		t.Errorf("expected captureRate 500ms, got %v", capturer.captureRate)
	}
}

func TestNewFrameCapturer_WithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
		Logger:    logger,
	})
	if capturer.logger == nil {
		t.Error("logger should be set")
	}
}

func TestNewFrameCapturer_WithDecoder(t *testing.T) {
	decoder := &mockDecoder{}
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
		Decoder:   decoder,
	})
	if capturer.decoder != decoder {
		t.Error("decoder should match")
	}
}

func TestFrameCapturer_Stop(t *testing.T) {
	decoder := &mockDecoder{}
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
		Decoder:   decoder,
	})

	capturer.Stop()

	if !capturer.stopped {
		t.Error("stopped should be true after Stop()")
	}
	if !decoder.closed {
		t.Error("decoder should be closed after Stop()")
	}
}

func TestFrameCapturer_Stop_NoDecoder(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
	})

	capturer.Stop()

	if !capturer.stopped {
		t.Error("stopped should be true after Stop()")
	}
}

func TestFrameCapturer_HandleRTPPacket_WhenStopped(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
	})
	capturer.stopped = true

	capturer.HandleRTPPacket([]byte{0x01, 0x02}, "video/VP8")
}

func TestFrameCapturer_HandleRTPPacket_UnsupportedCodec(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
	})

	capturer.HandleRTPPacket([]byte{0x01}, "video/UNSUPPORTED")

	if capturer.sampleBuilder != nil {
		t.Error("sampleBuilder should be nil for unsupported codec")
	}
}

func TestFrameCapturer_HandleRTPPacket_VP8(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
	})

	capturer.HandleRTPPacket([]byte{0x01, 0x02}, "video/VP8")

	if capturer.sampleBuilder == nil {
		t.Error("sampleBuilder should be created for VP8")
	}
	if capturer.mimeType != "video/VP8" {
		t.Errorf("expected mimeType 'video/VP8', got %s", capturer.mimeType)
	}
}

func TestFrameCapturer_HandleRTPPacket_VP9(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
	})

	capturer.HandleRTPPacket([]byte{0x01, 0x02}, "video/VP9")

	if capturer.sampleBuilder == nil {
		t.Error("sampleBuilder should be created for VP9")
	}
	if capturer.mimeType != "video/VP9" {
		t.Errorf("expected mimeType 'video/VP9', got %s", capturer.mimeType)
	}
}

func TestFrameCapturer_HandleRTPPacket_H264(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
	})

	capturer.HandleRTPPacket([]byte{0x01, 0x02}, "video/H264")

	if capturer.sampleBuilder == nil {
		t.Error("sampleBuilder should be created for H264")
	}
	if capturer.mimeType != "video/H264" {
		t.Errorf("expected mimeType 'video/H264', got %s", capturer.mimeType)
	}
}

func TestFrameCapturer_HandleRTPPacket_MimeTypeChange(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: "session-123",
	})

	capturer.HandleRTPPacket([]byte{0x01}, "video/VP8")
	firstBuilder := capturer.sampleBuilder

	capturer.HandleRTPPacket([]byte{0x01}, "video/VP9")

	if capturer.sampleBuilder == firstBuilder {
		t.Error("sampleBuilder should be recreated on mime type change")
	}
	if capturer.mimeType != "video/VP9" {
		t.Errorf("expected mimeType 'video/VP9', got %s", capturer.mimeType)
	}
}

func TestFrameCapturer_CreateSampleBuilder_VP8(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{SessionID: "s"})
	sb := capturer.createSampleBuilder("video/VP8")
	if sb == nil {
		t.Error("should create sampleBuilder for VP8")
	}
}

func TestFrameCapturer_CreateSampleBuilder_VP9(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{SessionID: "s"})
	sb := capturer.createSampleBuilder("video/VP9")
	if sb == nil {
		t.Error("should create sampleBuilder for VP9")
	}
}

func TestFrameCapturer_CreateSampleBuilder_H264(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{SessionID: "s"})
	sb := capturer.createSampleBuilder("video/H264")
	if sb == nil {
		t.Error("should create sampleBuilder for H264")
	}
}

func TestFrameCapturer_CreateSampleBuilder_Unknown(t *testing.T) {
	capturer := NewFrameCapturer(CapturerConfig{SessionID: "s"})
	sb := capturer.createSampleBuilder("video/UNKNOWN")
	if sb != nil {
		t.Error("should return nil for unknown codec")
	}
}

func TestFrameCapturer_StoreRawFrame(t *testing.T) {
	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-capturer-raw-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	defer store.DeleteFrames(ctx, testSessionID)

	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: testSessionID,
		Store:     store,
	})

	capturer.storeRawFrame([]byte("raw frame data"), time.Now().UnixMilli())

	time.Sleep(100 * time.Millisecond)

	frame, err := store.GetLatestFrame(ctx, testSessionID)
	if err != nil {
		t.Fatalf("GetLatestFrame failed: %v", err)
	}
	if frame == nil {
		t.Fatal("expected frame to be stored")
	}
	if string(frame.Data) != "raw frame data" {
		t.Errorf("expected 'raw frame data', got %s", string(frame.Data))
	}
}

func TestFrameCapturer_ProcessFrame_WithDecoder(t *testing.T) {
	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-capturer-decode-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	defer store.DeleteFrames(ctx, testSessionID)

	decoder := &mockDecoder{
		decodeFunc: func(data []byte, mimeType string) (image.Image, error) {
			return image.NewRGBA(image.Rect(0, 0, 640, 480)), nil
		},
	}

	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: testSessionID,
		Store:     store,
		Decoder:   decoder,
	})

	capturer.processFrame([]byte("encoded frame"), "video/VP8", time.Now().UnixMilli())

	time.Sleep(100 * time.Millisecond)

	frame, err := store.GetLatestFrame(ctx, testSessionID)
	if err != nil {
		t.Fatalf("GetLatestFrame failed: %v", err)
	}
	if frame == nil {
		t.Fatal("expected frame to be stored")
	}
	if len(frame.Data) == 0 {
		t.Error("expected frame data to be non-empty")
	}
	jpegMagic := []byte{0xFF, 0xD8, 0xFF}
	if len(frame.Data) < 3 || frame.Data[0] != jpegMagic[0] || frame.Data[1] != jpegMagic[1] || frame.Data[2] != jpegMagic[2] {
		t.Error("expected frame data to be JPEG encoded")
	}
}

func TestFrameCapturer_ProcessFrame_NoDecoder(t *testing.T) {
	redisOpts := &redis.Options{Addr: "localhost:6379"}
	redisClient := redis.NewClient(redisOpts)
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}

	testSessionID := "test-capturer-nodec-" + time.Now().Format("20060102150405")
	store := NewStore(redisClient, 60*time.Second)
	defer store.DeleteFrames(ctx, testSessionID)

	capturer := NewFrameCapturer(CapturerConfig{
		SessionID: testSessionID,
		Store:     store,
	})

	capturer.processFrame([]byte("raw data"), "video/VP8", time.Now().UnixMilli())

	time.Sleep(100 * time.Millisecond)

	frame, err := store.GetLatestFrame(ctx, testSessionID)
	if err != nil {
		t.Fatalf("GetLatestFrame failed: %v", err)
	}
	if frame == nil {
		t.Fatal("expected frame to be stored")
	}
	if string(frame.Data) != "raw data" {
		t.Errorf("expected 'raw data', got %s", string(frame.Data))
	}
}
