import io
import json
import unittest

from personal_search_extractor.worker import handle, run


class WorkerTests(unittest.TestCase):
    def test_ping_reports_ready(self) -> None:
        response = handle({"id": "request-1", "operation": "ping"})

        self.assertEqual(
            response,
            {"id": "request-1", "ok": True, "result": {"status": "ready"}},
        )

    def test_unknown_operation_returns_error(self) -> None:
        response = handle({"id": "request-2", "operation": "extract-video"})

        self.assertEqual(response["id"], "request-2")
        self.assertFalse(response["ok"])
        self.assertEqual(response["error"]["code"], "unsupported_operation")

    def test_invalid_json_does_not_stop_worker(self) -> None:
        input_stream = io.StringIO('not-json\n{"id":"request-3","operation":"ping"}\n')
        output_stream = io.StringIO()

        exit_code = run(input_stream, output_stream)
        responses = [json.loads(line) for line in output_stream.getvalue().splitlines()]

        self.assertEqual(exit_code, 0)
        self.assertEqual(responses[0]["error"]["code"], "invalid_request")
        self.assertTrue(responses[1]["ok"])


if __name__ == "__main__":
    unittest.main()
