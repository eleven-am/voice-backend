from __future__ import annotations

import logging
import os
import threading
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from huggingface_hub import hf_hub_download
from kokoro_onnx import Kokoro
from onnxruntime import InferenceSession, get_available_providers

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)

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
        self._lock = threading.RLock()
        self._ref_count = 0
        self._model_path: str | None = None
        self._voices_path: str | None = None

    def _download_model_files(self) -> tuple[str, str]:
        with self._lock:
            if self._model_path and self._voices_path:
                return self._model_path, self._voices_path
            logger.info(f"Downloading model files from {self.config.model_id}")
            self._model_path = hf_hub_download(self.config.model_id, "model.onnx")
            self._voices_path = hf_hub_download(self.config.model_id, "voices.bin")
            return self._model_path, self._voices_path

    def _load_model(self) -> None:
        with self._lock:
            if self._kokoro is not None:
                return

            logger.info(f"Loading Kokoro model {self.config.model_id}")
            model_path, voices_path = self._download_model_files()

            providers = get_available_providers()
            logger.info(f"Available ONNX providers: {providers}")

            if self.config.device == "cpu":
                session_providers = [("CPUExecutionProvider", {})]
            else:
                excluded = {"TensorrtExecutionProvider"}
                providers = [p for p in providers if p not in excluded]
                priority = {"CUDAExecutionProvider": 100, "CPUExecutionProvider": 0}
                providers = sorted(providers, key=lambda x: priority.get(x, 0), reverse=True)
                session_providers = [(p, {}) for p in providers]

            logger.info(f"Using ONNX providers: {session_providers}")

            session = InferenceSession(model_path, providers=session_providers)
            self._kokoro = Kokoro.from_session(session, voices_path)
            logger.info("Kokoro model loaded")

    def get_cpu_model(self) -> Kokoro:
        with self._lock:
            if self._cpu_kokoro is not None:
                return self._cpu_kokoro

            logger.info("Loading Kokoro CPU fallback model")
            model_path, voices_path = self._download_model_files()
            session = InferenceSession(model_path, providers=[("CPUExecutionProvider", {})])
            self._cpu_kokoro = Kokoro.from_session(session, voices_path)
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
        self._load_model()

    def __enter__(self) -> Kokoro:
        with self._lock:
            if self._kokoro is None:
                self._load_model()
            self._ref_count += 1
            return self._kokoro

    def __exit__(self, *_args) -> None:
        with self._lock:
            self._ref_count -= 1
