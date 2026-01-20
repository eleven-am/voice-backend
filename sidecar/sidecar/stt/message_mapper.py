from __future__ import annotations

import logging

from sidecar.stt import pb2 as stt_pb2
from sidecar.domain.constants import TARGET_SAMPLE_RATE
from sidecar.domain.exceptions import TranscriptionError
from sidecar.domain.types import SessionConfig, Transcript

logger = logging.getLogger(__name__)

DEFAULT_PARTIAL_WINDOW_MS = 1500
DEFAULT_PARTIAL_STRIDE_MS = 700


class MessageMapper:
    @staticmethod
    def to_domain_session_config(pb_config: stt_pb2.SessionConfig) -> SessionConfig:
        return SessionConfig(
            language=pb_config.language or "en",
            sample_rate=pb_config.sample_rate or TARGET_SAMPLE_RATE,
            initial_prompt=pb_config.initial_prompt or None,
            hotwords=pb_config.hotwords or None,
            partials=pb_config.partials,
            partial_window_ms=pb_config.partial_window_ms or DEFAULT_PARTIAL_WINDOW_MS,
            partial_stride_ms=pb_config.partial_stride_ms or DEFAULT_PARTIAL_STRIDE_MS,
            include_word_timestamps=pb_config.include_word_timestamps,
            model_id=pb_config.model_id or None,
            task=pb_config.task or None,
            temperature=pb_config.temperature if pb_config.temperature else None,
        )

    @staticmethod
    def to_grpc_transcript(transcript: Transcript, is_partial: bool) -> stt_pb2.ServerMessage:
        segments = []
        words = []
        if transcript.segments:
            for seg in transcript.segments:
                segments.append(
                    stt_pb2.Segment(
                        start=seg["start"],
                        end=seg["end"],
                        text=seg["text"],
                    )
                )
                for w in seg.get("words", []):
                    words.append(stt_pb2.TranscriptWord(start=w["start"], end=w["end"], word=w["word"]))

        usage = stt_pb2.Usage(input_tokens=0, output_tokens=0)

        kwargs = {
            "text": transcript.text,
            "is_partial": is_partial,
            "start_ms": transcript.start_ms,
            "end_ms": transcript.end_ms,
            "audio_duration_ms": transcript.audio_duration_ms,
            "processing_duration_ms": transcript.processing_duration_ms,
            "segments": segments,
            "words": words,
            "usage": usage,
            "model": transcript.model or "",
        }

        if transcript.eou_probability is not None:
            kwargs["eou_probability"] = transcript.eou_probability

        return stt_pb2.ServerMessage(transcript=stt_pb2.TranscriptResult(**kwargs))

    @staticmethod
    def to_grpc_error(error: Exception) -> stt_pb2.ServerMessage:
        if isinstance(error, TranscriptionError):
            logger.error(f"Transcription error: {error}")
            return stt_pb2.ServerMessage(error=stt_pb2.ErrorMessage(message=str(error)))
        logger.exception("Unexpected transcription failure")
        return stt_pb2.ServerMessage(error=stt_pb2.ErrorMessage(message=f"Unexpected error: {error}"))

    @staticmethod
    def to_grpc_ready() -> stt_pb2.ServerMessage:
        return stt_pb2.ServerMessage(ready=stt_pb2.ReadyMessage())

    @staticmethod
    def to_grpc_speech_started() -> stt_pb2.ServerMessage:
        return stt_pb2.ServerMessage(speech_started=stt_pb2.SpeechStartedMessage())

    @staticmethod
    def to_grpc_speech_stopped() -> stt_pb2.ServerMessage:
        return stt_pb2.ServerMessage(speech_stopped=stt_pb2.SpeechStoppedMessage())
