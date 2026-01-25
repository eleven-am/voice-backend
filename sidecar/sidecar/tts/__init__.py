from sidecar.tts.pb2 import *
from sidecar.tts.pb2_grpc import *

from sidecar.tts.chatterbox_model_manager import (
    TTSConfig,
    SynthesisConfig,
    ChatterboxModelManager,
    SAMPLE_RATE,
)
from sidecar.tts.voice_store import (
    VoiceStore,
    VoiceStoreError,
    VoiceMetadata,
)
from sidecar.tts.synthesis import (
    Synthesizer,
    SynthesisError,
    float32_to_pcm16,
)
from sidecar.tts.grpc_servicer import TextToSpeechServiceServicer
from sidecar.tts.encoding import encode_audio

__all__ = [
    "TTSConfig",
    "SynthesisConfig",
    "ChatterboxModelManager",
    "SAMPLE_RATE",
    "VoiceStore",
    "VoiceStoreError",
    "VoiceMetadata",
    "Synthesizer",
    "SynthesisError",
    "float32_to_pcm16",
    "TextToSpeechServiceServicer",
    "encode_audio",
]
