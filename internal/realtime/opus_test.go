package realtime

import (
	"testing"
	"time"
)

func TestOpusPacketDuration_EmptyPacket(t *testing.T) {
	samples, duration := OpusPacketDuration([]byte{}, 48000)
	if samples != 960 {
		t.Errorf("expected 960 samples for empty packet, got %d", samples)
	}
	if duration != 20*time.Millisecond {
		t.Errorf("expected 20ms for empty packet, got %v", duration)
	}
}

func TestOpusPacketDuration_20ms_SingleFrame(t *testing.T) {
	toc := byte((16 + 3) << 3)
	samples, duration := OpusPacketDuration([]byte{toc, 0x00}, 48000)
	if samples != 960 {
		t.Errorf("expected 960 samples for 20ms frame, got %d", samples)
	}
	if duration != 20*time.Millisecond {
		t.Errorf("expected 20ms, got %v", duration)
	}
}

func TestOpusPacketDuration_10ms_SingleFrame(t *testing.T) {
	toc := byte((16 + 2) << 3)
	samples, duration := OpusPacketDuration([]byte{toc, 0x00}, 48000)
	if samples != 480 {
		t.Errorf("expected 480 samples for 10ms frame, got %d", samples)
	}
	if duration != 10*time.Millisecond {
		t.Errorf("expected 10ms, got %v", duration)
	}
}

func TestOpusPacketDuration_40ms_TwoFrames(t *testing.T) {
	toc := byte(((16 + 3) << 3) | 1)
	samples, duration := OpusPacketDuration([]byte{toc, 0x00, 0x00}, 48000)
	if samples != 1920 {
		t.Errorf("expected 1920 samples for 2x20ms frames, got %d", samples)
	}
	if duration != 40*time.Millisecond {
		t.Errorf("expected 40ms, got %v", duration)
	}
}

func TestOpusPacketDuration_ArbitraryFrameCount(t *testing.T) {
	toc := byte(((16 + 3) << 3) | 3)
	frameCountByte := byte(4)
	samples, duration := OpusPacketDuration([]byte{toc, frameCountByte, 0x00}, 48000)
	if samples != 3840 {
		t.Errorf("expected 3840 samples for 4x20ms frames, got %d", samples)
	}
	if duration != 80*time.Millisecond {
		t.Errorf("expected 80ms, got %v", duration)
	}
}

func TestOpusPacketDuration_SILK_60ms(t *testing.T) {
	toc := byte(3 << 3)
	samples, duration := OpusPacketDuration([]byte{toc, 0x00}, 48000)
	if samples != 2880 {
		t.Errorf("expected 2880 samples for 60ms SILK frame, got %d", samples)
	}
	if duration != 60*time.Millisecond {
		t.Errorf("expected 60ms, got %v", duration)
	}
}

func TestOpusPacketDuration_CELT_2_5ms(t *testing.T) {
	toc := byte(16 << 3)
	samples, duration := OpusPacketDuration([]byte{toc, 0x00}, 48000)
	if samples != 120 {
		t.Errorf("expected 120 samples for 2.5ms CELT frame, got %d", samples)
	}
	if duration != 2500*time.Microsecond {
		t.Errorf("expected 2.5ms, got %v", duration)
	}
}
