package transcription

import "context"

type Transcriber interface {
	SendOpusFrame(data []byte, sampleRate, channels uint32) error
	SendAudio(pcm []byte) error
	WaitReady(ctx context.Context) bool
	IsConnected() bool
	IsReconnecting() bool
	Reconnect() error
	ReconnectSync() error
	WaitReconnect(ctx context.Context) error
	Close() error
}
