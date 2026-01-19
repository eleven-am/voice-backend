from dataclasses import dataclass
from typing import Literal


@dataclass
class SessionConfig:
    type: Literal["session.config"] = "session.config"
    language: str = "en"
    sample_rate: int = 16000
    initial_prompt: str | None = None
    hotwords: str | None = None
    partials: bool = False
    partial_window_ms: int = 1500
    partial_stride_ms: int = 700
    include_word_timestamps: bool = False
    model_id: str | None = None
    task: str | None = None
    temperature: float | None = None


@dataclass
class SessionReady:
    type: Literal["session.ready"] = "session.ready"


@dataclass
class SpeechStarted:
    type: Literal["speech_started"] = "speech_started"
    timestamp_ms: int = 0


@dataclass
class SpeechStopped:
    type: Literal["speech_stopped"] = "speech_stopped"
    timestamp_ms: int = 0


@dataclass
class Transcript:
    type: Literal["transcript"] = "transcript"
    text: str = ""
    is_partial: bool = False
    start_ms: int = 0
    end_ms: int = 0
    audio_duration_ms: int = 0
    processing_duration_ms: int = 0
    segments: list[dict] | None = None
    usage: dict | None = None
    model: str | None = None
    eou_probability: float | None = None


@dataclass
class Error:
    type: Literal["error"] = "error"
    message: str = ""
    code: str | None = None


Event = SessionReady | SpeechStarted | SpeechStopped | Transcript | Error
