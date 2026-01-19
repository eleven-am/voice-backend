package realtime

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/eleven-am/voice-backend/internal/transport"
)

type audioFrame struct {
	data      []byte
	frameSize int
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
}

func NewOutputWorker(peer *Peer, bufferSize int) *OutputWorker {
	if bufferSize <= 0 {
		bufferSize = 128
	}

	w := &OutputWorker{
		queue: make(chan audioFrame, bufferSize),
		peer:  peer,
	}

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

	const frameDuration = time.Duration(FrameDuration) * time.Millisecond

	for {
		stopCh := w.stopCh.Load()
		select {
		case <-*stopCh:
			w.drain()
			newCh := make(chan struct{})
			if w.stopCh.CompareAndSwap(stopCh, &newCh) {
				continue
			}
			return
		case frame, ok := <-w.queue:
			if !ok {
				return
			}

			if w.paused.Load() {
				continue
			}

			start := time.Now()
			_ = w.peer.WriteRTP(frame.data, frame.frameSize)

			if sleep := frameDuration - time.Since(start); sleep > 0 {
				time.Sleep(sleep)
			}
		}
	}
}

func (w *OutputWorker) Enqueue(data []byte, frameSize int) error {
	frame := audioFrame{
		data:      data,
		frameSize: frameSize,
	}

	select {
	case w.queue <- frame:
		return nil
	default:
		if w.bpCb != nil {
			w.bpCb(1)
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
	return w.drain()
}

func (w *OutputWorker) drain() int {
	count := 0
	for {
		select {
		case <-w.queue:
			count++
		default:
			return count
		}
	}
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
