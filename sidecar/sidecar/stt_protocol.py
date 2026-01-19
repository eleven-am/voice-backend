from __future__ import annotations

from typing import Protocol, runtime_checkable

import numpy as np
from numpy.typing import NDArray

from sidecar.types import Transcript


@runtime_checkable
class STTEngine(Protocol):
    def transcribe(
        self,
        audio: NDArray[np.float32],
        language: str | None = None,
        word_timestamps: bool = False,
    ) -> Transcript: ...

    def get_supported_languages(self) -> list[str]: ...

    def get_sample_rate(self) -> int: ...


@runtime_checkable
class STTEngineLifecycle(Protocol):
    def load(self) -> None: ...

    def unload(self) -> None: ...

    @property
    def is_loaded(self) -> bool: ...
