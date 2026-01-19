from __future__ import annotations

import io
import subprocess
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from sidecar.audio_preprocessing import (
    CHUNK_DURATION_MS,
    TARGET_SAMPLE_RATE,
    AudioChunk,
    _decode_with_ffmpeg,
    _decode_with_soundfile,
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


class TestDecodeWithSoundfile:
    def test_decode_mono_audio(self):
        mock_sf = MagicMock()
        mock_audio = np.array([0.1, 0.2, 0.3], dtype=np.float32)
        mock_sf.read.return_value = (mock_audio, TARGET_SAMPLE_RATE)

        with patch.dict("sys.modules", {"soundfile": mock_sf}):
            result = _decode_with_soundfile(b"fake audio data", None)

        assert result is not None
        assert len(result) == 3
        mock_sf.read.assert_called_once()

    def test_decode_stereo_audio_converts_to_mono(self):
        mock_sf = MagicMock()
        mock_audio = np.array([[0.1, 0.2], [0.3, 0.4], [0.5, 0.6]], dtype=np.float32)
        mock_sf.read.return_value = (mock_audio, TARGET_SAMPLE_RATE)

        with patch.dict("sys.modules", {"soundfile": mock_sf}):
            result = _decode_with_soundfile(b"fake audio data", None)

        assert result is not None
        assert len(result) == 3
        assert result.ndim == 1

    def test_decode_resamples_if_different_rate(self):
        mock_sf = MagicMock()
        mock_audio = np.array([0.1, 0.2, 0.3], dtype=np.float32)
        mock_sf.read.return_value = (mock_audio, 44100)

        with patch.dict("sys.modules", {"soundfile": mock_sf}):
            with patch("sidecar.audio_preprocessing.soxr.resample") as mock_resample:
                mock_resample.return_value = np.array([0.1, 0.15, 0.2, 0.25, 0.3], dtype=np.float32)
                result = _decode_with_soundfile(b"fake audio data", None)

        assert result is not None
        mock_resample.assert_called_once()

    def test_decode_returns_none_on_error(self):
        mock_sf = MagicMock()
        mock_sf.read.side_effect = Exception("decode error")

        with patch.dict("sys.modules", {"soundfile": mock_sf}):
            result = _decode_with_soundfile(b"fake audio data", None)

        assert result is None


class TestDecodeWithFfmpeg:
    def test_decode_success(self):
        with patch("sidecar.audio_preprocessing.subprocess.Popen") as mock_popen:
            mock_proc = MagicMock()
            mock_proc.communicate.return_value = (
                np.array([0.1, 0.2], dtype=np.float32).tobytes(),
                b"",
            )
            mock_proc.returncode = 0
            mock_popen.return_value = mock_proc

            result = _decode_with_ffmpeg(b"fake audio", "mp3")

            assert result is not None
            assert len(result) == 2

    def test_decode_failure_raises_error(self):
        with patch("sidecar.audio_preprocessing.subprocess.Popen") as mock_popen:
            mock_proc = MagicMock()
            mock_proc.communicate.return_value = (b"", b"decode error")
            mock_proc.returncode = 1
            mock_popen.return_value = mock_proc

            with pytest.raises(ValueError, match="ffmpeg decode error"):
                _decode_with_ffmpeg(b"fake audio", "mp3")

    def test_decode_timeout_raises_error(self):
        with patch("sidecar.audio_preprocessing.subprocess.Popen") as mock_popen:
            mock_proc = MagicMock()
            mock_proc.communicate.side_effect = [
                subprocess.TimeoutExpired("ffmpeg", 300),
                (b"", b""),
            ]
            mock_proc.kill = MagicMock()
            mock_popen.return_value = mock_proc

            with pytest.raises(ValueError, match="timed out"):
                _decode_with_ffmpeg(b"fake audio", "mp3")

            mock_proc.kill.assert_called_once()

    def test_decode_without_format(self):
        with patch("sidecar.audio_preprocessing.subprocess.Popen") as mock_popen:
            mock_proc = MagicMock()
            mock_proc.communicate.return_value = (
                np.array([0.1], dtype=np.float32).tobytes(),
                b"",
            )
            mock_proc.returncode = 0
            mock_popen.return_value = mock_proc

            result = _decode_with_ffmpeg(b"fake audio", None)

            assert result is not None
            call_args = mock_popen.call_args[0][0]
            assert "-f" not in call_args[:4]


class TestDecodeAudio:
    def test_uses_soundfile_first(self):
        with patch("sidecar.audio_preprocessing._decode_with_soundfile") as mock_sf:
            mock_sf.return_value = np.array([0.1, 0.2], dtype=np.float32)

            result = decode_audio(b"audio data", "mp3")

            assert result is not None
            mock_sf.assert_called_once()

    def test_falls_back_to_ffmpeg(self):
        with patch("sidecar.audio_preprocessing._decode_with_soundfile") as mock_sf:
            with patch("sidecar.audio_preprocessing._decode_with_ffmpeg") as mock_ff:
                mock_sf.return_value = None
                mock_ff.return_value = np.array([0.1, 0.2], dtype=np.float32)

                result = decode_audio(b"audio data", "mp3")

                assert result is not None
                mock_sf.assert_called_once()
                mock_ff.assert_called_once()


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
        with patch("sidecar.audio_preprocessing.decode_audio") as mock_decode:
            with patch("sidecar.audio_preprocessing.chunk_audio") as mock_chunk:
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
        with patch("sidecar.audio_preprocessing.decode_audio") as mock_decode:
            with patch("sidecar.audio_preprocessing.chunk_audio") as mock_chunk:
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
