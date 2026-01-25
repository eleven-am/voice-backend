from __future__ import annotations

import asyncio
import logging
import time
from typing import TYPE_CHECKING, AsyncIterator

import numpy as np

from sidecar.tts.chatterbox_model_manager import SAMPLE_RATE
from sidecar.shared.utils import chunk_text, is_oom_error

if TYPE_CHECKING:
    from sidecar.tts.chatterbox_model_manager import ChatterboxModelManager, SynthesisConfig
    from sidecar.tts.voice_store import VoiceStore

logger = logging.getLogger(__name__)

AUDIO_CHUNK_SIZE = 4096


class SynthesisError(Exception):
    def __init__(self, message: str, code: int = 1):
        super().__init__(message)
        self.code = code


class Synthesizer:
    def __init__(
        self,
        model_manager: ChatterboxModelManager,
        config: SynthesisConfig,
        voice_store: VoiceStore | None = None,
    ) -> None:
        self.model_manager = model_manager
        self.config = config
        self.voice_store = voice_store

    async def _generate_audio(
        self,
        model,
        text: str,
        voice_path: str | None,
    ) -> np.ndarray:
        loop = asyncio.get_running_loop()

        def run_generation():
            wav = model.generate(
                text=text,
                audio_prompt_path=voice_path,
                exaggeration=self.config.exaggeration,
                cfg_weight=self.config.cfg_weight,
            )
            return wav.cpu().numpy().squeeze()

        return await loop.run_in_executor(None, run_generation)

    async def synthesize(
        self,
        text: str,
        voice_id: str | None = None,
        speed: float | None = None,
        stop_event: asyncio.Event | None = None,
    ) -> AsyncIterator[np.ndarray]:
        if not text.strip():
            return

        if speed is None:
            speed = self.config.speed

        if speed < 0.5 or speed > 2.0:
            raise SynthesisError(f"Speed {speed} out of range (0.5-2.0)", code=6)

        voice_path: str | None = None
        if self.voice_store and voice_id and voice_id != "default":
            voice_path = await self.voice_store.get_voice_path(voice_id)

        text_chunks = chunk_text(text)

        start = time.perf_counter()
        used_cpu_fallback = False

        for text_chunk in text_chunks:
            if stop_event and stop_event.is_set():
                break

            try:
                model = await self.model_manager.get_model()
                audio = await self._generate_audio(model, text_chunk, voice_path)

                for i in range(0, len(audio), AUDIO_CHUNK_SIZE):
                    if stop_event and stop_event.is_set():
                        break
                    yield audio[i:i + AUDIO_CHUNK_SIZE]

            except Exception as e:
                if is_oom_error(e) and self.model_manager.config.fallback_to_cpu:
                    logger.warning(f"TTS OOM error, using CPU for this request: {e}")
                    used_cpu_fallback = True
                    try:
                        cpu_model = await self.model_manager.get_cpu_model()
                        audio = await self._generate_audio(cpu_model, text_chunk, voice_path)

                        for i in range(0, len(audio), AUDIO_CHUNK_SIZE):
                            if stop_event and stop_event.is_set():
                                break
                            yield audio[i:i + AUDIO_CHUNK_SIZE]

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
