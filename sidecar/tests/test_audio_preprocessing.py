from __future__ import annotations

import io
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from sidecar.audio.preprocessing import (
    CHUNK_DURATION_MS,
    TARGET_SAMPLE_RATE,
    AudioChunk,
    _detect_format,
    chunk_audio,
    decode_audio,
    preprocess_audio,
)


class TestDetectFormat:
    def test_detect_mp3(self):
        assert _detect_format("audio.mp3") == "mp3"

    def test_detect_wav(self):
        assert _detect_format("audio.wav") == "wav"

    def test_detect_ogg(self):
        assert _detect_format("audio.ogg") == "ogg"

    def test_detect_flac(self):
        assert _detect_format("audio.flac") == "flac"

    def test_detect_aac(self):
        assert _detect_format("audio.aac") == "aac"

    def test_detect_opus(self):
        assert _detect_format("audio.opus") == "opus"

    def test_detect_webm(self):
        assert _detect_format("audio.webm") == "webm"

    def test_detect_m4a_returns_mp4(self):
        assert _detect_format("audio.m4a") == "mp4"

    def test_detect_unknown_format_returns_none(self):
        assert _detect_format("audio.xyz") is None

    def test_detect_no_extension_returns_none(self):
        assert _detect_format("audio") is None

    def test_detect_none_returns_none(self):
        assert _detect_format(None) is None

    def test_detect_empty_string_returns_none(self):
        assert _detect_format("") is None

    def test_detect_uppercase_extension(self):
        assert _detect_format("audio.MP3") == "mp3"

    def test_detect_mixed_case(self):
        assert _detect_format("audio.Mp3") == "mp3"


class TestDecodeAudio:
    def test_decode_raises_on_invalid_audio(self):
        with pytest.raises(ValueError, match="Failed to"):
            decode_audio(b"not valid audio data", "mp3")

    def test_decode_with_format_hint(self):
        with patch("sidecar.audio.preprocessing.AudioSegment.from_file") as mock_from_file:
            mock_audio = MagicMock()
            mock_audio.set_frame_rate.return_value = mock_audio
            mock_audio.set_channels.return_value = mock_audio
            mock_audio.get_array_of_samples.return_value = np.array([0, 100, -100], dtype=np.int16)
            mock_from_file.return_value = mock_audio

            result = decode_audio(b"fake audio", "mp3")

            mock_from_file.assert_called_once()
            call_kwargs = mock_from_file.call_args[1]
            assert call_kwargs.get("format") == "mp3"
            assert result is not None

    def test_decode_without_format_hint(self):
        with patch("sidecar.audio.preprocessing.AudioSegment.from_file") as mock_from_file:
            mock_audio = MagicMock()
            mock_audio.set_frame_rate.return_value = mock_audio
            mock_audio.set_channels.return_value = mock_audio
            mock_audio.get_array_of_samples.return_value = np.array([0, 100, -100], dtype=np.int16)
            mock_from_file.return_value = mock_audio

            result = decode_audio(b"fake audio", None)

            mock_from_file.assert_called_once()
            call_kwargs = mock_from_file.call_args[1]
            assert "format" not in call_kwargs or call_kwargs.get("format") is None


class TestChunkAudio:
    def test_short_audio_returns_single_chunk(self):
        audio = np.zeros(TARGET_SAMPLE_RATE * 60, dtype=np.float32)

        chunks = chunk_audio(audio)

        assert len(chunks) == 1
        assert chunks[0].offset_ms == 0
        assert chunks[0].duration_ms == 60000

    def test_exactly_chunk_size_returns_single_chunk(self):
        chunk_samples = int(CHUNK_DURATION_MS * TARGET_SAMPLE_RATE / 1000)
        audio = np.zeros(chunk_samples, dtype=np.float32)

        chunks = chunk_audio(audio)

        assert len(chunks) == 1

    def test_long_audio_returns_multiple_chunks(self):
        audio = np.zeros(TARGET_SAMPLE_RATE * 60 * 12, dtype=np.float32)

        chunks = chunk_audio(audio)

        assert len(chunks) == 3
        assert chunks[0].offset_ms == 0
        assert chunks[1].offset_ms == CHUNK_DURATION_MS
        assert chunks[2].offset_ms == 2 * CHUNK_DURATION_MS

    def test_chunk_data_is_correct_slice(self):
        audio = np.arange(TARGET_SAMPLE_RATE * 60 * 6, dtype=np.float32)

        chunks = chunk_audio(audio)

        assert len(chunks) == 2
        expected_first_len = int(CHUNK_DURATION_MS * TARGET_SAMPLE_RATE / 1000)
        assert len(chunks[0].data) == expected_first_len
        assert chunks[0].data[0] == 0
        assert chunks[1].data[0] == expected_first_len

    def test_chunk_sample_rate_preserved(self):
        audio = np.zeros(TARGET_SAMPLE_RATE * 60, dtype=np.float32)

        chunks = chunk_audio(audio, sample_rate=24000)

        assert chunks[0].sample_rate == 24000

    def test_custom_chunk_duration(self):
        audio = np.zeros(TARGET_SAMPLE_RATE * 60 * 2, dtype=np.float32)

        chunks = chunk_audio(audio, chunk_duration_ms=60000)

        assert len(chunks) == 2


class TestPreprocessAudio:
    def test_preprocess_with_filename(self):
        with patch("sidecar.audio.preprocessing.decode_audio") as mock_decode:
            with patch("sidecar.audio.preprocessing.chunk_audio") as mock_chunk:
                mock_decode.return_value = np.zeros(TARGET_SAMPLE_RATE * 60, dtype=np.float32)
                mock_chunk.return_value = [
                    AudioChunk(
                        data=np.zeros(1000),
                        sample_rate=TARGET_SAMPLE_RATE,
                        duration_ms=60000,
                        offset_ms=0,
                    )
                ]

                chunks = preprocess_audio(b"audio data", "test.mp3")

                mock_decode.assert_called_once_with(b"audio data", "mp3")
                assert len(chunks) == 1

    def test_preprocess_without_filename(self):
        with patch("sidecar.audio.preprocessing.decode_audio") as mock_decode:
            with patch("sidecar.audio.preprocessing.chunk_audio") as mock_chunk:
                mock_decode.return_value = np.zeros(TARGET_SAMPLE_RATE * 60, dtype=np.float32)
                mock_chunk.return_value = [
                    AudioChunk(
                        data=np.zeros(1000),
                        sample_rate=TARGET_SAMPLE_RATE,
                        duration_ms=60000,
                        offset_ms=0,
                    )
                ]

                chunks = preprocess_audio(b"audio data")

                mock_decode.assert_called_once_with(b"audio data", None)


class TestAudioChunk:
    def test_audio_chunk_creation(self):
        data = np.array([0.1, 0.2, 0.3], dtype=np.float32)
        chunk = AudioChunk(
            data=data,
            sample_rate=16000,
            duration_ms=1000,
            offset_ms=5000,
        )

        assert np.array_equal(chunk.data, data)
        assert chunk.sample_rate == 16000
        assert chunk.duration_ms == 1000
        assert chunk.offset_ms == 5000
