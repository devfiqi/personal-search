import json
import sys
from typing import Any, TextIO


def handle(message: dict[str, Any]) -> dict[str, Any]:
    request_id = message.get("id")
    operation = message.get("operation")

    if operation == "ping":
        return {"id": request_id, "ok": True, "result": {"status": "ready"}}

    return {
        "id": request_id,
        "ok": False,
        "error": {"code": "unsupported_operation", "message": str(operation)},
    }


def run(input_stream: TextIO = sys.stdin, output_stream: TextIO = sys.stdout) -> int:
    for line in input_stream:
        try:
            message = json.loads(line)
            if not isinstance(message, dict):
                raise ValueError("request must be a JSON object")
            response = handle(message)
        except (json.JSONDecodeError, ValueError) as error:
            response = {
                "id": None,
                "ok": False,
                "error": {"code": "invalid_request", "message": str(error)},
            }

        output_stream.write(json.dumps(response, separators=(",", ":")) + "\n")
        output_stream.flush()

    return 0
