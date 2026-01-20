from __future__ import annotations

import asyncio
import io
import logging

import numpy as np
import soundfile as sf

from sidecar.tts.synthesis import SynthesisError

logger = logging.getLogger(__name__)

_HAS_OPUSLIB = False
try:
    import opuslib
    _HAS_OPUSLIB = True
    logger.info("opuslib available for native Opus encoding")
except ImportError:
    logger.debug("opuslib not available")

_HAS_LAMEENC = False
try:
    import lameenc
    _HAS_LAMEENC = True
    logger.info("lameenc available for native MP3 encoding")
except ImportError:
    logger.debug("lameenc not available")

OPUS_FRAME_MS = 20


def _encode_mp3_native(pcm16: bytes, sample_rate: int, bitrate: int = 128) -> bytes:
    if not _HAS_LAMEENC:
        raise SynthesisError("lameenc not available for MP3 encoding", code=5)

    encoder = lameenc.Encoder()
    encoder.set_bit_rate(bitrate)
    encoder.set_in_sample_rate(sample_rate)
    encoder.set_channels(1)
    encoder.set_quality(2)

    samples = np.frombuffer(pcm16, dtype=np.int16)
    mp3_data = encoder.encode(samples.tobytes())
    mp3_data += encoder.flush()

    return mp3_data


def _encode_opus_native(pcm16: bytes, sample_rate: int) -> bytes:
    if not _HAS_OPUSLIB:
        raise SynthesisError("opuslib not available for Opus encoding", code=5)

    encoder = opuslib.Encoder(sample_rate, 1, "audio")
    frame_samples = int(sample_rate * OPUS_FRAME_MS / 1000)

    samples = np.frombuffer(pcm16, dtype=np.int16)
    frames = []

    for i in range(0, len(samples), frame_samples):
        frame = samples[i:i + frame_samples]
        if len(frame) < frame_samples:
            frame = np.pad(frame, (0, frame_samples - len(frame)))
        encoded = encoder.encode(frame.tobytes(), frame_samples)
        frames.append(encoded)

    return b"".join(frames)


def encode_audio(pcm16: bytes, sample_rate: int, fmt: str) -> tuple[bytes, str]:
    fmt = (fmt or "pcm").lower()

    if fmt in ("pcm", "s16le"):
        return pcm16, "pcm"

    if fmt == "wav":
        buf = io.BytesIO()
        data = np.frombuffer(pcm16, dtype=np.int16)
        sf.write(buf, data, samplerate=sample_rate, format="WAV", subtype="PCM_16")
        return buf.getvalue(), "wav"

    if fmt == "flac":
        buf = io.BytesIO()
        data = np.frombuffer(pcm16, dtype=np.int16)
        sf.write(buf, data, samplerate=sample_rate, format="FLAC", subtype="PCM_16")
        return buf.getvalue(), "flac"

    if fmt == "opus":
        return _encode_opus_native(pcm16, sample_rate), "opus"

    if fmt == "mp3":
        return _encode_mp3_native(pcm16, sample_rate), "mp3"

    raise SynthesisError(f"unsupported response_format: {fmt}", code=5)


async def encode_audio_async(pcm16: bytes, sample_rate: int, fmt: str) -> tuple[bytes, str]:
    fmt = (fmt or "pcm").lower()

    if fmt in ("pcm", "s16le"):
        return pcm16, "pcm"

    if fmt == "wav":
        buf = io.BytesIO()
        data = np.frombuffer(pcm16, dtype=np.int16)
        sf.write(buf, data, samplerate=sample_rate, format="WAV", subtype="PCM_16")
        return buf.getvalue(), "wav"

    if fmt == "flac":
        buf = io.BytesIO()
        data = np.frombuffer(pcm16, dtype=np.int16)
        sf.write(buf, data, samplerate=sample_rate, format="FLAC", subtype="PCM_16")
        return buf.getvalue(), "flac"

    if fmt == "opus":
        loop = asyncio.get_running_loop()
        result = await loop.run_in_executor(None, _encode_opus_native, pcm16, sample_rate)
        return result, "opus"

    if fmt == "mp3":
        loop = asyncio.get_running_loop()
        result = await loop.run_in_executor(None, _encode_mp3_native, pcm16, sample_rate)
        return result, "mp3"

    raise SynthesisError(f"unsupported response_format: {fmt}", code=5)
