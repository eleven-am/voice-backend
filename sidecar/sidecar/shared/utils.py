from __future__ import annotations

import os
import re
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from typing import TypeVar, overload

T = TypeVar("T", int, float, bool, str)

MAX_CHUNK_CHARS = 250


def chunk_text(text: str, max_chars: int = MAX_CHUNK_CHARS) -> list[str]:
    if len(text) <= max_chars:
        return [text]

    sentence_pattern = re.compile(r'(?<=[.!?])\s+')
    sentences = sentence_pattern.split(text)

    chunks: list[str] = []
    current = ""

    for sentence in sentences:
        if not sentence.strip():
            continue

        if len(current) + len(sentence) + 1 <= max_chars:
            current = f"{current} {sentence}".strip() if current else sentence
        else:
            if current:
                chunks.append(current)
            if len(sentence) > max_chars:
                words = sentence.split()
                current = ""
                for word in words:
                    if len(current) + len(word) + 1 <= max_chars:
                        current = f"{current} {word}".strip() if current else word
                    else:
                        if current:
                            chunks.append(current)
                        current = word
            else:
                current = sentence

    if current:
        chunks.append(current)

    return chunks if chunks else [text]


@overload
def get_env(key: str, default: int) -> int: ...
@overload
def get_env(key: str, default: float) -> float: ...
@overload
def get_env(key: str, default: bool) -> bool: ...
@overload
def get_env(key: str, default: str) -> str: ...


def get_env(key: str, default: T) -> T:
    value = os.environ.get(key)
    if value is None:
        return default

    if isinstance(default, bool):
        return value.lower() in ("true", "1", "yes")  # type: ignore[return-value]
    if isinstance(default, int):
        return int(value)  # type: ignore[return-value]
    if isinstance(default, float):
        return float(value)  # type: ignore[return-value]
    return value  # type: ignore[return-value]


def is_oom_error(error: Exception) -> bool:
    err_str = str(error).lower()
    return any(keyword in err_str for keyword in [
        "out of memory",
        "oom",
        "cuda out of memory",
        "cuda error: out of memory",
        "cublas alloc",
        "alloc_failed",
        "failed to allocate",
        "memory allocation",
    ])


def token_auth_interceptor(expected_token: str) -> "grpc.ServerInterceptor":
    import grpc

    class AuthInterceptor(grpc.ServerInterceptor):
        def intercept_service(self, continuation, handler_call_details):
            meta = dict(handler_call_details.invocation_metadata or [])
            auth_header = meta.get("authorization") or meta.get("Authorization")
            if not auth_header or not auth_header.startswith("Bearer "):
                def deny(request, context):  # noqa: ANN001
                    context.abort(grpc.StatusCode.UNAUTHENTICATED, "missing bearer token")
                return grpc.unary_unary_rpc_method_handler(deny)

            token = auth_header.split(" ", 1)[1].strip()
            if token != expected_token:
                def deny(request, context):  # noqa: ANN001
                    context.abort(grpc.StatusCode.UNAUTHENTICATED, "invalid token")
                return grpc.unary_unary_rpc_method_handler(deny)

            return continuation(handler_call_details)

    return AuthInterceptor()


def start_health_server(port: int = 8081, metrics_fn=None) -> HTTPServer:
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):  # noqa: N802
            if self.path in ("/health", "/ready"):
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(b'{"status":"ok"}')
                return

            if self.path == "/metrics" and metrics_fn:
                try:
                    payload = metrics_fn()
                except Exception:  # noqa: BLE001
                    payload = "hu_sidecar_metrics_error 1\n"
                self.send_response(200)
                self.send_header("Content-Type", "text/plain")
                self.end_headers()
                self.wfile.write(payload.encode())
                return

            self.send_response(404)
            self.end_headers()

        def log_message(self, format, *args):  # noqa: A003,N802
            return

    server = HTTPServer(("", port), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server
