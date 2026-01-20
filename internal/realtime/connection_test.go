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
	expectedFrameSize := SampleRate * FrameDuration / 1000
	if FrameSize != expectedFrameSize {
		t.Errorf("expected FrameSize %d, got %d", expectedFrameSize, FrameSize)
	}
}

func TestFrameSizeCalculation(t *testing.T) {
	expected := 48000 * 20 / 1000
	if FrameSize != expected {
		t.Errorf("expected frame size %d (48000 * 20 / 1000), got %d", expected, FrameSize)
	}
}
