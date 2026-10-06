import json
import sys
from typing import Any, TextIO

from personal_search_extractor.pdf import PDFExtractionError, extract_pdf
from personal_search_extractor.semantic import SemanticEmbeddingError, embed


def handle(message: dict[str, Any]) -> dict[str, Any]:
    request_id = message.get("id")
    operation = message.get("operation")

    if operation == "ping":
        return {"id": request_id, "ok": True, "result": {"status": "ready"}}

    if operation == "extract_pdf":
        path = message.get("path")
        if not isinstance(path, str) or not path:
            return {
                "id": request_id,
                "ok": False,
                "error": {
                    "code": "invalid_request",
                    "message": "extract_pdf requires a path",
                },
            }

        try:
            result = extract_pdf(path)
        except PDFExtractionError as error:
            return {
                "id": request_id,
                "ok": False,
                "error": {"code": error.code, "message": str(error)},
            }

        return {
            "id": request_id,
            "ok": True,
            "result": {"text": result.text, "page_count": result.page_count},
        }

    if operation == "embed":
        texts = message.get("texts")
        if not isinstance(texts, list) or not all(isinstance(text, str) for text in texts):
            return {
                "id": request_id,
                "ok": False,
                "error": {"code": "invalid_request", "message": "embed requires text strings"},
            }
        try:
            vectors = embed(texts)
        except SemanticEmbeddingError as error:
            return {
                "id": request_id,
                "ok": False,
                "error": {"code": "semantic_unavailable", "message": str(error)},
            }
        return {"id": request_id, "ok": True, "result": {"vectors": vectors}}

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
