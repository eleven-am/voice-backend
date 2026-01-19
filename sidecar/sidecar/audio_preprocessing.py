from __future__ import annotations

import io
import logging
from dataclasses import dataclass

import numpy as np
from pydub import AudioSegment
from pydub.exceptions import CouldntDecodeError

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


def decode_audio(audio_bytes: bytes, source_format: str | None = None) -> np.ndarray:
    buffer = io.BytesIO(audio_bytes)
    try:
        if source_format:
            audio = AudioSegment.from_file(buffer, format=source_format)
        else:
            audio = AudioSegment.from_file(buffer)
    except CouldntDecodeError as e:
        raise ValueError(f"Failed to decode audio: {e}") from e
    except Exception as e:
        raise ValueError(f"Failed to process audio: {e}") from e

    audio = audio.set_frame_rate(TARGET_SAMPLE_RATE).set_channels(TARGET_CHANNELS)
    samples = np.array(audio.get_array_of_samples(), dtype=np.int16)
    return samples.astype(np.float32) / 32768.0


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
