from __future__ import annotations

import logging

import numpy as np
import soxr

logger = logging.getLogger(__name__)

try:
    import opuslib
    _HAS_OPUSLIB = True
except ImportError:
    _HAS_OPUSLIB = False
    logger.warning("opuslib not available - Opus streaming will not work")

WEBRTC_SAMPLE_RATE = 48000
OPUS_FRAME_MS = 20


class StreamingOpusEncoder:
    def __init__(self, source_rate: int = 24000, target_rate: int = 48000, channels: int = 1):
        if not _HAS_OPUSLIB:
            raise RuntimeError("opuslib not available")

        self._source_rate = source_rate
        self._target_rate = target_rate
        self._channels = channels
        self._frame_samples = int(target_rate * OPUS_FRAME_MS / 1000)
        self._buffer = np.array([], dtype=np.int16)
        self._encoder = opuslib.Encoder(target_rate, channels, "audio")
        self._closed = False

    def encode(self, pcm16: bytes) -> list[bytes]:
        if self._closed:
            return []

        samples = np.frombuffer(pcm16, dtype=np.int16)

        if self._source_rate != self._target_rate:
            float_samples = samples.astype(np.float32) / 32768.0
            resampled = soxr.resample(float_samples, self._source_rate, self._target_rate)
            samples = (resampled * 32768.0).clip(-32768, 32767).astype(np.int16)

        self._buffer = np.concatenate([self._buffer, samples])

        frames = []
        while len(self._buffer) >= self._frame_samples:
            frame_data = self._buffer[:self._frame_samples].tobytes()
            self._buffer = self._buffer[self._frame_samples:]
            encoded = self._encoder.encode(frame_data, self._frame_samples)
            frames.append(encoded)

        return frames

    def flush(self) -> list[bytes]:
        if self._closed:
            return []

        frames = []
        if len(self._buffer) > 0:
            padded = np.pad(self._buffer, (0, self._frame_samples - len(self._buffer)))
            encoded = self._encoder.encode(padded.tobytes(), self._frame_samples)
            frames.append(encoded)
            self._buffer = np.array([], dtype=np.int16)

        self._closed = True
        return frames

    def close(self) -> None:
        self._closed = True


def has_native_opus() -> bool:
    return _HAS_OPUSLIB


def create_streaming_encoder(source_rate: int = 24000, target_rate: int = 48000, channels: int = 1) -> StreamingOpusEncoder:
    return StreamingOpusEncoder(source_rate, target_rate, channels)
