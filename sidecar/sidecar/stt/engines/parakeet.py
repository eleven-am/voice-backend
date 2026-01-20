from __future__ import annotations

import logging
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path

import numpy as np
import onnx_asr
import soundfile as sf
from numpy.typing import NDArray

from sidecar.domain.types import Transcript

logger = logging.getLogger(__name__)

PARAKEET_SAMPLE_RATE = 16000
DEFAULT_MODEL_ID = "nemo-parakeet-tdt-0.6b-v3"


def _normalize_model_id(model_id: str) -> str:
    if model_id.startswith("nvidia/"):
        model_id = model_id.replace("nvidia/", "nemo-")
    return model_id


def _get_providers(device: str) -> list[str]:
    if device == "cuda":
        try:
            import onnxruntime as ort
            available = ort.get_available_providers()
            if "CUDAExecutionProvider" in available:
                return ["CUDAExecutionProvider", "CPUExecutionProvider"]
            logger.warning("CUDA requested but CUDAExecutionProvider not available, falling back to CPU")
        except Exception:
            logger.warning("Failed to check ONNX providers, falling back to CPU")
    return ["CPUExecutionProvider"]


@dataclass
class ParakeetConfig:
    model_id: str = DEFAULT_MODEL_ID
    device: str = "cuda"


@dataclass
class Word:
    word: str
    start: float
    end: float


def _tokens_to_words(tokens: list[str], timestamps: list[float]) -> list[Word]:
    if not tokens or not timestamps:
        return []

    if len(tokens) != len(timestamps):
        logger.warning(f"Token/timestamp length mismatch: {len(tokens)} tokens, {len(timestamps)} timestamps")
        min_len = min(len(tokens), len(timestamps))
        tokens = tokens[:min_len]
        timestamps = timestamps[:min_len]

    words: list[Word] = []
    current_word = ""
    current_start: float | None = None

    for token, ts in zip(tokens, timestamps):
        token_stripped = token.strip()
        if not token_stripped:
            continue

        is_punctuation = len(token_stripped) == 1 and not token_stripped.isalnum()

        if token.startswith(" ") or current_start is None:
            if current_word and current_start is not None:
                words.append(Word(word=current_word, start=current_start, end=ts))
            current_word = token_stripped
            current_start = ts
        elif is_punctuation:
            current_word += token_stripped
        else:
            current_word += token_stripped

    if current_word and current_start is not None:
        end_time = timestamps[-1] if timestamps else current_start
        words.append(Word(word=current_word, start=current_start, end=end_time))

    return words


class ParakeetEngine:
    def __init__(self, config: ParakeetConfig | None = None) -> None:
        self._config = config or ParakeetConfig()
        self._model: onnx_asr.adapters.TextResultsAsrAdapter | None = None
        self._model_with_ts: onnx_asr.adapters.TimestampedResultsAsrAdapter | None = None
        self._is_loaded = False

    def load(self) -> None:
        if self._model is None:
            model_id = _normalize_model_id(self._config.model_id)
            logger.info(f"Loading Parakeet ONNX model: {model_id}")
            start = time.perf_counter()
            providers = _get_providers(self._config.device)
            self._model = onnx_asr.load_model(model_id, providers=providers)
            self._model_with_ts = self._model.with_timestamps()
            elapsed = time.perf_counter() - start
            logger.info(f"Parakeet model loaded in {elapsed:.2f}s")
            self._is_loaded = True

    def unload(self) -> None:
        self._model = None
        self._model_with_ts = None
        self._is_loaded = False
        logger.info("Parakeet engine unloaded")

    @property
    def is_loaded(self) -> bool:
        return self._is_loaded

    def transcribe(
        self,
        audio: NDArray[np.float32],
        language: str | None = None,
        word_timestamps: bool = False,
    ) -> Transcript:
        start = time.perf_counter()

        if language and language not in ("en", "english"):
            logger.warning(f"Parakeet only supports English, ignoring language={language}")

        if self._model is None:
            self.load()

        if self._model is None:
            raise RuntimeError("Failed to load Parakeet model")

        if len(audio) == 0:
            logger.warning("Empty audio, returning empty transcript")
            return Transcript(text="", is_partial=False, start_ms=0, end_ms=0)

        audio_duration_ms = int(len(audio) / PARAKEET_SAMPLE_RATE * 1000)

        with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as f:
            sf.write(f.name, audio, PARAKEET_SAMPLE_RATE)
            temp_path = Path(f.name)

        try:
            if word_timestamps:
                result = self._model_with_ts.recognize(str(temp_path))
                text = result.text
                words = _tokens_to_words(result.tokens, result.timestamps)
                segments = [
                    {
                        "start": 0.0,
                        "end": audio_duration_ms / 1000.0,
                        "text": text,
                        "words": [{"start": w.start, "end": w.end, "word": w.word} for w in words],
                    }
                ] if text else None
            else:
                text = self._model.recognize(str(temp_path))
                segments = [
                    {
                        "start": 0.0,
                        "end": audio_duration_ms / 1000.0,
                        "text": text,
                        "words": [],
                    }
                ] if text else None
        finally:
            temp_path.unlink(missing_ok=True)

        processing_duration_ms = int((time.perf_counter() - start) * 1000)

        if not text:
            logger.warning(f"Empty transcription result for {audio_duration_ms}ms audio")
        else:
            logger.info(f"Transcribed {audio_duration_ms}ms audio in {processing_duration_ms}ms: {text[:50]}...")

        return Transcript(
            text=text.strip() if text else "",
            is_partial=False,
            start_ms=0,
            end_ms=audio_duration_ms,
            audio_duration_ms=audio_duration_ms,
            processing_duration_ms=processing_duration_ms,
            segments=segments,
            model=self._config.model_id,
        )

    def get_supported_languages(self) -> list[str]:
        return ["en"]

    def get_sample_rate(self) -> int:
        return PARAKEET_SAMPLE_RATE
