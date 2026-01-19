from __future__ import annotations

import logging
from concurrent.futures import ThreadPoolExecutor
import signal
import sys
import resource

import grpc

from sidecar import stt_pb2_grpc, tts_pb2_grpc
from sidecar.engine_manager import EngineConfig, STTEngineManager
from sidecar.grpc_server import TranscriptionServiceServicer
from sidecar.stt_pipeline import STTPipelineConfig, EOUConfig
from sidecar.tts_grpc_server import TextToSpeechServiceServicer
from sidecar.tts_models import KokoroModelManager, SynthesisConfig, TTSConfig
from sidecar.utils import get_env, start_health_server
from sidecar.vad_standalone import VADConfig

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

tts_config = TTSConfig(
    model_id=get_env("TTS_MODEL_ID", "hexgrad/Kokoro-82M-v1.0-ONNX"),
    device=get_env("TTS_DEVICE", "cpu"),
    ttl=get_env("TTS_MODEL_TTL", 300),
)

synthesis_config = SynthesisConfig(
    speed=get_env("TTS_SPEED", 1.0),
)

tts_model_manager = KokoroModelManager(tts_config)


def _preload_models() -> None:
    preload_stt = get_env("PRELOAD_STT", True)
    preload_tts = get_env("PRELOAD_TTS", True)

    if preload_tts:
        logger.info("Preloading TTS model...")
        tts_model_manager.preload()

    if preload_stt:
        logger.info("Preloading STT model...")
        engine_manager.preload()


def serve() -> None:
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

    grpc_executor = ThreadPoolExecutor(max_workers=grpc_workers, thread_name_prefix="grpc")
    stt_executor = ThreadPoolExecutor(max_workers=stt_workers, thread_name_prefix="stt")
    tts_executor = ThreadPoolExecutor(max_workers=tts_workers, thread_name_prefix="tts")

    interceptors = []
    if token:
        from sidecar.utils import token_auth_interceptor
        interceptors.append(token_auth_interceptor(token))

    server = grpc.server(grpc_executor, interceptors=interceptors)
    stt_pb2_grpc.add_TranscriptionServiceServicer_to_server(
        TranscriptionServiceServicer(engine_manager, pipeline_config, stt_executor),
        server,
    )
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
    logger.info(f"TTS model: {tts_config.model_id}")
    logger.info(f"TTS device: {tts_config.device}")
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

    def shutdown(*_args) -> None:
        logger.info("Shutting down gRPC server...")
        server.stop(grace=5)
        stt_executor.shutdown(wait=False, cancel_futures=True)
        tts_executor.shutdown(wait=False, cancel_futures=True)
        grpc_executor.shutdown(wait=False, cancel_futures=True)
        try:
            health_server.shutdown()
        except Exception:
            pass
        sys.exit(0)

    signal.signal(signal.SIGINT, shutdown)
    signal.signal(signal.SIGTERM, shutdown)

    server.start()
    server.wait_for_termination()


def cli() -> None:
    serve()


if __name__ == "__main__":
    cli()
