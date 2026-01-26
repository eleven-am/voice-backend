from __future__ import annotations

import time
from unittest.mock import MagicMock, patch

import pytest

from sidecar.stt.engine_manager import (
    MAX_OOM_RETRIES,
    EngineConfig,
    ManagedEngine,
    STTEngineManager,
    _clear_cuda_cache,
)


class TestEngineConfig:
    def test_default_values(self):
        config = EngineConfig()

        assert config.model_id == "nemo-parakeet-tdt-0.6b-v3"
        assert config.device == "cuda"
        assert config.ttl == 300
        assert config.fallback_models == []

    def test_custom_values(self):
        config = EngineConfig(
            model_id="custom-model",
            device="cpu",
            ttl=600,
            fallback_models=["fallback-1", "fallback-2"],
        )

        assert config.model_id == "custom-model"
        assert config.device == "cpu"
        assert config.ttl == 600
        assert config.fallback_models == ["fallback-1", "fallback-2"]


class TestClearCudaCache:
    def test_clear_with_cuda_available(self):
        mock_torch = MagicMock()
        mock_torch.cuda.is_available.return_value = True

        with patch.dict("sys.modules", {"torch": mock_torch}):
            _clear_cuda_cache()

        mock_torch.cuda.empty_cache.assert_called_once()
        mock_torch.cuda.synchronize.assert_called_once()

    def test_clear_without_cuda(self):
        mock_torch = MagicMock()
        mock_torch.cuda.is_available.return_value = False

        with patch.dict("sys.modules", {"torch": mock_torch}):
            _clear_cuda_cache()

        mock_torch.cuda.empty_cache.assert_not_called()

    def test_clear_without_torch(self):
        with patch.dict("sys.modules", {"torch": None}):
            with patch("sidecar.stt.engine_manager.gc.collect") as mock_gc:
                try:
                    _clear_cuda_cache()
                except (ImportError, TypeError):
                    pass


class TestManagedEngine:
    def test_context_manager_loads_engine(self):
        create_fn = MagicMock(return_value="mock_engine")
        managed = ManagedEngine("test", create_fn, ttl=300)

        with managed as engine:
            assert engine == "mock_engine"
            assert managed.ref_count == 1

        create_fn.assert_called_once()

    def test_context_manager_decrements_ref(self):
        create_fn = MagicMock(return_value="mock_engine")
        managed = ManagedEngine("test", create_fn, ttl=-1)

        with managed as engine:
            assert managed.ref_count == 1

        assert managed.ref_count == 0

    def test_multiple_context_managers(self):
        create_fn = MagicMock(return_value="mock_engine")
        managed = ManagedEngine("test", create_fn, ttl=300)

        with managed as e1:
            assert managed.ref_count == 1
            with managed as e2:
                assert managed.ref_count == 2
                assert e1 == e2
            assert managed.ref_count == 1

        assert managed.ref_count == 0

    def test_unload_when_no_refs(self):
        mock_engine = MagicMock()
        create_fn = MagicMock(return_value=mock_engine)
        managed = ManagedEngine("test", create_fn, ttl=-1)

        with managed:
            pass

        managed.unload()
        assert managed.engine is None

    def test_unload_with_refs_does_nothing(self):
        mock_engine = MagicMock()
        create_fn = MagicMock(return_value=mock_engine)
        managed = ManagedEngine("test", create_fn, ttl=-1)

        managed._load()
        managed._increment_ref()
        managed.unload()

        assert managed.engine is not None

    def test_ttl_schedules_unload(self):
        create_fn = MagicMock(return_value="mock_engine")
        managed = ManagedEngine("test", create_fn, ttl=0.1)

        with managed:
            pass

        time.sleep(0.2)
        assert managed.engine is None

    def test_removed_callback_called(self):
        callback = MagicMock()
        create_fn = MagicMock(return_value="mock_engine")
        managed = ManagedEngine("test", create_fn, ttl=-1, engine_removed_callback=callback)

        with managed:
            pass

        managed.unload()
        callback.assert_called_once_with("test")

    def test_unload_calls_lifecycle_method(self):
        from sidecar.stt.engines.protocol import STTEngineLifecycle

        mock_engine = MagicMock(spec=STTEngineLifecycle)
        create_fn = MagicMock(return_value=mock_engine)
        managed = ManagedEngine("test", create_fn, ttl=-1)

        with managed:
            mock_engine.load.assert_called_once()

        managed.unload()
        mock_engine.unload.assert_called_once()


class TestSTTEngineManager:
    def test_get_engine_creates_new(self):
        config = EngineConfig(model_id="test-model", device="cpu", ttl=-1)
        manager = STTEngineManager(config)

        with patch.object(manager, "_create_engine") as mock_create:
            mock_create.return_value = MagicMock()
            wrapper = manager.get_engine()

            assert "test-model" in manager.engines

    def test_get_engine_returns_existing(self):
        config = EngineConfig(model_id="test-model", device="cpu", ttl=-1)
        manager = STTEngineManager(config)

        with patch.object(manager, "_create_engine") as mock_create:
            mock_create.return_value = MagicMock()
            wrapper1 = manager.get_engine()
            wrapper2 = manager.get_engine()

            assert wrapper1 is wrapper2
            assert mock_create.call_count == 0

    def test_get_engine_with_custom_model(self):
        config = EngineConfig(model_id="default-model", device="cpu", ttl=-1)
        manager = STTEngineManager(config)

        with patch.object(manager, "_create_engine") as mock_create:
            mock_create.return_value = MagicMock()
            wrapper = manager.get_engine("custom-model")

            assert "custom-model" in manager.engines
            assert "default-model" not in manager.engines

    def test_mark_model_failed(self):
        config = EngineConfig(model_id="test-model", device="cpu", ttl=-1)
        manager = STTEngineManager(config)

        with patch.object(manager, "_create_engine") as mock_create:
            mock_create.return_value = MagicMock()
            wrapper = manager.get_engine()
            wrapper._load()
            wrapper._increment_ref()
            wrapper._decrement_ref()

        with patch("sidecar.stt.engine_manager._clear_cuda_cache"):
            manager.mark_model_failed("test-model")

        assert "test-model" in manager._failed_models
        assert "test-model" not in manager.engines

    def test_attempt_cpu_fallback_from_cuda(self):
        config = EngineConfig(model_id="test-model", device="cuda", ttl=-1)
        manager = STTEngineManager(config)

        with patch("sidecar.stt.engine_manager._clear_cuda_cache"):
            result = manager._attempt_cpu_fallback()

        assert result is True
        assert manager._current_device == "cpu"
        assert manager._tried_cpu_fallback is True
        assert len(manager._failed_models) == 0

    def test_attempt_cpu_fallback_already_on_cpu(self):
        config = EngineConfig(model_id="test-model", device="cpu", ttl=-1)
        manager = STTEngineManager(config)

        result = manager._attempt_cpu_fallback()

        assert result is False
        assert manager._current_device == "cpu"

    def test_attempt_cpu_fallback_already_tried(self):
        config = EngineConfig(model_id="test-model", device="cuda", ttl=-1)
        manager = STTEngineManager(config)
        manager._tried_cpu_fallback = True

        result = manager._attempt_cpu_fallback()

        assert result is False

    def test_try_fallback_with_remaining_models(self):
        config = EngineConfig(
            model_id="main-model",
            device="cuda",
            ttl=-1,
            fallback_models=["fallback-1", "fallback-2"],
        )
        manager = STTEngineManager(config)

        result = manager.try_fallback()

        assert result is True
        assert "main-model" in manager._failed_models

    def test_try_fallback_no_remaining_models_attempts_cpu(self):
        config = EngineConfig(model_id="main-model", device="cuda", ttl=-1, fallback_models=[])
        manager = STTEngineManager(config)

        with patch("sidecar.stt.engine_manager._clear_cuda_cache"):
            result = manager.try_fallback()

        assert result is True
        assert manager._current_device == "cpu"

    def test_try_fallback_exhausted(self):
        config = EngineConfig(model_id="main-model", device="cpu", ttl=-1, fallback_models=[])
        manager = STTEngineManager(config)
        manager._tried_cpu_fallback = True

        result = manager.try_fallback()

        assert result is False

    def test_reset_device_preference(self):
        config = EngineConfig(model_id="test-model", device="cuda", ttl=-1)
        manager = STTEngineManager(config)
        manager._current_device = "cpu"
        manager._tried_cpu_fallback = True
        manager._failed_models.add("some-model")

        with patch("sidecar.stt.engine_manager._clear_cuda_cache"):
            manager.reset_device_preference()

        assert manager._current_device == "cuda"
        assert manager._tried_cpu_fallback is False
        assert len(manager._failed_models) == 0

    def test_force_unload(self):
        config = EngineConfig(model_id="test-model", device="cpu", ttl=-1)
        manager = STTEngineManager(config)

        with patch.object(manager, "_create_engine") as mock_create:
            mock_create.return_value = MagicMock()
            wrapper = manager.get_engine()
            with wrapper:
                pass

        manager.force_unload()

        assert manager.engines["test-model"].engine is None

    def test_preload(self):
        config = EngineConfig(model_id="test-model", device="cpu", ttl=-1)
        manager = STTEngineManager(config)

        with patch.object(manager, "_create_engine") as mock_create:
            mock_create.return_value = MagicMock()
            manager.preload()

            assert "test-model" in manager.engines
            assert manager.engines["test-model"].engine is not None

    def test_get_engine_with_retry_success(self):
        config = EngineConfig(model_id="test-model", device="cpu", ttl=-1)
        manager = STTEngineManager(config)

        with patch.object(manager, "_create_engine") as mock_create:
            mock_create.return_value = MagicMock()
            wrapper = manager.get_engine_with_retry()

            assert wrapper is not None

    def test_get_engine_with_retry_oom_fallback(self):
        config = EngineConfig(
            model_id="main-model",
            device="cuda",
            ttl=-1,
            fallback_models=["fallback-model"],
        )
        manager = STTEngineManager(config)

        call_count = [0]

        def create_side_effect(model_id):
            call_count[0] += 1
            if model_id == "main-model":
                raise RuntimeError("CUDA out of memory")
            return MagicMock()

        with patch.object(manager, "_create_engine", side_effect=create_side_effect):
            with patch("sidecar.stt.engine_manager._clear_cuda_cache"):
                wrapper = manager.get_engine_with_retry()

                assert wrapper is not None
                assert "main-model" in manager._failed_models

    def test_get_engine_with_retry_all_fail(self):
        config = EngineConfig(model_id="main-model", device="cpu", ttl=-1, fallback_models=[])
        manager = STTEngineManager(config)
        manager._tried_cpu_fallback = True

        with patch.object(manager, "_create_engine") as mock_create:
            mock_create.side_effect = RuntimeError("CUDA out of memory")
            with patch("sidecar.stt.engine_manager._clear_cuda_cache"):
                with pytest.raises(RuntimeError, match="All STT models failed"):
                    manager.get_engine_with_retry()

    def test_get_engine_with_retry_non_oom_error_propagates(self):
        config = EngineConfig(model_id="test-model", device="cpu", ttl=-1)
        manager = STTEngineManager(config)

        with patch.object(manager, "_create_engine") as mock_create:
            mock_create.side_effect = ValueError("Some other error")

            with pytest.raises(ValueError, match="Some other error"):
                manager.get_engine_with_retry()


class TestMaxOomRetries:
    def test_max_oom_retries_value(self):
        assert MAX_OOM_RETRIES == 3
