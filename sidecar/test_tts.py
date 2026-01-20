#!/usr/bin/env python3
import grpc
import numpy as np
import sounddevice as sd

from sidecar.tts import pb2 as tts_pb2, pb2_grpc as tts_pb2_grpc

SIDECAR_ADDR = "localhost:50051"
SAMPLE_RATE = 24000


def main():
    text = input("Enter text to speak (or press Enter for default): ").strip()
    if not text:
        text = "Hello! This is a test of the text to speech system. How does it sound?"

    print(f"Text: {text}", flush=True)
    print(f"Connecting to sidecar at {SIDECAR_ADDR}...", flush=True)

    channel = grpc.insecure_channel(SIDECAR_ADDR)
    stub = tts_pb2_grpc.TextToSpeechServiceStub(channel)

    def message_generator():
        yield tts_pb2.TtsClientMessage(
            config=tts_pb2.TtsSessionConfig(
                voice_id="af_heart",
                sample_rate=SAMPLE_RATE,
                speed=1.0,
                response_format="pcm",
            )
        )
        yield tts_pb2.TtsClientMessage(
            text=tts_pb2.TextChunk(text=text)
        )
        yield tts_pb2.TtsClientMessage(
            end=tts_pb2.EndOfText()
        )

    audio_chunks = []
    total_duration_ms = 0

    try:
        for response in stub.Synthesize(message_generator()):
            msg_type = response.WhichOneof("msg")

            if msg_type == "ready":
                print(f"[ready] Voice: {response.ready.voice_id}, Sample rate: {response.ready.sample_rate}", flush=True)
            elif msg_type == "audio":
                chunk = response.audio
                audio_chunks.append(chunk.data)
                total_duration_ms = chunk.timestamp_ms
                print(f"[audio] Received {len(chunk.data)} bytes, timestamp: {chunk.timestamp_ms}ms", flush=True)
            elif msg_type == "done":
                d = response.done
                print(f"[done] Audio: {d.audio_duration_ms}ms, Processing: {d.processing_duration_ms}ms, Text: {d.text_length} chars", flush=True)
            elif msg_type == "error":
                print(f"[error] {response.error.message}", flush=True)
                return

    except grpc.RpcError as e:
        print(f"[grpc error] {e.code()}: {e.details()}", flush=True)
        return

    if audio_chunks:
        print(f"\nPlaying audio ({total_duration_ms}ms)...", flush=True)
        pcm_data = b"".join(audio_chunks)
        audio = np.frombuffer(pcm_data, dtype=np.int16).astype(np.float32) / 32768.0
        sd.play(audio, SAMPLE_RATE)
        sd.wait()
        print("Done!", flush=True)


if __name__ == "__main__":
    main()
