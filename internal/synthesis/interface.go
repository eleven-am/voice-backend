package synthesis

import "context"

type Synthesizer interface {
	Synthesize(ctx context.Context, req Request, cb Callbacks) error
	IsConnected() bool
	Reconnect() error
	Close() error
}
