from __future__ import annotations

import threading
from dataclasses import dataclass, field

import numpy as np
from numpy.typing import NDArray


@dataclass
class SpeechSession:
    lock: threading.Lock = field(default_factory=threading.Lock)
    active: bool = False
    buffer: list[NDArray[np.float32]] = field(default_factory=list)
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

    def append_audio(self, audio: NDArray[np.float32]) -> None:
        with self.lock:
            if self.active:
                self.buffer.append(audio)
                if len(self.buffer) > self.max_chunks:
                    self.buffer = self.buffer[-self.max_chunks :]

    def get_buffer_copy(self) -> list[NDArray[np.float32]]:
        with self.lock:
            return list(self.buffer)

    def update_partial(self, new_partial_ms: int, new_words: list[str]) -> None:
        with self.lock:
            self.last_partial_ms = new_partial_ms
            self.confirmed_words = new_words

    def get_partial_state(self) -> tuple[int, list[str]]:
        with self.lock:
            return self.last_partial_ms, list(self.confirmed_words)
