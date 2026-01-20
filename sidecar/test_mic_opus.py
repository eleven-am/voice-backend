#!/usr/bin/env python3
import sys
import grpc
import numpy as np
import opuslib
import sounddevice as sd

from sidecar.stt import pb2 as stt_pb2, pb2_grpc as stt_pb2_grpc

SAMPLE_RATE = 48000
CHANNELS = 1
FRAME_MS = 20
FRAME_SIZE = SAMPLE_RATE * FRAME_MS // 1000
SIDECAR_ADDR = "localhost:50051"


def main():
    encoder = opuslib.Encoder(SAMPLE_RATE, CHANNELS, opuslib.APPLICATION_VOIP)

    channel = grpc.insecure_channel(SIDECAR_ADDR)
    stub = stt_pb2_grpc.TranscriptionServiceStub(channel)

    audio_queue = []
    running = True

    def audio_callback(indata, frames, time_info, status):
        if status:
            print(f"[status] {status}", flush=True)
        audio_queue.append(indata.copy())

    def message_generator():
        yield stt_pb2.ClientMessage(
            config=stt_pb2.SessionConfig(
                language="en",
                sample_rate=SAMPLE_RATE,
                partials=True,
            )
        )

        pcm_buffer = np.array([], dtype=np.int16)

        print("Listening... (Ctrl+C to stop)", flush=True)
        while running:
            if not audio_queue:
                continue

            chunk = audio_queue.pop(0)
            samples = (chunk[:, 0] * 32767).astype(np.int16)
            pcm_buffer = np.concatenate([pcm_buffer, samples])

            while len(pcm_buffer) >= FRAME_SIZE:
                frame_samples = pcm_buffer[:FRAME_SIZE]
                pcm_buffer = pcm_buffer[FRAME_SIZE:]

                opus_data = encoder.encode(frame_samples.tobytes(), FRAME_SIZE)

                yield stt_pb2.ClientMessage(
                    opus_frame=stt_pb2.OpusFrame(
                        data=opus_data,
                        sample_rate=SAMPLE_RATE,
                        channels=CHANNELS,
                    )
                )

    print(f"Connecting to sidecar at {SIDECAR_ADDR}...", flush=True)

    with sd.InputStream(
        samplerate=SAMPLE_RATE,
        channels=CHANNELS,
        dtype=np.float32,
        blocksize=FRAME_SIZE,
        callback=audio_callback,
    ):
        try:
            for response in stub.Transcribe(message_generator()):
                msg_type = response.WhichOneof("msg")

                if msg_type == "ready":
                    print("[ready] Session started", flush=True)
                elif msg_type == "speech_started":
                    print("[speech] Started speaking", flush=True)
                elif msg_type == "speech_stopped":
                    print("[speech] Stopped speaking", flush=True)
                elif msg_type == "transcript":
                    t = response.transcript
                    prefix = "[partial]" if t.is_partial else "[final]"
                    print(f"{prefix} {t.text}", flush=True)
                elif msg_type == "error":
                    print(f"[error] {response.error.message}", flush=True)
        except KeyboardInterrupt:
            running = False
            print("\nStopping...", flush=True)
        except grpc.RpcError as e:
            print(f"[grpc error] {e.code()}: {e.details()}", flush=True)


if __name__ == "__main__":
    main()
