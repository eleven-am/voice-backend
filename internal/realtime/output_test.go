package realtime

import (
	"sync"
	"testing"
	"time"
)

type mockPeer struct {
	mu     sync.Mutex
	frames [][]byte
}

func (p *mockPeer) WriteRTP(data []byte, frameSize int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.frames = append(p.frames, data)
	return nil
}

func (p *mockPeer) GetFrames() [][]byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.frames
}

func TestNewOutputWorker(t *testing.T) {
	worker := NewOutputWorker(nil, 64)
	if worker == nil {
		t.Fatal("NewOutputWorker should not return nil")
	}
	if cap(worker.queue) != 64 {
		t.Errorf("expected buffer size 64, got %d", cap(worker.queue))
	}
}

func TestNewOutputWorker_DefaultBufferSize(t *testing.T) {
	worker := NewOutputWorker(nil, 0)
	if cap(worker.queue) != 4096 {
		t.Errorf("expected default buffer size 4096, got %d", cap(worker.queue))
	}
}

func TestNewOutputWorker_NegativeBufferSize(t *testing.T) {
	worker := NewOutputWorker(nil, -10)
	if cap(worker.queue) != 4096 {
		t.Errorf("expected default buffer size 4096 for negative, got %d", cap(worker.queue))
	}
}

func TestOutputWorker_Enqueue(t *testing.T) {
	worker := NewOutputWorker(nil, 10)
	err := worker.Enqueue([]byte{1, 2, 3}, 960, 20*time.Millisecond)
	if err != nil {
		t.Errorf("Enqueue should not error: %v", err)
	}
	if len(worker.queue) != 1 {
		t.Errorf("expected 1 item in queue, got %d", len(worker.queue))
	}
}

func TestOutputWorker_Enqueue_BufferFull(t *testing.T) {
	worker := NewOutputWorker(nil, 2)
	worker.Enqueue([]byte{1}, 960, 20*time.Millisecond)
	worker.Enqueue([]byte{2}, 960, 20*time.Millisecond)
	err := worker.Enqueue([]byte{3}, 960, 20*time.Millisecond)
	if err != nil {
		t.Errorf("Enqueue should not error even when full: %v", err)
	}
	if len(worker.queue) != 2 {
		t.Errorf("expected 2 items (dropped third), got %d", len(worker.queue))
	}
}

func TestOutputWorker_Enqueue_BackpressureCallback(t *testing.T) {
	worker := NewOutputWorker(nil, 1)
	callbackCalled := false
	worker.SetBackpressureCallback(func(dropped int) {
		callbackCalled = true
		if dropped != 1 {
			t.Errorf("expected dropped=1, got %d", dropped)
		}
	})
	worker.Enqueue([]byte{1}, 960, 20*time.Millisecond)
	worker.Enqueue([]byte{2}, 960, 20*time.Millisecond)
	if !callbackCalled {
		t.Error("backpressure callback should have been called")
	}
}

func TestOutputWorker_PauseResume(t *testing.T) {
	worker := NewOutputWorker(nil, 10)
	if worker.paused.Load() {
		t.Error("worker should not be paused initially")
	}
	worker.Pause()
	if !worker.paused.Load() {
		t.Error("worker should be paused after Pause()")
	}
	worker.Resume()
	if worker.paused.Load() {
		t.Error("worker should not be paused after Resume()")
	}
}

func TestOutputWorker_Flush(t *testing.T) {
	worker := NewOutputWorker(nil, 10)
	worker.Enqueue([]byte{1}, 960, 20*time.Millisecond)
	worker.Enqueue([]byte{2}, 960, 20*time.Millisecond)
	worker.Enqueue([]byte{3}, 960, 20*time.Millisecond)
	count := worker.Flush()
	if count != 3 {
		t.Errorf("expected 3 flushed, got %d", count)
	}
	if len(worker.queue) != 0 {
		t.Errorf("queue should be empty after flush, got %d", len(worker.queue))
	}
}

func TestOutputWorker_Flush_Empty(t *testing.T) {
	worker := NewOutputWorker(nil, 10)
	count := worker.Flush()
	if count != 0 {
		t.Errorf("expected 0 flushed from empty queue, got %d", count)
	}
}

func TestOutputWorker_ContinuesAfterFlush(t *testing.T) {
	worker := NewOutputWorker(nil, 10)
	worker.Pause()
	worker.Start()
	time.Sleep(10 * time.Millisecond)

	worker.Enqueue([]byte{1}, 960, 1*time.Millisecond)
	worker.Enqueue([]byte{2}, 960, 1*time.Millisecond)
	worker.Flush()

	time.Sleep(20 * time.Millisecond)

	worker.Enqueue([]byte{3}, 960, 1*time.Millisecond)
	worker.Enqueue([]byte{4}, 960, 1*time.Millisecond)

	time.Sleep(50 * time.Millisecond)

	worker.pendingMu.Lock()
	pending := worker.pending
	worker.pendingMu.Unlock()

	if pending != 0 {
		t.Errorf("worker should have processed frames after flush (discarded when paused), pending=%d", pending)
	}

	worker.Stop()
}

func TestOutputWorker_SetBackpressureCallback(t *testing.T) {
	worker := NewOutputWorker(nil, 10)
	cb := func(dropped int) {}
	worker.SetBackpressureCallback(cb)
	if worker.bpCb == nil {
		t.Error("callback should be set")
	}
}

func TestOutputWorker_Stop(t *testing.T) {
	worker := NewOutputWorker(nil, 10)
	worker.Start()
	done := make(chan struct{})
	go func() {
		worker.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Stop should complete")
	}
}

func TestOutputWorker_drain(t *testing.T) {
	worker := NewOutputWorker(nil, 10)
	worker.Enqueue([]byte{1}, 960, 20*time.Millisecond)
	worker.Enqueue([]byte{2}, 960, 20*time.Millisecond)
	count := worker.drain()
	if count != 2 {
		t.Errorf("expected 2 drained, got %d", count)
	}
}
