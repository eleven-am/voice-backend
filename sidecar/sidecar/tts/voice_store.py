from __future__ import annotations

import asyncio
import json
import logging
import re
import wave
from dataclasses import dataclass, asdict
from pathlib import Path

import aiofiles
import aiofiles.os

logger = logging.getLogger(__name__)

VOICE_ID_PATTERN = re.compile(r"^[a-zA-Z0-9_-]{1,64}$")
MAX_AUDIO_SIZE = 10 * 1024 * 1024
MIN_AUDIO_DURATION = 1.0
MAX_AUDIO_DURATION = 30.0
MAX_VOICES = 100


@dataclass
class VoiceMetadata:
    voice_id: str
    name: str
    language: str
    gender: str
    is_default: bool = False


class VoiceStoreError(Exception):
    def __init__(self, message: str, code: int = 1) -> None:
        super().__init__(message)
        self.code = code


class VoiceStore:
    def __init__(self, voices_dir: str = "/data/voices") -> None:
        self.voices_dir = Path(voices_dir)
        self._lock = asyncio.Lock()

    async def _ensure_dir(self) -> None:
        if not self.voices_dir.exists():
            await aiofiles.os.makedirs(self.voices_dir, exist_ok=True)

    def _audio_path(self, voice_id: str) -> Path:
        return self.voices_dir / f"{voice_id}.wav"

    def _metadata_path(self, voice_id: str) -> Path:
        return self.voices_dir / f"{voice_id}.json"

    async def validate_audio(self, audio_data: bytes) -> tuple[bool, str]:
        if len(audio_data) > MAX_AUDIO_SIZE:
            return False, f"Audio file exceeds maximum size of {MAX_AUDIO_SIZE // (1024 * 1024)}MB"

        if len(audio_data) < 44:
            return False, "Invalid WAV file: too small"

        if audio_data[:4] != b"RIFF" or audio_data[8:12] != b"WAVE":
            return False, "Invalid WAV file: missing RIFF/WAVE header"

        loop = asyncio.get_running_loop()

        def check_wav() -> tuple[bool, str]:
            import io
            try:
                with wave.open(io.BytesIO(audio_data), "rb") as wf:
                    frames = wf.getnframes()
                    rate = wf.getframerate()
                    duration = frames / rate

                    if duration < MIN_AUDIO_DURATION:
                        return False, f"Audio too short: {duration:.1f}s (minimum {MIN_AUDIO_DURATION}s)"

                    if duration > MAX_AUDIO_DURATION:
                        return False, f"Audio too long: {duration:.1f}s (maximum {MAX_AUDIO_DURATION}s)"

                    return True, f"Valid audio: {duration:.1f}s duration"
            except wave.Error as e:
                return False, f"Invalid WAV file: {e}"
            except Exception as e:
                return False, f"Error reading audio: {e}"

        return await loop.run_in_executor(None, check_wav)

    async def create_voice(
        self,
        voice_id: str,
        audio_data: bytes,
        name: str,
        language: str,
        gender: str,
    ) -> VoiceMetadata:
        if not VOICE_ID_PATTERN.match(voice_id):
            raise VoiceStoreError(
                "Invalid voice_id: must be 1-64 alphanumeric characters, dashes, or underscores",
                code=1,
            )

        valid, message = await self.validate_audio(audio_data)
        if not valid:
            raise VoiceStoreError(message, code=2)

        async with self._lock:
            await self._ensure_dir()

            voices = await self.list_voices()
            custom_count = sum(1 for v in voices if not v.is_default)
            if custom_count >= MAX_VOICES:
                raise VoiceStoreError(
                    f"Voice quota exceeded: maximum {MAX_VOICES} custom voices",
                    code=3,
                )

            audio_path = self._audio_path(voice_id)
            metadata_path = self._metadata_path(voice_id)

            async with aiofiles.open(audio_path, "wb") as f:
                await f.write(audio_data)

            metadata = VoiceMetadata(
                voice_id=voice_id,
                name=name or voice_id,
                language=language or "en",
                gender=gender or "neutral",
                is_default=False,
            )

            async with aiofiles.open(metadata_path, "w") as f:
                await f.write(json.dumps(asdict(metadata), indent=2))

            logger.info(f"Created voice: {voice_id}")
            return metadata

    async def delete_voice(self, voice_id: str) -> bool:
        async with self._lock:
            audio_path = self._audio_path(voice_id)
            metadata_path = self._metadata_path(voice_id)

            if not audio_path.exists():
                return False

            try:
                await aiofiles.os.remove(audio_path)
                if metadata_path.exists():
                    await aiofiles.os.remove(metadata_path)
                logger.info(f"Deleted voice: {voice_id}")
                return True
            except OSError as e:
                logger.error(f"Failed to delete voice {voice_id}: {e}")
                return False

    async def get_voice_path(self, voice_id: str | None) -> str | None:
        if voice_id is None:
            return None

        audio_path = self._audio_path(voice_id)
        if audio_path.exists():
            return str(audio_path)

        return None

    async def get_voice_metadata(self, voice_id: str) -> VoiceMetadata | None:
        metadata_path = self._metadata_path(voice_id)
        if not metadata_path.exists():
            return None

        try:
            async with aiofiles.open(metadata_path, "r") as f:
                data = json.loads(await f.read())
                return VoiceMetadata(**data)
        except (json.JSONDecodeError, TypeError, KeyError) as e:
            logger.warning(f"Failed to read metadata for {voice_id}: {e}")
            return None

    async def list_voices(self) -> list[VoiceMetadata]:
        voices: list[VoiceMetadata] = []

        voices.append(VoiceMetadata(
            voice_id="default",
            name="Default",
            language="en",
            gender="neutral",
            is_default=True,
        ))

        if not self.voices_dir.exists():
            return voices

        try:
            entries = await aiofiles.os.listdir(self.voices_dir)
        except OSError:
            return voices

        for entry in entries:
            if entry.endswith(".json"):
                voice_id = entry[:-5]
                metadata = await self.get_voice_metadata(voice_id)
                if metadata:
                    voices.append(metadata)

        return voices
