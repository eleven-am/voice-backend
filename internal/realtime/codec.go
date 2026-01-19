package realtime

import (
	"sync"

	"gopkg.in/hraban/opus.v2"
)

const (
	SampleRate     = 48000
	Channels       = 1
	SDPFmtpLine    = "minptime=10;useinbandfec=1;stereo=0;sprop-stereo=0"
	FrameDuration  = 20
	FrameSize      = SampleRate * FrameDuration / 1000
	maxEncodedSize = 1024
)

var encodePool = sync.Pool{
	New: func() any {
		buf := make([]byte, maxEncodedSize)
		return &buf
	},
}

type OpusCodec struct {
	encoder   *opus.Encoder
	decoder   *opus.Decoder
	frameSize int
}

func NewOpusCodec() (*OpusCodec, error) {
	enc, err := opus.NewEncoder(SampleRate, Channels, opus.AppVoIP)
	if err != nil {
		return nil, err
	}

	dec, err := opus.NewDecoder(SampleRate, Channels)
	if err != nil {
		return nil, err
	}

	return &OpusCodec{
		encoder:   enc,
		decoder:   dec,
		frameSize: FrameSize,
	}, nil
}

func (c *OpusCodec) Encode(pcm []int16) ([]byte, error) {
	bufPtr := encodePool.Get().(*[]byte)
	buf := *bufPtr
	n, err := c.encoder.Encode(pcm, buf)
	if err != nil {
		encodePool.Put(bufPtr)
		return nil, err
	}
	result := make([]byte, n)
	copy(result, buf[:n])
	encodePool.Put(bufPtr)
	return result, nil
}

func (c *OpusCodec) Decode(data []byte) ([]int16, error) {
	pcm := make([]int16, c.frameSize*Channels)
	n, err := c.decoder.Decode(data, pcm)
	if err != nil {
		return nil, err
	}
	return pcm[:n*Channels], nil
}

func (c *OpusCodec) FrameSamples() int {
	return c.frameSize
}
