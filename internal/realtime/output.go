package realtime

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/eleven-am/voice-backend/internal/transport"
)

type audioFrame struct {
	data     []byte
	samples  int
	duration time.Duration
}

type OutputWorker struct {
	queue  chan audioFrame
	stopCh atomic.Pointer[chan struct{}]
	peer   *Peer
	paused atomic.Bool

	mu       sync.Mutex
	wg       sync.WaitGroup
	stopOnce sync.Once

	bpCb transport.BackpressureCallback

	pendingMu   sync.Mutex
	pendingCond *sync.Cond
	pending     int64
}

func NewOutputWorker(peer *Peer, bufferSize int) *OutputWorker {
	if bufferSize <= 0 {
		bufferSize = 4096
	}

	w := &OutputWorker{
		queue: make(chan audioFrame, bufferSize),
		peer:  peer,
	}

	w.pendingCond = sync.NewCond(&w.pendingMu)

	stopCh := make(chan struct{})
	w.stopCh.Store(&stopCh)

	return w
}

func (w *OutputWorker) Start() {
	w.wg.Add(1)
	go w.run()
}

func (w *OutputWorker) run() {
	defer w.wg.Done()
	frameCount := 0

	for {
		stopCh := w.stopCh.Load()
		select {
		case <-*stopCh:
			w.drain()
			newCh := make(chan struct{})
			if w.stopCh.CompareAndSwap(stopCh, &newCh) {
				frameCount = 0
			}
			continue
		case frame, ok := <-w.queue:
			if !ok {
				return
			}

			if w.paused.Load() {
				w.decrementPending()
				continue
			}

			start := time.Now()
			_ = w.peer.WriteRTP(frame.data, frame.samples)
			frameCount++
			w.decrementPending()

			if sleep := frame.duration - time.Since(start); sleep > 0 {
				time.Sleep(sleep)
			}
		}
	}
}

func (w *OutputWorker) Enqueue(data []byte, samples int, duration time.Duration) error {
	frame := audioFrame{
		data:     data,
		samples:  samples,
		duration: duration,
	}

	select {
	case w.queue <- frame:
		w.pendingMu.Lock()
		w.pending++
		w.pendingMu.Unlock()
		return nil
	default:
		w.mu.Lock()
		cb := w.bpCb
		w.mu.Unlock()
		if cb != nil {
			cb(1)
		}
		return nil
	}
}

func (w *OutputWorker) Flush() int {
	newCh := make(chan struct{})
	oldPtr := w.stopCh.Swap(&newCh)
	if oldPtr != nil {
		close(*oldPtr)
	}
	count := w.drain()
	return count
}

func (w *OutputWorker) drain() int {
	count := 0
	for {
		select {
		case <-w.queue:
			count++
			w.decrementPending()
		default:
			return count
		}
	}
}

func (w *OutputWorker) decrementPending() {
	w.pendingMu.Lock()
	w.pending--
	if w.pending <= 0 {
		w.pending = 0
		w.pendingCond.Broadcast()
	}
	w.pendingMu.Unlock()
}

func (w *OutputWorker) WaitForDrain() {
	w.pendingMu.Lock()
	for w.pending > 0 {
		w.pendingCond.Wait()
	}
	w.pendingMu.Unlock()
}

func (w *OutputWorker) Pause() {
	w.paused.Store(true)
}

func (w *OutputWorker) Resume() {
	w.paused.Store(false)
}

func (w *OutputWorker) SetBackpressureCallback(cb transport.BackpressureCallback) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.bpCb = cb
}

func (w *OutputWorker) Stop() {
	w.stopOnce.Do(func() {
		close(w.queue)
	})
	w.wg.Wait()
}
