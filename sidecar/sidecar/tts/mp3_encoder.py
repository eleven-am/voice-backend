from __future__ import annotations

import logging

import numpy as np

logger = logging.getLogger(__name__)

try:
    import lameenc
    _HAS_LAMEENC = True
except ImportError:
    _HAS_LAMEENC = False
    logger.warning("lameenc not available - MP3 streaming will not work")


class StreamingMP3Encoder:
    def __init__(self, sample_rate: int = 24000, channels: int = 1, bitrate: int = 128):
        if not _HAS_LAMEENC:
            raise RuntimeError("lameenc not available")

        self._sample_rate = sample_rate
        self._channels = channels
        self._bitrate = bitrate
        self._encoder = lameenc.Encoder()
        self._encoder.set_bit_rate(bitrate)
        self._encoder.set_in_sample_rate(sample_rate)
        self._encoder.set_channels(channels)
        self._encoder.set_quality(2)
        self._closed = False

    def encode(self, pcm16: bytes) -> bytes:
        if self._closed:
            return b""

        samples = np.frombuffer(pcm16, dtype=np.int16)
        return self._encoder.encode(samples.tobytes())

    def flush(self) -> bytes:
        if self._closed:
            return b""

        self._closed = True
        return self._encoder.flush()

    def close(self) -> None:
        self._closed = True


def has_native_mp3() -> bool:
    return _HAS_LAMEENC


def create_streaming_mp3_encoder(
    sample_rate: int = 24000, channels: int = 1, bitrate: int = 128
) -> StreamingMP3Encoder:
    return StreamingMP3Encoder(sample_rate, channels, bitrate)
