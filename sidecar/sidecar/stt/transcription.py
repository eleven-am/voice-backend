from __future__ import annotations

import asyncio
import logging
from concurrent.futures import ThreadPoolExecutor

import numpy as np
from numpy.typing import NDArray

from sidecar.audio.preprocessing import preprocess_audio
from sidecar.domain.exceptions import TranscriptionError
from sidecar.domain.transcript_processor import merge_transcripts
from sidecar.domain.types import Transcript
from sidecar.shared.utils import is_oom_error
from sidecar.stt.engine_manager import MAX_OOM_RETRIES, STTEngineManager

logger = logging.getLogger(__name__)


class TranscriptionService:
    def __init__(
        self,
        engine_manager: STTEngineManager,
        executor: ThreadPoolExecutor | None = None,
    ) -> None:
        self._engine_manager = engine_manager
        self._executor = executor

    def _transcribe_sync(
        self,
        audio: NDArray[np.float32],
        language: str | None = None,
        word_timestamps: bool = False,
    ) -> Transcript:
        last_error: Exception | None = None
        for attempt in range(MAX_OOM_RETRIES):
            try:
                engine_wrapper = self._engine_manager.get_engine()
                with engine_wrapper as engine:
                    return engine.transcribe(
                        audio=audio,
                        language=language,
                        word_timestamps=word_timestamps,
                    )
            except Exception as e:
                if is_oom_error(e):
                    logger.warning(f"OOM error on attempt {attempt + 1}/{MAX_OOM_RETRIES}: {e}")
                    last_error = e
                    if not self._engine_manager.try_fallback():
                        raise TranscriptionError(f"OOM error with no fallback available: {e}") from e
                    continue
                raise
        raise TranscriptionError(f"Failed after {MAX_OOM_RETRIES} OOM retries: {last_error}") from last_error

    def transcribe_with_retry(
        self,
        audio: NDArray[np.float32],
        language: str | None = None,
        word_timestamps: bool = False,
    ) -> Transcript:
        return self._transcribe_sync(audio, language, word_timestamps)

    async def transcribe_async(
        self,
        audio: NDArray[np.float32],
        language: str | None = None,
        word_timestamps: bool = False,
    ) -> Transcript:
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(
            self._executor,
            lambda: self._transcribe_sync(audio, language, word_timestamps),
        )

    def transcribe_encoded(
        self,
        data: bytes,
        fmt: str | None,
        language: str | None = None,
        word_timestamps: bool = False,
    ) -> Transcript:
        chunks = preprocess_audio(data, fmt)
        total_duration_ms = sum(c.duration_ms for c in chunks)
        logger.info(f"Transcribing encoded audio: {total_duration_ms}ms ({len(chunks)} chunk(s))")

        if len(chunks) == 1:
            return self.transcribe_with_retry(
                audio=chunks[0].data,
                language=language,
                word_timestamps=word_timestamps,
            )

        transcripts_with_offsets: list[tuple[Transcript, float]] = []
        for i, chunk in enumerate(chunks):
            offset_s = chunk.offset_ms / 1000.0
            logger.info(f"Transcribing chunk {i + 1}/{len(chunks)} (offset: {offset_s:.1f}s)")
            transcript = self.transcribe_with_retry(
                audio=chunk.data,
                language=language,
                word_timestamps=word_timestamps,
            )
            transcripts_with_offsets.append((transcript, offset_s))

        merged = merge_transcripts(transcripts_with_offsets)
        logger.info(f"All {len(chunks)} chunks transcribed")
        return merged

    async def transcribe_encoded_async(
        self,
        data: bytes,
        fmt: str | None,
        language: str | None = None,
        word_timestamps: bool = False,
    ) -> Transcript:
        chunks = preprocess_audio(data, fmt)
        total_duration_ms = sum(c.duration_ms for c in chunks)
        logger.info(f"Transcribing encoded audio: {total_duration_ms}ms ({len(chunks)} chunk(s))")

        if len(chunks) == 1:
            return await self.transcribe_async(
                audio=chunks[0].data,
                language=language,
                word_timestamps=word_timestamps,
            )

        transcripts_with_offsets: list[tuple[Transcript, float]] = []
        for i, chunk in enumerate(chunks):
            offset_s = chunk.offset_ms / 1000.0
            logger.info(f"Transcribing chunk {i + 1}/{len(chunks)} (offset: {offset_s:.1f}s)")
            transcript = await self.transcribe_async(
                audio=chunk.data,
                language=language,
                word_timestamps=word_timestamps,
            )
            transcripts_with_offsets.append((transcript, offset_s))

        merged = merge_transcripts(transcripts_with_offsets)
        logger.info(f"All {len(chunks)} chunks transcribed")
        return merged
