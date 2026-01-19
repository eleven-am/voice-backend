ARG BASE_IMAGE=nvidia/cuda:12.8.1-cudnn-runtime-ubuntu24.04
FROM ${BASE_IMAGE}

LABEL org.opencontainers.image.source="https://github.com/eleven-am/hu"
LABEL org.opencontainers.image.licenses="MIT"

RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    ffmpeg \
    libopus0 \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /bin/bash --uid 1000 ubuntu || true

ENV HOME=/home/ubuntu \
    PATH=/home/ubuntu/.local/bin:$PATH

WORKDIR $HOME/sidecar

COPY --from=ghcr.io/astral-sh/uv:0.7.20 /uv /bin/uv

RUN --mount=type=cache,target=/root/.cache/uv \
    --mount=type=bind,source=uv.lock,target=uv.lock \
    --mount=type=bind,source=pyproject.toml,target=pyproject.toml \
    uv sync --frozen --compile-bytecode --no-install-project

COPY --chown=ubuntu . .

RUN --mount=type=cache,target=/root/.cache/uv \
    uv sync --frozen --compile-bytecode && \
    mkdir -p $HOME/.cache/huggingface/hub && \
    chown -R ubuntu:ubuntu $HOME

USER ubuntu

ENV PATH="$HOME/sidecar/.venv/bin:$PATH" \
    GRPC_PORT=50051 \
    STT_DEVICE=cuda \
    PARAKEET_MODEL_ID="nemo-parakeet-tdt-0.6b-v3" \
    TTS_MODEL_ID="hexgrad/Kokoro-82M-v1.0-ONNX" \
    HF_HUB_ENABLE_HF_TRANSFER=0 \
    DO_NOT_TRACK=1 \
    HF_HUB_DISABLE_TELEMETRY=1

EXPOSE 50051

CMD ["python", "-m", "sidecar.main"]
