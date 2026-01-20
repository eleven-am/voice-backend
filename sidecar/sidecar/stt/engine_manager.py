from __future__ import annotations

import gc
import logging
import threading
import time
from collections import OrderedDict
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Generic, TypeVar

from sidecar.stt.engines.protocol import STTEngine, STTEngineLifecycle
from sidecar.shared.utils import is_oom_error

logger = logging.getLogger(__name__)

T = TypeVar("T")

MAX_OOM_RETRIES = 3


def _clear_cuda_cache() -> None:
    try:
        import torch
        if torch.cuda.is_available():
            torch.cuda.empty_cache()
            torch.cuda.synchronize()
    except ImportError:
        pass
    gc.collect()


@dataclass
class EngineConfig:
    model_id: str = "nemo-parakeet-tdt-0.6b-v3"
    device: str = "cuda"
    ttl: int = 300
    fallback_models: list[str] = field(default_factory=list)


class ManagedEngine(Generic[T]):
    def __init__(
        self,
        engine_id: str,
        create_fn: Callable[[], T],
        ttl: int,
        engine_removed_callback: Callable[[str], None] | None = None,
    ) -> None:
        self.engine_id = engine_id
        self.create_fn = create_fn
        self.ttl = ttl
        self.engine_removed_callback = engine_removed_callback
        self.ref_count: int = 0
        self.rlock = threading.RLock()
        self.expire_timer: threading.Timer | None = None
        self.engine: T | None = None

    def unload(self) -> None:
        with self.rlock:
            if self.engine is None:
                return
            if self.ref_count > 0:
                return
            if self.expire_timer:
                self.expire_timer.cancel()
            if isinstance(self.engine, STTEngineLifecycle):
                self.engine.unload()
            self.engine = None
            gc.collect()
            logger.info(f"Engine {self.engine_id} unloaded")
            if self.engine_removed_callback is not None:
                self.engine_removed_callback(self.engine_id)

    def _load(self) -> None:
        with self.rlock:
            if self.engine is not None:
                return
            logger.info(f"Creating engine {self.engine_id}")
            start = time.perf_counter()
            self.engine = self.create_fn()
            if isinstance(self.engine, STTEngineLifecycle):
                self.engine.load()
            logger.info(f"Engine {self.engine_id} ready in {time.perf_counter() - start:.2f}s")

    def _increment_ref(self) -> None:
        with self.rlock:
            self.ref_count += 1
            if self.expire_timer:
                self.expire_timer.cancel()
                self.expire_timer = None

    def _decrement_ref(self) -> None:
        with self.rlock:
            self.ref_count -= 1
            if self.ref_count <= 0 and self.ttl > 0:
                logger.info(f"Engine {self.engine_id} idle, unloading in {self.ttl}s")
                self.expire_timer = threading.Timer(self.ttl, self.unload)
                self.expire_timer.start()

    def __enter__(self) -> T:
        with self.rlock:
            if self.engine is None:
                self._load()
            self._increment_ref()
            assert self.engine is not None
            return self.engine

    def __exit__(self, *_args) -> None:
        self._decrement_ref()


class STTEngineManager:
    def __init__(self, config: EngineConfig) -> None:
        self.config = config
        self.engines: OrderedDict[str, ManagedEngine[STTEngine]] = OrderedDict()
        self._lock = threading.Lock()
        self._original_device = config.device
        self._current_device = config.device
        self._failed_models: set[str] = set()
        self._tried_cpu_fallback: bool = False

    def _handle_engine_removed(self, engine_id: str) -> None:
        with self._lock:
            if engine_id in self.engines:
                del self.engines[engine_id]

    def _create_engine(self, model_id: str) -> STTEngine:
        from sidecar.stt.engines import ParakeetConfig, ParakeetEngine

        config = ParakeetConfig(model_id=model_id, device=self._current_device)
        return ParakeetEngine(config)

    def get_engine(self, model_id: str | None = None) -> ManagedEngine[STTEngine]:
        model_id = model_id or self.config.model_id
        with self._lock:
            if model_id in self.engines:
                return self.engines[model_id]
            self.engines[model_id] = ManagedEngine[STTEngine](
                model_id,
                create_fn=lambda mid=model_id: self._create_engine(mid),
                ttl=self.config.ttl,
                engine_removed_callback=self._handle_engine_removed,
            )
            return self.engines[model_id]

    def preload(self, model_id: str | None = None) -> None:
        model_id = model_id or self.config.model_id
        wrapper = self.get_engine(model_id)
        wrapper._load()
        wrapper._increment_ref()
        wrapper._decrement_ref()

    def force_unload(self, model_id: str | None = None) -> None:
        model_id = model_id or self.config.model_id
        with self._lock:
            if model_id in self.engines:
                m = self.engines[model_id]
                with m.rlock:
                    m.ref_count = 0
                m.unload()
        gc.collect()

    def mark_model_failed(self, model_id: str) -> None:
        with self._lock:
            self._failed_models.add(model_id)
            if model_id in self.engines:
                m = self.engines[model_id]
                with m.rlock:
                    m.ref_count = 0
                m.unload()
                del self.engines[model_id]
        _clear_cuda_cache()

    def _attempt_cpu_fallback(self) -> bool:
        with self._lock:
            if self._current_device == "cuda" and not self._tried_cpu_fallback:
                logger.warning("Switching to CPU fallback due to OOM errors")
                self._current_device = "cpu"
                self._tried_cpu_fallback = True
                self._failed_models.clear()
                for engine_id in list(self.engines.keys()):
                    m = self.engines[engine_id]
                    with m.rlock:
                        m.ref_count = 0
                    m.unload()
                self.engines.clear()
                _clear_cuda_cache()
                return True
            return False

    def try_fallback(self) -> bool:
        should_attempt_cpu = False
        with self._lock:
            current_model = self.config.model_id
            if current_model in self.engines:
                self._failed_models.add(current_model)
                m = self.engines[current_model]
                with m.rlock:
                    m.ref_count = 0
                m.unload()
                del self.engines[current_model]

            remaining = [
                m for m in self.config.fallback_models
                if m not in self._failed_models
            ]
            if not remaining:
                should_attempt_cpu = True

        if should_attempt_cpu:
            return self._attempt_cpu_fallback()
        return True

    def get_engine_with_retry(self, model_id: str | None = None) -> ManagedEngine[STTEngine]:
        model_id = model_id or self.config.model_id
        models_to_try = [model_id] + [
            m for m in self.config.fallback_models
            if m != model_id and m not in self._failed_models
        ]

        errors: list[tuple[str, Exception]] = []
        for mid in models_to_try:
            if mid in self._failed_models:
                continue
            try:
                _clear_cuda_cache()
                wrapper = self.get_engine(mid)
                wrapper._load()
                return wrapper
            except Exception as e:
                _clear_cuda_cache()
                if is_oom_error(e):
                    logger.warning(f"OOM loading {mid}, trying fallback")
                    self._failed_models.add(mid)
                    errors.append((mid, e))
                    continue
                raise

        if errors and self._attempt_cpu_fallback():
            return self.get_engine_with_retry(model_id)

        error_details = "; ".join(f"{m}: {e}" for m, e in errors)
        raise RuntimeError(f"All STT models failed to load: {error_details}")

    def reset_device_preference(self) -> None:
        with self._lock:
            for engine_id in list(self.engines.keys()):
                m = self.engines[engine_id]
                with m.rlock:
                    m.ref_count = 0
                m.unload()
            self.engines.clear()
            self._current_device = self._original_device
            self._tried_cpu_fallback = False
            self._failed_models.clear()
            _clear_cuda_cache()
            logger.info(f"Reset device preference to {self._current_device}")
