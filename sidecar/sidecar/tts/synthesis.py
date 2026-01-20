from __future__ import annotations

import asyncio
import logging
import time
from typing import TYPE_CHECKING, AsyncIterator

import numpy as np

from sidecar.tts.model_manager import SAMPLE_RATE
from sidecar.shared.utils import chunk_text, is_oom_error

if TYPE_CHECKING:
    from sidecar.tts.model_manager import KokoroModelManager, SynthesisConfig

logger = logging.getLogger(__name__)


class SynthesisError(Exception):
    def __init__(self, message: str, code: int = 1):
        super().__init__(message)
        self.code = code


class Synthesizer:
    def __init__(self, model_manager: KokoroModelManager, config: SynthesisConfig) -> None:
        self.model_manager = model_manager
        self.config = config

    async def synthesize(
        self,
        text: str,
        voice_id: str,
        speed: float | None = None,
        stop_event: asyncio.Event | None = None,
    ) -> AsyncIterator[np.ndarray]:
        if not text.strip():
            return

        if speed is None:
            speed = self.config.speed

        if speed < 0.5 or speed > 2.0:
            raise SynthesisError(f"Speed {speed} out of range (0.5-2.0)", code=6)

        voice_lang = self.model_manager.get_voice_lang(voice_id)
        text_chunks = chunk_text(text)

        start = time.perf_counter()
        used_cpu_fallback = False

        for text_chunk in text_chunks:
            if stop_event and stop_event.is_set():
                break

            try:
                kokoro = await self.model_manager.get_kokoro()

                async for audio_chunk, _ in kokoro.create_stream(
                    text_chunk, voice_id, lang=voice_lang, speed=speed
                ):
                    if stop_event and stop_event.is_set():
                        break
                    yield audio_chunk

            except Exception as e:
                if is_oom_error(e) and self.model_manager.config.fallback_to_cpu:
                    logger.warning(f"TTS OOM error, using CPU for this request: {e}")
                    used_cpu_fallback = True
                    try:
                        cpu_kokoro = await self.model_manager.get_cpu_model()
                        async for audio_chunk, _ in cpu_kokoro.create_stream(
                            text_chunk, voice_id, lang=voice_lang, speed=speed
                        ):
                            if stop_event and stop_event.is_set():
                                break
                            yield audio_chunk
                    except Exception as cpu_e:
                        raise SynthesisError(f"CPU fallback synthesis failed: {cpu_e}", code=3) from cpu_e
                else:
                    raise SynthesisError(f"Synthesis failed: {e}", code=3) from e

        device = "CPU" if used_cpu_fallback else "GPU"
        logger.info(f"Synthesized {len(text)} chars ({len(text_chunks)} chunks) in {time.perf_counter() - start:.2f}s on {device}")


def float32_to_pcm16(audio: np.ndarray) -> bytes:
    audio = np.clip(audio, -1.0, 1.0)
    pcm = (audio * 32767.0).astype(np.int16)
    return pcm.tobytes()
