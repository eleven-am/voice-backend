package vision

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{}
	if cfg.OllamaURL != "" {
		t.Error("OllamaURL should be empty by default")
	}
	if cfg.Model != "" {
		t.Error("Model should be empty by default")
	}
	if cfg.Timeout != 0 {
		t.Error("Timeout should be 0 by default")
	}
	if cfg.FrameTTL != 0 {
		t.Error("FrameTTL should be 0 by default")
	}
}

func TestConfig_WithValues(t *testing.T) {
	cfg := Config{
		OllamaURL: "http://localhost:11434",
		Model:     "qwen2.5vl",
		Timeout:   30 * time.Second,
		FrameTTL:  60 * time.Second,
	}
	if cfg.OllamaURL != "http://localhost:11434" {
		t.Errorf("expected OllamaURL 'http://localhost:11434', got %s", cfg.OllamaURL)
	}
	if cfg.Model != "qwen2.5vl" {
		t.Errorf("expected Model 'qwen2.5vl', got %s", cfg.Model)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected Timeout 30s, got %v", cfg.Timeout)
	}
	if cfg.FrameTTL != 60*time.Second {
		t.Errorf("expected FrameTTL 60s, got %v", cfg.FrameTTL)
	}
}

func TestFrame_Fields(t *testing.T) {
	frame := Frame{
		SessionID: "session-123",
		Timestamp: 1234567890,
		Data:      []byte{0x01, 0x02, 0x03},
		Width:     1920,
		Height:    1080,
	}
	if frame.SessionID != "session-123" {
		t.Errorf("expected SessionID 'session-123', got %s", frame.SessionID)
	}
	if frame.Timestamp != 1234567890 {
		t.Errorf("expected Timestamp 1234567890, got %d", frame.Timestamp)
	}
	if len(frame.Data) != 3 {
		t.Errorf("expected Data length 3, got %d", len(frame.Data))
	}
	if frame.Width != 1920 {
		t.Errorf("expected Width 1920, got %d", frame.Width)
	}
	if frame.Height != 1080 {
		t.Errorf("expected Height 1080, got %d", frame.Height)
	}
}

func TestVisionContext_JSONSerialization(t *testing.T) {
	ctx := VisionContext{
		Description: "A screenshot of code editor",
		Timestamp:   1234567890,
		Available:   true,
	}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded VisionContext
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Description != ctx.Description {
		t.Errorf("expected Description %q, got %q", ctx.Description, decoded.Description)
	}
	if decoded.Timestamp != ctx.Timestamp {
		t.Errorf("expected Timestamp %d, got %d", ctx.Timestamp, decoded.Timestamp)
	}
	if decoded.Available != ctx.Available {
		t.Errorf("expected Available %v, got %v", ctx.Available, decoded.Available)
	}
}

func TestVisionContext_OmitEmpty(t *testing.T) {
	ctx := VisionContext{Available: false}

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	str := string(data)
	if str != `{"available":false}` {
		t.Errorf("expected minimal JSON, got %s", str)
	}
}

func TestAnalyzeRequest_Fields(t *testing.T) {
	frame := &Frame{SessionID: "s1", Data: []byte{0xFF}}
	req := AnalyzeRequest{
		Frame:  frame,
		Prompt: "Describe this image",
	}
	if req.Frame != frame {
		t.Error("Frame should match")
	}
	if req.Prompt != "Describe this image" {
		t.Errorf("expected Prompt 'Describe this image', got %s", req.Prompt)
	}
}

func TestAnalyzeResponse_Fields(t *testing.T) {
	resp := AnalyzeResponse{
		Description: "A code editor showing Go code",
		Timestamp:   1234567890,
	}
	if resp.Description != "A code editor showing Go code" {
		t.Errorf("unexpected Description: %s", resp.Description)
	}
	if resp.Timestamp != 1234567890 {
		t.Errorf("expected Timestamp 1234567890, got %d", resp.Timestamp)
	}
}

func TestFrameRequest_Fields(t *testing.T) {
	req := FrameRequest{
		SessionID: "session-456",
		StartTime: 1000,
		EndTime:   2000,
		Limit:     10,
		RawBase64: true,
	}
	if req.SessionID != "session-456" {
		t.Errorf("expected SessionID 'session-456', got %s", req.SessionID)
	}
	if req.StartTime != 1000 {
		t.Errorf("expected StartTime 1000, got %d", req.StartTime)
	}
	if req.EndTime != 2000 {
		t.Errorf("expected EndTime 2000, got %d", req.EndTime)
	}
	if req.Limit != 10 {
		t.Errorf("expected Limit 10, got %d", req.Limit)
	}
	if !req.RawBase64 {
		t.Error("expected RawBase64 true")
	}
}

func TestFrameResponse_JSONSerialization(t *testing.T) {
	resp := FrameResponse{
		Frames: []FrameData{
			{Timestamp: 1000, Base64: "abc123"},
			{Timestamp: 2000, Base64: "def456"},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded FrameResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded.Frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(decoded.Frames))
	}
	if decoded.Frames[0].Timestamp != 1000 {
		t.Errorf("expected first frame timestamp 1000, got %d", decoded.Frames[0].Timestamp)
	}
	if decoded.Frames[1].Base64 != "def456" {
		t.Errorf("expected second frame Base64 'def456', got %s", decoded.Frames[1].Base64)
	}
}

func TestFrameResponse_Descriptions(t *testing.T) {
	resp := FrameResponse{
		Descriptions: []string{"Frame 1 description", "Frame 2 description"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded FrameResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(decoded.Descriptions) != 2 {
		t.Fatalf("expected 2 descriptions, got %d", len(decoded.Descriptions))
	}
}

func TestFrameData_JSONSerialization(t *testing.T) {
	fd := FrameData{
		Timestamp: 1234567890,
		Base64:    "SGVsbG8gV29ybGQ=",
	}

	data, err := json.Marshal(fd)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded FrameData
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Timestamp != fd.Timestamp {
		t.Errorf("expected Timestamp %d, got %d", fd.Timestamp, decoded.Timestamp)
	}
	if decoded.Base64 != fd.Base64 {
		t.Errorf("expected Base64 %q, got %q", fd.Base64, decoded.Base64)
	}
}

func TestFrameData_OmitEmptyBase64(t *testing.T) {
	fd := FrameData{Timestamp: 1000}

	data, err := json.Marshal(fd)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	str := string(data)
	if str != `{"timestamp":1000}` {
		t.Errorf("expected minimal JSON without base64, got %s", str)
	}
}
