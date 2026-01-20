package voicesession

import (
	"context"
	"log/slog"
	"sync"
)

type TTSQueue struct {
	bridge *TTSBridge
	log    *slog.Logger

	mu      sync.Mutex
	queue   []string
	ctx     context.Context
	cancel  context.CancelFunc
	playing bool
	started bool
	onStart func()
	onEnd   func()
}

func NewTTSQueue(bridge *TTSBridge, log *slog.Logger) *TTSQueue {
	if log == nil {
		log = slog.Default()
	}
	return &TTSQueue{
		bridge: bridge,
		log:    log,
		queue:  make([]string, 0),
	}
}

func (q *TTSQueue) SetCallbacks(onStart, onEnd func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.onStart = onStart
	q.onEnd = onEnd
}

func (q *TTSQueue) Enqueue(ctx context.Context, sentence string) {
	q.mu.Lock()
	wasEmpty := len(q.queue) == 0 && !q.playing
	q.queue = append(q.queue, sentence)

	if wasEmpty {
		q.ctx, q.cancel = context.WithCancel(ctx)
		q.started = false
	}
	q.mu.Unlock()

	if wasEmpty {
		go q.processQueue()
	}
}

func (q *TTSQueue) processQueue() {
	q.mu.Lock()
	onStart := q.onStart
	q.started = true
	q.mu.Unlock()

	if onStart != nil {
		onStart()
	}

	for {
		q.mu.Lock()
		if len(q.queue) == 0 {
			q.playing = false
			q.started = false
			onEnd := q.onEnd
			q.mu.Unlock()

			if onEnd != nil {
				onEnd()
			}
			return
		}

		sentence := q.queue[0]
		q.queue = q.queue[1:]
		q.playing = true
		ctx := q.ctx
		q.mu.Unlock()

		done := make(chan struct{})
		q.bridge.StartStream(ctx, sentence, func() {
			close(done)
		})

		select {
		case <-done:
		case <-ctx.Done():
			q.mu.Lock()
			q.queue = nil
			q.playing = false
			q.started = false
			q.mu.Unlock()
			return
		}
	}
}

func (q *TTSQueue) Clear() {
	q.mu.Lock()
	wasStarted := q.started
	q.queue = nil
	if q.cancel != nil {
		q.cancel()
		q.cancel = nil
	}
	q.ctx = nil
	q.playing = false
	q.started = false
	q.mu.Unlock()

	q.bridge.Stop()

	if wasStarted && q.onEnd != nil {
		q.onEnd()
	}
}

func (q *TTSQueue) IsPlaying() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.playing || len(q.queue) > 0
}
