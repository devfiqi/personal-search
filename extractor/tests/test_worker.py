import io
import json
import tempfile
import unittest
from pathlib import Path

from pypdf import PdfWriter
from pypdf.generic import DecodedStreamObject, DictionaryObject, NameObject

from personal_search_extractor.pdf import PDFExtractionError, extract_pdf
from personal_search_extractor.worker import handle, run


def write_text_pdf(path: Path, text: str) -> None:
    writer = PdfWriter()
    page = writer.add_blank_page(width=612, height=792)

    font = DictionaryObject(
        {
            NameObject("/Type"): NameObject("/Font"),
            NameObject("/Subtype"): NameObject("/Type1"),
            NameObject("/BaseFont"): NameObject("/Helvetica"),
        }
    )
    font_reference = writer._add_object(font)
    page[NameObject("/Resources")] = DictionaryObject(
        {
            NameObject("/Font"): DictionaryObject(
                {NameObject("/F1"): font_reference}
            )
        }
    )

    stream = DecodedStreamObject()
    escaped_text = text.replace("\\", "\\\\").replace("(", "\\(").replace(")", "\\)")
    stream.set_data(f"BT /F1 12 Tf 72 720 Td ({escaped_text}) Tj ET".encode("ascii"))
    page[NameObject("/Contents")] = writer._add_object(stream)

    with path.open("wb") as output:
        writer.write(output)


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

    def test_extract_pdf_returns_text(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            pdf_path = Path(directory) / "notes.pdf"
            write_text_pdf(pdf_path, "replication lag notes")

            response = handle(
                {"id": "request-pdf", "operation": "extract_pdf", "path": str(pdf_path)}
            )

        self.assertTrue(response["ok"])
        self.assertEqual(response["result"]["page_count"], 1)
        self.assertIn("replication lag notes", response["result"]["text"])

    def test_textless_pdf_reports_ocr_requirement(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            pdf_path = Path(directory) / "scan.pdf"
            writer = PdfWriter()
            writer.add_blank_page(width=612, height=792)
            with pdf_path.open("wb") as output:
                writer.write(output)

            response = handle(
                {"id": "request-scan", "operation": "extract_pdf", "path": str(pdf_path)}
            )

        self.assertFalse(response["ok"])
        self.assertEqual(response["error"]["code"], "no_extractable_text")

    def test_encrypted_pdf_reports_password_requirement(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            pdf_path = Path(directory) / "encrypted.pdf"
            writer = PdfWriter()
            writer.add_blank_page(width=612, height=792)
            writer.encrypt("secret")
            with pdf_path.open("wb") as output:
                writer.write(output)

            response = handle(
                {
                    "id": "request-encrypted",
                    "operation": "extract_pdf",
                    "path": str(pdf_path),
                }
            )

        self.assertFalse(response["ok"])
        self.assertEqual(response["error"]["code"], "encrypted_pdf")

    def test_invalid_pdf_reports_error(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            pdf_path = Path(directory) / "damaged.pdf"
            pdf_path.write_bytes(b"not a pdf")

            with self.assertRaises(PDFExtractionError) as raised:
                extract_pdf(str(pdf_path))

        self.assertEqual(raised.exception.code, "invalid_pdf")

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
