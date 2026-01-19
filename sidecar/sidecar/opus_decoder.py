import numpy as np
import opuslib

TARGET_SAMPLE_RATE = 16000
OPUS_SAMPLE_RATE = 48000
OPUS_FRAME_MS = 20
OPUS_FRAME_SAMPLES = OPUS_SAMPLE_RATE * OPUS_FRAME_MS // 1000


class OpusStreamDecoder:
    def __init__(self, sample_rate: int = OPUS_SAMPLE_RATE, channels: int = 1):
        self.sample_rate = sample_rate
        self.channels = channels
        self.decoder = opuslib.Decoder(sample_rate, channels)
        self.frame_size = sample_rate * OPUS_FRAME_MS // 1000

    def decode_frame(self, opus_data: bytes) -> np.ndarray:
        pcm = self.decoder.decode(opus_data, self.frame_size)
        samples = np.frombuffer(pcm, dtype=np.int16)

        if self.channels == 2:
            samples = self._stereo_to_mono(samples)

        return samples.astype(np.float32) / 32768.0

    def _stereo_to_mono(self, samples: np.ndarray) -> np.ndarray:
        left = samples[0::2]
        right = samples[1::2]
        return ((left.astype(np.float32) + right.astype(np.float32)) / 2.0).astype(np.int16)

    def reset(self) -> None:
        self.decoder = opuslib.Decoder(self.sample_rate, self.channels)
