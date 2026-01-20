from __future__ import annotations

import logging
import time
from dataclasses import dataclass, field
from typing import Generator

import threading

import numpy as np
import onnxruntime as ort
from huggingface_hub import hf_hub_download
from numpy.typing import NDArray
from transformers import AutoTokenizer

from sidecar.stt.engine_manager import EngineConfig, STTEngineManager
from sidecar.domain.types import SessionConfig, SpeechStarted, SpeechStopped, Transcript
from sidecar.stt.vad import SpeechSegment, VADConfig, VADProcessor

logger = logging.getLogger(__name__)

MAX_HISTORY_TURNS = 4
EOU_MODEL_ID = "livekit/turn-detector"
EOU_MODEL_FILE = "model_q8.onnx"
EOU_MODEL_SUBFOLDER = "onnx"
EOU_MODEL_REVISION = "v1.2.0"


@dataclass
class ConversationTurn:
    role: str
    content: str


@dataclass
class EOUConfig:
    threshold: float = 0.5
    max_context_turns: int = MAX_HISTORY_TURNS


class EOUModel:
    _instance: EOUModel | None = None
    _session: ort.InferenceSession | None = None
    _tokenizer: AutoTokenizer | None = None
    _lock = threading.Lock()

    def __new__(cls) -> EOUModel:
        with cls._lock:
            if cls._instance is None:
                cls._instance = super().__new__(cls)
            return cls._instance

    def _ensure_loaded(self) -> None:
        if EOUModel._session is not None:
            return
        with EOUModel._lock:
            if EOUModel._session is None:
                logger.info(f"Loading EOUModel from {EOU_MODEL_ID}")
                start = time.perf_counter()

                model_path = hf_hub_download(
                    repo_id=EOU_MODEL_ID,
                    filename=EOU_MODEL_FILE,
                    subfolder=EOU_MODEL_SUBFOLDER,
                    revision=EOU_MODEL_REVISION,
                )

                EOUModel._tokenizer = AutoTokenizer.from_pretrained(EOU_MODEL_ID)
                EOUModel._session = ort.InferenceSession(
                    model_path, providers=["CPUExecutionProvider"]
                )

                elapsed = time.perf_counter() - start
                logger.info(f"EOUModel loaded in {elapsed:.2f}s")

    def predict(self, turns: list[ConversationTurn]) -> float:
        self._ensure_loaded()

        if not turns:
            return 0.0

        messages = [{"role": t.role, "content": t.content} for t in turns[-MAX_HISTORY_TURNS:]]
        text = EOUModel._tokenizer.apply_chat_template(
            messages, tokenize=False, add_special_tokens=False
        )

        ix = text.rfind("<|im_end|>")
        if ix != -1:
            text = text[:ix]

        inputs = EOUModel._tokenizer(
            text, return_tensors="np", truncation=True, max_length=512
        )

        outputs = EOUModel._session.run(None, {"input_ids": inputs["input_ids"]})

        if not outputs or len(outputs) == 0 or len(outputs[0]) == 0:
            logger.warning("EOU model returned empty output")
            return 0.0

        logits = outputs[0][0]
        if len(logits) < 2:
            logger.warning(f"EOU model returned unexpected logits shape: {len(logits)}")
            return 0.0

        exp_logits = np.exp(logits - np.max(logits))
        probabilities = exp_logits / exp_logits.sum()
        eou_probability = float(probabilities[1])

        return eou_probability


@dataclass
class STTPipelineConfig:
    engine_config: EngineConfig = field(default_factory=EngineConfig)
    vad_config: VADConfig = field(default_factory=VADConfig)
    eou_config: EOUConfig = field(default_factory=EOUConfig)


class STTPipeline:
    def __init__(
        self,
        engine_manager: STTEngineManager,
        config: STTPipelineConfig | None = None,
    ) -> None:
        self._engine_manager = engine_manager
        self._config = config or STTPipelineConfig()
        self._vad = VADProcessor(config=self._config.vad_config)
        self._eou_model = EOUModel()
        self._conversation_history: list[ConversationTurn] = []
        self._session_config: SessionConfig | None = None
        self._pending_user_text = ""

    def configure(self, session_config: SessionConfig) -> None:
        self._session_config = session_config
        self._vad.reset()
        self._conversation_history.clear()
        self._pending_user_text = ""

    def add_assistant_turn(self, text: str) -> None:
        if text.strip():
            self._conversation_history.append(ConversationTurn(role="assistant", content=text.strip()))
            if len(self._conversation_history) > MAX_HISTORY_TURNS * 2:
                self._conversation_history = self._conversation_history[-MAX_HISTORY_TURNS * 2:]

    def reset(self) -> None:
        self._vad.reset()
        self._pending_user_text = ""

    def process_audio(
        self,
        audio: NDArray[np.float32],
    ) -> Generator[SpeechStarted | SpeechStopped | Transcript, None, None]:
        event, segment = self._vad.append(audio)

        if isinstance(event, SpeechStarted):
            yield event

        if isinstance(event, SpeechStopped):
            yield event

            if segment is not None and len(segment.audio) > 0:
                transcript = self._transcribe_segment(segment)
                if transcript.text:
                    transcript = self._add_eou_probability(transcript)
                yield transcript

    def _transcribe_segment(self, segment: SpeechSegment) -> Transcript:
        model_id = self._session_config.model_id if self._session_config else None
        language = self._session_config.language if self._session_config else None
        word_timestamps = self._session_config.include_word_timestamps if self._session_config else False

        engine_wrapper = self._engine_manager.get_engine(model_id)

        with engine_wrapper as engine:
            transcript = engine.transcribe(
                audio=segment.audio,
                language=language,
                word_timestamps=word_timestamps,
            )

        transcript.start_ms = segment.start_ms
        transcript.end_ms = segment.end_ms

        return transcript

    def _add_eou_probability(self, transcript: Transcript) -> Transcript:
        self._pending_user_text = (self._pending_user_text + " " + transcript.text).strip()

        history_with_current = self._conversation_history.copy()
        history_with_current.append(ConversationTurn(role="user", content=self._pending_user_text))

        eou_probability = self._eou_model.predict(history_with_current)
        transcript.eou_probability = eou_probability

        if eou_probability >= self._config.eou_config.threshold:
            self._conversation_history.append(
                ConversationTurn(role="user", content=self._pending_user_text)
            )
            self._pending_user_text = ""
            if len(self._conversation_history) > MAX_HISTORY_TURNS * 2:
                self._conversation_history = self._conversation_history[-MAX_HISTORY_TURNS * 2:]

        logger.debug(f"EOU probability: {eou_probability:.3f} for: {transcript.text[:50]}...")

        return transcript
