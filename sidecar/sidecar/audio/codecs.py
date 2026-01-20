from __future__ import annotations

import subprocess
from io import BytesIO

import numpy as np
import soxr
from numpy.typing import NDArray

from sidecar.domain.constants import TARGET_SAMPLE_RATE
from sidecar.domain.exceptions import TranscriptionError


def decode_encoded_audio(data: bytes, fmt: str | None = None) -> bytes:
    try:
        import soundfile as sf

        audio_data, sr = sf.read(BytesIO(data), dtype="int16", format=fmt or None)
        if sr != TARGET_SAMPLE_RATE:
            audio_data = soxr.resample(audio_data.astype(np.float32), sr, TARGET_SAMPLE_RATE).astype(np.int16)
        return audio_data.tobytes()
    except Exception:
        pass

    cmd = [
        "ffmpeg",
        "-hide_banner",
        "-loglevel",
        "error",
    ]
    if fmt:
        cmd += ["-f", fmt]
    cmd += [
        "-i",
        "pipe:0",
        "-f",
        "s16le",
        "-ar",
        str(TARGET_SAMPLE_RATE),
        "-ac",
        "1",
        "pipe:1",
    ]
    proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    stdout, stderr = proc.communicate(input=data)
    if proc.returncode != 0:
        raise TranscriptionError(f"ffmpeg decode error: {stderr.decode().strip()}")
    return stdout


def pcm16_to_float32(pcm_bytes: bytes) -> NDArray[np.float32]:
    pcm_array = np.frombuffer(pcm_bytes, dtype=np.int16)
    return pcm_array.astype(np.float32) / 32768.0


def resample_audio(audio: NDArray[np.float32], source_rate: int, target_rate: int) -> NDArray[np.float32]:
    if source_rate == target_rate:
        return audio
    return soxr.resample(audio, source_rate, target_rate).astype(np.float32)
