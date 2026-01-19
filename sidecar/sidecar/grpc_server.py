from __future__ import annotations

import logging
import subprocess
from collections.abc import Iterable, Iterator
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass, field
from io import BytesIO

import numpy as np
import soxr

from sidecar import stt_pb2, stt_pb2_grpc
from sidecar.audio_preprocessing import preprocess_audio
from sidecar.engine_manager import MAX_OOM_RETRIES, STTEngineManager
from sidecar.opus_decoder import OPUS_SAMPLE_RATE, OpusStreamDecoder
from sidecar.stt_pipeline import STTPipeline, STTPipelineConfig
from sidecar.types import SessionConfig, SpeechStarted, SpeechStopped, Transcript
from sidecar.utils import is_oom_error
from sidecar.vad_standalone import SpeechSegment

logger = logging.getLogger(__name__)

TARGET_SAMPLE_RATE = 16000
DEFAULT_PARTIAL_WINDOW_MS = 1500
DEFAULT_PARTIAL_STRIDE_MS = 700
PARTIAL_OVERLAP_MS = 300


class TranscriptionError(Exception):
    pass


@dataclass
class SpeechState:
    lock: object = field(default_factory=lambda: __import__('threading').Lock())
    active: bool = False
    buffer: list[np.ndarray] = field(default_factory=list)
    confirmed_words: list[str] = field(default_factory=list)
    last_partial_ms: int = 0
    max_chunks: int = 256

    def start_speech(self) -> None:
        with self.lock:
            self.active = True
            self.buffer = []
            self.confirmed_words = []
            self.last_partial_ms = 0

    def stop_speech(self) -> None:
        with self.lock:
            self.active = False

    def is_active(self) -> bool:
        with self.lock:
            return self.active

    def append_audio(self, audio: np.ndarray) -> None:
        with self.lock:
            if self.active:
                self.buffer.append(audio)
                if len(self.buffer) > self.max_chunks:
                    self.buffer = self.buffer[-self.max_chunks:]

    def get_buffer_copy(self) -> list[np.ndarray]:
        with self.lock:
            return list(self.buffer)

    def update_partial(self, new_partial_ms: int, new_words: list[str]) -> None:
        with self.lock:
            self.last_partial_ms = new_partial_ms
            self.confirmed_words = new_words

    def get_partial_state(self) -> tuple[int, list[str]]:
        with self.lock:
            return self.last_partial_ms, list(self.confirmed_words)


def decode_encoded_audio(encoded_msg: stt_pb2.EncodedAudio) -> bytes:
    fmt = encoded_msg.format or ""

    try:
        import soundfile as sf

        data, sr = sf.read(BytesIO(encoded_msg.data), dtype="int16", format=fmt or None)
        if sr != TARGET_SAMPLE_RATE:
            data = soxr.resample(data.astype(np.float32), sr, TARGET_SAMPLE_RATE).astype(np.int16)
        return data.tobytes()
    except Exception:
        pass

    cmd = [
        "ffmpeg",
        "-hide_banner",
        "-loglevel",
        "error",
    ]
    if fmt:
        cmd += ["-f", fmt]
    cmd += [
        "-i",
        "pipe:0",
        "-f",
        "s16le",
        "-ar",
        str(TARGET_SAMPLE_RATE),
        "-ac",
        "1",
        "pipe:1",
    ]
    proc = subprocess.Popen(
        cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE
    )
    stdout, stderr = proc.communicate(input=encoded_msg.data)
    if proc.returncode != 0:
        raise TranscriptionError(f"ffmpeg decode error: {stderr.decode().strip()}")
    return stdout


def pcm16_to_float32(pcm_bytes: bytes) -> np.ndarray:
    pcm_array = np.frombuffer(pcm_bytes, dtype=np.int16)
    return pcm_array.astype(np.float32) / 32768.0


def resample_audio(audio: np.ndarray, source_rate: int, target_rate: int) -> np.ndarray:
    if source_rate == target_rate:
        return audio
    return soxr.resample(audio, source_rate, target_rate).astype(np.float32)


def dedup_new_words(text: str, confirmed_words: list[str]) -> tuple[str, list[str]]:
    words = [w for w in text.strip().split() if w]
    overlap = 0
    max_overlap = min(len(words), len(confirmed_words))
    for i in range(max_overlap, 0, -1):
        if [w.lower() for w in confirmed_words[-i:]] == [w.lower() for w in words[:i]]:
            overlap = i
            break
    new_words = words[overlap:]
    if new_words:
        confirmed_words.extend(new_words)
    return " ".join(new_words), confirmed_words


def merge_transcripts(transcripts: list[tuple[Transcript, float]]) -> Transcript:
    if not transcripts:
        raise ValueError("Cannot merge empty transcript list")
    if len(transcripts) == 1:
        return transcripts[0][0]

    merged_text_parts: list[str] = []
    merged_segments: list[dict] = []
    total_audio_ms = 0
    total_processing_ms = 0

    for transcript, offset_s in transcripts:
        if transcript.text.strip():
            merged_text_parts.append(transcript.text.strip())

        if transcript.segments:
            for seg in transcript.segments:
                merged_segments.append({
                    "start": seg["start"] + offset_s,
                    "end": seg["end"] + offset_s,
                    "text": seg["text"],
                    "words": [
                        {"word": w["word"], "start": w["start"] + offset_s, "end": w["end"] + offset_s}
                        for w in seg.get("words", [])
                    ],
                })

        total_audio_ms += transcript.audio_duration_ms
        total_processing_ms += transcript.processing_duration_ms

    first = transcripts[0][0]
    last = transcripts[-1][0]
    last_offset_s = transcripts[-1][1]

    return Transcript(
        text=" ".join(merged_text_parts),
        start_ms=first.start_ms,
        end_ms=int(last_offset_s * 1000) + last.end_ms,
        audio_duration_ms=total_audio_ms,
        processing_duration_ms=total_processing_ms,
        segments=merged_segments if merged_segments else None,
        model=first.model,
        eou_probability=None,
    )


class TranscriptionServiceServicer(stt_pb2_grpc.TranscriptionServiceServicer):
    def __init__(
        self,
        engine_manager: STTEngineManager,
        pipeline_config: STTPipelineConfig,
        executor: ThreadPoolExecutor,
    ) -> None:
        self.engine_manager = engine_manager
        self.pipeline_config = pipeline_config
        self.executor = executor

    def _error_message(self, error: Exception, code: str) -> stt_pb2.ServerMessage:
        if isinstance(error, TranscriptionError):
            logger.error(f"Transcription error: {error}")
            return stt_pb2.ServerMessage(
                error=stt_pb2.ErrorMessage(message=str(error))
            )
        logger.exception("Unexpected transcription failure")
        return stt_pb2.ServerMessage(
            error=stt_pb2.ErrorMessage(message=f"Unexpected error: {error}")
        )

    def _transcribe_with_retry(
        self,
        audio: np.ndarray,
        language: str | None = None,
        word_timestamps: bool = False,
    ) -> Transcript:
        last_error: Exception | None = None
        for attempt in range(MAX_OOM_RETRIES):
            try:
                engine_wrapper = self.engine_manager.get_engine()
                with engine_wrapper as engine:
                    return engine.transcribe(
                        audio=audio,
                        language=language,
                        word_timestamps=word_timestamps,
                    )
            except Exception as e:
                if is_oom_error(e):
                    logger.warning(f"OOM error on attempt {attempt + 1}/{MAX_OOM_RETRIES}: {e}")
                    last_error = e
                    if not self.engine_manager.try_fallback():
                        raise TranscriptionError(f"OOM error with no fallback available: {e}") from e
                    continue
                raise
        raise TranscriptionError(f"Failed after {MAX_OOM_RETRIES} OOM retries: {last_error}") from last_error

    def _handle_session_config(
        self, session_config: stt_pb2.SessionConfig
    ) -> STTPipeline:
        pipeline = STTPipeline(
            engine_manager=self.engine_manager,
            config=self.pipeline_config,
        )

        config = SessionConfig(
            language=session_config.language or "en",
            sample_rate=session_config.sample_rate or TARGET_SAMPLE_RATE,
            initial_prompt=session_config.initial_prompt or None,
            hotwords=session_config.hotwords or None,
            partials=session_config.partials,
            partial_window_ms=session_config.partial_window_ms or DEFAULT_PARTIAL_WINDOW_MS,
            partial_stride_ms=session_config.partial_stride_ms or DEFAULT_PARTIAL_STRIDE_MS,
            include_word_timestamps=session_config.include_word_timestamps,
            model_id=session_config.model_id or None,
            task=session_config.task or None,
            temperature=session_config.temperature if session_config.temperature else None,
        )
        pipeline.configure(config)

        logger.info(
            f"Session configured: language={config.language}, sample_rate={config.sample_rate}"
        )
        return pipeline

    def _process_pipeline_events(
        self,
        pipeline: STTPipeline,
        audio: np.ndarray,
        state: SpeechState,
        context=None,
    ) -> Iterator[stt_pb2.ServerMessage]:
        for event in pipeline.process_audio(audio):
            if isinstance(event, SpeechStarted):
                yield stt_pb2.ServerMessage(
                    speech_started=stt_pb2.SpeechStartedMessage()
                )
                state.start_speech()

            elif isinstance(event, SpeechStopped):
                yield stt_pb2.ServerMessage(
                    speech_stopped=stt_pb2.SpeechStoppedMessage()
                )
                state.stop_speech()

            elif isinstance(event, Transcript):
                yield self._make_transcript_message(event, is_partial=False)

    def _generate_partial_transcript(
        self,
        state: SpeechState,
        pipeline: STTPipeline,
        session_config: stt_pb2.SessionConfig,
        context=None,
    ) -> Iterator[stt_pb2.ServerMessage]:
        buffer_copy = state.get_buffer_copy()
        buffer_audio = np.concatenate(buffer_copy) if buffer_copy else np.array([], dtype=np.float32)
        buf_ms = int(len(buffer_audio) / TARGET_SAMPLE_RATE * 1000)

        partial_window_ms = session_config.partial_window_ms or DEFAULT_PARTIAL_WINDOW_MS
        partial_stride_ms = session_config.partial_stride_ms or DEFAULT_PARTIAL_STRIDE_MS
        partial_overlap_ms = PARTIAL_OVERLAP_MS

        last_partial_ms, confirmed_words = state.get_partial_state()

        if buf_ms - last_partial_ms >= partial_stride_ms and buf_ms >= partial_window_ms:
            try:
                tail_window_ms = partial_window_ms + partial_overlap_ms
                tail_samples = int(tail_window_ms * TARGET_SAMPLE_RATE / 1000)
                if len(buffer_audio) > tail_samples:
                    tail_audio = buffer_audio[-tail_samples:]
                    tail_start_ms = buf_ms - tail_window_ms
                else:
                    tail_audio = buffer_audio
                    tail_start_ms = 0

                partial_segment = SpeechSegment(
                    audio=tail_audio,
                    start_ms=tail_start_ms,
                    end_ms=buf_ms,
                )

                transcript = self._transcribe_with_retry(
                    audio=partial_segment.audio,
                    language=session_config.language or None,
                    word_timestamps=session_config.include_word_timestamps,
                )

                if context and not context.is_active():
                    return

                new_text, updated_words = dedup_new_words(transcript.text, confirmed_words)
                state.update_partial(buf_ms, updated_words)
                if new_text:
                    transcript.text = new_text
                    yield self._make_transcript_message(transcript, is_partial=True)
            except Exception as e:
                yield self._error_message(e, "partial_transcription_error")

    def _flush_remaining_audio(
        self, state: SpeechState, pipeline: STTPipeline
    ) -> Iterator[stt_pb2.ServerMessage]:
        buffer_copy = state.get_buffer_copy()
        if buffer_copy:
            buffer_audio = np.concatenate(buffer_copy)
            duration_ms = int(len(buffer_audio) / TARGET_SAMPLE_RATE * 1000)
            segment = SpeechSegment(audio=buffer_audio, start_ms=0, end_ms=duration_ms)
            try:
                transcript = self._transcribe_with_retry(audio=segment.audio)
                yield self._make_transcript_message(transcript, is_partial=False)
            except Exception as e:
                yield self._error_message(e, "transcription_error")

    def Transcribe(self, request_iterator: Iterable[stt_pb2.ClientMessage], context):
        pipeline: STTPipeline | None = None
        session_config: stt_pb2.SessionConfig | None = None
        speech_state = SpeechState()
        opus_decoder: OpusStreamDecoder | None = None

        for client_msg in request_iterator:
            if context and not context.is_active():
                logger.info("Client cancelled, stopping transcription")
                break
            msg_type = client_msg.WhichOneof("msg")

            if msg_type == "config":
                if session_config is not None:
                    yield stt_pb2.ServerMessage(
                        error=stt_pb2.ErrorMessage(message="Session already configured")
                    )
                    continue

                session_config = client_msg.config
                pipeline = self._handle_session_config(session_config)
                yield stt_pb2.ServerMessage(ready=stt_pb2.ReadyMessage())
                continue

            if msg_type == "audio":
                if session_config is None or pipeline is None:
                    yield stt_pb2.ServerMessage(
                        error=stt_pb2.ErrorMessage(message="Session not configured")
                    )
                    continue

                audio_frame = client_msg.audio
                pcm_bytes = audio_frame.pcm16
                audio = pcm16_to_float32(pcm_bytes)

                src_rate = audio_frame.sample_rate or session_config.sample_rate or TARGET_SAMPLE_RATE
                if src_rate != TARGET_SAMPLE_RATE:
                    audio = resample_audio(audio, src_rate, TARGET_SAMPLE_RATE)

                yield from self._process_pipeline_events(pipeline, audio, speech_state, context)

                if speech_state.is_active():
                    speech_state.append_audio(audio)
                    if session_config.partials:
                        yield from self._generate_partial_transcript(speech_state, pipeline, session_config, context)

            if msg_type == "encoded_audio":
                if session_config is None or pipeline is None:
                    yield stt_pb2.ServerMessage(
                        error=stt_pb2.ErrorMessage(message="Session not configured")
                    )
                    continue
                try:
                    encoded_msg = client_msg.encoded_audio
                    chunks = preprocess_audio(encoded_msg.data, encoded_msg.format or None)
                    total_duration_ms = sum(c.duration_ms for c in chunks)
                    logger.info(f"Transcribing encoded audio: {total_duration_ms}ms ({len(chunks)} chunk(s))")

                    if len(chunks) == 1:
                        chunk = chunks[0]
                        transcript = self._transcribe_with_retry(
                            audio=chunk.data,
                            language=session_config.language or None,
                            word_timestamps=session_config.include_word_timestamps,
                        )
                        yield self._make_transcript_message(transcript, is_partial=False)
                    else:
                        transcripts_with_offsets: list[tuple[Transcript, float]] = []
                        for i, chunk in enumerate(chunks):
                            offset_s = chunk.offset_ms / 1000.0
                            logger.info(f"Transcribing chunk {i + 1}/{len(chunks)} (offset: {offset_s:.1f}s)")
                            transcript = self._transcribe_with_retry(
                                audio=chunk.data,
                                language=session_config.language or None,
                                word_timestamps=session_config.include_word_timestamps,
                            )
                            transcripts_with_offsets.append((transcript, offset_s))

                        merged = merge_transcripts(transcripts_with_offsets)
                        logger.info(f"All {len(chunks)} chunks transcribed")
                        yield self._make_transcript_message(merged, is_partial=False)
                except Exception as e:
                    yield self._error_message(e, "transcription_error")

            if msg_type == "opus_frame":
                if session_config is None or pipeline is None:
                    yield stt_pb2.ServerMessage(
                        error=stt_pb2.ErrorMessage(message="Session not configured")
                    )
                    continue

                opus_frame = client_msg.opus_frame
                if opus_decoder is None:
                    sample_rate = opus_frame.sample_rate or OPUS_SAMPLE_RATE
                    channels = opus_frame.channels or 1
                    opus_decoder = OpusStreamDecoder(sample_rate=sample_rate, channels=channels)

                try:
                    audio = opus_decoder.decode_frame(opus_frame.data)
                    audio = resample_audio(audio, OPUS_SAMPLE_RATE, TARGET_SAMPLE_RATE)

                    yield from self._process_pipeline_events(pipeline, audio, speech_state, context)

                    if speech_state.is_active():
                        speech_state.append_audio(audio)
                        if session_config.partials:
                            yield from self._generate_partial_transcript(
                                speech_state, pipeline, session_config, context
                            )
                except Exception as e:
                    yield self._error_message(e, "opus_decode_error")

        if pipeline is not None:
            yield from self._flush_remaining_audio(speech_state, pipeline)
            pipeline.reset()

    def _make_transcript_message(self, transcript: Transcript, is_partial: bool) -> stt_pb2.ServerMessage:
        segments = []
        words = []
        if transcript.segments:
            for seg in transcript.segments:
                segments.append(stt_pb2.Segment(
                    start=seg["start"],
                    end=seg["end"],
                    text=seg["text"],
                ))
                for w in seg.get("words", []):
                    words.append(stt_pb2.TranscriptWord(start=w["start"], end=w["end"], word=w["word"]))

        usage = stt_pb2.Usage(
            input_tokens=0,
            output_tokens=0,
        )

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

        return stt_pb2.ServerMessage(
            transcript=stt_pb2.TranscriptResult(**kwargs)
        )

    def ListModels(self, request, context):
        models = [
            stt_pb2.STTModel(
                id=self.engine_manager.config.model_id,
                name="Parakeet",
                description="NVIDIA Parakeet ONNX STT model",
            ),
        ]
        return stt_pb2.ListModelsResponse(models=models)

    def ListLanguages(self, request, context):
        languages = ["en"]
        return stt_pb2.ListLanguagesResponse(languages=languages)
