package transport

import "context"

type Connection interface {
	Send(ctx context.Context, event ServerEvent) error
	SendAudio(ctx context.Context, chunk AudioChunk) error
	Messages() <-chan ClientEnvelope
	AudioIn() <-chan []byte
	AudioFormat() AudioFormat
	IsConnected() bool
	Close() error
	FlushAudioQueue() int
	SetBackpressureCallback(cb BackpressureCallback)
}

type OutputController interface {
	PauseOutput()
	ResumeOutput()
	StopTTS()
	WaitForAudioDrain()
}
