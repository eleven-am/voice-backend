from sidecar.tts.encoding import encode_audio
from sidecar.tts.grpc_servicer import TextToSpeechServiceServicer
from sidecar.tts.model_manager import (
    KOKORO_VOICES,
    SAMPLE_RATE,
    KokoroModelManager,
    SynthesisConfig,
    TTSConfig,
)
from sidecar.tts.pb2 import *
from sidecar.tts.pb2_grpc import *
from sidecar.tts.synthesis import (
    SynthesisError,
    Synthesizer,
    float32_to_pcm16,
)

__all__ = [
    "TTSConfig",
    "SynthesisConfig",
    "KokoroModelManager",
    "KOKORO_VOICES",
    "SAMPLE_RATE",
    "Synthesizer",
    "SynthesisError",
    "float32_to_pcm16",
    "TextToSpeechServiceServicer",
    "encode_audio",
]
