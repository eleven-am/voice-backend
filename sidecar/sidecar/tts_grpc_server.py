from __future__ import annotations

import io
import logging
import queue
import subprocess
import threading
import time
from collections.abc import Iterator
from concurrent.futures import ThreadPoolExecutor

import numpy as np
import soundfile as sf

from sidecar import tts_pb2, tts_pb2_grpc
from sidecar.tts import SAMPLE_RATE, Synthesizer, SynthesisError, float32_to_pcm16
from sidecar.tts_models import KOKORO_VOICES, KokoroModelManager, SynthesisConfig

logger = logging.getLogger(__name__)

OPUS_FLUSH_TIMEOUT_S = 10.0
OPUS_QUEUE_TIMEOUT_S = 1.0


def _cleanup_ffmpeg(
    ffmpeg_proc: subprocess.Popen | None,
    ffmpeg_reader: threading.Thread | None,
    reader_stop: threading.Event | None,
    stream_opus: bool,
) -> None:
    if ffmpeg_proc:
        if stream_opus and reader_stop:
            reader_stop.set()
        ffmpeg_proc.kill()
        ffmpeg_proc.wait(timeout=2)
        if ffmpeg_reader:
            ffmpeg_reader.join(timeout=2)


class TextToSpeechServiceServicer(tts_pb2_grpc.TextToSpeechServiceServicer):
    def __init__(
        self,
        model_manager: KokoroModelManager,
        config: SynthesisConfig,
        executor: ThreadPoolExecutor,
    ) -> None:
        self.model_manager = model_manager
        self.config = config
        self.executor = executor

    def _parse_tts_config(
        self, request_iterator
    ) -> tuple[tts_pb2.TtsSessionConfig | None, list[str], str, str, list[tts_pb2.TtsServerMessage]]:
        session_config: tts_pb2.TtsSessionConfig | None = None
        text_chunks: list[str] = []
        voice_id = "af_heart"
        response_format = "pcm"
        errors: list[tts_pb2.TtsServerMessage] = []
        ready_msg: tts_pb2.TtsServerMessage | None = None

        for client_msg in request_iterator:
            msg_type = client_msg.WhichOneof("msg")

            if msg_type == "config":
                if session_config is not None:
                    errors.append(tts_pb2.TtsServerMessage(
                        error=tts_pb2.TtsError(message="Session already configured", code=1)
                    ))
                    continue

                session_config = client_msg.config
                voice_id = session_config.voice_id or "af_heart"
                if session_config.response_format:
                    response_format = session_config.response_format.lower()

                logger.info(f"TTS session configured: voice={voice_id}")
                ready_msg = tts_pb2.TtsServerMessage(
                    ready=tts_pb2.TtsSessionReady(
                        voice_id=voice_id,
                        sample_rate=SAMPLE_RATE,
                    )
                )
                continue

            if msg_type == "text":
                if session_config is None:
                    errors.append(tts_pb2.TtsServerMessage(
                        error=tts_pb2.TtsError(message="Session not configured", code=2)
                    ))
                    continue
                text_chunks.append(client_msg.text.text)
                continue

            if msg_type == "end":
                break

        return session_config, text_chunks, voice_id, response_format, errors

    def _setup_opus_encoder(self) -> tuple[subprocess.Popen, queue.Queue, threading.Thread, threading.Event]:
        cmd = [
            "ffmpeg",
            "-f",
            "s16le",
            "-ar",
            str(SAMPLE_RATE),
            "-ac",
            "1",
            "-i",
            "pipe:0",
            "-c:a",
            "libopus",
            "-f",
            "ogg",
            "-loglevel",
            "error",
            "pipe:1",
        ]
        ffmpeg_proc = subprocess.Popen(
            cmd,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            bufsize=0,
        )
        ffmpeg_queue: queue.Queue[bytes | None] = queue.Queue(maxsize=32)
        reader_stop = threading.Event()

        def _reader():
            assert ffmpeg_proc is not None
            assert ffmpeg_proc.stdout is not None
            try:
                while not reader_stop.is_set():
                    data = ffmpeg_proc.stdout.read(4096)
                    if not data:
                        break
                    try:
                        ffmpeg_queue.put(data, timeout=1.0)
                    except queue.Full:
                        if reader_stop.is_set():
                            break
            except Exception as e:
                logger.warning(f"FFmpeg reader error: {e}")
            finally:
                try:
                    ffmpeg_queue.put(None, timeout=1.0)
                except queue.Full:
                    pass

        ffmpeg_reader = threading.Thread(target=_reader, daemon=False)
        ffmpeg_reader.start()

        return ffmpeg_proc, ffmpeg_queue, ffmpeg_reader, reader_stop

    def _create_audio_chunk(
        self, data: bytes, format: str, audio_samples: int, transcript: str | None = None
    ) -> tts_pb2.TtsServerMessage:
        return tts_pb2.TtsServerMessage(
            audio=tts_pb2.AudioChunk(
                data=data,
                format=format,
                sample_rate=SAMPLE_RATE,
                timestamp_ms=int(audio_samples * 1000 / SAMPLE_RATE),
                transcript=transcript or "",
            )
        )

    def _drain_opus_queue(
        self, ffmpeg_queue: queue.Queue, audio_samples: int
    ) -> Iterator[tts_pb2.TtsServerMessage]:
        while not ffmpeg_queue.empty():
            chunk = ffmpeg_queue.get_nowait()
            if chunk is None:
                break
            yield self._create_audio_chunk(chunk, "opus", audio_samples)

    def _flush_opus_encoder(
        self,
        ffmpeg_proc: subprocess.Popen,
        ffmpeg_queue: queue.Queue,
        ffmpeg_reader: threading.Thread,
        reader_stop: threading.Event,
        audio_samples: int,
        transcript: str,
    ) -> Iterator[tts_pb2.TtsServerMessage]:
        if ffmpeg_proc.stdin:
            ffmpeg_proc.stdin.close()

        flush_timeout = OPUS_FLUSH_TIMEOUT_S
        flush_start = time.perf_counter()

        while True:
            remaining = flush_timeout - (time.perf_counter() - flush_start)
            if remaining <= 0:
                logger.warning("FFmpeg flush timed out")
                reader_stop.set()
                break
            try:
                chunk = ffmpeg_queue.get(timeout=min(OPUS_QUEUE_TIMEOUT_S, remaining))
            except queue.Empty:
                continue
            if chunk is None:
                break
            yield self._create_audio_chunk(chunk, "opus", audio_samples, transcript)

        reader_stop.set()
        if ffmpeg_reader:
            ffmpeg_reader.join(timeout=2)
            if ffmpeg_reader.is_alive():
                logger.warning("FFmpeg reader thread did not terminate")
        try:
            ffmpeg_proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            logger.warning("FFmpeg process did not terminate, killing")
            ffmpeg_proc.kill()
            ffmpeg_proc.wait(timeout=1)

        if ffmpeg_proc.returncode != 0:
            stderr = ffmpeg_proc.stderr.read().decode().strip() if ffmpeg_proc.stderr else ""
            yield tts_pb2.TtsServerMessage(
                error=tts_pb2.TtsError(message=f"ffmpeg encode error: {stderr}", code=6)
            )

    def _create_done_message(
        self, audio_samples: int, start_time: float, full_text: str
    ) -> tts_pb2.TtsServerMessage:
        processing_ms = int((time.perf_counter() - start_time) * 1000)
        audio_ms = int(audio_samples * 1000 / SAMPLE_RATE)

        logger.info(
            f"TTS done: {audio_ms}ms audio, {processing_ms}ms processing"
        )

        return tts_pb2.TtsServerMessage(
            done=tts_pb2.TtsDone(
                audio_duration_ms=audio_ms,
                processing_duration_ms=processing_ms,
                text_length=len(full_text),
                usage=tts_pb2.TtsUsage(
                    audio_ms=audio_ms,
                    processing_ms=processing_ms,
                    text_chars=len(full_text),
                ),
                transcript=full_text,
            )
        )

    def Synthesize(self, request_iterator, context):
        session_config, text_chunks, voice_id, response_format, parse_errors = self._parse_tts_config(request_iterator)

        if session_config is not None:
            yield tts_pb2.TtsServerMessage(
                ready=tts_pb2.TtsSessionReady(
                    voice_id=voice_id,
                    sample_rate=SAMPLE_RATE,
                )
            )

        for err in parse_errors:
            yield err

        if session_config is None:
            yield tts_pb2.TtsServerMessage(
                error=tts_pb2.TtsError(message="No session config received", code=3)
            )
            return

        full_text = " ".join(text_chunks).strip()
        if not full_text:
            yield tts_pb2.TtsServerMessage(
                error=tts_pb2.TtsError(message="No text provided", code=4)
            )
            return

        speed = session_config.speed if session_config.speed > 0 else self.config.speed
        synth_config = SynthesisConfig(speed=speed)

        synthesizer = Synthesizer(self.model_manager, synth_config)
        stop_event = threading.Event()

        start_time = time.perf_counter()
        audio_samples = 0
        chunk_count = 0
        buffer = bytearray()
        stream_pcm = response_format == "pcm"
        stream_opus = response_format == "opus"

        ffmpeg_proc = None
        ffmpeg_queue: queue.Queue[bytes | None] | None = None
        ffmpeg_reader: threading.Thread | None = None
        reader_stop: threading.Event | None = None

        if stream_opus:
            ffmpeg_proc, ffmpeg_queue, ffmpeg_reader, reader_stop = self._setup_opus_encoder()

        try:
            for audio_chunk in synthesizer.synthesize(
                text=full_text,
                voice_id=voice_id,
                stop_event=stop_event,
            ):
                pcm16 = float32_to_pcm16(audio_chunk)
                audio_samples += len(audio_chunk)
                chunk_count += 1

                if stream_pcm:
                    yield self._create_audio_chunk(pcm16, "pcm", audio_samples, full_text)
                elif stream_opus:
                    assert ffmpeg_proc is not None and ffmpeg_proc.stdin is not None
                    ffmpeg_proc.stdin.write(pcm16)
                    yield from self._drain_opus_queue(ffmpeg_queue, audio_samples)
                else:
                    buffer.extend(pcm16)

        except SynthesisError as e:
            logger.error(f"Synthesis error: {e}")
            _cleanup_ffmpeg(ffmpeg_proc, ffmpeg_reader, reader_stop, stream_opus)
            yield tts_pb2.TtsServerMessage(
                error=tts_pb2.TtsError(message=str(e), code=e.code)
            )
            return

        except Exception as e:
            logger.exception("Unexpected TTS failure")
            _cleanup_ffmpeg(ffmpeg_proc, ffmpeg_reader, reader_stop, stream_opus)
            yield tts_pb2.TtsServerMessage(
                error=tts_pb2.TtsError(message=f"Unexpected error: {e}", code=99)
            )
            return

        if stream_opus:
            assert ffmpeg_proc is not None and ffmpeg_queue is not None and reader_stop is not None
            for msg in self._flush_opus_encoder(ffmpeg_proc, ffmpeg_queue, ffmpeg_reader, reader_stop, audio_samples, full_text):
                if msg.HasField("error"):
                    yield msg
                    return
                if msg.HasField("audio"):
                    msg.audio.transcript = full_text
                yield msg
        elif not stream_pcm:
            try:
                encoded, enc_format = encode_audio(bytes(buffer), SAMPLE_RATE, response_format)
                yield self._create_audio_chunk(encoded, enc_format, audio_samples, full_text)
            except SynthesisError as e:
                logger.error(f"Encoding error: {e}")
                yield tts_pb2.TtsServerMessage(
                    error=tts_pb2.TtsError(message=str(e), code=e.code)
                )
                return

        yield self._create_done_message(audio_samples, start_time, full_text)

    def ListVoices(self, request, context):
        voices = []
        for voice_id, lang, gender in KOKORO_VOICES:
            voices.append(tts_pb2.Voice(
                id=voice_id,
                name=voice_id.split("_")[1].title(),
                language=lang,
                gender=gender,
            ))
        return tts_pb2.ListVoicesResponse(voices=voices)

    def ListModels(self, request, context):
        models = [
            tts_pb2.TTSModel(
                id=self.model_manager.config.model_id,
                name="Kokoro",
                description="Kokoro TTS model",
            ),
        ]
        return tts_pb2.ListModelsResponse(models=models)


def encode_audio(pcm16: bytes, sample_rate: int, fmt: str) -> tuple[bytes, str]:
    fmt = (fmt or "pcm").lower()
    if fmt in ("pcm", "s16le"):
        return pcm16, "pcm"
    if fmt == "wav":
        buf = io.BytesIO()
        data = np.frombuffer(pcm16, dtype=np.int16)
        sf.write(buf, data, samplerate=sample_rate, format="WAV", subtype="PCM_16")
        return buf.getvalue(), "wav"
    if fmt == "flac":
        buf = io.BytesIO()
        data = np.frombuffer(pcm16, dtype=np.int16)
        sf.write(buf, data, samplerate=sample_rate, format="FLAC", subtype="PCM_16")
        return buf.getvalue(), "flac"

    codec_map = {
        "mp3": "libmp3lame",
        "aac": "aac",
        "opus": "libopus",
    }
    codec = codec_map.get(fmt)
    if codec is None:
        raise SynthesisError(f"unsupported response_format: {fmt}", code=5)

    cmd = [
        "ffmpeg",
        "-f", "s16le",
        "-ar", str(sample_rate),
        "-ac", "1",
        "-i", "pipe:0",
        "-acodec", codec,
        "-f", fmt,
        "pipe:1",
        "-loglevel", "error",
    ]
    proc = subprocess.Popen(
        cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE
    )
    stdout, stderr = proc.communicate(input=pcm16)
    if proc.returncode != 0:
        raise SynthesisError(f"ffmpeg encode error: {stderr.decode().strip()}", code=6)

    return stdout, fmt
