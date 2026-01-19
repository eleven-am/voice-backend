from __future__ import annotations

import asyncio
import logging
import threading
import time
from typing import TYPE_CHECKING, Iterator

import numpy as np

from .tts_models import SAMPLE_RATE
from .utils import chunk_text, is_oom_error

if TYPE_CHECKING:
    from sidecar.tts_models import KokoroModelManager, SynthesisConfig

logger = logging.getLogger(__name__)


def run_async_in_thread(coro):
    loop = asyncio.new_event_loop()
    try:
        return loop.run_until_complete(coro)
    finally:
        loop.close()


class SynthesisError(Exception):
    def __init__(self, message: str, code: int = 1):
        super().__init__(message)
        self.code = code


class Synthesizer:
    def __init__(self, model_manager: KokoroModelManager, config: SynthesisConfig) -> None:
        self.model_manager = model_manager
        self.config = config

    def synthesize(
        self,
        text: str,
        voice_id: str,
        speed: float | None = None,
        stop_event: threading.Event | None = None,
    ) -> Iterator[np.ndarray]:
        if not text.strip():
            return

        if speed is None:
            speed = self.config.speed

        speed = max(0.5, min(2.0, speed))

        voice_lang = self.model_manager.get_voice_lang(voice_id)
        text_chunks = chunk_text(text)

        start = time.perf_counter()
        used_cpu_fallback = False

        async def collect_audio(kokoro, chunk_text: str):
            audio_chunks = []
            async for audio_chunk, _ in kokoro.create_stream(chunk_text, voice_id, lang=voice_lang, speed=speed):
                if stop_event and stop_event.is_set():
                    break
                audio_chunks.append(audio_chunk)
            return audio_chunks

        for text_chunk in text_chunks:
            if stop_event and stop_event.is_set():
                break

            try:
                with self.model_manager as kokoro:
                    if kokoro is None:
                        raise SynthesisError("Model not loaded", code=2)

                    audio_chunks = run_async_in_thread(collect_audio(kokoro, text_chunk))
                    for audio_chunk in audio_chunks:
                        yield audio_chunk
            except Exception as e:
                if is_oom_error(e) and self.model_manager.config.fallback_to_cpu:
                    logger.warning(f"TTS OOM error, using CPU for this request: {e}")
                    used_cpu_fallback = True
                    try:
                        cpu_kokoro = self.model_manager.get_cpu_model()
                        audio_chunks = run_async_in_thread(collect_audio(cpu_kokoro, text_chunk))
                        for audio_chunk in audio_chunks:
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
