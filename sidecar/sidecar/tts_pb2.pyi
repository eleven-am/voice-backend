from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class TtsSessionConfig(_message.Message):
    __slots__ = ("voice_id", "sample_rate", "speed", "model_id", "instructions", "language", "response_format")
    VOICE_ID_FIELD_NUMBER: _ClassVar[int]
    SAMPLE_RATE_FIELD_NUMBER: _ClassVar[int]
    SPEED_FIELD_NUMBER: _ClassVar[int]
    MODEL_ID_FIELD_NUMBER: _ClassVar[int]
    INSTRUCTIONS_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_FORMAT_FIELD_NUMBER: _ClassVar[int]
    voice_id: str
    sample_rate: int
    speed: float
    model_id: str
    instructions: str
    language: str
    response_format: str
    def __init__(self, voice_id: _Optional[str] = ..., sample_rate: _Optional[int] = ..., speed: _Optional[float] = ..., model_id: _Optional[str] = ..., instructions: _Optional[str] = ..., language: _Optional[str] = ..., response_format: _Optional[str] = ...) -> None: ...

class ListVoicesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListVoicesResponse(_message.Message):
    __slots__ = ("voices",)
    VOICES_FIELD_NUMBER: _ClassVar[int]
    voices: _containers.RepeatedCompositeFieldContainer[Voice]
    def __init__(self, voices: _Optional[_Iterable[_Union[Voice, _Mapping]]] = ...) -> None: ...

class Voice(_message.Message):
    __slots__ = ("id", "name", "language", "gender")
    ID_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    GENDER_FIELD_NUMBER: _ClassVar[int]
    id: str
    name: str
    language: str
    gender: str
    def __init__(self, id: _Optional[str] = ..., name: _Optional[str] = ..., language: _Optional[str] = ..., gender: _Optional[str] = ...) -> None: ...

class ListModelsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListModelsResponse(_message.Message):
    __slots__ = ("models",)
    MODELS_FIELD_NUMBER: _ClassVar[int]
    models: _containers.RepeatedCompositeFieldContainer[TTSModel]
    def __init__(self, models: _Optional[_Iterable[_Union[TTSModel, _Mapping]]] = ...) -> None: ...

class TTSModel(_message.Message):
    __slots__ = ("id", "name", "description")
    ID_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    id: str
    name: str
    description: str
    def __init__(self, id: _Optional[str] = ..., name: _Optional[str] = ..., description: _Optional[str] = ...) -> None: ...

class TextChunk(_message.Message):
    __slots__ = ("text",)
    TEXT_FIELD_NUMBER: _ClassVar[int]
    text: str
    def __init__(self, text: _Optional[str] = ...) -> None: ...

class EndOfText(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class TtsClientMessage(_message.Message):
    __slots__ = ("config", "text", "end")
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    TEXT_FIELD_NUMBER: _ClassVar[int]
    END_FIELD_NUMBER: _ClassVar[int]
    config: TtsSessionConfig
    text: TextChunk
    end: EndOfText
    def __init__(self, config: _Optional[_Union[TtsSessionConfig, _Mapping]] = ..., text: _Optional[_Union[TextChunk, _Mapping]] = ..., end: _Optional[_Union[EndOfText, _Mapping]] = ...) -> None: ...

class TtsSessionReady(_message.Message):
    __slots__ = ("voice_id", "sample_rate")
    VOICE_ID_FIELD_NUMBER: _ClassVar[int]
    SAMPLE_RATE_FIELD_NUMBER: _ClassVar[int]
    voice_id: str
    sample_rate: int
    def __init__(self, voice_id: _Optional[str] = ..., sample_rate: _Optional[int] = ...) -> None: ...

class AudioChunk(_message.Message):
    __slots__ = ("data", "format", "sample_rate", "timestamp_ms", "transcript")
    DATA_FIELD_NUMBER: _ClassVar[int]
    FORMAT_FIELD_NUMBER: _ClassVar[int]
    SAMPLE_RATE_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_MS_FIELD_NUMBER: _ClassVar[int]
    TRANSCRIPT_FIELD_NUMBER: _ClassVar[int]
    data: bytes
    format: str
    sample_rate: int
    timestamp_ms: int
    transcript: str
    def __init__(self, data: _Optional[bytes] = ..., format: _Optional[str] = ..., sample_rate: _Optional[int] = ..., timestamp_ms: _Optional[int] = ..., transcript: _Optional[str] = ...) -> None: ...

class TtsUsage(_message.Message):
    __slots__ = ("audio_ms", "processing_ms", "text_chars")
    AUDIO_MS_FIELD_NUMBER: _ClassVar[int]
    PROCESSING_MS_FIELD_NUMBER: _ClassVar[int]
    TEXT_CHARS_FIELD_NUMBER: _ClassVar[int]
    audio_ms: int
    processing_ms: int
    text_chars: int
    def __init__(self, audio_ms: _Optional[int] = ..., processing_ms: _Optional[int] = ..., text_chars: _Optional[int] = ...) -> None: ...

class TtsDone(_message.Message):
    __slots__ = ("audio_duration_ms", "processing_duration_ms", "text_length", "usage", "transcript")
    AUDIO_DURATION_MS_FIELD_NUMBER: _ClassVar[int]
    PROCESSING_DURATION_MS_FIELD_NUMBER: _ClassVar[int]
    TEXT_LENGTH_FIELD_NUMBER: _ClassVar[int]
    USAGE_FIELD_NUMBER: _ClassVar[int]
    TRANSCRIPT_FIELD_NUMBER: _ClassVar[int]
    audio_duration_ms: int
    processing_duration_ms: int
    text_length: int
    usage: TtsUsage
    transcript: str
    def __init__(self, audio_duration_ms: _Optional[int] = ..., processing_duration_ms: _Optional[int] = ..., text_length: _Optional[int] = ..., usage: _Optional[_Union[TtsUsage, _Mapping]] = ..., transcript: _Optional[str] = ...) -> None: ...

class TtsError(_message.Message):
    __slots__ = ("message", "code")
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    CODE_FIELD_NUMBER: _ClassVar[int]
    message: str
    code: int
    def __init__(self, message: _Optional[str] = ..., code: _Optional[int] = ...) -> None: ...

class TtsServerMessage(_message.Message):
    __slots__ = ("ready", "audio", "done", "error")
    READY_FIELD_NUMBER: _ClassVar[int]
    AUDIO_FIELD_NUMBER: _ClassVar[int]
    DONE_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    ready: TtsSessionReady
    audio: AudioChunk
    done: TtsDone
    error: TtsError
    def __init__(self, ready: _Optional[_Union[TtsSessionReady, _Mapping]] = ..., audio: _Optional[_Union[AudioChunk, _Mapping]] = ..., done: _Optional[_Union[TtsDone, _Mapping]] = ..., error: _Optional[_Union[TtsError, _Mapping]] = ...) -> None: ...
