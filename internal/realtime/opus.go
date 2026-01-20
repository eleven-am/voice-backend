package realtime

import "time"

var frameDurationsMs = [32]float64{
	10, 20, 40, 60,
	10, 20, 40, 60,
	10, 20, 40, 60,
	10, 20,
	10, 20,
	2.5, 5, 10, 20,
	2.5, 5, 10, 20,
	2.5, 5, 10, 20,
	2.5, 5, 10, 20,
}

func OpusPacketDuration(packet []byte, sampleRate int) (samples int, duration time.Duration) {
	if len(packet) < 1 {
		return 960, 20 * time.Millisecond
	}

	toc := packet[0]
	config := (toc >> 3) & 0x1F
	frameCountCode := toc & 0x03

	frameDurMs := frameDurationsMs[config]

	frameCount := 1
	switch frameCountCode {
	case 0:
		frameCount = 1
	case 1, 2:
		frameCount = 2
	case 3:
		if len(packet) > 1 {
			frameCount = int(packet[1] & 0x3F)
			if frameCount == 0 {
				frameCount = 1
			}
		}
	}

	totalDurMs := frameDurMs * float64(frameCount)
	samples = int(totalDurMs * float64(sampleRate) / 1000)
	duration = time.Duration(totalDurMs * float64(time.Millisecond))

	return samples, duration
}
