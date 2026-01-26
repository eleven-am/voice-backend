from __future__ import annotations

from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from sidecar.domain.exceptions import TranscriptionError
from sidecar.domain.transcript_processor import deduplicate_words, merge_transcripts
from sidecar.domain.types import Transcript
from sidecar.infrastructure.codecs.audio_codec import pcm16_to_float32, resample_audio


class TestPcm16ToFloat32:
    def test_converts_silence(self):
        pcm = np.zeros(100, dtype=np.int16).tobytes()

        result = pcm16_to_float32(pcm)

        assert result.dtype == np.float32
        assert np.allclose(result, 0.0)

    def test_converts_max_positive(self):
        pcm = np.array([32767], dtype=np.int16).tobytes()

        result = pcm16_to_float32(pcm)

        assert result.dtype == np.float32
        assert np.isclose(result[0], 32767 / 32768.0)

    def test_converts_max_negative(self):
        pcm = np.array([-32768], dtype=np.int16).tobytes()

        result = pcm16_to_float32(pcm)

        assert result.dtype == np.float32
        assert np.isclose(result[0], -1.0)

    def test_output_range(self):
        pcm = np.random.randint(-32768, 32768, size=1000, dtype=np.int16).tobytes()

        result = pcm16_to_float32(pcm)

        assert result.min() >= -1.0
        assert result.max() <= 1.0


class TestResampleAudio:
    def test_same_rate_no_change(self):
        audio = np.array([0.1, 0.2, 0.3], dtype=np.float32)

        result = resample_audio(audio, 16000, 16000)

        assert np.array_equal(result, audio)

    def test_upsample(self):
        audio = np.array([0.1, 0.2, 0.3], dtype=np.float32)

        with patch("sidecar.infrastructure.codecs.audio_codec.soxr.resample") as mock_resample:
            mock_resample.return_value = np.array([0.1, 0.15, 0.2, 0.25, 0.3], dtype=np.float32)

            result = resample_audio(audio, 8000, 16000)

            mock_resample.assert_called_once()
            assert len(result) > len(audio)

    def test_downsample(self):
        audio = np.array([0.1, 0.15, 0.2, 0.25, 0.3], dtype=np.float32)

        with patch("sidecar.infrastructure.codecs.audio_codec.soxr.resample") as mock_resample:
            mock_resample.return_value = np.array([0.1, 0.2, 0.3], dtype=np.float32)

            result = resample_audio(audio, 32000, 16000)

            mock_resample.assert_called_once()


class TestDeduplicateWords:
    def test_no_overlap_all_new(self):
        text = "hello world"
        confirmed = []

        new_text, updated = deduplicate_words(text, confirmed)

        assert new_text == "hello world"
        assert updated == ["hello", "world"]

    def test_full_overlap_no_new(self):
        text = "hello world"
        confirmed = ["hello", "world"]

        new_text, updated = deduplicate_words(text, confirmed)

        assert new_text == ""
        assert updated == ["hello", "world"]

    def test_partial_overlap(self):
        text = "world foo bar"
        confirmed = ["hello", "world"]

        new_text, updated = deduplicate_words(text, confirmed)

        assert new_text == "foo bar"
        assert updated == ["hello", "world", "foo", "bar"]

    def test_case_insensitive_overlap(self):
        text = "World foo"
        confirmed = ["hello", "world"]

        new_text, updated = deduplicate_words(text, confirmed)

        assert new_text == "foo"

    def test_empty_text(self):
        text = ""
        confirmed = ["hello", "world"]

        new_text, updated = deduplicate_words(text, confirmed)

        assert new_text == ""
        assert updated == ["hello", "world"]

    def test_empty_confirmed(self):
        text = "hello world"
        confirmed = []

        new_text, updated = deduplicate_words(text, confirmed)

        assert new_text == "hello world"
        assert updated == ["hello", "world"]

    def test_whitespace_handling(self):
        text = "  hello   world  "
        confirmed = []

        new_text, updated = deduplicate_words(text, confirmed)

        assert new_text == "hello world"


class TestMergeTranscripts:
    def test_empty_list_raises(self):
        with pytest.raises(ValueError, match="Cannot merge empty"):
            merge_transcripts([])

    def test_single_transcript_returns_same(self):
        transcript = Transcript(
            text="hello world",
            start_ms=0,
            end_ms=1000,
            audio_duration_ms=1000,
            processing_duration_ms=100,
            segments=None,
            model="test",
            eou_probability=None,
        )

        result = merge_transcripts([(transcript, 0.0)])

        assert result.text == "hello world"
        assert result.start_ms == 0
        assert result.end_ms == 1000

    def test_merge_two_transcripts(self):
        t1 = Transcript(
            text="hello",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=None,
            model="test",
            eou_probability=None,
        )
        t2 = Transcript(
            text="world",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=None,
            model="test",
            eou_probability=None,
        )

        result = merge_transcripts([(t1, 0.0), (t2, 0.5)])

        assert result.text == "hello world"
        assert result.audio_duration_ms == 1000
        assert result.processing_duration_ms == 100

    def test_merge_preserves_model(self):
        t1 = Transcript(
            text="hello",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=None,
            model="parakeet",
            eou_probability=None,
        )
        t2 = Transcript(
            text="world",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=None,
            model="parakeet",
            eou_probability=None,
        )

        result = merge_transcripts([(t1, 0.0), (t2, 0.5)])

        assert result.model == "parakeet"

    def test_merge_with_segments(self):
        t1 = Transcript(
            text="hello",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=[{"start": 0.0, "end": 0.5, "text": "hello", "words": []}],
            model="test",
            eou_probability=None,
        )
        t2 = Transcript(
            text="world",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=[{"start": 0.0, "end": 0.5, "text": "world", "words": []}],
            model="test",
            eou_probability=None,
        )

        result = merge_transcripts([(t1, 0.0), (t2, 5.0)])

        assert len(result.segments) == 2
        assert result.segments[0]["start"] == 0.0
        assert result.segments[1]["start"] == 5.0

    def test_merge_with_words(self):
        t1 = Transcript(
            text="hello",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=[
                {
                    "start": 0.0,
                    "end": 0.5,
                    "text": "hello",
                    "words": [{"word": "hello", "start": 0.0, "end": 0.5}],
                }
            ],
            model="test",
            eou_probability=None,
        )
        t2 = Transcript(
            text="world",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=[
                {
                    "start": 0.0,
                    "end": 0.5,
                    "text": "world",
                    "words": [{"word": "world", "start": 0.0, "end": 0.5}],
                }
            ],
            model="test",
            eou_probability=None,
        )

        result = merge_transcripts([(t1, 0.0), (t2, 10.0)])

        assert result.segments[0]["words"][0]["start"] == 0.0
        assert result.segments[1]["words"][0]["start"] == 10.0
        assert result.segments[1]["words"][0]["end"] == 10.5

    def test_merge_empty_text_skipped(self):
        t1 = Transcript(
            text="hello",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=None,
            model="test",
            eou_probability=None,
        )
        t2 = Transcript(
            text="",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=None,
            model="test",
            eou_probability=None,
        )
        t3 = Transcript(
            text="world",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=None,
            model="test",
            eou_probability=None,
        )

        result = merge_transcripts([(t1, 0.0), (t2, 0.5), (t3, 1.0)])

        assert result.text == "hello world"

    def test_merge_calculates_end_ms(self):
        t1 = Transcript(
            text="hello",
            start_ms=0,
            end_ms=500,
            audio_duration_ms=500,
            processing_duration_ms=50,
            segments=None,
            model="test",
            eou_probability=None,
        )
        t2 = Transcript(
            text="world",
            start_ms=0,
            end_ms=600,
            audio_duration_ms=600,
            processing_duration_ms=60,
            segments=None,
            model="test",
            eou_probability=None,
        )

        result = merge_transcripts([(t1, 0.0), (t2, 5.0)])

        assert result.end_ms == 5000 + 600


class TestTranscriptionError:
    def test_error_message(self):
        error = TranscriptionError("test error")
        assert str(error) == "test error"

    def test_error_inheritance(self):
        error = TranscriptionError("test")
        assert isinstance(error, Exception)


class TestTranscriptionServiceServicer:
    def test_transcribe_with_retry_success(self):
        from sidecar.stt.grpc_servicer import TranscriptionServiceServicer
        from sidecar.stt.pipeline import STTPipelineConfig

        mock_manager = MagicMock()
        mock_engine = MagicMock()
        mock_engine.transcribe.return_value = Transcript(
            text="hello",
            start_ms=0,
            end_ms=1000,
            audio_duration_ms=1000,
            processing_duration_ms=100,
            segments=None,
            model="test",
            eou_probability=None,
        )
        mock_wrapper = MagicMock()
        mock_wrapper.__enter__ = MagicMock(return_value=mock_engine)
        mock_wrapper.__exit__ = MagicMock(return_value=False)
        mock_manager.get_engine.return_value = mock_wrapper

        servicer = TranscriptionServiceServicer(
            engine_manager=mock_manager,
            pipeline_config=STTPipelineConfig(),
            executor=MagicMock(),
        )

        audio = np.zeros(1000, dtype=np.float32)
        result = servicer._transcription_service.transcribe_with_retry(audio)

        assert result.text == "hello"

    def test_transcribe_with_retry_oom_retries(self):
        from sidecar.stt.grpc_servicer import TranscriptionServiceServicer
        from sidecar.stt.pipeline import STTPipelineConfig

        mock_manager = MagicMock()
        mock_engine = MagicMock()

        call_count = [0]

        def transcribe_side_effect(**kwargs):
            call_count[0] += 1
            if call_count[0] < 3:
                raise RuntimeError("CUDA out of memory")
            return Transcript(
                text="hello",
                start_ms=0,
                end_ms=1000,
                audio_duration_ms=1000,
                processing_duration_ms=100,
                segments=None,
                model="test",
                eou_probability=None,
            )

        mock_engine.transcribe.side_effect = transcribe_side_effect
        mock_wrapper = MagicMock()
        mock_wrapper.__enter__ = MagicMock(return_value=mock_engine)
        mock_wrapper.__exit__ = MagicMock(return_value=False)
        mock_manager.get_engine.return_value = mock_wrapper
        mock_manager.try_fallback.return_value = True

        servicer = TranscriptionServiceServicer(
            engine_manager=mock_manager,
            pipeline_config=STTPipelineConfig(),
            executor=MagicMock(),
        )

        audio = np.zeros(1000, dtype=np.float32)
        result = servicer._transcription_service.transcribe_with_retry(audio)

        assert result.text == "hello"
        assert call_count[0] == 3

    def test_transcribe_with_retry_oom_exhausted(self):
        from sidecar.stt.grpc_servicer import TranscriptionServiceServicer
        from sidecar.stt.pipeline import STTPipelineConfig

        mock_manager = MagicMock()
        mock_engine = MagicMock()
        mock_engine.transcribe.side_effect = RuntimeError("CUDA out of memory")
        mock_wrapper = MagicMock()
        mock_wrapper.__enter__ = MagicMock(return_value=mock_engine)
        mock_wrapper.__exit__ = MagicMock(return_value=False)
        mock_manager.get_engine.return_value = mock_wrapper
        mock_manager.try_fallback.return_value = False

        servicer = TranscriptionServiceServicer(
            engine_manager=mock_manager,
            pipeline_config=STTPipelineConfig(),
            executor=MagicMock(),
        )

        audio = np.zeros(1000, dtype=np.float32)

        with pytest.raises(TranscriptionError, match="no fallback available"):
            servicer._transcription_service.transcribe_with_retry(audio)

    def test_transcribe_with_retry_non_oom_propagates(self):
        from sidecar.stt.grpc_servicer import TranscriptionServiceServicer
        from sidecar.stt.pipeline import STTPipelineConfig

        mock_manager = MagicMock()
        mock_engine = MagicMock()
        mock_engine.transcribe.side_effect = ValueError("Some other error")
        mock_wrapper = MagicMock()
        mock_wrapper.__enter__ = MagicMock(return_value=mock_engine)
        mock_wrapper.__exit__ = MagicMock(return_value=False)
        mock_manager.get_engine.return_value = mock_wrapper

        servicer = TranscriptionServiceServicer(
            engine_manager=mock_manager,
            pipeline_config=STTPipelineConfig(),
            executor=MagicMock(),
        )

        audio = np.zeros(1000, dtype=np.float32)

        with pytest.raises(ValueError, match="Some other error"):
            servicer._transcription_service.transcribe_with_retry(audio)
