package vision

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/transport"
)

type Analyzer struct {
	client *Client
	store  *Store
	logger *slog.Logger

	mu           sync.RWMutex
	lastResult   *AnalyzeResponse
	analyzing    bool
	analysisDone chan struct{}
}

func NewAnalyzer(client *Client, store *Store, logger *slog.Logger) *Analyzer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Analyzer{
		client: client,
		store:  store,
		logger: logger.With("component", "vision-analyzer"),
	}
}

func (a *Analyzer) StoreFrame(ctx context.Context, frame *Frame) error {
	return a.store.StoreFrame(ctx, frame)
}

func (a *Analyzer) StartAnalysis(ctx context.Context, sessionID string) {
	fmt.Printf("VISION DEBUG analyzer.StartAnalysis: called for session=%s\n", sessionID)
	a.mu.Lock()
	if a.analyzing {
		fmt.Printf("VISION DEBUG analyzer.StartAnalysis: already analyzing, skipping\n")
		a.mu.Unlock()
		return
	}
	a.analyzing = true
	a.analysisDone = make(chan struct{})
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.analyzing = false
			close(a.analysisDone)
			a.mu.Unlock()
		}()

		fmt.Printf("VISION DEBUG analyzer.StartAnalysis: getting latest frame from store\n")
		frame, err := a.store.GetLatestFrame(ctx, sessionID)
		if err != nil {
			fmt.Printf("VISION DEBUG analyzer.StartAnalysis: store error: %v\n", err)
			a.logger.Debug("no frame available for analysis", "session_id", sessionID)
			return
		}
		if frame == nil {
			fmt.Printf("VISION DEBUG analyzer.StartAnalysis: no frame found in store\n")
			a.logger.Debug("no frame available for analysis", "session_id", sessionID)
			return
		}

		fmt.Printf("VISION DEBUG analyzer.StartAnalysis: got frame, size=%d bytes, timestamp=%d, calling client.Analyze\n", len(frame.Data), frame.Timestamp)

		if len(frame.Data) < 5000 {
			fmt.Printf("VISION DEBUG analyzer.StartAnalysis: frame too small (%d bytes), likely black - skipping\n", len(frame.Data))
			return
		}

		result, err := a.client.Analyze(ctx, AnalyzeRequest{Frame: frame})
		if err != nil {
			fmt.Printf("VISION DEBUG analyzer.StartAnalysis: client.Analyze error: %v\n", err)
			a.logger.Error("vision analysis failed", "error", err)
			return
		}

		a.mu.Lock()
		a.lastResult = result
		a.mu.Unlock()

		fmt.Printf("VISION DEBUG analyzer.StartAnalysis: analysis complete, description=%d chars\n", len(result.Description))
		a.logger.Debug("vision analysis complete",
			"session_id", sessionID,
			"timestamp", result.Timestamp,
			"description_len", len(result.Description))
	}()
}

func (a *Analyzer) GetResult(timeout time.Duration) *transport.VisionContext {
	a.mu.RLock()
	doneCh := a.analysisDone
	result := a.lastResult
	a.mu.RUnlock()

	if result != nil {
		return &transport.VisionContext{
			Description: result.Description,
			Timestamp:   result.Timestamp,
			Available:   true,
		}
	}

	if doneCh == nil {
		return &transport.VisionContext{Available: false}
	}

	select {
	case <-doneCh:
		a.mu.RLock()
		result = a.lastResult
		a.mu.RUnlock()
		if result != nil {
			return &transport.VisionContext{
				Description: result.Description,
				Timestamp:   result.Timestamp,
				Available:   true,
			}
		}
	case <-time.After(timeout):
	}

	return &transport.VisionContext{Available: false}
}

func (a *Analyzer) Reset() {
	a.mu.Lock()
	a.lastResult = nil
	a.mu.Unlock()
}

func (a *Analyzer) GetFrames(ctx context.Context, req FrameRequest) (*FrameResponse, error) {
	frames, err := a.store.GetFrames(ctx, req.SessionID, req.StartTime, req.EndTime, req.Limit)
	if err != nil {
		return nil, err
	}

	resp := &FrameResponse{}

	if req.RawBase64 {
		resp.Frames = make([]transport.FrameData, 0, len(frames))
		for _, f := range frames {
			resp.Frames = append(resp.Frames, transport.FrameData{
				Timestamp: f.Timestamp,
				Base64:    string(f.Data),
			})
		}
	} else {
		resp.Descriptions = make([]string, 0, len(frames))
		for _, f := range frames {
			result, err := a.client.Analyze(ctx, AnalyzeRequest{Frame: f})
			if err != nil {
				continue
			}
			resp.Descriptions = append(resp.Descriptions, result.Description)
		}
	}

	return resp, nil
}

func (a *Analyzer) Cleanup(ctx context.Context, sessionID string) error {
	return a.store.DeleteFrames(ctx, sessionID)
}
