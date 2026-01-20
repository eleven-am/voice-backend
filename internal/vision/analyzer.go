package vision

import (
	"context"
	"log/slog"
	"sync"
	"time"
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
	a.mu.Lock()
	if a.analyzing {
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

		frame, err := a.store.GetLatestFrame(ctx, sessionID)
		if err != nil || frame == nil {
			a.logger.Debug("no frame available for analysis", "session_id", sessionID)
			return
		}

		result, err := a.client.Analyze(ctx, AnalyzeRequest{Frame: frame})
		if err != nil {
			a.logger.Error("vision analysis failed", "error", err)
			return
		}

		a.mu.Lock()
		a.lastResult = result
		a.mu.Unlock()

		a.logger.Debug("vision analysis complete",
			"session_id", sessionID,
			"timestamp", result.Timestamp,
			"description_len", len(result.Description))
	}()
}

func (a *Analyzer) GetResult(timeout time.Duration) *VisionContext {
	a.mu.RLock()
	doneCh := a.analysisDone
	result := a.lastResult
	a.mu.RUnlock()

	if result != nil {
		return &VisionContext{
			Description: result.Description,
			Timestamp:   result.Timestamp,
			Available:   true,
		}
	}

	if doneCh == nil {
		return &VisionContext{Available: false}
	}

	select {
	case <-doneCh:
		a.mu.RLock()
		result = a.lastResult
		a.mu.RUnlock()
		if result != nil {
			return &VisionContext{
				Description: result.Description,
				Timestamp:   result.Timestamp,
				Available:   true,
			}
		}
	case <-time.After(timeout):
	}

	return &VisionContext{Available: false}
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
		resp.Frames = make([]FrameData, 0, len(frames))
		for _, f := range frames {
			resp.Frames = append(resp.Frames, FrameData{
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
