from __future__ import annotations

import asyncio
import logging
import os
import platform
import threading
import time
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from huggingface_hub import hf_hub_download
from kokoro_onnx import Kokoro
from onnxruntime import InferenceSession, get_available_providers

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)


def _get_onnx_providers(device: str) -> list[tuple[str, dict]]:
    available = get_available_providers()
    system = platform.system()
    machine = platform.machine()

    logger.info(f"Available ONNX providers: {available}")

    if device == "cpu":
        return [("CPUExecutionProvider", {})]

    providers = []

    if system == "Darwin" and machine == "arm64" and "CoreMLExecutionProvider" in available:
        providers.append(("CoreMLExecutionProvider", {}))

    if "CUDAExecutionProvider" in available:
        providers.append(("CUDAExecutionProvider", {}))

    providers.append(("CPUExecutionProvider", {}))

    logger.info(f"Using ONNX providers: {providers}")
    return providers

SAMPLE_RATE = 24000

KOKORO_VOICES = [
    ("af_heart", "en-us", "female"),
    ("af_alloy", "en-us", "female"),
    ("af_aoede", "en-us", "female"),
    ("af_bella", "en-us", "female"),
    ("af_jessica", "en-us", "female"),
    ("af_kore", "en-us", "female"),
    ("af_nicole", "en-us", "female"),
    ("af_nova", "en-us", "female"),
    ("af_river", "en-us", "female"),
    ("af_sarah", "en-us", "female"),
    ("af_sky", "en-us", "female"),
    ("am_adam", "en-us", "male"),
    ("am_echo", "en-us", "male"),
    ("am_eric", "en-us", "male"),
    ("am_fenrir", "en-us", "male"),
    ("am_liam", "en-us", "male"),
    ("am_michael", "en-us", "male"),
    ("am_onyx", "en-us", "male"),
    ("am_puck", "en-us", "male"),
    ("bf_alice", "en-gb", "female"),
    ("bf_emma", "en-gb", "female"),
    ("bf_isabella", "en-gb", "female"),
    ("bf_lily", "en-gb", "female"),
    ("bm_daniel", "en-gb", "male"),
    ("bm_fable", "en-gb", "male"),
    ("bm_george", "en-gb", "male"),
    ("bm_lewis", "en-gb", "male"),
    ("jf_alpha", "ja", "female"),
    ("jf_gongitsune", "ja", "female"),
    ("jf_nezumi", "ja", "female"),
    ("jf_tebukuro", "ja", "female"),
    ("jm_kumo", "ja", "male"),
    ("zf_xiaobei", "cmn", "female"),
    ("zf_xiaoni", "cmn", "female"),
    ("zf_xiaoxiao", "cmn", "female"),
    ("zf_xiaoyi", "cmn", "female"),
    ("zm_yunjian", "cmn", "male"),
    ("zm_yunxi", "cmn", "male"),
    ("zm_yunxia", "cmn", "male"),
    ("zm_yunyang", "cmn", "male"),
    ("ef_dora", "es", "female"),
    ("em_alex", "es", "male"),
    ("em_santa", "es", "male"),
    ("ff_siwis", "fr", "female"),
    ("hf_alpha", "hi", "female"),
    ("hf_beta", "hi", "female"),
    ("hm_omega", "hi", "male"),
    ("hm_psi", "hi", "male"),
    ("if_sara", "it", "female"),
    ("im_nicola", "it", "male"),
    ("pf_dora", "pt", "female"),
    ("pm_alex", "pt", "male"),
    ("pm_santa", "pt", "male"),
]


@dataclass
class TTSConfig:
    model_id: str = field(
        default_factory=lambda: os.environ.get("TTS_MODEL_ID", "hexgrad/Kokoro-82M-v1.0-ONNX")
    )
    device: str = "cuda"
    ttl: int = 300
    fallback_to_cpu: bool = True


@dataclass
class SynthesisConfig:
    speed: float = 1.0


class KokoroModelManager:
    def __init__(self, config: TTSConfig) -> None:
        self.config = config
        self._kokoro: Kokoro | None = None
        self._cpu_kokoro: Kokoro | None = None
        self._sync_lock = threading.RLock()
        self._async_lock: asyncio.Lock | None = None
        self._model_path: str | None = None
        self._voices_path: str | None = None
        self._last_used: float = 0
        self._cleanup_task: asyncio.Task | None = None
        self._cleanup_interval: float = 30.0

    def _get_async_lock(self) -> asyncio.Lock:
        if self._async_lock is None:
            self._async_lock = asyncio.Lock()
        return self._async_lock

    def _download_model_files_sync(self) -> tuple[str, str]:
        if self._model_path and self._voices_path:
            return self._model_path, self._voices_path
        logger.info(f"Downloading model files from {self.config.model_id}")
        self._model_path = hf_hub_download(self.config.model_id, "model.onnx")
        self._voices_path = hf_hub_download(self.config.model_id, "voices.bin")
        return self._model_path, self._voices_path

    def _load_model_sync(self, model_path: str, voices_path: str) -> Kokoro:
        logger.info(f"Loading Kokoro model {self.config.model_id}")
        session_providers = _get_onnx_providers(self.config.device)
        session = InferenceSession(model_path, providers=session_providers)
        kokoro = Kokoro.from_session(session, voices_path)
        logger.info("Kokoro model loaded")
        return kokoro

    async def get_kokoro(self) -> Kokoro:
        async with self._get_async_lock():
            if self._kokoro is not None:
                self._last_used = time.monotonic()
                return self._kokoro

            loop = asyncio.get_running_loop()

            model_path, voices_path = await loop.run_in_executor(
                None, self._download_model_files_sync
            )

            self._kokoro = await loop.run_in_executor(
                None, lambda: self._load_model_sync(model_path, voices_path)
            )
            self._last_used = time.monotonic()
            return self._kokoro

    async def get_cpu_model(self) -> Kokoro:
        async with self._get_async_lock():
            if self._cpu_kokoro is not None:
                return self._cpu_kokoro

            loop = asyncio.get_running_loop()

            model_path, voices_path = await loop.run_in_executor(
                None, self._download_model_files_sync
            )

            logger.info("Loading Kokoro CPU fallback model")
            session = await loop.run_in_executor(
                None, lambda: InferenceSession(model_path, providers=[("CPUExecutionProvider", {})])
            )
            self._cpu_kokoro = await loop.run_in_executor(
                None, lambda: Kokoro.from_session(session, voices_path)
            )
            logger.info("Kokoro CPU fallback model loaded")
            return self._cpu_kokoro

    def list_voices(self) -> list[str]:
        return [v[0] for v in KOKORO_VOICES]

    def get_voice_lang(self, voice_id: str) -> str:
        for name, lang, _ in KOKORO_VOICES:
            if name == voice_id:
                return lang
        return "en-us"

    def preload(self) -> None:
        with self._sync_lock:
            if self._kokoro is not None:
                return
            model_path, voices_path = self._download_model_files_sync()
            self._kokoro = self._load_model_sync(model_path, voices_path)
            self._last_used = time.monotonic()

    def __enter__(self) -> Kokoro:
        with self._sync_lock:
            if self._kokoro is None:
                model_path, voices_path = self._download_model_files_sync()
                self._kokoro = self._load_model_sync(model_path, voices_path)
            self._last_used = time.monotonic()
            return self._kokoro

    def __exit__(self, *_args) -> None:
        pass

    def _unload_kokoro(self) -> None:
        if self._kokoro is not None:
            self._kokoro = None
            logger.info("Unloaded TTS model due to TTL")

    async def _cleanup_loop(self) -> None:
        while True:
            await asyncio.sleep(self._cleanup_interval)
            now = time.monotonic()

            if self.config.ttl > 0 and self._kokoro is not None:
                async with self._get_async_lock():
                    if self._kokoro is not None:
                        idle_time = now - self._last_used
                        if idle_time >= self.config.ttl:
                            self._unload_kokoro()

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
        self._unload_kokoro()
        if self._cpu_kokoro is not None:
            self._cpu_kokoro = None
            logger.info("Unloaded TTS CPU fallback model")
