from __future__ import annotations

import asyncio
import io
import logging
import os
import threading
import time
from collections.abc import AsyncIterator
from dataclasses import dataclass, field
from pathlib import Path
from typing import TYPE_CHECKING

import numpy as np
import soundfile as sf
import torch

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)

SAMPLE_RATE = 24000


@dataclass
class ClonedVoice:
    voice_id: str
    name: str
    language: str
    gender: str
    ref_audio: np.ndarray
    ref_audio_sr: int
    ref_text: str


@dataclass
class Qwen3TTSConfig:
    model_id: str = field(
        default_factory=lambda: os.environ.get("TTS_MODEL_ID", "Qwen/Qwen3-TTS-12Hz-0.6B-Base")
    )
    device: str = "cuda"
    ttl: int = 300
    fallback_to_cpu: bool = True
    voices_dir: str = field(
        default_factory=lambda: os.environ.get("VOICES_DIR", "/tmp/qwen3_voices")
    )


@dataclass
class Qwen3SynthesisConfig:
    speed: float = 1.0


class Qwen3ModelManager:
    def __init__(self, config: Qwen3TTSConfig) -> None:
        self.config = config
        self._model = None
        self._cpu_model = None
        self._sync_lock = threading.RLock()
        self._async_lock: asyncio.Lock | None = None
        self._last_used: float = 0
        self._cleanup_task: asyncio.Task | None = None
        self._cleanup_interval: float = 30.0
        self._cloned_voices: dict[str, ClonedVoice] = {}
        self._voices_lock = threading.Lock()

        Path(self.config.voices_dir).mkdir(parents=True, exist_ok=True)
        self._load_saved_voices()

    def _load_saved_voices(self) -> None:
        voices_path = Path(self.config.voices_dir)
        if not voices_path.exists():
            return

        for voice_dir in voices_path.iterdir():
            if not voice_dir.is_dir():
                continue

            meta_path = voice_dir / "meta.txt"
            audio_path = voice_dir / "ref.wav"

            if not meta_path.exists() or not audio_path.exists():
                continue

            try:
                lines = meta_path.read_text().strip().split("\n")
                if len(lines) < 4:
                    continue

                voice_id = voice_dir.name
                name = lines[0]
                language = lines[1]
                gender = lines[2]
                ref_text = lines[3]

                audio_data, sr = sf.read(str(audio_path))
                self._cloned_voices[voice_id] = ClonedVoice(
                    voice_id=voice_id,
                    name=name,
                    language=language,
                    gender=gender,
                    ref_audio=audio_data.astype(np.float32),
                    ref_audio_sr=sr,
                    ref_text=ref_text,
                )
                logger.info(f"Loaded cloned voice: {voice_id}")
            except Exception as e:
                logger.warning(f"Failed to load voice {voice_dir.name}: {e}")

    def _get_async_lock(self) -> asyncio.Lock:
        if self._async_lock is None:
            self._async_lock = asyncio.Lock()
        return self._async_lock

    def _load_model_sync(self, device: str = None):
        if device is None:
            device = self.config.device

        logger.info(f"Loading Qwen3-TTS model {self.config.model_id} on {device}")

        from qwen_tts import Qwen3TTSModel

        dtype = torch.bfloat16 if device == "cuda" else torch.float32

        model = Qwen3TTSModel.from_pretrained(
            self.config.model_id,
            device_map=device,
            dtype=dtype,
        )

        logger.info(f"Qwen3-TTS model loaded on {device}")
        return model

    async def get_model(self):
        async with self._get_async_lock():
            if self._model is not None:
                self._last_used = time.monotonic()
                return self._model

            loop = asyncio.get_running_loop()
            self._model = await loop.run_in_executor(None, self._load_model_sync)
            self._last_used = time.monotonic()
            return self._model

    async def get_cpu_model(self):
        async with self._get_async_lock():
            if self._cpu_model is not None:
                return self._cpu_model

            loop = asyncio.get_running_loop()
            self._cpu_model = await loop.run_in_executor(
                None, lambda: self._load_model_sync("cpu")
            )
            return self._cpu_model

    def list_voices(self) -> list[str]:
        with self._voices_lock:
            return list(self._cloned_voices.keys())

    def get_voice(self, voice_id: str) -> ClonedVoice | None:
        with self._voices_lock:
            return self._cloned_voices.get(voice_id)

    def get_voice_lang(self, voice_id: str) -> str:
        voice = self.get_voice(voice_id)
        return voice.language if voice else "English"

    def create_voice(
        self,
        voice_id: str,
        audio_data: bytes,
        name: str,
        language: str,
        gender: str,
        ref_text: str,
    ) -> ClonedVoice:
        audio_array, sr = sf.read(io.BytesIO(audio_data))
        audio_array = audio_array.astype(np.float32)

        voice = ClonedVoice(
            voice_id=voice_id,
            name=name,
            language=language,
            gender=gender,
            ref_audio=audio_array,
            ref_audio_sr=sr,
            ref_text=ref_text,
        )

        voice_dir = Path(self.config.voices_dir) / voice_id
        voice_dir.mkdir(parents=True, exist_ok=True)

        sf.write(str(voice_dir / "ref.wav"), audio_array, sr)
        (voice_dir / "meta.txt").write_text(f"{name}\n{language}\n{gender}\n{ref_text}")

        with self._voices_lock:
            self._cloned_voices[voice_id] = voice

        logger.info(f"Created cloned voice: {voice_id}")
        return voice

    def delete_voice(self, voice_id: str) -> bool:
        with self._voices_lock:
            if voice_id not in self._cloned_voices:
                return False
            del self._cloned_voices[voice_id]

        voice_dir = Path(self.config.voices_dir) / voice_id
        if voice_dir.exists():
            import shutil
            shutil.rmtree(voice_dir)

        logger.info(f"Deleted cloned voice: {voice_id}")
        return True

    def preload(self) -> None:
        with self._sync_lock:
            if self._model is not None:
                return
            self._model = self._load_model_sync()
            self._last_used = time.monotonic()

    async def synthesize_stream(
        self,
        text: str,
        voice_id: str,
        speed: float = 1.0,
        use_cpu: bool = False,
    ) -> AsyncIterator[np.ndarray]:
        voice = self.get_voice(voice_id)
        if voice is None:
            raise ValueError(f"Voice '{voice_id}' not found. Create it first using CreateVoice.")

        model = await (self.get_cpu_model() if use_cpu else self.get_model())

        loop = asyncio.get_running_loop()

        ref_audio_tuple = (voice.ref_audio, voice.ref_audio_sr)

        def _generate():
            wavs, sr = model.generate_voice_clone(
                text=text,
                language=voice.language,
                ref_audio=ref_audio_tuple,
                ref_text=voice.ref_text,
                max_new_tokens=2048,
                do_sample=True,
                top_k=50,
                top_p=1.0,
                temperature=0.9,
                repetition_penalty=1.05,
            )
            return wavs[0] if isinstance(wavs, list) else wavs, sr

        audio, sr = await loop.run_in_executor(None, _generate)

        if isinstance(audio, torch.Tensor):
            audio = audio.cpu().numpy()

        if audio.ndim > 1:
            audio = audio.squeeze()

        chunk_size = sr // 4
        for i in range(0, len(audio), chunk_size):
            yield audio[i:i + chunk_size].astype(np.float32)

    def _unload_model(self) -> None:
        if self._model is not None:
            del self._model
            self._model = None
            if torch.cuda.is_available():
                torch.cuda.empty_cache()
            logger.info("Unloaded Qwen3-TTS model due to TTL")

    async def _cleanup_loop(self) -> None:
        while True:
            await asyncio.sleep(self._cleanup_interval)
            now = time.monotonic()

            if self.config.ttl > 0 and self._model is not None:
                async with self._get_async_lock():
                    if self._model is not None:
                        idle_time = now - self._last_used
                        if idle_time >= self.config.ttl:
                            self._unload_model()

    def start_cleanup_task(self) -> None:
        if self._cleanup_task is not None:
            return

        if self.config.ttl <= 0:
            logger.info("TTS TTL disabled, cleanup task not started")
            return

        self._cleanup_task = asyncio.create_task(self._cleanup_loop())
        logger.info(f"Started Qwen3-TTS model cleanup task (TTL: {self.config.ttl}s)")

    def stop_cleanup_task(self) -> None:
        if self._cleanup_task is not None:
            self._cleanup_task.cancel()
            self._cleanup_task = None
            logger.info("Stopped Qwen3-TTS model cleanup task")

    def unload_all(self) -> None:
        self._unload_model()
        if self._cpu_model is not None:
            del self._cpu_model
            self._cpu_model = None
            logger.info("Unloaded Qwen3-TTS CPU fallback model")
