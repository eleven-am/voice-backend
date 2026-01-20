package vision

import "time"

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

type VisionContext struct {
	Description string `json:"description,omitempty"`
	Timestamp   int64  `json:"timestamp,omitempty"`
	Available   bool   `json:"available"`
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
	Frames       []FrameData `json:"frames,omitempty"`
	Descriptions []string    `json:"descriptions,omitempty"`
}

type FrameData struct {
	Timestamp int64  `json:"timestamp"`
	Base64    string `json:"base64,omitempty"`
}
