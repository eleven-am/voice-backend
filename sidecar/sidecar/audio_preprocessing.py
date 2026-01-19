from __future__ import annotations

import io
import logging
import subprocess
from dataclasses import dataclass

import numpy as np
import soxr

logger = logging.getLogger(__name__)

TARGET_SAMPLE_RATE = 16000
TARGET_CHANNELS = 1
CHUNK_DURATION_MS = 5 * 60 * 1000


@dataclass
class AudioChunk:
    data: np.ndarray
    sample_rate: int
    duration_ms: int
    offset_ms: int


def _detect_format(filename: str | None) -> str | None:
    if not filename:
        return None
    ext = filename.rsplit(".", 1)[-1].lower()
    if ext == "m4a":
        return "mp4"
    return ext if ext in ("mp3", "wav", "ogg", "flac", "aac", "opus", "webm") else None


def _decode_with_soundfile(audio_bytes: bytes, source_format: str | None) -> np.ndarray | None:
    try:
        import soundfile as sf
        data, sr = sf.read(io.BytesIO(audio_bytes), dtype="float32")
        if len(data.shape) > 1:
            data = data.mean(axis=1)
        if sr != TARGET_SAMPLE_RATE:
            data = soxr.resample(data, sr, TARGET_SAMPLE_RATE)
        return data.astype(np.float32)
    except Exception:
        return None


def _decode_with_ffmpeg(audio_bytes: bytes, source_format: str | None) -> np.ndarray:
    cmd = ["ffmpeg", "-hide_banner", "-loglevel", "error"]
    if source_format:
        cmd += ["-f", source_format]
    cmd += [
        "-i", "pipe:0",
        "-f", "f32le",
        "-ar", str(TARGET_SAMPLE_RATE),
        "-ac", "1",
        "pipe:1",
    ]
    proc = subprocess.Popen(
        cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE
    )
    try:
        stdout, stderr = proc.communicate(input=audio_bytes, timeout=300)
    except subprocess.TimeoutExpired as e:
        proc.kill()
        proc.communicate()
        raise ValueError("ffmpeg decode timed out after 300 seconds") from e
    if proc.returncode != 0:
        raise ValueError(f"ffmpeg decode error: {stderr.decode().strip()}")
    return np.frombuffer(stdout, dtype=np.float32)


def decode_audio(audio_bytes: bytes, source_format: str | None = None) -> np.ndarray:
    audio = _decode_with_soundfile(audio_bytes, source_format)
    if audio is not None:
        return audio
    return _decode_with_ffmpeg(audio_bytes, source_format)


def chunk_audio(
    audio: np.ndarray,
    sample_rate: int = TARGET_SAMPLE_RATE,
    chunk_duration_ms: int = CHUNK_DURATION_MS,
) -> list[AudioChunk]:
    total_samples = len(audio)
    total_duration_ms = int(total_samples / sample_rate * 1000)

    if total_duration_ms <= chunk_duration_ms:
        return [
            AudioChunk(
                data=audio,
                sample_rate=sample_rate,
                duration_ms=total_duration_ms,
                offset_ms=0,
            )
        ]

    chunk_samples = int(chunk_duration_ms * sample_rate / 1000)
    chunks = []
    offset_samples = 0

    while offset_samples < total_samples:
        end_samples = min(offset_samples + chunk_samples, total_samples)
        segment = audio[offset_samples:end_samples]
        segment_duration_ms = int(len(segment) / sample_rate * 1000)
        offset_ms = int(offset_samples / sample_rate * 1000)

        chunks.append(
            AudioChunk(
                data=segment,
                sample_rate=sample_rate,
                duration_ms=segment_duration_ms,
                offset_ms=offset_ms,
            )
        )
        offset_samples = end_samples

    logger.info(f"Split audio ({total_duration_ms / 1000:.1f}s) into {len(chunks)} chunks")
    return chunks


def preprocess_audio(audio_bytes: bytes, filename: str | None = None) -> list[AudioChunk]:
    source_format = _detect_format(filename)
    audio = decode_audio(audio_bytes, source_format)
    duration_s = len(audio) / TARGET_SAMPLE_RATE
    logger.info(f"Audio duration: {duration_s:.1f}s, converted to {TARGET_SAMPLE_RATE}Hz mono")
    return chunk_audio(audio)
