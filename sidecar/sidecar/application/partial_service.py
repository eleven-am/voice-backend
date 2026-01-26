from __future__ import annotations

import logging
from collections.abc import Awaitable, Callable

import numpy as np
from numpy.typing import NDArray

from sidecar.domain.constants import TARGET_SAMPLE_RATE, samples_to_ms
from sidecar.domain.entities import SpeechSession
from sidecar.domain.transcript_processor import deduplicate_words
from sidecar.domain.types import SessionConfig, Transcript
from sidecar.stt.vad import SpeechSegment

logger = logging.getLogger(__name__)

PARTIAL_OVERLAP_MS = 300

TranscribeFn = Callable[[NDArray[np.float32], str | None, bool], Transcript]
AsyncTranscribeFn = Callable[[NDArray[np.float32], str | None, bool], Awaitable[Transcript]]


class PartialTranscriptService:
    def __init__(
        self,
        transcribe_fn: TranscribeFn,
        transcribe_async_fn: AsyncTranscribeFn | None = None,
    ) -> None:
        self._transcribe_fn = transcribe_fn
        self._transcribe_async_fn = transcribe_async_fn

    def generate_partial(
        self,
        session: SpeechSession,
        config: SessionConfig,
    ) -> Transcript | None:
        buffer_copy = session.get_buffer_copy()
        if not buffer_copy:
            return None

        buffer_audio = np.concatenate(buffer_copy)
        buf_ms = samples_to_ms(len(buffer_audio))

        partial_window_ms = config.partial_window_ms
        partial_stride_ms = config.partial_stride_ms

        last_partial_ms, confirmed_words = session.get_partial_state()

        if buf_ms - last_partial_ms < partial_stride_ms or buf_ms < partial_window_ms:
            return None

        tail_window_ms = partial_window_ms + PARTIAL_OVERLAP_MS
        tail_samples = int(tail_window_ms * TARGET_SAMPLE_RATE / 1000)

        if len(buffer_audio) > tail_samples:
            tail_audio = buffer_audio[-tail_samples:]
            tail_start_ms = buf_ms - tail_window_ms
        else:
            tail_audio = buffer_audio
            tail_start_ms = 0

        segment = SpeechSegment(
            audio=tail_audio,
            start_ms=tail_start_ms,
            end_ms=buf_ms,
        )

        transcript = self._transcribe_fn(
            segment.audio,
            config.language,
            config.include_word_timestamps,
        )

        new_text, updated_words = deduplicate_words(transcript.text, confirmed_words)
        session.update_partial(buf_ms, updated_words)

        if new_text:
            transcript.text = new_text
            return transcript
        return None

    async def generate_partial_async(
        self,
        session: SpeechSession,
        config: SessionConfig,
    ) -> Transcript | None:
        if self._transcribe_async_fn is None:
            raise RuntimeError("Async transcribe function not provided")

        buffer_copy = session.get_buffer_copy()
        if not buffer_copy:
            return None

        buffer_audio = np.concatenate(buffer_copy)
        buf_ms = samples_to_ms(len(buffer_audio))

        partial_window_ms = config.partial_window_ms
        partial_stride_ms = config.partial_stride_ms

        last_partial_ms, confirmed_words = session.get_partial_state()

        if buf_ms - last_partial_ms < partial_stride_ms or buf_ms < partial_window_ms:
            return None

        tail_window_ms = partial_window_ms + PARTIAL_OVERLAP_MS
        tail_samples = int(tail_window_ms * TARGET_SAMPLE_RATE / 1000)

        if len(buffer_audio) > tail_samples:
            tail_audio = buffer_audio[-tail_samples:]
            tail_start_ms = buf_ms - tail_window_ms
        else:
            tail_audio = buffer_audio
            tail_start_ms = 0

        segment = SpeechSegment(
            audio=tail_audio,
            start_ms=tail_start_ms,
            end_ms=buf_ms,
        )

        transcript = await self._transcribe_async_fn(
            segment.audio,
            config.language,
            config.include_word_timestamps,
        )

        new_text, updated_words = deduplicate_words(transcript.text, confirmed_words)
        session.update_partial(buf_ms, updated_words)

        if new_text:
            transcript.text = new_text
            return transcript
        return None

    def flush_remaining_audio(self, session: SpeechSession) -> NDArray[np.float32] | None:
        buffer_copy = session.get_buffer_copy()
        if buffer_copy:
            return np.concatenate(buffer_copy)
        return None
