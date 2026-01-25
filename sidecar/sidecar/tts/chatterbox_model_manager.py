from __future__ import annotations

import asyncio
import logging
import threading
import time
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

import torch

if TYPE_CHECKING:
    from chatterbox.tts import ChatterboxTTS

logger = logging.getLogger(__name__)

SAMPLE_RATE = 24000


@dataclass
class TTSConfig:
    device: str = "cuda"
    ttl: int = 300
    fallback_to_cpu: bool = True


@dataclass
class SynthesisConfig:
    speed: float = 1.0
    exaggeration: float = 0.5
    cfg_weight: float = 0.5


class ChatterboxModelManager:
    def __init__(self, config: TTSConfig) -> None:
        self.config = config
        self._model: ChatterboxTTS | None = None
        self._cpu_model: ChatterboxTTS | None = None
        self._sync_lock = threading.RLock()
        self._async_lock: asyncio.Lock | None = None
        self._last_used: float = 0
        self._cleanup_task: asyncio.Task | None = None
        self._cleanup_interval: float = 30.0

    def _get_async_lock(self) -> asyncio.Lock:
        if self._async_lock is None:
            self._async_lock = asyncio.Lock()
        return self._async_lock

    def _detect_device(self) -> str:
        if self.config.device == "cpu":
            return "cpu"
        if torch.cuda.is_available():
            return "cuda"
        if hasattr(torch.backends, "mps") and torch.backends.mps.is_available():
            return "mps"
        return "cpu"

    def _load_model_sync(self, device: str) -> ChatterboxTTS:
        from chatterbox.tts import ChatterboxTTS as ChatterboxTTSClass
        logger.info(f"Loading Chatterbox model on {device}")
        model = ChatterboxTTSClass.from_pretrained(device=device)
        logger.info("Chatterbox model loaded")
        return model

    async def get_model(self) -> ChatterboxTTS:
        async with self._get_async_lock():
            if self._model is not None:
                self._last_used = time.monotonic()
                return self._model

            loop = asyncio.get_running_loop()
            device = self._detect_device()
            self._model = await loop.run_in_executor(
                None, lambda: self._load_model_sync(device)
            )
            self._last_used = time.monotonic()
            return self._model

    async def get_cpu_model(self) -> ChatterboxTTS:
        async with self._get_async_lock():
            if self._cpu_model is not None:
                return self._cpu_model

            loop = asyncio.get_running_loop()
            logger.info("Loading Chatterbox CPU fallback model")
            self._cpu_model = await loop.run_in_executor(
                None, lambda: self._load_model_sync("cpu")
            )
            logger.info("Chatterbox CPU fallback model loaded")
            return self._cpu_model

    def preload(self) -> None:
        with self._sync_lock:
            if self._model is not None:
                return
            device = self._detect_device()
            self._model = self._load_model_sync(device)
            self._last_used = time.monotonic()

    def _unload_model(self) -> None:
        if self._model is not None:
            del self._model
            self._model = None
            if torch.cuda.is_available():
                torch.cuda.empty_cache()
            logger.info("Unloaded TTS model due to TTL")

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
        logger.info(f"Started TTS model cleanup task (TTL: {self.config.ttl}s)")

    def stop_cleanup_task(self) -> None:
        if self._cleanup_task is not None:
            self._cleanup_task.cancel()
            self._cleanup_task = None
            logger.info("Stopped TTS model cleanup task")

    def unload_all(self) -> None:
        self._unload_model()
        if self._cpu_model is not None:
            del self._cpu_model
            self._cpu_model = None
            if torch.cuda.is_available():
                torch.cuda.empty_cache()
            logger.info("Unloaded TTS CPU fallback model")
