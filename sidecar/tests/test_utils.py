from __future__ import annotations

import os
import threading
import time
from http.client import HTTPConnection
from unittest.mock import MagicMock, patch

import pytest

from sidecar.utils import (
    MAX_CHUNK_CHARS,
    chunk_text,
    get_env,
    is_oom_error,
    start_health_server,
    token_auth_interceptor,
)


class TestIsOomError:
    def test_cuda_out_of_memory(self):
        error = RuntimeError("CUDA out of memory")
        assert is_oom_error(error) is True

    def test_cuda_error_out_of_memory(self):
        error = RuntimeError("CUDA error: out of memory")
        assert is_oom_error(error) is True

    def test_generic_out_of_memory(self):
        error = RuntimeError("out of memory")
        assert is_oom_error(error) is True

    def test_oom_abbreviation(self):
        error = RuntimeError("OOM detected")
        assert is_oom_error(error) is True

    def test_cublas_alloc(self):
        error = RuntimeError("cuBLAS alloc failed")
        assert is_oom_error(error) is True

    def test_alloc_failed(self):
        error = RuntimeError("alloc_failed during tensor allocation")
        assert is_oom_error(error) is True

    def test_failed_to_allocate(self):
        error = RuntimeError("Failed to allocate 1GB of memory")
        assert is_oom_error(error) is True

    def test_memory_allocation(self):
        error = RuntimeError("memory allocation error")
        assert is_oom_error(error) is True

    def test_case_insensitive(self):
        error = RuntimeError("CUDA OUT OF MEMORY")
        assert is_oom_error(error) is True

    def test_unrelated_cuda_error(self):
        error = RuntimeError("CUDA initialization failed")
        assert is_oom_error(error) is False

    def test_unrelated_error(self):
        error = ValueError("Invalid input")
        assert is_oom_error(error) is False

    def test_empty_error_message(self):
        error = RuntimeError("")
        assert is_oom_error(error) is False

    def test_onnxruntime_not_matched(self):
        error = RuntimeError("onnxruntime session error")
        assert is_oom_error(error) is False


class TestChunkText:
    def test_short_text_returns_single_chunk(self):
        text = "Hello world"
        result = chunk_text(text)
        assert result == ["Hello world"]

    def test_exactly_max_chars_returns_single_chunk(self):
        text = "a" * MAX_CHUNK_CHARS
        result = chunk_text(text)
        assert result == [text]

    def test_splits_on_sentence_boundary(self):
        text = "First sentence. Second sentence. Third sentence."
        result = chunk_text(text, max_chars=30)
        assert len(result) >= 2
        assert "First sentence" in result[0]

    def test_long_sentence_splits_on_words(self):
        text = " ".join(["word"] * 100)
        result = chunk_text(text, max_chars=50)
        assert len(result) > 1
        for chunk in result:
            assert len(chunk) <= 50 or len(chunk.split()) == 1

    def test_empty_text_returns_list_with_empty(self):
        result = chunk_text("")
        assert result == [""]

    def test_whitespace_only_text(self):
        result = chunk_text("   ")
        assert result == ["   "]

    def test_multiple_sentence_endings(self):
        text = "Hello! How are you? I'm fine."
        result = chunk_text(text, max_chars=20)
        assert len(result) >= 2

    def test_preserves_content(self):
        text = "First. Second. Third."
        result = chunk_text(text, max_chars=100)
        combined = " ".join(result)
        assert "First" in combined
        assert "Second" in combined
        assert "Third" in combined


class TestGetEnv:
    def test_returns_default_when_not_set(self):
        result = get_env("NONEXISTENT_VAR_12345", "default")
        assert result == "default"

    def test_returns_string_value(self):
        with patch.dict(os.environ, {"TEST_VAR": "hello"}):
            result = get_env("TEST_VAR", "default")
            assert result == "hello"

    def test_returns_int_value(self):
        with patch.dict(os.environ, {"TEST_INT": "42"}):
            result = get_env("TEST_INT", 0)
            assert result == 42
            assert isinstance(result, int)

    def test_returns_float_value(self):
        with patch.dict(os.environ, {"TEST_FLOAT": "3.14"}):
            result = get_env("TEST_FLOAT", 0.0)
            assert result == 3.14
            assert isinstance(result, float)

    def test_returns_bool_true(self):
        for val in ["true", "True", "TRUE", "1", "yes", "YES"]:
            with patch.dict(os.environ, {"TEST_BOOL": val}):
                result = get_env("TEST_BOOL", False)
                assert result is True

    def test_returns_bool_false(self):
        for val in ["false", "False", "0", "no", "anything"]:
            with patch.dict(os.environ, {"TEST_BOOL": val}):
                result = get_env("TEST_BOOL", True)
                assert result is False

    def test_default_type_preserved_int(self):
        result = get_env("NONEXISTENT_VAR", 100)
        assert result == 100
        assert isinstance(result, int)

    def test_default_type_preserved_float(self):
        result = get_env("NONEXISTENT_VAR", 1.5)
        assert result == 1.5
        assert isinstance(result, float)


class TestTokenAuthInterceptor:
    def test_valid_token_allows_request(self):
        interceptor = token_auth_interceptor("secret-token")
        mock_continuation = MagicMock(return_value="handler_result")
        mock_call_details = MagicMock()
        mock_call_details.invocation_metadata = [("authorization", "Bearer secret-token")]

        result = interceptor.intercept_service(mock_continuation, mock_call_details)

        assert result == "handler_result"
        mock_continuation.assert_called_once_with(mock_call_details)

    def test_missing_token_denies_request(self):
        import grpc

        interceptor = token_auth_interceptor("secret-token")
        mock_continuation = MagicMock()
        mock_call_details = MagicMock()
        mock_call_details.invocation_metadata = []

        result = interceptor.intercept_service(mock_continuation, mock_call_details)

        mock_continuation.assert_not_called()
        assert result is not None

    def test_invalid_token_denies_request(self):
        import grpc

        interceptor = token_auth_interceptor("secret-token")
        mock_continuation = MagicMock()
        mock_call_details = MagicMock()
        mock_call_details.invocation_metadata = [("authorization", "Bearer wrong-token")]

        result = interceptor.intercept_service(mock_continuation, mock_call_details)

        mock_continuation.assert_not_called()
        assert result is not None

    def test_malformed_auth_header_denies(self):
        interceptor = token_auth_interceptor("secret-token")
        mock_continuation = MagicMock()
        mock_call_details = MagicMock()
        mock_call_details.invocation_metadata = [("authorization", "Basic abc123")]

        result = interceptor.intercept_service(mock_continuation, mock_call_details)

        mock_continuation.assert_not_called()

    def test_capitalized_authorization_header(self):
        interceptor = token_auth_interceptor("secret-token")
        mock_continuation = MagicMock(return_value="handler_result")
        mock_call_details = MagicMock()
        mock_call_details.invocation_metadata = [("Authorization", "Bearer secret-token")]

        result = interceptor.intercept_service(mock_continuation, mock_call_details)

        assert result == "handler_result"


class TestStartHealthServer:
    def test_health_endpoint(self):
        server = start_health_server(port=18081)
        time.sleep(0.1)

        try:
            conn = HTTPConnection("localhost", 18081)
            conn.request("GET", "/health")
            response = conn.getresponse()

            assert response.status == 200
            body = response.read()
            assert b"ok" in body
        finally:
            server.shutdown()

    def test_ready_endpoint(self):
        server = start_health_server(port=18082)
        time.sleep(0.1)

        try:
            conn = HTTPConnection("localhost", 18082)
            conn.request("GET", "/ready")
            response = conn.getresponse()

            assert response.status == 200
        finally:
            server.shutdown()

    def test_404_for_unknown_path(self):
        server = start_health_server(port=18083)
        time.sleep(0.1)

        try:
            conn = HTTPConnection("localhost", 18083)
            conn.request("GET", "/unknown")
            response = conn.getresponse()

            assert response.status == 404
        finally:
            server.shutdown()

    def test_metrics_endpoint_with_fn(self):
        def metrics_fn():
            return "test_metric 1\n"

        server = start_health_server(port=18084, metrics_fn=metrics_fn)
        time.sleep(0.1)

        try:
            conn = HTTPConnection("localhost", 18084)
            conn.request("GET", "/metrics")
            response = conn.getresponse()

            assert response.status == 200
            body = response.read()
            assert b"test_metric" in body
        finally:
            server.shutdown()

    def test_metrics_endpoint_error_handling(self):
        def failing_metrics_fn():
            raise ValueError("metrics error")

        server = start_health_server(port=18085, metrics_fn=failing_metrics_fn)
        time.sleep(0.1)

        try:
            conn = HTTPConnection("localhost", 18085)
            conn.request("GET", "/metrics")
            response = conn.getresponse()

            assert response.status == 200
            body = response.read()
            assert b"error" in body
        finally:
            server.shutdown()
