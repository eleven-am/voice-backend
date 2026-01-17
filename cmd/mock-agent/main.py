#!/usr/bin/env python3
import asyncio
import json
import sys
from datetime import datetime, timezone

import websockets

API_KEY = "sk-voice-b2733a52fbfd76e9d5d44e9bc2af2cdd62f0eae880e62c78f26e223ac1f8ae4f"
AGENT_ID = "test-agent-001"
WS_URL = f"ws://localhost:8080/api/v1/gateway/ws?api_key={API_KEY}&agent_id={AGENT_ID}"


async def handle_message(msg: dict) -> dict | None:
    msg_type = msg.get("type")

    if msg_type == "utterance":
        payload = msg.get("payload", {})
        text = payload.get("text", "")
        is_final = payload.get("is_final", False)

        print(f"[UTTERANCE] {text} (final={is_final})")

        if is_final:
            response_text = f"Echo: {text}"
            return {
                "type": "response",
                "request_id": msg.get("request_id", ""),
                "session_id": msg.get("session_id", ""),
                "timestamp": datetime.now(timezone.utc).isoformat(),
                "payload": {
                    "text": response_text,
                    "from_agent": AGENT_ID,
                },
            }

    elif msg_type == "session_start":
        print(f"[SESSION START] user={msg.get('user_id')} room={msg.get('room_id')}")

    elif msg_type == "session_end":
        print(f"[SESSION END] session={msg.get('session_id')}")

    elif msg_type == "error":
        payload = msg.get("payload", {})
        print(f"[ERROR] {payload.get('message')}")

    else:
        print(f"[UNKNOWN] type={msg_type}")

    return None


async def main():
    print(f"Connecting to {WS_URL}")

    try:
        async with websockets.connect(WS_URL) as ws:
            print(f"Connected as agent {AGENT_ID}")
            print("Waiting for messages...")

            async for message in ws:
                try:
                    msg = json.loads(message)
                    response = await handle_message(msg)

                    if response:
                        print(f"[SENDING] {response['payload']['text']}")
                        await ws.send(json.dumps(response))

                except json.JSONDecodeError as e:
                    print(f"[ERROR] Invalid JSON: {e}")

    except websockets.exceptions.ConnectionClosed as e:
        print(f"Connection closed: {e}")
    except Exception as e:
        print(f"Error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    asyncio.run(main())
