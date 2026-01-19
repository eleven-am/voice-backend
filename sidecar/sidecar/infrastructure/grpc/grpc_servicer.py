from __future__ import annotations

import logging
from collections.abc import Iterable, Iterator
from concurrent.futures import ThreadPoolExecutor

from sidecar import stt_pb2, stt_pb2_grpc
from sidecar.application.partial_service import PartialTranscriptService
from sidecar.application.transcription_service import TranscriptionService
from sidecar.domain.constants import TARGET_SAMPLE_RATE, samples_to_ms
from sidecar.domain.entities import SpeechSession
from sidecar.engine_manager import STTEngineManager
from sidecar.infrastructure.codecs.audio_codec import pcm16_to_float32, resample_audio
from sidecar.infrastructure.grpc.message_mapper import MessageMapper
from sidecar.opus_decoder import OPUS_SAMPLE_RATE, OpusStreamDecoder
from sidecar.stt_pipeline import STTPipeline, STTPipelineConfig
from sidecar.types import SessionConfig, SpeechStarted, SpeechStopped, Transcript

logger = logging.getLogger(__name__)

_NOT_CONFIGURED_ERROR = stt_pb2.ServerMessage(
    error=stt_pb2.ErrorMessage(message="Session not configured")
)


class TranscriptionServiceServicer(stt_pb2_grpc.TranscriptionServiceServicer):
    def __init__(
        self,
        engine_manager: STTEngineManager,
        pipeline_config: STTPipelineConfig,
        executor: ThreadPoolExecutor,
    ) -> None:
        self._engine_manager = engine_manager
        self._pipeline_config = pipeline_config
        self._executor = executor
        self._transcription_service = TranscriptionService(engine_manager)

    def _create_pipeline(self, pb_config: stt_pb2.SessionConfig) -> tuple[STTPipeline, SessionConfig]:
        pipeline = STTPipeline(
            engine_manager=self._engine_manager,
            config=self._pipeline_config,
        )
        domain_config = MessageMapper.to_domain_session_config(pb_config)
        pipeline.configure(domain_config)
        logger.info(f"Session configured: language={domain_config.language}, sample_rate={domain_config.sample_rate}")
        return pipeline, domain_config

    def _process_pipeline_events(
        self,
        pipeline: STTPipeline,
        audio,
        session: SpeechSession,
    ) -> Iterator[stt_pb2.ServerMessage]:
        for event in pipeline.process_audio(audio):
            if isinstance(event, SpeechStarted):
                yield MessageMapper.to_grpc_speech_started()
                session.start_speech()
            elif isinstance(event, SpeechStopped):
                yield MessageMapper.to_grpc_speech_stopped()
                session.stop_speech()
            elif isinstance(event, Transcript):
                yield MessageMapper.to_grpc_transcript(event, is_partial=False)

    def _handle_active_session(
        self,
        session: SpeechSession,
        audio,
        partial_service: PartialTranscriptService,
        domain_config: SessionConfig,
    ) -> Iterator[stt_pb2.ServerMessage]:
        if not session.is_active():
            return
        session.append_audio(audio)
        if domain_config.partials:
            try:
                transcript = partial_service.generate_partial(session, domain_config)
                if transcript:
                    yield MessageMapper.to_grpc_transcript(transcript, is_partial=True)
            except Exception as e:
                yield MessageMapper.to_grpc_error(e)

    def Transcribe(self, request_iterator: Iterable[stt_pb2.ClientMessage], context) -> Iterator[stt_pb2.ServerMessage]:
        pipeline: STTPipeline | None = None
        pb_config: stt_pb2.SessionConfig | None = None
        domain_config: SessionConfig | None = None
        session = SpeechSession()
        opus_decoder: OpusStreamDecoder | None = None
        partial_service: PartialTranscriptService | None = None

        for client_msg in request_iterator:
            if context and not context.is_active():
                logger.info("Client cancelled, stopping transcription")
                break

            msg_type = client_msg.WhichOneof("msg")

            if msg_type == "config":
                if pb_config is not None:
                    yield stt_pb2.ServerMessage(error=stt_pb2.ErrorMessage(message="Session already configured"))
                    continue
                pb_config = client_msg.config
                pipeline, domain_config = self._create_pipeline(pb_config)
                partial_service = PartialTranscriptService(self._transcription_service.transcribe_with_retry)
                yield MessageMapper.to_grpc_ready()
                continue

            if pb_config is None or pipeline is None or domain_config is None or partial_service is None:
                yield _NOT_CONFIGURED_ERROR
                continue

            if msg_type == "audio":
                audio = pcm16_to_float32(client_msg.audio.pcm16)
                src_rate = client_msg.audio.sample_rate or pb_config.sample_rate or TARGET_SAMPLE_RATE
                if src_rate != TARGET_SAMPLE_RATE:
                    audio = resample_audio(audio, src_rate, TARGET_SAMPLE_RATE)

                yield from self._process_pipeline_events(pipeline, audio, session)
                yield from self._handle_active_session(session, audio, partial_service, domain_config)

            elif msg_type == "encoded_audio":
                try:
                    encoded_msg = client_msg.encoded_audio
                    transcript = self._transcription_service.transcribe_encoded(
                        data=encoded_msg.data,
                        fmt=encoded_msg.format or None,
                        language=domain_config.language,
                        word_timestamps=domain_config.include_word_timestamps,
                    )
                    yield MessageMapper.to_grpc_transcript(transcript, is_partial=False)
                except Exception as e:
                    yield MessageMapper.to_grpc_error(e)

            elif msg_type == "opus_frame":
                opus_frame = client_msg.opus_frame
                if opus_decoder is None:
                    sample_rate = opus_frame.sample_rate or OPUS_SAMPLE_RATE
                    channels = opus_frame.channels or 1
                    opus_decoder = OpusStreamDecoder(sample_rate=sample_rate, channels=channels)

                try:
                    audio = opus_decoder.decode_frame(opus_frame.data)
                    audio = resample_audio(audio, OPUS_SAMPLE_RATE, TARGET_SAMPLE_RATE)

                    yield from self._process_pipeline_events(pipeline, audio, session)
                    yield from self._handle_active_session(session, audio, partial_service, domain_config)
                except Exception as e:
                    yield MessageMapper.to_grpc_error(e)

            elif msg_type == "end_of_stream":
                break

        if pipeline is not None and partial_service is not None:
            remaining_audio = partial_service.flush_remaining_audio(session)
            if remaining_audio is not None and len(remaining_audio) > 0:
                try:
                    duration_ms = samples_to_ms(len(remaining_audio))
                    if duration_ms > 0:
                        transcript = self._transcription_service.transcribe_with_retry(audio=remaining_audio)
                        yield MessageMapper.to_grpc_transcript(transcript, is_partial=False)
                except Exception as e:
                    yield MessageMapper.to_grpc_error(e)
            pipeline.reset()

    def ListModels(self, request, context):
        models = [
            stt_pb2.STTModel(
                id=self._engine_manager.config.model_id,
                name="Parakeet",
                description="NVIDIA Parakeet ONNX STT model",
            ),
        ]
        return stt_pb2.ListModelsResponse(models=models)

    def ListLanguages(self, request, context):
        return stt_pb2.ListLanguagesResponse(languages=["en"])
