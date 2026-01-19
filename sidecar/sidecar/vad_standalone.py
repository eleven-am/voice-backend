from __future__ import annotations

import logging
from dataclasses import dataclass, field

import numpy as np
import torch
from numpy.typing import NDArray

from sidecar.types import SpeechStarted, SpeechStopped

logger = logging.getLogger(__name__)

SAMPLE_RATE = 16000
MS_SAMPLE_RATE = SAMPLE_RATE // 1000
VAD_WINDOW_SIZE_SAMPLES = 3000 * MS_SAMPLE_RATE


@dataclass
class VADConfig:
    threshold: float = 0.6
    min_silence_duration_ms: int = 500
    speech_pad_ms: int = 100
    min_speech_duration_ms: int = 250
    min_audio_duration_ms: int = 300
    max_utterance_ms: int = 15000


@dataclass
class VADState:
    audio_start_ms: int | None = None
    audio_end_ms: int | None = None


@dataclass
class SpeechSegment:
    audio: NDArray[np.float32]
    start_ms: int
    end_ms: int


class SileroVAD:
    _instance: SileroVAD | None = None
    _model = None
    _get_speech_timestamps = None
    _lock = __import__('threading').Lock()

    def __new__(cls) -> SileroVAD:
        with cls._lock:
            if cls._instance is None:
                cls._instance = super().__new__(cls)
            return cls._instance

    def _ensure_model(self) -> None:
        if SileroVAD._model is not None:
            return
        with SileroVAD._lock:
            if SileroVAD._model is None:
                logger.info("Loading Silero VAD model from torch.hub")
                model, utils = torch.hub.load(
                    repo_or_dir="snakers4/silero-vad",
                    model="silero_vad",
                    force_reload=False,
                    onnx=False,
                )
                SileroVAD._model = model
                SileroVAD._get_speech_timestamps = utils[0]
                logger.info("Silero VAD model loaded")

    def get_speech_timestamps(
        self,
        audio: NDArray[np.float32],
        threshold: float = 0.5,
        min_silence_duration_ms: int = 500,
        speech_pad_ms: int = 100,
        min_speech_duration_ms: int = 250,
    ) -> list[dict[str, int]]:
        self._ensure_model()

        audio_tensor = torch.from_numpy(audio)
        timestamps = SileroVAD._get_speech_timestamps(
            audio_tensor,
            SileroVAD._model,
            threshold=threshold,
            min_silence_duration_ms=min_silence_duration_ms,
            speech_pad_ms=speech_pad_ms,
            min_speech_duration_ms=min_speech_duration_ms,
            sampling_rate=SAMPLE_RATE,
        )
        return timestamps


@dataclass
class VADProcessor:
    config: VADConfig = field(default_factory=VADConfig)
    state: VADState = field(default_factory=VADState)
    buffer: NDArray[np.float32] = field(default_factory=lambda: np.array([], dtype=np.float32))
    _vad_model: SileroVAD = field(default_factory=SileroVAD)

    def _duration_ms(self) -> int:
        return len(self.buffer) // MS_SAMPLE_RATE

    def append(self, audio: NDArray[np.float32]) -> tuple[SpeechStarted | SpeechStopped | None, SpeechSegment | None]:
        self.buffer = np.append(self.buffer, audio)

        audio_window = self.buffer[-VAD_WINDOW_SIZE_SAMPLES:]
        window_duration_ms = len(audio_window) // MS_SAMPLE_RATE

        raw_timestamps = self._vad_model.get_speech_timestamps(
            audio_window,
            threshold=self.config.threshold,
            min_silence_duration_ms=self.config.min_silence_duration_ms,
            speech_pad_ms=self.config.speech_pad_ms,
            min_speech_duration_ms=self.config.min_speech_duration_ms,
        )

        speech_ts = None
        if raw_timestamps:
            merged = {
                "start": min(ts["start"] for ts in raw_timestamps),
                "end": max(ts["end"] for ts in raw_timestamps),
            }
            speech_ts = merged

        if self.state.audio_start_ms is None:
            if speech_ts is None:
                return None, None

            self.state.audio_start_ms = (
                self._duration_ms() - window_duration_ms + (speech_ts["start"] // MS_SAMPLE_RATE)
            )
            return SpeechStarted(timestamp_ms=self.state.audio_start_ms), None

        else:
            if speech_ts is None:
                self.state.audio_end_ms = self._duration_ms() - self.config.speech_pad_ms
                segment = self._extract_segment()
                self._clear_buffer()
                if segment.end_ms - segment.start_ms < self.config.min_audio_duration_ms:
                    logger.debug(f"Segment too short ({segment.end_ms - segment.start_ms}ms), skipping STT")
                    return SpeechStopped(timestamp_ms=self.state.audio_end_ms), None
                return SpeechStopped(timestamp_ms=self.state.audio_end_ms), segment

            if self._duration_ms() >= self.config.max_utterance_ms:
                self.state.audio_end_ms = self._duration_ms() - self.config.speech_pad_ms
                segment = self._extract_segment()
                self._clear_buffer()
                if segment.end_ms - segment.start_ms < self.config.min_audio_duration_ms:
                    logger.debug(
                        f"Segment too short after max_utterance cap ({segment.end_ms - segment.start_ms}ms), skipping STT"
                    )
                    return SpeechStopped(timestamp_ms=self.state.audio_end_ms), None
                return SpeechStopped(timestamp_ms=self.state.audio_end_ms), segment

        return None, None

    def _extract_segment(self) -> SpeechSegment:
        if self.state.audio_start_ms is None or self.state.audio_end_ms is None:
            return SpeechSegment(audio=np.array([], dtype=np.float32), start_ms=0, end_ms=0)

        start_sample = self.state.audio_start_ms * MS_SAMPLE_RATE
        end_sample = self.state.audio_end_ms * MS_SAMPLE_RATE

        start_sample = max(0, min(start_sample, len(self.buffer)))
        end_sample = max(0, min(end_sample, len(self.buffer)))

        return SpeechSegment(
            audio=self.buffer[start_sample:end_sample].copy(),
            start_ms=self.state.audio_start_ms,
            end_ms=self.state.audio_end_ms,
        )

    def _clear_buffer(self) -> None:
        self.buffer = np.array([], dtype=np.float32)
        self.state = VADState()

    def reset(self) -> None:
        self._clear_buffer()
