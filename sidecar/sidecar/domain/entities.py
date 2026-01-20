from __future__ import annotations

import threading
from dataclasses import dataclass, field

import numpy as np
from numpy.typing import NDArray

from sidecar.domain.constants import TARGET_SAMPLE_RATE

MAX_SESSION_BUFFER_MS = 30000
MAX_SESSION_BUFFER_SAMPLES = MAX_SESSION_BUFFER_MS * TARGET_SAMPLE_RATE // 1000


class SessionAudioBuffer:
    __slots__ = ("_buffer", "_write_pos", "_length")

    def __init__(self, max_samples: int = MAX_SESSION_BUFFER_SAMPLES) -> None:
        self._buffer = np.zeros(max_samples, dtype=np.float32)
        self._write_pos = 0
        self._length = 0

    def append(self, audio: NDArray[np.float32]) -> None:
        n = len(audio)
        if n == 0:
            return

        if n >= len(self._buffer):
            self._buffer[:] = audio[-len(self._buffer) :]
            self._write_pos = 0
            self._length = len(self._buffer)
            return

        end_pos = self._write_pos + n
        if end_pos <= len(self._buffer):
            self._buffer[self._write_pos : end_pos] = audio
        else:
            first_part = len(self._buffer) - self._write_pos
            self._buffer[self._write_pos :] = audio[:first_part]
            self._buffer[: n - first_part] = audio[first_part:]

        self._write_pos = end_pos % len(self._buffer)
        self._length = min(self._length + n, len(self._buffer))

    def get_all(self) -> NDArray[np.float32]:
        if self._length == 0:
            return np.array([], dtype=np.float32)

        if self._length < len(self._buffer):
            return self._buffer[: self._length].copy()

        start_pos = self._write_pos
        if start_pos == 0:
            return self._buffer.copy()
        return np.concatenate([self._buffer[start_pos:], self._buffer[:start_pos]])

    def get_tail(self, n_samples: int) -> NDArray[np.float32]:
        n = min(n_samples, self._length)
        if n == 0:
            return np.array([], dtype=np.float32)

        end_pos = self._write_pos
        start_pos = (end_pos - n) % len(self._buffer)

        if start_pos < end_pos:
            return self._buffer[start_pos:end_pos].copy()
        return np.concatenate([self._buffer[start_pos:], self._buffer[:end_pos]])

    def __len__(self) -> int:
        return self._length

    def clear(self) -> None:
        self._write_pos = 0
        self._length = 0


def _create_session_buffer() -> SessionAudioBuffer:
    return SessionAudioBuffer()


@dataclass
class SpeechSession:
    lock: threading.Lock = field(default_factory=threading.Lock)
    active: bool = False
    buffer: SessionAudioBuffer = field(default_factory=_create_session_buffer)
    confirmed_words: list[str] = field(default_factory=list)
    last_partial_ms: int = 0

    def start_speech(self) -> None:
        with self.lock:
            self.active = True
            self.buffer.clear()
            self.confirmed_words = []
            self.last_partial_ms = 0

    def stop_speech(self) -> None:
        with self.lock:
            self.active = False

    def is_active(self) -> bool:
        with self.lock:
            return self.active

    def append_audio(self, audio: NDArray[np.float32]) -> None:
        with self.lock:
            if self.active:
                self.buffer.append(audio)

    def get_buffer_audio(self) -> NDArray[np.float32]:
        with self.lock:
            return self.buffer.get_all()

    def get_buffer_tail(self, n_samples: int) -> NDArray[np.float32]:
        with self.lock:
            return self.buffer.get_tail(n_samples)

    def get_buffer_length(self) -> int:
        with self.lock:
            return len(self.buffer)

    def update_partial(self, new_partial_ms: int, new_words: list[str]) -> None:
        with self.lock:
            self.last_partial_ms = new_partial_ms
            self.confirmed_words = new_words

    def get_partial_state(self) -> tuple[int, list[str]]:
        with self.lock:
            return self.last_partial_ms, list(self.confirmed_words)
