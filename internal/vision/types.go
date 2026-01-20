package vision

import (
	"time"

	"github.com/eleven-am/voice-backend/internal/transport"
)

type Config struct {
	OllamaURL string
	Model     string
	Timeout   time.Duration
	FrameTTL  time.Duration
}

type Frame struct {
	SessionID string
	Timestamp int64
	Data      []byte
	Width     int
	Height    int
}

type AnalyzeRequest struct {
	Frame  *Frame
	Prompt string
}

type AnalyzeResponse struct {
	Description string
	Timestamp   int64
}

type FrameRequest struct {
	SessionID string
	StartTime int64
	EndTime   int64
	Limit     int
	RawBase64 bool
}

type FrameResponse struct {
	Frames       []transport.FrameData `json:"frames,omitempty"`
	Descriptions []string              `json:"descriptions,omitempty"`
}
