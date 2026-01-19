from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SessionConfig(_message.Message):
    __slots__ = ("language", "sample_rate", "initial_prompt", "hotwords", "partials", "partial_window_ms", "partial_stride_ms", "include_word_timestamps", "model_id", "task", "temperature")
    LANGUAGE_FIELD_NUMBER: _ClassVar[int]
    SAMPLE_RATE_FIELD_NUMBER: _ClassVar[int]
    INITIAL_PROMPT_FIELD_NUMBER: _ClassVar[int]
    HOTWORDS_FIELD_NUMBER: _ClassVar[int]
    PARTIALS_FIELD_NUMBER: _ClassVar[int]
    PARTIAL_WINDOW_MS_FIELD_NUMBER: _ClassVar[int]
    PARTIAL_STRIDE_MS_FIELD_NUMBER: _ClassVar[int]
    INCLUDE_WORD_TIMESTAMPS_FIELD_NUMBER: _ClassVar[int]
    MODEL_ID_FIELD_NUMBER: _ClassVar[int]
    TASK_FIELD_NUMBER: _ClassVar[int]
    TEMPERATURE_FIELD_NUMBER: _ClassVar[int]
    language: str
    sample_rate: int
    initial_prompt: str
    hotwords: str
    partials: bool
    partial_window_ms: int
    partial_stride_ms: int
    include_word_timestamps: bool
    model_id: str
    task: str
    temperature: float
    def __init__(self, language: _Optional[str] = ..., sample_rate: _Optional[int] = ..., initial_prompt: _Optional[str] = ..., hotwords: _Optional[str] = ..., partials: bool = ..., partial_window_ms: _Optional[int] = ..., partial_stride_ms: _Optional[int] = ..., include_word_timestamps: bool = ..., model_id: _Optional[str] = ..., task: _Optional[str] = ..., temperature: _Optional[float] = ...) -> None: ...

class ListModelsRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListModelsResponse(_message.Message):
    __slots__ = ("models",)
    MODELS_FIELD_NUMBER: _ClassVar[int]
    models: _containers.RepeatedCompositeFieldContainer[STTModel]
    def __init__(self, models: _Optional[_Iterable[_Union[STTModel, _Mapping]]] = ...) -> None: ...

class STTModel(_message.Message):
    __slots__ = ("id", "name", "description")
    ID_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    id: str
    name: str
    description: str
    def __init__(self, id: _Optional[str] = ..., name: _Optional[str] = ..., description: _Optional[str] = ...) -> None: ...

class ListLanguagesRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ListLanguagesResponse(_message.Message):
    __slots__ = ("languages",)
    LANGUAGES_FIELD_NUMBER: _ClassVar[int]
    languages: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, languages: _Optional[_Iterable[str]] = ...) -> None: ...

class AudioFrame(_message.Message):
    __slots__ = ("sample_rate", "pcm16")
    SAMPLE_RATE_FIELD_NUMBER: _ClassVar[int]
    PCM16_FIELD_NUMBER: _ClassVar[int]
    sample_rate: int
    pcm16: bytes
    def __init__(self, sample_rate: _Optional[int] = ..., pcm16: _Optional[bytes] = ...) -> None: ...

class EncodedAudio(_message.Message):
    __slots__ = ("format", "data")
    FORMAT_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    format: str
    data: bytes
    def __init__(self, format: _Optional[str] = ..., data: _Optional[bytes] = ...) -> None: ...

class OpusFrame(_message.Message):
    __slots__ = ("data", "sample_rate", "channels")
    DATA_FIELD_NUMBER: _ClassVar[int]
    SAMPLE_RATE_FIELD_NUMBER: _ClassVar[int]
    CHANNELS_FIELD_NUMBER: _ClassVar[int]
    data: bytes
    sample_rate: int
    channels: int
    def __init__(self, data: _Optional[bytes] = ..., sample_rate: _Optional[int] = ..., channels: _Optional[int] = ...) -> None: ...

class ReadyMessage(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class SpeechStartedMessage(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class SpeechStoppedMessage(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class ErrorMessage(_message.Message):
    __slots__ = ("message",)
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    message: str
    def __init__(self, message: _Optional[str] = ...) -> None: ...

class Segment(_message.Message):
    __slots__ = ("text", "start", "end")
    TEXT_FIELD_NUMBER: _ClassVar[int]
    START_FIELD_NUMBER: _ClassVar[int]
    END_FIELD_NUMBER: _ClassVar[int]
    text: str
    start: float
    end: float
    def __init__(self, text: _Optional[str] = ..., start: _Optional[float] = ..., end: _Optional[float] = ...) -> None: ...

class Usage(_message.Message):
    __slots__ = ("input_tokens", "output_tokens")
    INPUT_TOKENS_FIELD_NUMBER: _ClassVar[int]
    OUTPUT_TOKENS_FIELD_NUMBER: _ClassVar[int]
    input_tokens: int
    output_tokens: int
    def __init__(self, input_tokens: _Optional[int] = ..., output_tokens: _Optional[int] = ...) -> None: ...

class TranscriptWord(_message.Message):
    __slots__ = ("word", "start", "end")
    WORD_FIELD_NUMBER: _ClassVar[int]
    START_FIELD_NUMBER: _ClassVar[int]
    END_FIELD_NUMBER: _ClassVar[int]
    word: str
    start: float
    end: float
    def __init__(self, word: _Optional[str] = ..., start: _Optional[float] = ..., end: _Optional[float] = ...) -> None: ...

class TranscriptResult(_message.Message):
    __slots__ = ("text", "is_partial", "start_ms", "end_ms", "audio_duration_ms", "processing_duration_ms", "segments", "usage", "model", "words", "eou_probability")
    TEXT_FIELD_NUMBER: _ClassVar[int]
    IS_PARTIAL_FIELD_NUMBER: _ClassVar[int]
    START_MS_FIELD_NUMBER: _ClassVar[int]
    END_MS_FIELD_NUMBER: _ClassVar[int]
    AUDIO_DURATION_MS_FIELD_NUMBER: _ClassVar[int]
    PROCESSING_DURATION_MS_FIELD_NUMBER: _ClassVar[int]
    SEGMENTS_FIELD_NUMBER: _ClassVar[int]
    USAGE_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    WORDS_FIELD_NUMBER: _ClassVar[int]
    EOU_PROBABILITY_FIELD_NUMBER: _ClassVar[int]
    text: str
    is_partial: bool
    start_ms: int
    end_ms: int
    audio_duration_ms: int
    processing_duration_ms: int
    segments: _containers.RepeatedCompositeFieldContainer[Segment]
    usage: Usage
    model: str
    words: _containers.RepeatedCompositeFieldContainer[TranscriptWord]
    eou_probability: float
    def __init__(self, text: _Optional[str] = ..., is_partial: bool = ..., start_ms: _Optional[int] = ..., end_ms: _Optional[int] = ..., audio_duration_ms: _Optional[int] = ..., processing_duration_ms: _Optional[int] = ..., segments: _Optional[_Iterable[_Union[Segment, _Mapping]]] = ..., usage: _Optional[_Union[Usage, _Mapping]] = ..., model: _Optional[str] = ..., words: _Optional[_Iterable[_Union[TranscriptWord, _Mapping]]] = ..., eou_probability: _Optional[float] = ...) -> None: ...

class ClientMessage(_message.Message):
    __slots__ = ("config", "audio", "encoded_audio", "end_of_stream", "opus_frame")
    CONFIG_FIELD_NUMBER: _ClassVar[int]
    AUDIO_FIELD_NUMBER: _ClassVar[int]
    ENCODED_AUDIO_FIELD_NUMBER: _ClassVar[int]
    END_OF_STREAM_FIELD_NUMBER: _ClassVar[int]
    OPUS_FRAME_FIELD_NUMBER: _ClassVar[int]
    config: SessionConfig
    audio: AudioFrame
    encoded_audio: EncodedAudio
    end_of_stream: bool
    opus_frame: OpusFrame
    def __init__(self, config: _Optional[_Union[SessionConfig, _Mapping]] = ..., audio: _Optional[_Union[AudioFrame, _Mapping]] = ..., encoded_audio: _Optional[_Union[EncodedAudio, _Mapping]] = ..., end_of_stream: bool = ..., opus_frame: _Optional[_Union[OpusFrame, _Mapping]] = ...) -> None: ...

class ServerMessage(_message.Message):
    __slots__ = ("ready", "speech_started", "speech_stopped", "transcript", "error")
    READY_FIELD_NUMBER: _ClassVar[int]
    SPEECH_STARTED_FIELD_NUMBER: _ClassVar[int]
    SPEECH_STOPPED_FIELD_NUMBER: _ClassVar[int]
    TRANSCRIPT_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    ready: ReadyMessage
    speech_started: SpeechStartedMessage
    speech_stopped: SpeechStoppedMessage
    transcript: TranscriptResult
    error: ErrorMessage
    def __init__(self, ready: _Optional[_Union[ReadyMessage, _Mapping]] = ..., speech_started: _Optional[_Union[SpeechStartedMessage, _Mapping]] = ..., speech_stopped: _Optional[_Union[SpeechStoppedMessage, _Mapping]] = ..., transcript: _Optional[_Union[TranscriptResult, _Mapping]] = ..., error: _Optional[_Union[ErrorMessage, _Mapping]] = ...) -> None: ...
