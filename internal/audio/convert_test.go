package audio

import (
	"math"
	"testing"
)

func TestResample_SameRate(t *testing.T) {
	input := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	output := Resample(input, 16000, 16000)
	if len(output) != len(input) {
		t.Errorf("expected same length %d, got %d", len(input), len(output))
	}
	for i := range input {
		if output[i] != input[i] {
			t.Errorf("sample %d: expected %f, got %f", i, input[i], output[i])
		}
	}
}

func TestResample_Upsample(t *testing.T) {
	input := []float32{0.0, 1.0}
	output := Resample(input, 8000, 16000)
	expectedLen := 4
	if len(output) != expectedLen {
		t.Errorf("expected length %d, got %d", expectedLen, len(output))
	}
	if math.Abs(float64(output[0])) > 0.01 {
		t.Errorf("first sample should be ~0, got %f", output[0])
	}
	if math.Abs(float64(output[len(output)-1]-1.0)) > 0.01 {
		t.Errorf("last sample should be ~1, got %f", output[len(output)-1])
	}
}

func TestResample_Downsample(t *testing.T) {
	input := []float32{0.0, 0.25, 0.5, 0.75, 1.0}
	output := Resample(input, 20000, 10000)
	expectedLen := 3
	if len(output) != expectedLen {
		t.Errorf("expected length %d, got %d", expectedLen, len(output))
	}
}

func TestResample_EmptyInput(t *testing.T) {
	input := []float32{}
	output := Resample(input, 16000, 8000)
	if len(output) != 0 {
		t.Errorf("expected empty output, got length %d", len(output))
	}
}

func TestResampleInt16_SameRate(t *testing.T) {
	input := []int16{100, 200, 300, 400, 500}
	output := ResampleInt16(input, 16000, 16000)
	if len(output) != len(input) {
		t.Errorf("expected same length %d, got %d", len(input), len(output))
	}
	for i := range input {
		if output[i] != input[i] {
			t.Errorf("sample %d: expected %d, got %d", i, input[i], output[i])
		}
	}
}

func TestResampleInt16_Upsample(t *testing.T) {
	input := []int16{0, 16384, 32767}
	output := ResampleInt16(input, 8000, 16000)
	expectedLen := 6
	if len(output) != expectedLen {
		t.Errorf("expected length %d, got %d", expectedLen, len(output))
	}
}

func TestPCMBytesToInt16(t *testing.T) {
	pcm := []byte{0x00, 0x00, 0xFF, 0x7F, 0x00, 0x80}
	samples := PCMBytesToInt16(pcm)
	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}
	if samples[0] != 0 {
		t.Errorf("sample 0: expected 0, got %d", samples[0])
	}
	if samples[1] != 32767 {
		t.Errorf("sample 1: expected 32767, got %d", samples[1])
	}
	if samples[2] != -32768 {
		t.Errorf("sample 2: expected -32768, got %d", samples[2])
	}
}

func TestPCMBytesToInt16_Empty(t *testing.T) {
	pcm := []byte{}
	samples := PCMBytesToInt16(pcm)
	if len(samples) != 0 {
		t.Errorf("expected empty samples, got length %d", len(samples))
	}
}

func TestPCMBytesToInt16_OddBytes(t *testing.T) {
	pcm := []byte{0x00, 0x00, 0xFF}
	samples := PCMBytesToInt16(pcm)
	if len(samples) != 1 {
		t.Errorf("expected 1 sample for 3 bytes, got %d", len(samples))
	}
}

func TestInt16ToFloat32(t *testing.T) {
	samples := []int16{0, 32767, -32768, 16384}
	result := Int16ToFloat32(samples)
	if len(result) != len(samples) {
		t.Fatalf("expected %d samples, got %d", len(samples), len(result))
	}
	if math.Abs(float64(result[0])) > 0.001 {
		t.Errorf("sample 0: expected ~0, got %f", result[0])
	}
	if math.Abs(float64(result[1]-1.0)) > 0.001 {
		t.Errorf("sample 1: expected ~1.0, got %f", result[1])
	}
	if math.Abs(float64(result[2]+1.0)) > 0.001 {
		t.Errorf("sample 2: expected ~-1.0, got %f", result[2])
	}
	if math.Abs(float64(result[3]-0.5)) > 0.001 {
		t.Errorf("sample 3: expected ~0.5, got %f", result[3])
	}
}

func TestInt16ToFloat32_Empty(t *testing.T) {
	samples := []int16{}
	result := Int16ToFloat32(samples)
	if len(result) != 0 {
		t.Errorf("expected empty result, got length %d", len(result))
	}
}

func TestFloat32ToInt16(t *testing.T) {
	samples := []float32{0.0, 1.0, -1.0, 0.5}
	result := Float32ToInt16(samples)
	if len(result) != len(samples) {
		t.Fatalf("expected %d samples, got %d", len(samples), len(result))
	}
	if result[0] != 0 {
		t.Errorf("sample 0: expected 0, got %d", result[0])
	}
	if result[1] != 32767 {
		t.Errorf("sample 1: expected 32767, got %d", result[1])
	}
	if result[2] != -32767 {
		t.Errorf("sample 2: expected -32767, got %d", result[2])
	}
	if math.Abs(float64(result[3]-16383)) > 1 {
		t.Errorf("sample 3: expected ~16383, got %d", result[3])
	}
}

func TestFloat32ToInt16_Clipping(t *testing.T) {
	samples := []float32{2.0, -2.0, 1.5, -1.5}
	result := Float32ToInt16(samples)
	if result[0] != 32767 {
		t.Errorf("sample 0: should clip to 32767, got %d", result[0])
	}
	if result[1] != -32767 {
		t.Errorf("sample 1: should clip to -32767, got %d", result[1])
	}
	if result[2] != 32767 {
		t.Errorf("sample 2: should clip to 32767, got %d", result[2])
	}
	if result[3] != -32767 {
		t.Errorf("sample 3: should clip to -32767, got %d", result[3])
	}
}

func TestFloat32ToInt16_Empty(t *testing.T) {
	samples := []float32{}
	result := Float32ToInt16(samples)
	if len(result) != 0 {
		t.Errorf("expected empty result, got length %d", len(result))
	}
}

func TestRoundTrip_Int16ToFloat32ToInt16(t *testing.T) {
	original := []int16{0, 1000, -1000, 32767, -32768, 16384}
	floats := Int16ToFloat32(original)
	recovered := Float32ToInt16(floats)
	for i := range original {
		diff := int(original[i]) - int(recovered[i])
		if diff < -1 || diff > 1 {
			t.Errorf("sample %d: expected ~%d, got %d (diff %d)", i, original[i], recovered[i], diff)
		}
	}
}

func TestResampleCore_EdgeCases(t *testing.T) {
	input := []float32{1.0}
	output := make([]float32, 3)
	resampleCore(output, input, 0.5)
	if output[0] != 1.0 {
		t.Errorf("first output should be 1.0, got %f", output[0])
	}
}
