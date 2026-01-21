package vision

import (
	"bytes"
	"fmt"
	"image"
	"sync"

	"golang.org/x/image/vp8"
)

type VPXDecoder struct {
	mu sync.Mutex
}

func NewVPXDecoder() *VPXDecoder {
	return &VPXDecoder{}
}

func (d *VPXDecoder) Decode(data []byte, mimeType string) (image.Image, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(data) == 0 {
		return nil, fmt.Errorf("empty frame data")
	}

	if mimeType != "video/VP8" {
		return nil, fmt.Errorf("unsupported codec: %s (only VP8 supported)", mimeType)
	}

	isKey := (data[0] & 0x01) == 0
	fmt.Printf("VISION DEBUG decoder: data size=%d, isKeyframe=%v, first bytes=%x\n", len(data), isKey, data[:min(10, len(data))])

	decoder := vp8.NewDecoder()
	decoder.Init(bytes.NewReader(data), len(data))

	fh, err := decoder.DecodeFrameHeader()
	if err != nil {
		return nil, fmt.Errorf("decode frame header: %w", err)
	}

	fmt.Printf("VISION DEBUG decoder: header keyframe=%v, width=%d, height=%d\n", fh.KeyFrame, fh.Width, fh.Height)

	if fh.Width == 0 || fh.Height == 0 {
		return nil, fmt.Errorf("invalid frame dimensions: %dx%d", fh.Width, fh.Height)
	}

	img, err := decoder.DecodeFrame()
	if err != nil {
		return nil, fmt.Errorf("decode frame: %w", err)
	}

	return img, nil
}

func (d *VPXDecoder) Close() error {
	return nil
}
