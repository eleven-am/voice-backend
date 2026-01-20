from __future__ import annotations

from sidecar.domain.types import Transcript


def deduplicate_words(text: str, confirmed_words: list[str]) -> tuple[str, list[str]]:
    words = [w for w in text.strip().split() if w]
    overlap = 0
    max_overlap = min(len(words), len(confirmed_words))
    for i in range(max_overlap, 0, -1):
        if [w.lower() for w in confirmed_words[-i:]] == [w.lower() for w in words[:i]]:
            overlap = i
            break
    new_words = words[overlap:]
    if new_words:
        confirmed_words.extend(new_words)
    return " ".join(new_words), confirmed_words


def merge_transcripts(transcripts: list[tuple[Transcript, float]]) -> Transcript:
    if not transcripts:
        raise ValueError("Cannot merge empty transcript list")
    if len(transcripts) == 1:
        return transcripts[0][0]

    merged_text_parts: list[str] = []
    merged_segments: list[dict] = []
    total_audio_ms = 0
    total_processing_ms = 0

    for transcript, offset_s in transcripts:
        if transcript.text.strip():
            merged_text_parts.append(transcript.text.strip())

        if transcript.segments:
            for seg in transcript.segments:
                merged_segments.append(
                    {
                        "start": seg["start"] + offset_s,
                        "end": seg["end"] + offset_s,
                        "text": seg["text"],
                        "words": [
                            {"word": w["word"], "start": w["start"] + offset_s, "end": w["end"] + offset_s}
                            for w in seg.get("words", [])
                        ],
                    }
                )

        total_audio_ms += transcript.audio_duration_ms
        total_processing_ms += transcript.processing_duration_ms

    first = transcripts[0][0]
    last = transcripts[-1][0]
    last_offset_s = transcripts[-1][1]

    return Transcript(
        text=" ".join(merged_text_parts),
        start_ms=first.start_ms,
        end_ms=int(last_offset_s * 1000) + last.end_ms,
        audio_duration_ms=total_audio_ms,
        processing_duration_ms=total_processing_ms,
        segments=merged_segments if merged_segments else None,
        model=first.model,
        eou_probability=None,
    )
