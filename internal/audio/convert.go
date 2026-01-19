package audio

import (
	"encoding/binary"
	"math"
)

func Resample(input []float32, fromRate, toRate int) []float32 {
	if fromRate == toRate {
		return input
	}

	ratio := float64(toRate) / float64(fromRate)
	outputLen := int(math.Ceil(float64(len(input)) * ratio))
	output := make([]float32, outputLen)

	resampleCore(output, input, ratio)
	return output
}

func resampleCore(output, input []float32, ratio float64) {
	for i := 0; i < len(output); i++ {
		srcPos := float64(i) / ratio
		srcIdx := int(srcPos)
		frac := float32(srcPos - float64(srcIdx))

		if srcIdx+1 < len(input) {
			output[i] = input[srcIdx]*(1-frac) + input[srcIdx+1]*frac
		} else if srcIdx < len(input) {
			output[i] = input[srcIdx]
		}
	}
}

func ResampleInt16(samples []int16, fromRate, toRate int) []int16 {
	if fromRate == toRate {
		return samples
	}

	floats := Int16ToFloat32(samples)
	resampled := Resample(floats, fromRate, toRate)
	return Float32ToInt16(resampled)
}

func PCMBytesToInt16(pcm []byte) []int16 {
	samples := make([]int16, len(pcm)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	return samples
}

func Int16ToFloat32(samples []int16) []float32 {
	result := make([]float32, len(samples))
	for i, s := range samples {
		result[i] = float32(s) / 32768.0
	}
	return result
}

func Float32ToInt16(samples []float32) []int16 {
	result := make([]int16, len(samples))
	for i, s := range samples {
		if s > 1.0 {
			s = 1.0
		} else if s < -1.0 {
			s = -1.0
		}
		result[i] = int16(s * 32767.0)
	}
	return result
}
