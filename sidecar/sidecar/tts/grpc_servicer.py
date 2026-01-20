from __future__ import annotations

import asyncio
import logging
import time
from collections.abc import AsyncIterator
from concurrent.futures import ThreadPoolExecutor

from sidecar.tts import pb2 as tts_pb2
from sidecar.tts import pb2_grpc as tts_pb2_grpc
from sidecar.tts.synthesis import SAMPLE_RATE, Synthesizer, SynthesisError, float32_to_pcm16
from sidecar.tts.model_manager import KOKORO_VOICES, KokoroModelManager, SynthesisConfig
from sidecar.tts.encoding import encode_audio_async
from sidecar.tts.opus_encoder import StreamingOpusEncoder, has_native_opus
from sidecar.tts.mp3_encoder import StreamingMP3Encoder, has_native_mp3

logger = logging.getLogger(__name__)


class TextToSpeechServiceServicer(tts_pb2_grpc.TextToSpeechServiceServicer):
    def __init__(
        self,
        model_manager: KokoroModelManager,
        config: SynthesisConfig,
        executor: ThreadPoolExecutor,
    ) -> None:
        self.model_manager = model_manager
        self.config = config
        self.executor = executor

    async def _parse_tts_config(
        self, request_iterator
    ) -> tuple[tts_pb2.TtsSessionConfig | None, list[str], str, str, list[tts_pb2.TtsServerMessage]]:
        session_config: tts_pb2.TtsSessionConfig | None = None
        text_chunks: list[str] = []
        voice_id = "af_heart"
        response_format = "pcm"
        errors: list[tts_pb2.TtsServerMessage] = []

        async for client_msg in request_iterator:
            msg_type = client_msg.WhichOneof("msg")

            if msg_type == "config":
                if session_config is not None:
                    errors.append(tts_pb2.TtsServerMessage(
                        error=tts_pb2.TtsError(message="Session already configured", code=1)
                    ))
                    continue

                session_config = client_msg.config
                voice_id = session_config.voice_id or "af_heart"
                if session_config.response_format:
                    response_format = session_config.response_format.lower()

                logger.info(f"TTS session configured: voice={voice_id}")
                continue

            if msg_type == "text":
                if session_config is None:
                    errors.append(tts_pb2.TtsServerMessage(
                        error=tts_pb2.TtsError(message="Session not configured", code=2)
                    ))
                    continue
                text_chunks.append(client_msg.text.text)
                continue

            if msg_type == "end":
                break

        return session_config, text_chunks, voice_id, response_format, errors

    def _create_audio_chunk(
        self, data: bytes, format: str, audio_samples: int, transcript: str | None = None
    ) -> tts_pb2.TtsServerMessage:
        sample_rate = 48000 if format == "opus" else SAMPLE_RATE
        return tts_pb2.TtsServerMessage(
            audio=tts_pb2.AudioChunk(
                data=data,
                format=format,
                sample_rate=sample_rate,
                timestamp_ms=int(audio_samples * 1000 / SAMPLE_RATE),
                transcript=transcript or "",
            )
        )

    def _create_done_message(
        self, audio_samples: int, start_time: float, full_text: str
    ) -> tts_pb2.TtsServerMessage:
        processing_ms = int((time.perf_counter() - start_time) * 1000)
        audio_ms = int(audio_samples * 1000 / SAMPLE_RATE)

        logger.info(f"TTS done: {audio_ms}ms audio, {processing_ms}ms processing")

        return tts_pb2.TtsServerMessage(
            done=tts_pb2.TtsDone(
                audio_duration_ms=audio_ms,
                processing_duration_ms=processing_ms,
                text_length=len(full_text),
                usage=tts_pb2.TtsUsage(
                    audio_ms=audio_ms,
                    processing_ms=processing_ms,
                    text_chars=len(full_text),
                ),
                transcript=full_text,
            )
        )

    async def Synthesize(self, request_iterator, context) -> AsyncIterator[tts_pb2.TtsServerMessage]:
        session_config, text_chunks, voice_id, response_format, parse_errors = await self._parse_tts_config(request_iterator)

        if session_config is not None:
            yield tts_pb2.TtsServerMessage(
                ready=tts_pb2.TtsSessionReady(
                    voice_id=voice_id,
                    sample_rate=SAMPLE_RATE,
                )
            )

        for err in parse_errors:
            yield err

        if session_config is None:
            yield tts_pb2.TtsServerMessage(
                error=tts_pb2.TtsError(message="No session config received", code=3)
            )
            return

        full_text = " ".join(text_chunks).strip()
        if not full_text:
            yield tts_pb2.TtsServerMessage(
                error=tts_pb2.TtsError(message="No text provided", code=4)
            )
            return

        valid_formats = {"pcm", "opus", "mp3", "wav", "flac"}
        if response_format not in valid_formats:
            yield tts_pb2.TtsServerMessage(
                error=tts_pb2.TtsError(
                    message=f"Invalid format '{response_format}'. Supported: {', '.join(sorted(valid_formats))}",
                    code=7,
                )
            )
            return

        speed = session_config.speed if session_config.speed > 0 else self.config.speed
        synth_config = SynthesisConfig(speed=speed)

        synthesizer = Synthesizer(self.model_manager, synth_config)
        stop_event = asyncio.Event()

        start_time = time.perf_counter()
        audio_samples = 0
        chunk_count = 0
        buffer = bytearray()
        stream_pcm = response_format == "pcm"
        stream_opus = response_format == "opus"
        stream_mp3 = response_format == "mp3"

        opus_encoder: StreamingOpusEncoder | None = None
        mp3_encoder: StreamingMP3Encoder | None = None

        if stream_opus:
            if not has_native_opus():
                yield tts_pb2.TtsServerMessage(
                    error=tts_pb2.TtsError(message="Opus encoding not available", code=5)
                )
                return
            opus_encoder = StreamingOpusEncoder(source_rate=SAMPLE_RATE, target_rate=48000, channels=1)

        if stream_mp3:
            if not has_native_mp3():
                yield tts_pb2.TtsServerMessage(
                    error=tts_pb2.TtsError(message="MP3 encoding not available", code=5)
                )
                return
            mp3_encoder = StreamingMP3Encoder(sample_rate=SAMPLE_RATE, channels=1, bitrate=128)

        try:
            async for audio_chunk in synthesizer.synthesize(
                text=full_text,
                voice_id=voice_id,
                stop_event=stop_event,
            ):
                pcm16 = float32_to_pcm16(audio_chunk)
                audio_samples += len(audio_chunk)
                chunk_count += 1

                if stream_pcm:
                    yield self._create_audio_chunk(pcm16, "pcm", audio_samples, full_text)
                elif stream_opus and opus_encoder is not None:
                    encoded_frames = opus_encoder.encode(pcm16)
                    for frame in encoded_frames:
                        yield self._create_audio_chunk(frame, "opus", audio_samples)
                elif stream_mp3 and mp3_encoder is not None:
                    mp3_data = mp3_encoder.encode(pcm16)
                    if mp3_data:
                        yield self._create_audio_chunk(mp3_data, "mp3", audio_samples)
                else:
                    buffer.extend(pcm16)

        except SynthesisError as e:
            logger.error(f"Synthesis error: {e}")
            if opus_encoder is not None:
                opus_encoder.close()
            if mp3_encoder is not None:
                mp3_encoder.close()
            yield tts_pb2.TtsServerMessage(
                error=tts_pb2.TtsError(message=str(e), code=e.code)
            )
            return

        except Exception as e:
            logger.exception("Unexpected TTS failure")
            if opus_encoder is not None:
                opus_encoder.close()
            if mp3_encoder is not None:
                mp3_encoder.close()
            yield tts_pb2.TtsServerMessage(
                error=tts_pb2.TtsError(message=f"Unexpected error: {e}", code=99)
            )
            return

        if stream_opus and opus_encoder is not None:
            flush_frames = opus_encoder.flush()
            for frame in flush_frames:
                yield self._create_audio_chunk(frame, "opus", audio_samples, full_text)
        elif stream_mp3 and mp3_encoder is not None:
            flush_data = mp3_encoder.flush()
            if flush_data:
                yield self._create_audio_chunk(flush_data, "mp3", audio_samples, full_text)
        elif not stream_pcm:
            try:
                encoded, enc_format = await encode_audio_async(bytes(buffer), SAMPLE_RATE, response_format)
                yield self._create_audio_chunk(encoded, enc_format, audio_samples, full_text)
            except SynthesisError as e:
                logger.error(f"Encoding error: {e}")
                yield tts_pb2.TtsServerMessage(
                    error=tts_pb2.TtsError(message=str(e), code=e.code)
                )
                return

        yield self._create_done_message(audio_samples, start_time, full_text)

    def ListVoices(self, request, context):
        voices = []
        for voice_id, lang, gender in KOKORO_VOICES:
            voices.append(tts_pb2.Voice(
                id=voice_id,
                name=voice_id.split("_")[1].title(),
                language=lang,
                gender=gender,
            ))
        return tts_pb2.ListVoicesResponse(voices=voices)

    def ListModels(self, request, context):
        models = [
            tts_pb2.TTSModel(
                id=self.model_manager.config.model_id,
                name="Kokoro",
                description="Kokoro TTS model",
            ),
        ]
        return tts_pb2.ListModelsResponse(models=models)
