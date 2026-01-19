from __future__ import annotations

TARGET_SAMPLE_RATE = 16000


def samples_to_ms(sample_count: int, sample_rate: int = TARGET_SAMPLE_RATE) -> int:
    return int(sample_count / sample_rate * 1000)
