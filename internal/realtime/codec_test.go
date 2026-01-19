package realtime

import (
	"testing"
)

func TestConstants(t *testing.T) {
	if SampleRate != 48000 {
		t.Errorf("expected SampleRate 48000, got %d", SampleRate)
	}
	if Channels != 1 {
		t.Errorf("expected Channels 1, got %d", Channels)
	}
	if FrameDuration != 20 {
		t.Errorf("expected FrameDuration 20, got %d", FrameDuration)
	}
	expectedFrameSize := 48000 * 20 / 1000
	if FrameSize != expectedFrameSize {
		t.Errorf("expected FrameSize %d, got %d", expectedFrameSize, FrameSize)
	}
}

func TestNewOpusCodec(t *testing.T) {
	codec, err := NewOpusCodec()
	if err != nil {
		t.Fatalf("NewOpusCodec error: %v", err)
	}
	if codec == nil {
		t.Fatal("codec should not be nil")
	}
	if codec.encoder == nil {
		t.Error("encoder should not be nil")
	}
	if codec.decoder == nil {
		t.Error("decoder should not be nil")
	}
	if codec.frameSize != FrameSize {
		t.Errorf("expected frameSize %d, got %d", FrameSize, codec.frameSize)
	}
}

func TestOpusCodec_FrameSamples(t *testing.T) {
	codec, err := NewOpusCodec()
	if err != nil {
		t.Fatalf("NewOpusCodec error: %v", err)
	}
	if codec.FrameSamples() != FrameSize {
		t.Errorf("expected FrameSamples %d, got %d", FrameSize, codec.FrameSamples())
	}
}

func TestOpusCodec_Encode(t *testing.T) {
	codec, err := NewOpusCodec()
	if err != nil {
		t.Fatalf("NewOpusCodec error: %v", err)
	}
	pcm := make([]int16, FrameSize)
	for i := range pcm {
		pcm[i] = int16(i % 1000)
	}
	encoded, err := codec.Encode(pcm)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	if len(encoded) == 0 {
		t.Error("encoded data should not be empty")
	}
	if len(encoded) >= len(pcm)*2 {
		t.Error("encoded data should be smaller than PCM input")
	}
}

func TestOpusCodec_Encode_Silence(t *testing.T) {
	codec, err := NewOpusCodec()
	if err != nil {
		t.Fatalf("NewOpusCodec error: %v", err)
	}
	pcm := make([]int16, FrameSize)
	encoded, err := codec.Encode(pcm)
	if err != nil {
		t.Fatalf("Encode silence error: %v", err)
	}
	if len(encoded) == 0 {
		t.Error("encoded silence should not be empty")
	}
}

func TestOpusCodec_Decode(t *testing.T) {
	codec, err := NewOpusCodec()
	if err != nil {
		t.Fatalf("NewOpusCodec error: %v", err)
	}
	pcm := make([]int16, FrameSize)
	for i := range pcm {
		pcm[i] = int16((i * 100) % 30000)
	}
	encoded, err := codec.Encode(pcm)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	decoded, err := codec.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(decoded) != FrameSize {
		t.Errorf("expected decoded length %d, got %d", FrameSize, len(decoded))
	}
}

func TestOpusCodec_RoundTrip(t *testing.T) {
	codec, err := NewOpusCodec()
	if err != nil {
		t.Fatalf("NewOpusCodec error: %v", err)
	}
	original := make([]int16, FrameSize)
	for i := range original {
		original[i] = int16((i * 50) % 20000)
	}
	encoded, err := codec.Encode(original)
	if err != nil {
		t.Fatalf("Encode error: %v", err)
	}
	decoded, err := codec.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if len(decoded) != len(original) {
		t.Errorf("length mismatch: expected %d, got %d", len(original), len(decoded))
	}
}

func TestEncodePool(t *testing.T) {
	buf := encodePool.Get().(*[]byte)
	if buf == nil {
		t.Fatal("pool should return non-nil buffer")
	}
	if len(*buf) != maxEncodedSize {
		t.Errorf("expected buffer size %d, got %d", maxEncodedSize, len(*buf))
	}
	encodePool.Put(buf)
	buf2 := encodePool.Get().(*[]byte)
	if buf2 == nil {
		t.Fatal("pool should return non-nil buffer after put")
	}
	encodePool.Put(buf2)
}
