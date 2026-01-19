from __future__ import annotations

import sys
from unittest.mock import MagicMock

sys.modules["opuslib"] = MagicMock()
sys.modules["opuslib.api"] = MagicMock()
sys.modules["opuslib.api.decoder"] = MagicMock()
sys.modules["opuslib.api.info"] = MagicMock()
sys.modules["opuslib.exceptions"] = MagicMock()
