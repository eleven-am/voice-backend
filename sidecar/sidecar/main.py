from __future__ import annotations

import asyncio
import logging
import resource
import signal
import sys
from concurrent.futures import ThreadPoolExecutor

import grpc
from grpc import aio as grpc_aio

from sidecar.shared.utils import get_env, start_health_server, token_auth_interceptor
from sidecar.stt import pb2_grpc as stt_pb2_grpc
from sidecar.stt.engine_manager import EngineConfig, STTEngineManager
from sidecar.stt.grpc_servicer import TranscriptionServiceServicer
from sidecar.stt.pipeline import EOUConfig, STTPipelineConfig
from sidecar.stt.vad import SileroVAD, VADConfig
from sidecar.tts import pb2_grpc as tts_pb2_grpc
from sidecar.tts.grpc_servicer import TextToSpeechServiceServicer
from sidecar.tts.model_manager import KokoroModelManager, SynthesisConfig, TTSConfig
from sidecar.tts.qwen3_grpc_servicer import Qwen3TextToSpeechServiceServicer
from sidecar.tts.qwen3_model_manager import Qwen3ModelManager, Qwen3SynthesisConfig, Qwen3TTSConfig

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger(__name__)


def _has_cuda() -> bool:
    try:
        import onnxruntime as ort
        return "CUDAExecutionProvider" in ort.get_available_providers()
    except Exception:
        return False


_CUDA_AVAILABLE = _has_cuda()

engine_config = EngineConfig(
    model_id=get_env("PARAKEET_MODEL_ID", "nemo-parakeet-tdt-0.6b-v3"),
    device=get_env("STT_DEVICE", "cuda" if _CUDA_AVAILABLE else "cpu"),
    ttl=get_env("STT_MODEL_TTL", 300),
)

vad_config = VADConfig(
    threshold=get_env("VAD_THRESHOLD", 0.6),
    min_silence_duration_ms=get_env("VAD_SILENCE_MS", 500),
    speech_pad_ms=get_env("VAD_PAD_MS", 100),
    min_speech_duration_ms=get_env("VAD_MIN_SPEECH_MS", 250),
    min_audio_duration_ms=get_env("VAD_MIN_AUDIO_MS", 300),
    max_utterance_ms=get_env("VAD_MAX_UTTERANCE_MS", 15000),
)

eou_config = EOUConfig(
    threshold=get_env("EOU_THRESHOLD", 0.5),
    max_context_turns=get_env("EOU_MAX_CONTEXT_TURNS", 4),
)

pipeline_config = STTPipelineConfig(
    engine_config=engine_config,
    vad_config=vad_config,
    eou_config=eou_config,
)

engine_manager = STTEngineManager(engine_config)

tts_model_id = get_env("TTS_MODEL_ID", "hexgrad/Kokoro-82M-v1.0-ONNX")
tts_ttl = get_env("TTS_MODEL_TTL", 300)
tts_speed = get_env("TTS_SPEED", 1.0)

_USE_QWEN3_TTS = "qwen" in tts_model_id.lower()

def _has_torch_cuda() -> bool:
    try:
        import torch
        return torch.cuda.is_available()
    except Exception:
        return False

_TORCH_CUDA_AVAILABLE = _has_torch_cuda() if _USE_QWEN3_TTS else False
_tts_default_device = "cuda" if _USE_QWEN3_TTS and _TORCH_CUDA_AVAILABLE else "cpu"
tts_device = get_env("TTS_DEVICE", _tts_default_device)

if _USE_QWEN3_TTS:
    qwen3_tts_config = Qwen3TTSConfig(
        model_id=tts_model_id,
        device=tts_device,
        ttl=tts_ttl,
    )
    qwen3_synthesis_config = Qwen3SynthesisConfig(speed=tts_speed)
    tts_model_manager = Qwen3ModelManager(qwen3_tts_config)
else:
    tts_config = TTSConfig(
        model_id=tts_model_id,
        device=tts_device,
        ttl=tts_ttl,
    )
    synthesis_config = SynthesisConfig(speed=tts_speed)
    tts_model_manager = KokoroModelManager(tts_config)


def _preload_models() -> None:
    preload_stt = get_env("PRELOAD_STT", True)
    preload_tts = get_env("PRELOAD_TTS", True)
    preload_vad = get_env("PRELOAD_VAD", True)

    if preload_vad:
        logger.info("Preloading VAD model...")
        SileroVAD()._ensure_model()

    if preload_tts:
        logger.info("Preloading TTS model...")
        tts_model_manager.preload()

    if preload_stt:
        logger.info("Preloading STT model...")
        engine_manager.preload()


async def serve() -> None:
    port = get_env("GRPC_PORT", 50051)
    grpc_workers = get_env("GRPC_WORKERS", 10)
    stt_workers = get_env("STT_WORKERS", 4)
    tts_workers = get_env("TTS_WORKERS", 4)
    tls_cert = get_env("GRPC_TLS_CERT", "")
    tls_key = get_env("GRPC_TLS_KEY", "")
    token = get_env("SIDECAR_TOKEN", "")
    health_port = get_env("HEALTH_PORT", 8081)

    for name, val in [
        ("GRPC_PORT", port),
        ("GRPC_WORKERS", grpc_workers),
        ("STT_WORKERS", stt_workers),
        ("TTS_WORKERS", tts_workers),
        ("HEALTH_PORT", health_port),
    ]:
        if isinstance(val, int) and val <= 0:
            logger.error("Invalid setting %s=%s (must be > 0)", name, val)
            sys.exit(1)

    stt_executor = ThreadPoolExecutor(max_workers=stt_workers, thread_name_prefix="stt")
    tts_executor = ThreadPoolExecutor(max_workers=tts_workers, thread_name_prefix="tts")

    interceptors = []
    if token:
        interceptors.append(token_auth_interceptor(token))

    max_msg_size = get_env("GRPC_MAX_MESSAGE_SIZE", 512) * 1024 * 1024
    server = grpc_aio.server(
        interceptors=interceptors,
        options=[
            ('grpc.max_send_message_length', max_msg_size),
            ('grpc.max_receive_message_length', max_msg_size),
        ],
    )
    stt_pb2_grpc.add_TranscriptionServiceServicer_to_server(
        TranscriptionServiceServicer(engine_manager, pipeline_config, stt_executor),
        server,
    )
    if _USE_QWEN3_TTS:
        tts_pb2_grpc.add_TextToSpeechServiceServicer_to_server(
            Qwen3TextToSpeechServiceServicer(tts_model_manager, qwen3_synthesis_config, tts_executor),
            server,
        )
    else:
        tts_pb2_grpc.add_TextToSpeechServiceServicer_to_server(
            TextToSpeechServiceServicer(tts_model_manager, synthesis_config, tts_executor),
            server,
        )
    if tls_cert and tls_key:
        with open(tls_cert, "rb") as f:
            cert = f.read()
        with open(tls_key, "rb") as f:
            key = f.read()
        server_credentials = grpc.ssl_server_credentials(((cert, key),))
        server.add_secure_port(f"[::]:{port}", server_credentials)
        logger.info("gRPC server using TLS")
    else:
        server.add_insecure_port(f"[::]:{port}")
        logger.info("gRPC server using insecure transport")

    logger.info(f"Starting gRPC server on port {port}")
    logger.info(f"STT model: {engine_config.model_id}")
    logger.info(f"STT device: {engine_config.device}")
    logger.info(f"TTS backend: {'Qwen3-TTS' if _USE_QWEN3_TTS else 'Kokoro'}")
    logger.info(f"TTS model: {tts_model_id}")
    logger.info(f"TTS device: {tts_device}")
    logger.info(f"VAD threshold: {vad_config.threshold}")
    logger.info(f"EOU threshold: {eou_config.threshold}")
    logger.info(f"gRPC workers: {grpc_workers}, STT workers: {stt_workers}, TTS workers: {tts_workers}")
    logger.info(f"Health server on :{health_port}")

    _preload_models()

    def metrics_fn() -> str:
        rss_kb = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
        return "\n".join([
            f"hu_sidecar_rss_kb {rss_kb}",
            f"hu_sidecar_loaded_engines {len(engine_manager.engines)}",
        ]) + "\n"

    health_server = start_health_server(health_port, metrics_fn=metrics_fn)

    shutdown_event = asyncio.Event()

    def shutdown(*_args) -> None:
        logger.info("Shutting down gRPC server...")
        shutdown_event.set()

    signal.signal(signal.SIGINT, shutdown)
    signal.signal(signal.SIGTERM, shutdown)

    await server.start()

    tts_model_manager.start_cleanup_task()

    await shutdown_event.wait()

    tts_model_manager.stop_cleanup_task()
    tts_model_manager.unload_all()
    await server.stop(grace=5)
    stt_executor.shutdown(wait=False, cancel_futures=True)
    tts_executor.shutdown(wait=False, cancel_futures=True)
    try:
        health_server.shutdown()
    except Exception:
        pass


def cli() -> None:
    asyncio.run(serve())


if __name__ == "__main__":
    cli()
